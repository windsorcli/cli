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

- Test public contracts, state transitions, and externally visible outcomes.
- Avoid testing private methods directly; allow it only for complex pure logic (parsers, edge-case math) where public coverage is impractical.
- Keep tests resilient to refactors that preserve behavior.
- Use Go standard `testing` package. No `testify`, no `ginkgo`, no external test frameworks.
- Use BDD `t.Run` scenario naming with Given/When/Then flow inside each case.

## Test file pattern

```go
// =============================================================================
// Test Setup
// =============================================================================

type mocks struct {
    configHandler *MockConfigHandler
    shell         *MockShell
}

func setupMocks(t *testing.T) *mocks {
    t.Helper()
    return &mocks{
        configHandler: NewMockConfigHandler(),
        shell:         NewMockShell(),
    }
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestFoo_Bar(t *testing.T) {
    t.Run("Success", func(t *testing.T) {
        // Given a valid configuration
        m := setupMocks(t)

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

## Test-writing workflow

- Default: write the full set of `t.Run` cases for the behavior in one pass, then run the whole file (or `-run` the new test function) and report pass/fail. Don't stop to confirm each case individually — that ceremony has outlived its usefulness for routine test-writing.
- Reserve stub-first-then-confirm for cases where the *shape* of coverage is itself a design decision worth checking before investing in it — e.g., scoping a brand-new package's test suite, or a case list long enough that getting it wrong wastes real effort. Propose the `t.Run` names, get a nod, then implement all of them.
- Run tests after writing, not after every single case.

## Prohibited

- Modifying source code during test-engineering-only tasks.
- Using `testify` or any non-standard test library.

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
