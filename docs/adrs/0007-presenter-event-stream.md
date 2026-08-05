# ADR 0007 — Presenter / event stream: the keystone seam

- Status: Proposed
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

**No global `--output json` flag exists today** (checked directly — the only `"output"` flag in
`cmd/` is `bundle`'s unrelated archive-path flag). The release doc's open question #2 — "is JSON a
global surface covering logs and errors and progress uniformly, or per-concern flags" — is still
open, and both ADR 0005 and ADR 0006 explicitly punted it here rather than deciding it twice. This
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
    KindFailed      Kind = "failed"       // an operation finished with a *werror.WindsorError
    KindProgress    Kind = "progress"     // a status update mid-operation (today's outputFunc case)
    KindLog         Kind = "log"          // a structured log record (bridges ADR 0006's slog handler)
    KindInputNeeded Kind = "input_needed" // reserved — Wave 3's interactive-input ADR fills this in
)

type Event struct {
    Kind     Kind
    Subject  string // "terraform:cluster", "kustomization:cert-manager" — what this is about
    ParentID string // empty for top-level; set for a sub-resource event (Wave 3 hierarchy — reserved now)
    Message  string
    Err      *werror.WindsorError // set only when Kind == KindFailed
    Record   *slog.Record         // set only when Kind == KindLog
}
```

One envelope type with a discriminated `Kind`, not a Go interface with per-kind concrete types —
deliberately matching Terraform's `-json` shape (Context) over the alternative sum-type-via-interface
pattern, because every renderer (console, JSON, eventually TUI) needs to switch on kind anyway, and a
single struct serializes to stable JSON without a custom `MarshalJSON` per event type. `ParentID` and
`KindInputNeeded` are reserved now, unused until Wave 3, for the same reason ADR 0006 reserved its
TUI-routing handler: the vocabulary shouldn't need to change shape once Wave 3 has something to put
in those fields.

`Err`/`Record` are declared as typed optional fields directly on `Event`, not behind a generic
`any Data` field requiring a type assertion at every renderer — this specific choice is validated
directly by `cli-utils`' `Event` (Context): it uses the identical pattern (`ApplyEvent`,
`PruneEvent`, `WaitEvent`, … as typed fields on one struct, populated according to `Type`) at
production scale behind `kubectl apply`. A generic payload field was not seriously considered as an
alternative for this reason — real prior art already settled the question in favor of typed fields.

### 2. The presenter port: one method, plus one convenience wrapper

```go
type Presenter interface {
    Emit(ctx context.Context, event Event)
}

// Track is sugar over Emit, matching today's tui.WithProgress call shape exactly — emits
// KindApplying, runs fn, emits KindApplied or KindFailed based on the result. This is what most of
// the ~13 WithProgress call sites convert to, nearly mechanically.
func Track(ctx context.Context, p Presenter, subject string, fn func() error) error
```

`Emit` alone is deliberately the entire port — "one seam" from the keystone section, not a growing
interface. `Track` exists purely to keep the migration cheap for the majority-case call site
(1 above): a call site that reads `tui.WithProgress(msg, fn)` today reads `presenter.Track(ctx,
subject, fn)` after, same shape, same risk profile. The asymmetric `tui.Start`/`Fail` sites (2) and
the `outputFunc` sites (3) convert to explicit `Emit` calls instead, since they don't fit the
wrap-a-closure shape `Track` assumes.

### 3. Presenter is injected, constructed by `Runtime`, same placement as the logger

Per ADR 0002's layer table (Runtime owns lifecycle/dependency wiring) and ADR 0006's precedent
(logger constructed once, injected via context): `NewRuntime` constructs the active `Presenter`
(console, JSON, or — once Wave 3 lands — TUI, selected the same way the logger's handler is
selected) and makes it available for constructor injection into `TerraformStack`,
`KubernetesManager`, `Provisioner`, `WorkstationVirt`/`NetworkManager`, and `ArtifactBuilder` — the
same five call sites currently reaching `tui.Active` directly. This is dependency injection, not
context-carried, unlike the logger: a presenter is a structural collaborator business types already
take constructor dependencies for (`ConfigHandler`, `Shell`), where the logger is inherently
per-call-site ambient state. Matching each to the injection style already established for its kind
of dependency, not introducing a third pattern.

### 4. Three renderers behind the port

- **Console** (default) — `KindApplying`/`KindApplied`/`KindProgress` drive spinner-style terminal
  output (the rendering logic already in `pkg/tui/tui.go`'s `termSpinner` moves here, adapted to
  read `Event` instead of raw `Start`/`Update`/`Done`/`Fail` calls); `KindFailed` renders through
  the same format ADR 0005's central renderer already defined (`Error [CODE]: message` +
  remediation); `KindLog` renders through the `charmbracelet/log` console handler ADR 0006 already
  selected.
- **JSON** (`--output json`, now a **global persistent flag** — this ADR's resolution of the release
  doc's open question #2, decided as "yes, uniform," see Consequences) — every `Emit` call
  marshals the `Event` as one newline-delimited JSON object to stdout, directly matching Terraform's
  own `-json` shape (Context). Logs, errors, and progress are the same stream, distinguished only by
  `"kind"` — exactly what the keystone section asks for ("renderings of the same event stream, not
  four independent subsystems").
- **TUI** (reserved, Wave 3) — routes `Event`s into a BubbleTea program's message channel instead of
  stdout. Not built here; the port and the `KindInputNeeded`/`ParentID` reservations exist so Wave 3
  implements a fourth renderer against an unchanged contract, not a contract redesign.

### 5. How this reconciles with ADR 0006 (logging) and ADR 0005 (errors) — neither is superseded

- **Logging call sites are unchanged.** Business code still calls `slog.InfoContext(ctx, ...)`
  exactly as ADR 0006 decided. What changes is the **handler** `Runtime` wires into that logger:
  `logging.NewPresenterHandler(presenter)` implements `slog.Handler` and, on `Handle()`, builds a
  `KindLog` event and calls `presenter.Emit`. This is precisely the "TUI-routing handler" seam ADR
  0006 reserved — the presenter *is* the routing target that handler was reserved for.
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

- **Resolves the release doc's open question #2**: `--output json` is a single global persistent
  flag (same tier as `--verbose`), and it covers logs, errors, and progress uniformly through one
  `Presenter` implementation — not per-concern flags. Decided here because the presenter is the only
  place this question has a real answer; deferring it further (as both 0005 and 0006 did) would have
  left it unresolved indefinitely.
- **Three ad-hoc progress mechanisms collapse to one port** — `tui.WithProgress`, the asymmetric
  `tui.Start`/`Fail` pattern, and bespoke `outputFunc` parameters all become `Emit`/`Track` calls,
  closing a fragmentation problem this round's survey found was worse than the release doc's
  original framing (three mechanisms, not one, needed replacing).
- **`pkg/tui` doesn't disappear** — its spinner *rendering* logic (the actual terminal animation
  code in `termSpinner`) is reused inside the console renderer, not thrown away; what's removed is
  the global `Active` singleton and direct business-layer reach into it.
- **Presenter is a fifth constructor dependency** on `TerraformStack`, `KubernetesManager`,
  `Provisioner`, workstation types, and `ArtifactBuilder` — a real, if mechanical, signature change
  to five widely-used types, landing during each one's already-scheduled Wave 2 pass rather than as
  a separate cross-cutting commit.
- **The event vocabulary is a new piece of public-ish surface** (even though `internal/presenter`
  isn't externally importable, it's shared across every business package) — adding a `Kind` later is
  cheap and backward-compatible for JSON consumers (an unrecognized kind just doesn't match a
  switch case); removing or renegotiating one is not, once `--output json` has real consumers.

## Alternatives considered

- **A Go interface with one concrete type per event kind** (`Applying{}`, `Failed{Err error}`, …)
  instead of one struct with a `Kind` tag. Rejected: every renderer has to type-switch across all
  kinds anyway, and a single struct maps directly onto both Terraform's proven `-json` stream shape
  and `cli-utils`' `Event` struct (Context) without per-type `MarshalJSON` implementations.
- **Buffer the full event sequence and emit one JSON document**, Pulumi's approach (Context), instead
  of streaming one JSON object per `Emit` call. Rejected for Windsor's case: a CI pipeline tailing
  `windsor apply --output json` benefits more from incremental, tailable output (Terraform's choice,
  and this ADR's) than from Pulumi's well-formedness guarantee — a killed process leaves a
  truncated-but-still-line-valid NDJSON stream, which is an acceptable failure mode for a log,
  whereas losing the whole document is not. Revisit only if a consumer needs the "always one valid
  JSON blob" guarantee Pulumi optimized for, which no current Windsor use case does.
- **Context-injected presenter, matching the logger's injection style.** Rejected: a presenter is a
  structural collaborator the affected types already take similar dependencies for
  (`ConfigHandler`, `Shell`) via constructor injection; forcing it through context would be a third
  injection pattern for no benefit, where the logger's context injection exists specifically because
  logging is called from many more places than the five presenter-consuming types.
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
