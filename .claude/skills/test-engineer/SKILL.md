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

## Mandatory TDD workflow

1. Present the full test suite structure as `t.Run` stubs first.
2. Get explicit confirmation before implementing any case.
3. Implement exactly **one** `t.Run` case at a time.
4. Run tests after each case. Report pass/fail.
5. Request confirmation before the next case.

## Prohibited

- Implementing multiple test cases in one step.
- Table-driven or matrix-based tests.
- Modifying source code during test-engineering-only tasks.
- Using `testify` or any non-standard test library.

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
