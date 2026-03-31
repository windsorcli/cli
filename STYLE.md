# Windsor CLI Style Guide (Concise)

Detailed execution playbooks are now defined in project skills:
- `.cursor/skills/windsor-go-style/`
- `.cursor/skills/windsor-test-engineer-protocol/`
- `.cursor/skills/windsor-runtime-boundaries/`
- `.cursor/skills/windsor-large-pr-control/`

This file defines the durable style contract for contributors and reviewers.

## 1) Package Structure

Prefer packages with:
1. implementation file
2. test file
3. mock implementation file (when useful)
4. shims file for system-call abstraction

## 2) File Organization

Section headers must use:
```go
// =============================================================================
// [SECTION NAME]
// =============================================================================
```

Implementation files may include (in order, only when non-empty):
1. Constants
2. Types
3. Interfaces
4. Constructor
5. Public Methods
6. Private Methods
7. Helpers

Test files may include (in order, only when non-empty):
1. Test Setup
2. Test Constructor
3. Test Public Methods
4. Test Private Methods
5. Test Helpers

## 3) Documentation Style

### Class header
Implementation files should include a 4-line class header at the top:
- line 1 starts with `The <Name> is a`
- line 2 starts with `It provides`
- lines 3-4 explain role and capabilities

### Function comments
- Every function/method has a header comment.
- No explanatory comments inside function bodies.
- Keep behavior/context in the function header.

## 4) Testing Style

- Use Go standard `testing` package.
- Prefer BDD `t.Run` scenario naming and Given/When/Then flow.
- For strict test-engineering workflows, follow `.cursor/skills/windsor-test-engineer-protocol/`.

## 5) Code Organization Principles

- Split files by functional boundaries, not arbitrary helper collections.
- Keep interfaces focused and minimal.
- Keep implementation details private unless required by external package consumers.
- Prefer explicit constructor/method APIs over hidden side channels.

## 6) Shims

- Use shims for OS/runtime/system interactions.
- Keep shims simple and dependency-injectable for tests.
- Override shim functions in tests rather than monkey-patching behavior elsewhere.
