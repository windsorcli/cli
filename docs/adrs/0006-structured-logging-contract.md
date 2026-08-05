# ADR 0006 — Structured logging contract: `slog`, context-injected, level-aware

- Status: Proposed
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- Fills the "Structured logging contract" placeholder from `release-v0.10.0.md`'s Wave 1 — the last
  content gap before the Presenter/event-stream ADR can be drafted, since Presenter unifies logging,
  errors, and progress rendering rather than inventing what any of them contain.

## Context

Verified directly: **no `log/slog` usage anywhere in the codebase today**, and **no
`charmbracelet`/`bubbletea`/`lipgloss` dependency** in `go.mod` — both named in the release doc's
scoping table as the intended stack, neither started. **46 files** call `fmt.Print*`/`fmt.Fprint*`
directly. The one existing output abstraction is `pkg/tui/tui.go`: a package-level `Active Spinner`
global plus an `activeDepth` nesting counter and `WithProgress` wrapper — and it's reached directly
from business layers, not just `cmd/`: `pkg/workstation/{network,virt}/*.go`,
`pkg/composer/artifact/artifact.go`, `pkg/runtime/shell/shell.go`, `pkg/provisioner/terraform/
stack.go`, `pkg/provisioner/provisioner.go`, and `pkg/provisioner/kubernetes/kubernetes_manager.go`
all call `tui.Active`/`tui.WithProgress` directly — nine files confirming exactly the coupling the
release doc's keystone section names as the root cause of the UX complaints.

Verbosity plumbing already exists, but split across two disconnected mechanisms: `cmd/root.go`
calls `proj.Runtime.Shell.SetVerbosity(verbose)` **and separately** does
`ctx = context.WithValue(ctx, "verbose", true)` — a **string-keyed** `context.WithValue` call,
which is a known Go anti-pattern (unexported typed keys avoid collision risk; a bare string key
doesn't). Two calls, two mechanisms, one flag. `Shell.IsVerbose()` also does double duty today —
it's read both for log-level-style decisions and for actual execution-mode branching (stream
subprocess output live vs. capture silently), which are different concerns currently bundled into
one bool.

Some scaffolding already exists to build on: `cmd/root.go` does construct a real `context.Context`
(`context.Background()`, then threaded through the command), so context-injected logging is an
extension of an already-partially-plumbed mechanism, not new infrastructure from nothing.

## Decision

### 1. `log/slog` is the call-site API; business code never touches a handler directly

`pkg/`/`cmd/` code logs via `slog.InfoContext(ctx, "message", "key", value, ...)` and friends,
retrieving the active `*slog.Logger` from `context.Context` — never a package-level global, never a
handler constructed inline. This is the direct fix for the `tui.Active`-from-business-layers problem
in Context: a logger obtained from context is trivially fakeable in tests and has no reach-around
path the way a package global does.

### 2. Context injection: typed key, `internal/logging`, closes the string-key smell

```go
// internal/logging (sibling to internal/shims/util/werror from ADR 0002/0005 — reachable from
// both cmd/ and pkg/, invisible to anything outside this module).

type ctxKey struct{}

func NewContext(ctx context.Context, logger *slog.Logger) context.Context
func FromContext(ctx context.Context) *slog.Logger // never nil — falls back to a safe default
```

This retires `cmd/root.go`'s bare `context.WithValue(ctx, "verbose", true)` in favor of the typed
key above, closing the collision-risk smell found in Context — one mechanism, not two, and it's the
existing `ctx` variable already being threaded, not a new context to plumb in.

### 3. Handlers: console (default), JSON (`--output json`), TUI-routing (reserved seam)

Three backends behind the same `slog.Handler` interface, selected once at construction:

- **Console** (default) — `charmbracelet/log`, human-readable, leveled, colored. New dependency,
  added by this ADR (the release doc's scoping table already named it; this is where it lands).
- **JSON** (`--output json` / CI, detected the same way other output-format decisions already are)
  — `slog.JSONHandler`, stdlib, no new dependency.
- **TUI-routing** (reserved now, implemented in Wave 3) — while a BubbleTea program owns the
  screen, a log record must not hit stdout directly, or it corrupts the TUI's own rendering. This
  ADR reserves the hook — a handler that checks an active-TUI signal (the same shape as `tui.go`'s
  existing `activeDepth`, generalized) and routes to an event channel instead of `os.Stdout` when
  set — without building the channel or the BubbleTea program itself. Call sites never change when
  Wave 3 fills this in; only the handler's internal routing does.

### 4. Level mapping: `--verbose` maps to `slog.LevelDebug`, unifies the two existing mechanisms

`--verbose`/`-v` (already exists, `cmd/root.go`) sets the injected logger's level to
`slog.LevelDebug`; unset, it's `slog.LevelInfo`. This becomes the **single** source of truth for
verbosity — `cmd/root.go`'s current dual plumbing (`Shell.SetVerbosity` call plus the string-keyed
context value) collapses to one: construct the logger with the right level, inject it via point 2,
done. `Shell.IsVerbose()` stays, but as a **derived** read from the context logger's level via
`logging.FromContext(ctx).Enabled(ctx, slog.LevelDebug)`, not a second independently-set bool — this
is the one place this ADR reaches into Shell's existing surface, and it's a narrowing (one flag,
one flow), not new API. Shell's separate execution-mode behavior (stream vs. capture subprocess
output) is a distinct concern this ADR does not touch — it may still read the same derived signal,
but that's a `pkg/runtime/shell` implementation choice, not a logging-contract decision.

### 5. Logger construction lives in `Runtime`, injected at the `cmd/` boundary

Per the already-ratified layer table (ADR 0002: "Runtime — Lifecycle orchestration and dependency
wiring"), `NewRuntime` constructs the base `*slog.Logger` (handler chosen by `--output`, level by
`--verbose`) as part of its existing dependency-wiring responsibility. `cmd/root.go`'s
`PersistentPreRun`-equivalent injects it into the command's `context.Context` via
`logging.NewContext` (point 2) once, at the top — every downstream `pkg/` call retrieves the same
logger via `logging.FromContext(ctx)`, never constructs its own.

### 6. `WindsorError` integration: structured fields, not string interpolation

When a caller logs an error that `errors.As`-unwraps to a `*WindsorError`
([ADR 0005](0005-typed-error-model.md)), the log call passes it as a structured attribute
(`slog.Any("error", err)`) and a custom `LogValue()` method on `*WindsorError` expands it to
`code`, `category`, and `message` fields — never string-interpolated into the log message itself.
This is the same principle as `go-style`'s `errors.As` rule applied to logging: the code and
category stay machine-readable fields in JSON output, not buried in free text.

### 7. Migration: per-package, selective — not a mechanical 46-site sweep

Consistent with ADR 0005's error-migration approach: a raw `fmt.Print*`/`fmt.Fprint*` call converts
to a `slog` call when its package's Wave 2 cleanup pass reaches it, not as a separate upfront sweep.
The nine files reaching `tui.Active` directly are **not** converted by this ADR — removing that
direct reach is explicitly the Presenter ADR's job (per the release doc: Presenter "removes direct
`os.Stdout` / `tui.Active` reach from business layers"), since it requires the domain-event
vocabulary this ADR doesn't define. This ADR's job is narrower: the call-site API, the handlers, and
context injection are ready for Presenter to build on, not a completed migration.

## Consequences

- **One flag, one flow** for verbosity — closes the dual-mechanism (`SetVerbosity` +
  string-keyed context value) smell found in Context, and the string-key anti-pattern with it.
- **New dependency** (`charmbracelet/log`) lands with this ADR rather than waiting for full
  BubbleTea adoption in Wave 3 — reasonable since the console handler is needed immediately, not
  just once the TUI exists.
- **The TUI-routing handler is a reserved seam, not a working feature** until Wave 3 — a log call
  made today behaves correctly (console or JSON) but doesn't yet coordinate with a TUI program,
  because no TUI program exists yet. No regression; nothing to coordinate with.
- **Business layers still reach `tui.Active` directly until Presenter's migration lands** — this ADR
  does not fix the nine-file coupling found in Context, it only stops making it worse (new/touched
  log call sites use the injected logger, not a new global) and prepares the handler seam Presenter
  will route through.
- **`--output json` for logging is decided; the release doc's global-JSON-surface open question
  (errors + progress + logs uniformly, or per-concern) stays open**, same posture ADR 0005 took for
  errors — this ADR's handler choice satisfies either resolution.

## Alternatives considered

- **A third-party structured-logging library** (`zap`, `zerolog`, `logrus` — the last already a
  transitive dep per the release doc's current-state survey). Rejected: `log/slog` is stdlib as of
  Go 1.21 (this repo targets 1.26), needs no new dependency for the core API, and the release doc's
  own scoping table already locked in `slog` — this ADR implements that choice rather than
  relitigating it.
- **Keep `Shell.SetVerbosity`/`IsVerbose` as the sole verbosity mechanism, no context-injected
  logger.** Rejected: doesn't give `pkg/` packages outside `pkg/runtime/shell` any way to read the
  level, and doesn't solve the `tui.Active`-global-reach problem at all.
- **Build the TUI-routing handler now, ahead of BubbleTea.** Rejected: there's nothing to route to
  yet — the seam is reserved so call sites don't change later, but building the routing logic before
  Wave 3's TUI program exists would mean guessing its shape.
- **Convert all 46 raw print sites in one mechanical pass.** Rejected for the same reason ADR 0005
  rejected a mechanical 882-site error conversion — most convert naturally as their package's Wave 2
  pass reaches them; forcing it now is effort disconnected from when the code is actually touched.
