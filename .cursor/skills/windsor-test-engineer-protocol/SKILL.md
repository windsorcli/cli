---
name: windsor-test-engineer-protocol
description: Apply Windsor test workflow and test-design standards for unit and integration tests. Use when asked to write, expand, or refactor tests, especially when deciding boundary-focused expectations vs internal/private method coverage.
---

# Windsor Test Engineer Protocol

## Scope
Use this protocol when implementing or changing tests.

## Test design principles
- Prefer boundary and behavioral expectations over internal implementation details.
- Test public contracts, state transitions, and externally visible outcomes.
- Avoid direct private-method testing by default.
- Allow direct private-method tests only when they provide high value for complex pure logic (for example parsers with many edge combinations).
- Keep tests resilient to refactors that preserve behavior.

## Unit vs integration intent
- Unit tests validate package boundaries and explicit expectations with controlled dependencies.
- Integration tests validate CLI command behavior end-to-end:
  - command/subcommand execution
  - parameter/flag behavior
  - real file handling flows (including facets and related config artifacts)
- Integration tests should exercise realistic paths and outputs rather than internal helper call patterns.

## Mandatory workflow
1. Present the full test suite structure first using `t.Run(...)` stubs.
2. Obtain explicit user confirmation before implementing any test case.
3. Implement exactly one `t.Run` case at a time.
4. Run tests after each implemented case.
5. Report results and request confirmation before the next case.

## Prohibited during this protocol
- Implementing multiple new test cases in one step.
- Table-driven or matrix-based tests.
- Modifying source code while executing test-engineering-only tasks.
- Using `testify`; use standard Go `testing`.

## Verification requirements
- Show proposed test case before coding.
- Run only relevant tests after each change.
- Report pass/fail and next proposed case.

## Coverage commands
- `go test -coverprofile=coverage.out ./pkg/<package>`
- `go tool cover -func=coverage.out`
- `go test ./pkg/<package>/... -v`
- `go test ./pkg/<package>/... -run TestName`

