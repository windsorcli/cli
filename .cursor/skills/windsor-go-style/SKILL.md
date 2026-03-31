---
name: windsor-go-style
description: Enforce Windsor Go file organization, documentation, and package structure conventions from STYLE.md. Use when creating or editing Go implementation, test, mock, or shim files.
---

# Windsor Go Style

## Apply when
- Editing any `*.go` file in this repository
- Creating new package files
- Refactoring file layout or comments

## Required file organization
- Use section headers exactly:
  - `Constants`
  - `Types`
  - `Interfaces`
  - `Constructor`
  - `Public Methods`
  - `Private Methods`
  - `Helpers`
- Omit empty sections.
- Keep sections in the required order.

## Required documentation style
- Add a 4-line class header at top of implementation files:
  - line 1 starts with `The <Name> is a`
  - line 2 starts with `It provides`
  - lines 3-4 describe role/capabilities
- Every function/method has a header comment.
- Never place explanatory comments inside function bodies.

## Package and file expectations
- Prefer package layouts with:
  - implementation file
  - test file
  - mock implementation (when needed)
  - shims file for system calls
- Keep naming boundary-oriented, not generic buckets (`misc`, `utils`, `helpers` as dump files).

## Editing checklist
- Confirm section headers are valid and ordered.
- Confirm all functions have header comments.
- Remove inline comments inside function bodies.
- Keep naming consistent with existing package terminology.

