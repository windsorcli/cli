# ADR 0008 — Context resolution: one read per process, not per call

- Status: Proposed
- Date: 2026-09-17
- Deciders: Ryan VanGundy
- Surfaced while investigating destroy-isolation guarantees for cli#3385/#3395: neither issue
  needed this fix, but confirming `--context`'s behavior led to auditing whether a running
  command is isolated from a concurrent `windsor set context` in another terminal. It is not,
  except where a subsystem happened to add its own ad hoc caching.

## Context

`configHandler.GetContext()` (`pkg/runtime/config/handler.go:397-420`) resolves the active
context in priority order: an in-memory override, then `.windsor/context` on disk, then
`WINDSOR_CONTEXT`, then `"local"`. Without `--context`, the override is always empty, so **every
call re-reads the file from disk** — there is no caching in the base path.

That live-read design is deliberate for one specific case, documented at handler.go:394-396: file
takes precedence over the env var so that a command run immediately after `windsor set context`
sees the new context before the user's shell hook has updated `WINDSOR_CONTEXT`. That reasoning
is about **sequencing across processes** — `set context` in process A, then a plain command in
process B. It does not address, and was not written to address, a second process mutating that
same file while process A is still running.

`windsor set context` (`cmd/set.go`'s `setContextCmd`) calls `rt.ConfigHandler.SetContext(name)`
(`handler.go:451-473`), which writes `.windsor/context` and sets `WINDSOR_CONTEXT` in its own
process. Nothing about `SetContext` touches any cache — despite its doc comment ("updates the
cache") — because there is no cache to update.

Two places have already independently reinvented a fix for the same race, each scoped narrowly to
its own subsystem:

- `Runtime.ContextName` (`pkg/runtime/runtime.go:202-208`) — calls `GetContext()` once at
  `NewRuntime()` and holds the result in a plain struct field for the process's life.
- `terraformProvider.providerScope()` (`pkg/runtime/terraform/provider.go:849-875`) — resolves
  context, config root, and scratch path once behind a `sync.RWMutex`, with a comment naming the
  exact hazard: *"configHandler.GetContext() reads a file shared by every windsor process in the
  checkout. A live read on each call would let a concurrent windsor process for another context
  change this provider's Terraform args mid-run."*

Everywhere else still calls `GetContext()` live, per call, with no such guard — roughly 39 call
sites across `pkg/workstation/virt/{colima,incus}_virt.go`,
`pkg/workstation/network/{colima,linux}_network.go`, `pkg/workstation/workstation.go`,
`pkg/runtime/tools/tools_manager.go`, `pkg/runtime/evaluator/evaluator.go`,
`pkg/runtime/env/{aws,virt,windsor}_env.go`, `pkg/composer/blueprint/composer.go`, and
`pkg/provisioner/kubernetes/kubernetes_manager.go`. Any of these, called after a concurrent
`windsor set context` lands mid-run, picks up the new context value for the rest of that command —
so a single `windsor destroy` (or `apply`, or `bootstrap`) could resolve workstation/network state
against one context while provisioner/kubernetes state still reflects the context it started
with, entirely silently.

## Decision

**Resolve context once per `ConfigHandler` instance, at first read, and cache it for the life of
that instance.**

### 1. `GetContext()` caches its result on first resolution

A second, unexported cache field, distinct from the existing override field `c.context` (set only
by `WithContext`, which already takes priority). On first call, `GetContext()` resolves exactly as
today — override, then file, then env, then default — and stores the result. Every later call on
that same instance returns the cached value without touching disk or the environment again.

### 2. One fix, at the one place every call site already funnels through

This generalizes what `Runtime.ContextName` and `providerScope()` each built for themselves, and
applies it once, where all ~39 call sites already converge — no call-site changes required to get
the guarantee.

### 3. The documented cross-process case is preserved

A new `windsor` invocation constructs a new, uncached `ConfigHandler`; its first `GetContext()`
call still reads whatever `.windsor/context` holds at that moment. `set context` in process A,
then a plain command in process B, keeps working exactly as it does today — only a single
process's *own* view is now stable for its own duration.

### 4. Confirmed safe against the one in-process caller that both sets and reads

`setContextCmd` calls `LoadConfig()` before `SetContext()`, and never calls `GetContext()`
afterward. `cmd/bootstrap.go:94` and `cmd/init.go:91` go further and call `SetContext` on a
disposable `tempRt.ConfigHandler`, never the main runtime's — independent evidence the codebase
already treats "write a new context mid-process" and "keep using the one this process resolved"
as separate concerns.

### 5. `SetContext` sets this same instance's override, not just the file

Discovered during implementation: this repo's tests routinely construct a handler, call
`SetContext` to seed a fixture, then immediately read context back (directly, or indirectly
through something like `GetEnvVars`) on that same instance — a legitimate in-process
"change it, then use it" sequence, distinct from the cross-process race this ADR closes.

First attempt: `SetContext` wrote the new value into `resolvedContext`. Wrong on a
`WithContext`-derived handler, since `GetContext` checks the override field `c.context` before
ever consulting `resolvedContext` — the write was silently dead code there. Fixed by having
`SetContext` set `c.context` directly, the same field `WithContext` sets: `SetContext` now wins
over any earlier override, on any handler, immediately. This does not weaken the isolation
guarantee: a *different* handler (a different process, or a concurrent `windsor set context`)
still only sees the change on its own next resolution.

### 6. Retire the now-redundant duplicate caches

Once the source is stable, `providerScope()`'s mutex-guarded cache becomes redundant and should be
deleted in the same change. `Runtime.ContextName` can stay as a convenience field, now just
mirroring an already-stable `GetContext()`.

### 7. The cache degrades gracefully, and reads don't serialize on writers

Found in review, not in the original survey:

- A `configHandler` built by hand rather than through `NewConfigHandler` (the test suite does
  this for a few unrelated error-path cases) would have a nil cache field. `GetContext` and
  `SetContext` both check for nil first and fall back to the old, uncached behavior rather than
  panicking — self-healing rather than asserting an invariant this package can't actually
  enforce, since nothing stops a struct literal from skipping the constructor.
- `contextCache` uses `sync.RWMutex` with a double-checked read, not a plain `Mutex`: with ~39
  call sites now converging on one cache, an already-resolved read should not queue behind other
  readers just to check a bool.

Out of scope: exclusive access to a context's resources across concurrent `windsor` processes
(e.g., two `apply` runs against the same context at once). This ADR only guarantees that one run
picks a context and keeps it — not that nothing else may act on that context concurrently.

## Consequences

- A `windsor` process resolves its context exactly once, at first use, and is isolated from any
  `windsor set context` run anywhere else — same or different process — for the rest of its run.
- The fix collapses two independent, narrowly-scoped reimplementations into one place, and closes
  the gap for every other call site without touching them.
- The change sits underneath nearly every command, so verification has to be broad even though the
  diff is small: the ~39 call sites need to be swept to confirm none secretly depended on a live
  re-read (none found in this survey, but the surface spans six packages).
- Implemented as a `large-pr`: the handler-level cache landed first, with its own unit tests
  (including a `-race` run proving no data race across concurrent readers). `providerScope()`'s
  redundant cache was then deleted. The call-site sweep found no site that assumed a live
  re-read; the one real regression it caught was in test fixtures across several packages that
  call `SetContext` then read context back on the same handler — fixed by Decision #5, not by
  reverting the cache.
- A true multi-process integration test (start a real `windsor` subprocess, mutate
  `.windsor/context` from the test while it runs, assert no behavior change) was considered and
  dropped: this harness runs each CLI invocation as a separate process, so exercising a
  mid-single-process race would need a fragile, timing-based hook with no natural point to pause
  the subprocess. The isolation guarantee is a property of `ConfigHandler` itself, and is proven
  at that level under `-race`; the existing `integration/context_flag_test.go` continues to pass
  and covers real subprocess context resolution end to end.

## Alternatives considered

- **Leave it to each subsystem to cache for itself.** Rejected — this is the status quo, and it
  already produced two divergent reimplementations while leaving 39 sites unprotected. Nothing
  forces a new subsystem to remember the guard.
- **Lock or PID-file gate against a concurrent `windsor set context`.** Rejected — heavier than the
  problem calls for, and it solves the wrong thing. The actual requirement is "this process's own
  view of context stays stable," not "no other process may change context while I run," which is
  normal, desired usage (switching context for the next command while a long destroy of the old
  one finishes).
- **Thread the resolved context name explicitly through every constructor and method signature.**
  More strictly correct, but a far larger diff for the identical guarantee a source-level cache
  already provides. Rejected as disproportionate given the fix is available closer to the root.

## References

- `pkg/runtime/config/handler.go:113-117` (`contextCache`), `:414-438` (`GetContext`),
  `:448-462` (`WithContext`), `:473-...` (`SetContext`), `:694-...` (`resolveContextFromFileOrEnv`,
  Private Methods section)
- `pkg/runtime/runtime.go:202-208` (`Runtime.ContextName`)
- `pkg/runtime/terraform/provider.go:842-...` (`providerScope`, cache removed)
- `cmd/set.go` (`setContextCmd`), `cmd/bootstrap.go:94`, `cmd/init.go:91` (disposable-runtime
  `SetContext` usage)
