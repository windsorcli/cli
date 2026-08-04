# ADR 0003 — Test strategy: black-box default and characterize-then-refactor

- Status: Proposed
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- Promoted from Wave 2 to Wave 1 — this is a contract every later per-package cleanup pass depends
  on, not an independent cleanup item, and [ADR 0001](0001-apiv2-common-header-schema.md) needs a
  scoped slice of it immediately, before its own implementation starts.

## Context

Surveyed directly (`grep -L "_test$" package declarations` across every `*_test.go` file in `pkg/`
and `cmd/`): **zero** of the 158 existing unit/cmd test files (113 `pkg/` unit + 28 `cmd/` + 17
`integration/`, matching the count already recorded in `release-v0.10.0.md`) declare `package
xxx_test`. Every one declares the same package as the code it tests. The `_public_test.go` /
`_private_test.go` filename split that exists in 6+6 files is a **naming convention**, not black-box
testing — both still compile as `package xxx` with full access to unexported identifiers. 100%
white-box, confirmed rather than assumed.

Two wide interfaces, exact counts (`awk` over the interface block, not a loose grep): `ConfigHandler`
(33 methods, `pkg/runtime/config/handler.go`) and `Shell` (25 methods, `pkg/runtime/shell/shell.go`).
35 hand-written mock files exist across `pkg/`/`cmd/` (no `mockgen`); a mock implementing one of
these interfaces must carry all 33/25 methods regardless of which ones a given test suite exercises.

Checked directly whether any of that surface is genuinely dead, not just theoretically excessive:
cross-referencing each `ConfigHandler` method against real call sites (not test-customization
frequency — that undercounts, see below) turns up **`RegisterProvider`**
(`pkg/runtime/config/handler.go:56`) as a confirmed case — declared on the interface, implemented on
the mock, unit-tested against the concrete `configHandler` in isolation
(`pkg/runtime/config/accessors_test.go`), but called by **nothing** outside its own package's tests:
no `cmd/`, no other `pkg/`, no runtime wiring. That's real dead interface surface, not a hypothetical.
Contrast with three other methods that also show zero test-customization
(`LoadConfigForContextFunc`, `SaveWorkstationStateFunc`, `SetApplySchemaDefaultsFunc` never have
their `Func` field assigned anywhere in the test suite) but that turned out, on the same call-site
check, to be genuinely used in production (`cmd/get.go`, `pkg/workstation/workstation.go`,
`pkg/runtime/runtime.go`, `pkg/test/runner.go`) — those are a mock-coverage gap, not dead code, and
removing them would be wrong. **The distinguishing signal is real call sites, not mock-customization
frequency** — a method can be production-critical and still never need a custom mock return value.

This matters immediately, not just for Wave 2's codebase sweep: [ADR 0001](0001-apiv2-common-header-schema.md)
rewrites `pkg/runtime/config`'s read path (v1alpha1-unmarshal-everything → typed v1alpha2 header +
dynamic map), and [ADR 0002](0002-target-architecture-and-package-topology.md) splits thirteen
monolith files in place and extracts `internal/shims`/`internal/util` out of 16 packages plus `cmd/`.
Every one of those is an internal-representation change with no intended change in observable
behavior — exactly the situation where a white-box test suite becomes a liability instead of a
safety net: it's coupled to *how* the code is organized (which private method holds which logic,
which file a function lives in, which struct field a shim uses), not to *what* the code does, so it
breaks on the refactor whether or not the refactor was correct. Left unaddressed, the white-box
suite would need patching once to survive ADR 0001/0002's structural moves, then patched again
later when Wave 2's sweep converts it to black-box — the same code paying for two test rewrites.

## Decision

### 1. Black-box (`package xxx_test`) is the default for new and touched test files

A test file asserts through the package's exported contract — its constructors, public methods,
and observable state transitions — not through unexported identifiers. This is `test-engineer`'s
existing "test public contracts... avoid testing private methods directly" principle, made the
default rather than a preference, and given a concrete package-declaration marker
(`package xxx_test`) so adherence is mechanically checkable, not just a style convention nobody
enforces.

### 2. White-box is a deliberate, narrow exemption — not a fallback

A private-method test is permitted only for **complex unexported logic where public coverage is
impractical** — parsers, edge-case math, the kind of pure-function case enumeration `test-engineer`
already names (see `pkg/composer/terraform/oci_module_resolver_private_test.go`'s
`HandlesEdgeCases` as the existing worked example). The bar is "public coverage genuinely can't
reach this branch economically," not "it's more convenient to call the private function directly."
Concretely: the deferred-substitution scope-blinding logic in `pkg/composer/blueprint/processor.go`,
the semver/tag-walk logic in `pkg/composer/artifact/artifact.go`, and comparable dense-algorithm
code are the kind of thing this exemption is *for*; a config handler's getter/setter pair is not.

### 3. Characterize-then-refactor is binding on every structural change, starting with ADR 0001

For a package about to be structurally touched (file split, shims extraction, interface split, or
an internal-representation rewrite like ADR 0001's), the order is fixed:

1. **Black-box-characterize current public behavior first.** If black-box coverage of the package's
   public surface doesn't already exist, write it before touching internals — this is the safety
   net, and it has to exist *before* the refactor to do its job.
2. **Do the structural refactor** under that net.
3. **The black-box tests pass unchanged.** That's the proof the refactor preserved behavior — the
   same property a `terraform plan` gives you for infrastructure, applied to code structure.
4. **Only then add white-box tests**, and only for whatever complex unexported logic survives the
   refactor and still meets the point-2 bar. Never the reverse order — a private-method test written
   before the refactor is exactly the thing that breaks during it for no behavioral reason.

This is scoped per package, not a global up-front sweep: front-loading black-box conversion across
the whole codebase before any refactor touches most of it would be speculative effort disconnected
from when the actual restructuring happens. The rule fires exactly when a package is about to move
under 0001 or a Wave 2 pass — not before, not as a separate disconnected pass after.

**Immediate application:** ADR 0001's first implementation step is black-box characterization of
`pkg/runtime/config`'s current behavior — asserting through `LoadConfig`/`LoadContext`/
`GetContextValues`, never through `typedSource` internals — before the v1alpha1→v1alpha2 header
rewrite begins. This is not deferred to whenever Wave 2 reaches `pkg/runtime/config` in its
per-package order; it's pulled forward as part of 0001 itself.

### 4. Coverage floor: >80%, measured on the package after conversion, not before

The `>80%` figure from `release-v0.10.0.md`'s original scoping table is ratified as a floor per
package once its black-box conversion lands — not a repo-wide average that lets one well-tested
package subsidize a neglected one. `task test:all`'s existing coverage tooling
(`go tool cover -func=coverage.out`) is the measurement, no new tooling needed.

### 5. Mock rationalization: drop what nothing calls, keep what does — checked by real call sites

No mock, and no mock method, exists for its own sake. Every package's cleanup pass audits its
interface(s) and mock(s) against real call sites (`grep` the method name across the codebase
excluding the interface's own package tests and mock file — the check that found `RegisterProvider`
above) before touching them, and:

- **A method with zero call sites outside its own package's tests is dead interface surface.**
  Remove it from the interface, its concrete implementation, and its mock — not just "unused," a
  method nobody calls is code with no reason to exist. `RegisterProvider` is the first confirmed
  instance; each package's pass repeats this check for its own interfaces rather than assuming the
  rest are clean.
- **A method with real call sites but no test customization is a coverage gap, not dead code** — it
  stays, and if the package's black-box conversion (point 3) doesn't already exercise it through a
  scenario that needs a custom return value, that's a test to add, not a method to delete. Mock
  customization frequency alone is the wrong signal (see Context) — it flags `RegisterProvider` and
  three genuinely-needed methods identically; only the call-site check tells them apart.
- **Wide interfaces split along their existing seams** once the dead methods are gone —
  `ConfigHandler`: load/get-set/schema-validate/render/sensitive-paths; `Shell`: exec/verbosity/
  secret-registration/session-lifecycle — when their owning package's cleanup pass reaches them. A
  split interface's mock then only carries the methods its own seam actually needs.

This rides the same per-package pass as everything else in this ADR — not a separate mock-only
sweep across all 35 mock files up front, for the same reason point 3 scopes structurally: most
mocks aren't being touched yet, and auditing them before their package's pass reaches them is
speculative effort against code that may still change shape before it matters.

### 6. Fixture cleanup is opportunistic, not a scheduled sweep

Stale or duplicated test fixtures (YAML blueprints, context templates) get cleaned up as part of
whichever package's black-box conversion touches the tests that reference them — not tracked as a
separate backlog item, since fixture staleness is only ever visible in the context of the tests
using it.

## Consequences

- **ADR 0001 and every Wave 2 per-package pass now carry a test-conversion step as part of their own
  scope**, not as follow-on work from a separate ADR — this is intentional; it's what keeps the
  double-rework this ADR exists to prevent from happening.
- **Removing a dead interface method is a breaking change to that interface**, not just a test-file
  edit — every mock and every real implementation of the interface must drop the method together in
  the same commit, verified by the compiler (a mock left implementing a stale interface method is
  harmless; a real implementation missing a still-declared method fails to build). `RegisterProvider`
  removal touches `handler.go`, `accessors.go`, `mock_handler.go`, and `accessors_test.go` together.
- **Coverage may dip temporarily mid-conversion** for a package where the black-box net doesn't yet
  reach every branch the old white-box suite happened to cover — acceptable as a transient state
  within one package's pass, not acceptable to leave unresolved once that pass is marked done.
- **The white-box exemption is a judgment call each pass has to make explicitly** — "public coverage
  is impractical" is not self-evident, so each pass's PR description should name which unexported
  logic it kept white-box tests for and why, per the `large-pr` change-map convention.
- **No mass mechanical rewrite of all 158 existing test files up front.** Files convert exactly when
  their package is touched by 0001 or a Wave 2 pass — some packages may not convert until late in
  the release, and that's fine; the rule only binds packages actually being restructured.

## Alternatives considered

- **Convert every existing test file to black-box before any structural work starts.** Rejected:
  speculative effort against packages that may not be restructured for months, and delays Wave 0/1
  for no immediate benefit — the value of black-box coverage is realized at the moment of refactor,
  not before.
- **Refactor first across 0001/0002, fix tests afterward.** Rejected: this is the exact double-rework
  pattern this ADR exists to prevent — white-box tests break during the structural move for reasons
  unrelated to correctness, then break again when later converted to black-box.
- **Keep the `_public_test.go`/`_private_test.go` naming convention as sufficient.** Rejected: it
  documents intent but doesn't enforce it — both file variants compile as the same white-box
  package today, so the convention alone doesn't deliver the refactor-resilience this ADR needs.
- **Repo-wide coverage average instead of a per-package floor.** Rejected: lets a strong package mask
  a weak one; a per-package floor is the only version that actually gates each pass's completion.
- **A dedicated, upfront mock-audit pass across all 35 mock files, ahead of Wave 2.** Rejected: same
  reasoning as declining an upfront black-box conversion — most mocks aren't touched by 0001/0002
  yet, so auditing them now is speculative. Also rejected: **using mock-customization frequency
  (never-assigned `Func` fields) as the removal signal.** Checked directly and found it produces
  false positives — three genuinely production-critical methods showed zero customization, the same
  signal `RegisterProvider`'s real dead code showed. Only a real call-site check distinguishes them.
