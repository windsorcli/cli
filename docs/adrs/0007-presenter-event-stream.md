# ADR 0007 — Presenter / event stream: the keystone seam

- Status: Accepted. `internal/presenter`, the global `--format` flag, and the central error
  renderer shipped. `cmd/` builds the presenter, writes it to stderr, and puts it in the command
  context. Decision 3 changed from constructor injection to context injection; see that section.
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- Fills the "Presenter / event stream" placeholder from `release-v0.10.0.md`'s Wave 1 — the last
  Wave 1 gap. Builds directly on [ADR 0005](0005-typed-error-model.md) (errors) and
  [ADR 0006](0006-structured-logging-contract.md) (logging), which both explicitly deferred their
  own rendering-unification and business-layer-migration questions here rather than deciding them
  twice.

## Context

ADR 0006 found nine files reaching `pkg/tui`'s global `Active`/`WithProgress` directly from business
layers. This round's survey goes one level deeper — that coupling isn't the only ad-hoc mechanism in
play; there are **three**, each solving a fragment of what a presenter should own:

1. **`tui.WithProgress(message, fn func() error) error`** — wraps a synchronous closure, starts a
   spinner, calls `Done()`/`Fail()` based on the closure's error. Used 10+ times across
   `pkg/provisioner/terraform/stack.go` and `pkg/provisioner/provisioner.go` for "Applying %s" /
   "Destroying %s" / "Migrating terraform state" style single-operation progress.
2. **`tui.Start(message)` / `tui.Fail()` called directly, unpaired from `WithProgress`** —
   `pkg/provisioner/kubernetes/kubernetes_manager.go` calls these asymmetrically for
   longer-running waits that don't fit the closure-wrapping shape.
3. **Ad-hoc `outputFunc func(string)` parameters** — `WaitForNodesHealthy`, `CheckNodeHealth`, and
   similar wait-phase functions take a bespoke string-callback parameter, documented inline as
   "receives status messages during the wait phases." Same job as (1)/(2), third independent shape.

Three different shapes for the same underlying need — reporting "something is happening" to
whatever's watching — is exactly the "four independent subsystems" problem the release doc's
keystone section names, just discovered to be worse than "logging vs. errors vs. progress vs.
prompts": progress reporting alone already has three competing internal shapes before a presenter
unifies anything.

**No global JSON output flag exists today.** Two commands already define a local `--output` flag:
`bundle` (archive path) and `plan terraform` (`json-plan`). The release doc's open question #2 —
"is JSON a global surface covering logs and errors and progress uniformly, or per-concern flags" —
is still open, and both ADR 0005 and ADR 0006 explicitly punted it here rather than deciding it twice. This
ADR is where it gets resolved, since the presenter is the one seam all three concerns pass through.

Checked against real prior art before finalizing this shape, not just recalled from memory — this
is an established idiom, not a novel design, verified across four independent, widely-used systems
solving the identical problem (a heterogeneous progress/log/diagnostic stream from a multi-resource
orchestration tool):

- **Terraform's `-json` machine-readable output**
  ([docs](https://developer.hashicorp.com/terraform/internals/machine-readable-ui)) — directly
  relevant since the release doc's async wave targets concurrent Terraform. Every message shares a
  common envelope — `@level`, `@message`, `@module` (always `"terraform.ui"`), `@timestamp`, and
  `type` — where `type` (`"version"`, `"planned_change"`, `"apply_start"`, `"apply_progress"`,
  `"diagnostic"`, `"log"`, …) drives which additional fields are present, streamed as **one JSON
  object per line**, not one buffered document.
- **`sigs.k8s.io/cli-utils/pkg/apply/event`** ([pkg.go.dev](https://pkg.go.dev/sigs.k8s.io/cli-utils/pkg/apply/event))
  — the event package behind `kubectl apply`'s and `kpt live apply`'s apply/prune/wait pipeline.
  `Event` has one `Type` enum (`ApplyType`, `PruneType`, `WaitType`, `ErrorType`, `StatusType`, …)
  and — notably — **typed fields declared directly on the struct** (`ApplyEvent`, `PruneEvent`,
  `WaitEvent`, `ErrorEvent`, …), not a generic `any` payload requiring a type assertion. Separate
  `table` and `json` printer packages consume the same event channel.
- **Pulumi's `engine.Event`** ([events.go](https://github.com/pulumi/pulumi/blob/master/pkg/engine/events.go),
  [display/json.go](https://github.com/pulumi/pulumi/blob/master/pkg/backend/display/json.go)) —
  rendered either as an interactive progress tree (`ShowProgressEvents`) or as JSON
  (`ShowJSONEvents`/`ShowPreviewDigest`). A real fork worth naming: Pulumi's JSON renderer
  deliberately does **not** stream incrementally — it buffers the whole event sequence to guarantee
  one well-formed JSON document, trading incremental/tailable output for a validity guarantee.
- **BuildKit's `util/progress/progressui`** ([display.go](https://github.com/moby/buildkit/blob/master/util/progress/progressui/display.go),
  powering `docker build`) — a `Display` interface with `auto`/`tty`/`plain`/`rawjson` modes,
  `AutoMode` picking `tty` vs. `plain` by terminal detection. The closest match to this ADR's
  console/JSON/(eventually TUI) three-renderer split specifically.

Every one of these converges on the same core shape — one discriminated envelope, pluggable
renderers selected by output mode — which is the strongest signal this ADR is applying an
established idiom, not inventing one. Where they diverge (Terraform/this ADR's per-line NDJSON
streaming vs. Pulumi's buffered single document) is called out explicitly in Alternatives, since
it's a real tradeoff, not an oversight.

## Decision

### 1. Domain event vocabulary: one envelope, a small closed set of kinds

```go
// internal/presenter (sibling to logging/werror/shims/util — reachable from cmd/ and pkg/ alike)

type Kind string

const (
    KindApplying    Kind = "applying"     // an operation has started
    KindApplied     Kind = "applied"      // an operation finished successfully
    KindFailed      Kind = "failed"       // an operation finished with an error
    KindProgress    Kind = "progress"     // replaces the status text of a running operation
    KindMessage     Kind = "message"      // a narrative line for the user, at info or warn level
    KindLog         Kind = "log"          // a structured log record (bridges ADR 0006's slog handler)
    KindInputNeeded Kind = "input_needed" // reserved — Wave 3's interactive-input ADR fills this in
)

type Event struct {
    Kind     Kind
    ID       string        // operation ID; set on applying, applied, failed, and progress
    ParentID string        // ID of the enclosing operation; empty at the top level
    Subject  string        // "terraform:cluster", "kustomization:cert-manager" — what this is about
    Message  string
    Level    slog.Level    // info or warn, for KindMessage
    Duration time.Duration // elapsed time, for KindApplied and KindFailed
    Err      error         // set only when Kind == KindFailed; renderers errors.As for a code
    Record   *slog.Record  // set only when Kind == KindLog
}
```

One envelope type with a discriminated `Kind`, not a Go interface with per-kind concrete types —
deliberately matching Terraform's `-json` shape (Context) over the alternative sum-type-via-interface
pattern, because every renderer (console, JSON, eventually TUI) needs to switch on kind anyway, and a
single struct serializes to stable JSON without a custom `MarshalJSON` per event type.
`KindInputNeeded` is reserved, unused until Wave 3, for the same reason ADR 0006 reserved its
TUI-routing handler.

The output model is a transcript of operations, not a log. `KindMessage` is the narrative channel:
lines the user reads as part of the run ("found 3 components to apply"). `KindLog` carries
diagnostic records, which the console shows only at the configured level. Business code MUST NOT
use `slog.Info` for narrative output, because a log record is not part of the transcript.

`ID` and `ParentID` make the stream a tree. Operations nest, and messages, progress, and log records
attach to the operation that encloses them. The context carries the current operation, so call
sites never pass IDs by hand (point 2).

`Err`/`Record` are declared as typed optional fields directly on `Event`, not behind a generic
`any Data` field requiring a type assertion at every renderer — this specific choice is validated
directly by `cli-utils`' `Event` (Context): it uses the identical pattern (`ApplyEvent`,
`PruneEvent`, `WaitEvent`, … as typed fields on one struct, populated according to `Type`) at
production scale behind `kubectl apply`. A generic payload field was not seriously considered as an
alternative for this reason — real prior art already settled the question in favor of typed fields.

### 2. The presenter port: one method, plus context-aware helpers

```go
type Presenter interface {
    Emit(ctx context.Context, event Event)
}

// Begin emits KindApplying under the operation in ctx and returns a context that carries the new
// operation, plus an end function that emits KindApplied or KindFailed with the elapsed time.
func Begin(ctx context.Context, subject, message string) (context.Context, func(error))

// Track runs fn as one operation and passes fn the operation's context.
func Track(ctx context.Context, subject, message string, fn func(context.Context) error) error

// Message, Warn, and Progress attach to the operation in ctx. Each helper reads the presenter
// from ctx (point 3).
func Message(ctx context.Context, text string)
func Warn(ctx context.Context, text string)
func Progress(ctx context.Context, text string)
```

`Emit` alone is deliberately the entire port — "one seam" from the keystone section, not a growing
interface. The helpers are package functions over `Emit`, so a renderer implements one method.
`Track` keeps the migration cheap for the majority-case call site (1 above):
`tui.WithProgress(msg, fn)` becomes `presenter.Track(ctx, subject, msg, fn)`, where `fn` takes
the operation's context. The asymmetric `tui.Start`/`Fail` sites (2) convert to `Begin` and its end
function, and the `outputFunc` sites (3) convert to `Progress` or `Message`. Because each helper
reads its parent from `ctx`, nesting needs no extra arguments at any call site.

### 3. The presenter travels in the context, the same as the logger

`cmd/` builds the active `Presenter` once (console or JSON, chosen by `--format`) and stores it in
the command context with `presenter.NewContext`. Business code reads it with
`presenter.FromContext`, which returns `Discard` when the context has none, so code under test stays
silent without setup. The helpers in point 2 take only `ctx`.

An earlier draft chose constructor injection through `Runtime`. It changed for two reasons found
when the first package migrated:

- `cmd/` builds a `Runtime` at about 20 sites, and `project.NewProject` uses an override `Runtime`
  as-is. Constructor injection would need the presenter at every one of those sites.
- Nesting already requires `ctx` on every method that emits events, because the context carries the
  current operation. With `ctx` present, the presenter in the same context costs nothing more.

Methods that emit events MUST take `ctx context.Context` as their first parameter.

### 4. Three renderers behind the port

- **Console** (default) — renders the transcript as an indented tree. Finished steps stay on
  screen with their elapsed time, and nested steps print under their parent. In plain mode (not a
  terminal, or `--verbose`) it prints one line per event, which suits CI logs. In interactive mode
  it keeps a live region below the tree with one animated line per active leaf operation, its
  running time, and its progress text. Times read as seconds (`14s`), then minutes (`2m05s`), then
  hours (`1h02m`). A parent operation that succeeds prints no end line, because its
  header and its children's end lines already show the result; a failed parent still prints one. While that region is on screen, the console hides the cursor and turns off input
  echo, so a typed key cannot move the cursor under the redraw. It discards keys typed meanwhile,
  and restores the terminal on pause, on exit, and on an interrupt. On Windows it also turns on
  virtual terminal processing, and it falls back to plain mode when that fails. `KindFailed` ends a step with a failure mark; the central `cmd/` renderer prints
  the error itself once (ADR 0005). `KindLog` renders through the `charmbracelet/log` console
  handler ADR 0006 selected. The console renderer does not use `pkg/tui`.

  ```
  ● Applying terraform
    ✔ network       14s
    ✔ cluster       2m03s
    ⠋ gitops        waiting on 2 kustomizations
  ```
- **JSON** (`--format json`, now a **global persistent flag** — this ADR's resolution of the release
  doc's open question #2, decided as "yes, uniform," see Consequences) — every `Emit` call
  marshals the `Event` as one newline-delimited JSON object to stderr, in the same envelope as
  Terraform's own `-json` output (Context). Stderr keeps stdout for each command's own output, such
  as the `export` lines that `windsor env` prints. Logs, errors, and progress are the same stream, distinguished only by
  `"kind"` — exactly what the keystone section asks for ("renderings of the same event stream, not
  four independent subsystems").
- **TUI** (reserved, Wave 3) — routes `Event`s into a BubbleTea program's message channel instead of
  stdout. Not built here; the port, the operation tree, and the `KindInputNeeded` reservation exist
  so Wave 3 implements a fourth renderer against an unchanged contract, not a contract redesign.

### 5. How this reconciles with ADR 0006 (logging) and ADR 0005 (errors) — neither is superseded

- **Logging call sites are unchanged.** Business code still calls `slog.InfoContext(ctx, ...)`
  exactly as ADR 0006 decided. What changes is the **handler** `Runtime` wires into that logger:
  `presenter.NewLogHandler(p, level)` implements `slog.Handler` and, on `Handle()`, builds a
  `KindLog` event and calls `presenter.Emit`. This is precisely the "TUI-routing handler" seam ADR
  0006 reserved — the presenter *is* the routing target that handler was reserved for. The
  bridge lives in `presenter`, not `logging`, because the console renderer imports `logging`.
- **`WindsorError` rendering is unchanged in shape, relocated in mechanism.** ADR 0005's central
  `cmd/`-boundary renderer stops formatting directly and instead builds `Event{Kind: KindFailed, Err:
  we}` and calls `presenter.Emit` — the format string, the `DocsURL` line, the `--verbose` breadcrumb
  trail (`werror.Breadcrumbs`) all move into the console/JSON renderers' `KindFailed` handling
  unchanged from how ADR 0005 specified them. ADR 0005 itself called this out as expected: "this ADR
  defines the type and an interim renderer the presenter work subsumes."

### 6. Migration: per-package, not mechanical — same discipline as 0005 and 0006

This ADR does not migrate the nine `tui.Active`-reaching files, the `outputFunc` call sites, or the
`kubernetes_manager.go` asymmetric `Start`/`Fail` calls. Every one of the affected packages
(`pkg/provisioner`, `pkg/provisioner/kubernetes`, `pkg/workstation`, `pkg/composer/artifact`,
`pkg/runtime/shell`) is already in Wave 2's per-package cleanup order — the migration happens as
each package's pass reaches it, following ADR 0003's characterize-then-refactor rule: black-box
characterize current progress-reporting behavior first (a fake `Presenter` capturing emitted events
is the natural test double), then swap the mechanism, confirm the black-box tests still pass. `onApply
func(id string) (bool, error)` hooks on `TerraformStack.Up`/`Provisioner.Up` are explicitly **out of
scope** — they compose secret-placement and other post-apply logic, not user-facing output, and
converting them would conflate two unrelated mechanisms that happen to nest today.

## Consequences

- **Resolves the release doc's open question #2**: `--format json` is a single global persistent
  flag (same tier as `--verbose`), and it covers logs, errors, and progress uniformly through one
  `Presenter` implementation — not per-concern flags. Decided here because the presenter is the only
  place this question has a real answer; deferring it further (as both 0005 and 0006 did) would have
  left it unresolved indefinitely.
- **Three ad-hoc progress mechanisms collapse to one port** — `tui.WithProgress`, the asymmetric
  `tui.Start`/`Fail` pattern, and bespoke `outputFunc` parameters all become `Emit`/`Track` calls,
  closing a fragmentation problem this round's survey found was worse than the release doc's
  original framing (three mechanisms, not one, needed replacing).
- **`pkg/tui` retires as packages migrate.** The console renderer has its own line-based tree and
  animation, so `pkg/tui`'s spinner, its global `Active`, and `WithProgress` lose their last caller
  when the final package moves to the presenter.
  Until then, `cmd/` bridges the two. The console holds the legacy spinner silent with `tui.Hold`
  while it shows an operation, `tui.Pause` and `tui.Resume` also pause the console around password
  prompts, and `--format json` silences the legacy spinner. The bridge goes away with `pkg/tui`.
- **Methods that emit events gain a `ctx` parameter** on `TerraformStack`, `KubernetesManager`,
  `Provisioner`, workstation types, and `ArtifactBuilder`. This is a mechanical signature change,
  landed in each package's Wave 2 pass, and it also prepares those methods for Wave 3 cancellation.
- **The event vocabulary is a new piece of public-ish surface** (even though `internal/presenter`
  isn't externally importable, it's shared across every business package) — adding a `Kind` later is
  cheap and backward-compatible for JSON consumers (an unrecognized kind just doesn't match a
  switch case); removing or renegotiating one is not, once `--format json` has real consumers.

## Alternatives considered

- **A Go interface with one concrete type per event kind** (`Applying{}`, `Failed{Err error}`, …)
  instead of one struct with a `Kind` tag. Rejected: every renderer has to type-switch across all
  kinds anyway, and a single struct maps directly onto both Terraform's proven `-json` stream shape
  and `cli-utils`' `Event` struct (Context) without per-type `MarshalJSON` implementations.
- **Buffer the full event sequence and emit one JSON document**, Pulumi's approach (Context), instead
  of streaming one JSON object per `Emit` call. Rejected for Windsor's case: a CI pipeline tailing
  `windsor apply --format json` benefits more from incremental, tailable output (Terraform's choice,
  and this ADR's) than from Pulumi's well-formedness guarantee — a killed process leaves a
  truncated-but-still-line-valid NDJSON stream, which is an acceptable failure mode for a log,
  whereas losing the whole document is not. Revisit only if a consumer needs the "always one valid
  JSON blob" guarantee Pulumi optimized for, which no current Windsor use case does.
- **Constructor injection through `Runtime`** (this ADR's first draft). Rejected after the first
  migration: it needs the presenter at about 20 `Runtime` construction sites in `cmd/`, and nesting
  needs `ctx` on the same methods anyway (point 3).
- **Keep JSON output as a per-command, per-concern flag** (e.g. `apply --json-errors`,
  `plan --json-output`) rather than one global flag. Rejected: this is exactly the fragmentation
  problem being solved for progress mechanisms; doing the same thing to output-format selection
  would just relocate the inconsistency.
- **Migrate all nine known call sites as part of this ADR.** Rejected for the same reason ADR 0005
  and 0006 kept their own migrations selective and per-package: the affected packages are already
  scheduled in Wave 2's order, and converting them here would mean touching them again during their
  own cleanup pass for unrelated reasons (file splits, black-box tests).
- **Build the Wave 3 TUI renderer now, alongside the port.** Rejected, consistent with ADR 0006:
  nothing exists yet to route TUI events *to* — BubbleTea adoption is its own Wave 3 ADR, and
  building the renderer first would mean guessing its shape before that ADR decides it.

## References

Prior art verified via live search, not recalled from memory — see Context for how each maps onto
this ADR's decisions:

- [Terraform machine-readable UI](https://developer.hashicorp.com/terraform/internals/machine-readable-ui) — envelope shape, NDJSON streaming
- [`sigs.k8s.io/cli-utils/pkg/apply/event`](https://pkg.go.dev/sigs.k8s.io/cli-utils/pkg/apply/event) — typed-fields-on-one-struct pattern
- [Pulumi `pkg/engine/events.go`](https://github.com/pulumi/pulumi/blob/master/pkg/engine/events.go) / [`pkg/backend/display/json.go`](https://github.com/pulumi/pulumi/blob/master/pkg/backend/display/json.go) — buffered-JSON alternative, considered and rejected
- [BuildKit `util/progress/progressui/display.go`](https://github.com/moby/buildkit/blob/master/util/progress/progressui/display.go) — pluggable-renderer-by-output-mode precedent
