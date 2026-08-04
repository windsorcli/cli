---
name: test-engineer
description: Apply Windsor test workflow and design standards for unit tests. Use when writing, expanding, or refactoring tests, or deciding between public boundary coverage vs private method testing.
---

# Windsor Test Engineer

## Apply when
- Writing or expanding unit tests.
- Refactoring existing test files.
- Deciding what to test and at which boundary.

## Test design principles

- **Black-box (`package xxx_test`) is the default for new and touched test files** — test through the
  package's exported constructors and methods, not its unexported identifiers. This is enforced by
  the package declaration itself, not just a preference: if a test needs an unexported identifier to
  compile, that's the signal to check whether it belongs under the exemption below or whether the
  package's public surface is missing something it should expose.
- **White-box is a narrow, deliberate exemption, not a fallback.** Permitted only for complex
  unexported logic where public coverage is genuinely impractical to reach — parsers, edge-case math,
  dense algorithmic branches (see `pkg/composer/terraform/oci_module_resolver_private_test.go`'s
  `HandlesEdgeCases`). "It's more convenient to call the private function directly" is not the bar;
  "no reasonable sequence of public calls exercises this branch" is.
- Keep tests resilient to refactors that preserve behavior — a black-box test by construction doesn't
  care which file or private method the behavior lives in, only that the public contract holds.
- Use Go standard `testing` package. No `testify`, no `ginkgo`, no external test frameworks.
- Use BDD `t.Run` scenario naming with Given/When/Then flow inside each case (see below — this is a
  structural requirement, not a loose suggestion).

### When refactoring a package's internals (file splits, shims extraction, interface changes)

Characterize before you move anything: if black-box coverage of the package's current public
behavior doesn't already exist, write it first. Do the structural change under that net. The
black-box tests should pass unchanged — that's the proof the refactor preserved behavior. Only after
that does any new white-box test get added, and only for unexported logic that still meets the
exemption bar above post-refactor. Never refactor first and patch tests afterward — a private-method
test written before a refactor is exactly the thing that breaks during it for reasons that have
nothing to do with correctness.

### Mocks: only what's actually called

A mock implements every method its interface declares (the compiler requires it), but that is not
license for the interface to carry a method nothing calls. Before adding a new interface method (and
its mock), confirm it has a real production call site planned, not just "the interface should
probably support this." When touching an existing mock, check each method against real call sites —
excluding its own package's tests and the mock file itself — not against whether tests customize its
return value: a method can be production-critical and never need a custom mock return, and a method
can look identically "unused" by customization frequency while actually being genuine dead code. Only
the call-site check tells the two apart. A confirmed-dead method is removed from the interface, its
real implementation, and its mock together, in the same change — never left on the mock "just in
case."

## Test file pattern

Package declaration is `foo_test`, not `foo` — the black-box default from above, made concrete:

```go
package foo_test

// =============================================================================
// Test Setup
// =============================================================================

type mocks struct {
    configHandler *foo.MockConfigHandler
    shell         *foo.MockShell
}

func setupMocks(t *testing.T) *mocks {
    t.Helper()
    return &mocks{
        configHandler: foo.NewMockConfigHandler(),
        shell:         foo.NewMockShell(),
    }
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFoo_Bar(t *testing.T) {
    t.Run("Success", func(t *testing.T) {
        // Given a valid configuration
        m := setupMocks(t)
        subject := foo.NewFoo(m.configHandler, m.shell)

        // When Bar is called
        result, err := subject.Bar()

        // Then no error is returned
        if err != nil {
            t.Fatalf("expected no error, got %v", err)
        }
        _ = result
    })
}
```

### BDD comment discipline — exactly three lines, nothing more

Each `t.Run` case gets exactly one `// Given`, one `// When`, one `// Then` comment, immediately
above the code they describe — no additional narrative, no restating what the next line does beyond
those three labels. This mirrors the production-code rule against in-body novels: the comment names
the phase, the code shows the mechanics.

```go
// ✅ Three labeled phases, nothing extra
t.Run("ReturnsErrorWhenConfigMissing", func(t *testing.T) {
    // Given no config file exists
    m := setupMocks(t)
    m.configHandler.LoadConfigFunc = func() error { return os.ErrNotExist }

    // When LoadOrDefault is called
    err := subject.LoadOrDefault()

    // Then a not-found error is returned
    if !errors.Is(err, os.ErrNotExist) {
        t.Fatalf("expected not-exist error, got %v", err)
    }
})

// ❌ Narrative padding beyond the three labels
t.Run("ReturnsErrorWhenConfigMissing", func(t *testing.T) {
    // Given no config file exists — this simulates a fresh checkout where
    // windsor.yaml hasn't been created yet, which is the most common case
    // new users hit on their first `windsor init`
    ...
})
```

Subtest names are `PascalCaseScenarioDescription` (`ReturnsErrorWhenConfigMissing`, not
`"returns error when config missing"` or `"Success"` alone once a file has more than one scenario) —
specific enough that a failing subtest name alone tells you what broke.

## Test-writing workflow

- Default: write the full set of `t.Run` cases for the behavior in one pass, then run the whole file (or `-run` the new test function) and report pass/fail. Don't stop to confirm each case individually — that ceremony has outlived its usefulness for routine test-writing.
- Reserve stub-first-then-confirm for cases where the *shape* of coverage is itself a design decision worth checking before investing in it — e.g., scoping a brand-new package's test suite, or a case list long enough that getting it wrong wastes real effort. Propose the `t.Run` names, get a nod, then implement all of them.
- Run tests after writing, not after every single case.

## Prohibited

- Modifying source code during test-engineering-only tasks.
- Using `testify` or any non-standard test library.
- Adding a white-box test as the first move on a package about to be structurally refactored —
  characterize with black-box coverage first (see above).
- Adding a mock method for an interface method with no real, currently-planned production call site.

## Table-driven tests

Prefer `t.Run` BDD scenarios (Given/When/Then) for behavior-level tests — they name what's being verified and fail with a readable subtest name. Table-driven (`tests := []struct{...}`) is fine, and already used in this codebase, specifically for enumerating edge cases of pure logic (parsers, validators, format checks) where the cases are homogeneous and the table itself is the clearest representation — see `pkg/composer/terraform/oci_module_resolver_private_test.go`'s `HandlesEdgeCases` for a real example. Don't reach for it to cover heterogeneous behavior that reads better as named `t.Run` cases.

## Test commands

Full suite with coverage:
```
task test:all
```

Targeted package tests:
```
go test ./pkg/<package>/... -v
go test ./pkg/<package>/... -run TestName
go test -coverprofile=coverage.out ./pkg/<package>
go tool cover -func=coverage.out
```

Filtered via task:
```
task test -- -run TestName
```
