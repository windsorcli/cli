---
name: go-style
description: Enforce Windsor Go file organization, documentation, and package structure conventions. Use when creating or editing any Go implementation, test, mock, or shim file.
---

# Windsor Go Style

## Apply when
- Editing any `*.go` file in this repository.
- Creating new package files.
- Refactoring file layout or comments.

## Package structure

Prefer four files per package (omit any that are not needed):

1. Implementation file (`<name>.go`)
2. Test file (`<name>_test.go`)
3. Mock implementation (`mock_<name>.go`)
4. Shims file (`shims.go`) for OS/runtime/system call abstraction

Never create arbitrary dump files (`misc.go`, `utils.go`, `helpers.go`). Name files by functional boundary.

## File organization

Use section dividers in this exact format:

```go
// =============================================================================
// [SECTION NAME]
// =============================================================================
```

**Implementation files** — sections in order, omit when empty:

1. Constants
2. Types
3. Interfaces
4. Constructor
5. Public Methods
6. Private Methods
7. Helpers

**Test files** — sections in order, omit when empty:

1. Test Setup
2. Test Constructor
3. Test Public Methods
4. Test Private Methods
5. Test Helpers

## Documentation style

Every implementation file begins with a 4-line class header:

```go
// The <Name> is a <brief noun phrase>.
// It provides <primary capability>.
// <Line 3: role context or constraint.>
// <Line 4: secondary capability or design note.>
```

Every function and method has a header comment. No explanatory comments inside function bodies — context belongs in the header.

## Section header naming rule

Section headers must use the **generic category names** listed above — never the name of a specific method, type, or feature. For example:

- ✅ `// Test Public Methods`
- ❌ `// Test FluxStack Plan`
- ❌ `// Test PlanSummary`

All tests for public methods belong under a single `// Test Public Methods` header, regardless of how many distinct methods are covered. Do not create one header per method.

## Editing checklist

- Section headers are valid, in required order, and use generic category names only.
- All functions have header comments.
- No inline comments inside function bodies.
- Naming is consistent with existing package terminology.
- No dump files introduced.
