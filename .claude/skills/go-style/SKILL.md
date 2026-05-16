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

## In-body comments are a smell, not a tool

The non-negotiable: **do not write explanatory comments inside function bodies.** Not single lines, not multi-line "novels," not "// Note: ..." asides. If you feel the urge to explain *why* the code does what it does at the point of the code, that belongs in:

1. **The function header** — for behavior that callers and future readers need to understand.
2. **The PR description or chat reply** — for justification of an implementation choice the reader can infer from the code itself.
3. **Nowhere** — if the code is self-evident from naming and structure (the common case).

In-body comments are reserved for the rare case where a hidden constraint, subtle invariant, or known bug would surprise a reader who only sees the local code. If removing the comment would not confuse a future reader, the comment must not exist.

### Anti-patterns to delete on sight

These shapes are common Claude failure modes; if you find yourself typing one, stop and either move the content to the header or delete it:

```go
// ❌ Multi-line "what + why + how" novel in body
output, err := s.runtime.Shell.ExecSilentWithEnv(...)
// Surface terraform's per-resource diagnostics to the operator on failure.
// ExecSilentWithEnv captures stdout+stderr but discarding it would leave the
// operator with only "exit status 1" — terraform's own message naming the
// failed resource and provider error is the actionable signal. Routed through
// warningWriter (rather than os.Stderr directly) so tests can capture it
// deterministically and Windows TUI redirection stays safe.
if err != nil { ... }

// ❌ "See X for the full note" cross-reference inside a body
// Set is in-memory only; restore-failure is a stale in-process value, not a
// persisted-config corruption — see runFullCycleDestroyAll for the full note.
if err := i.configHandler.Set(...); err != nil { ... }

// ❌ Restating what the next line does
// Capture the skip list and treat non-empty as a hard error.
skipped, err := i.MigrateState(blueprint)
```

The replacement for all three is the same: delete the comment. If the rationale truly cannot be reconstructed from the code and the function header together, expand the function header.

### Header novels are also bad, just less obvious

Function headers should describe behavior, not narrate motivation. **Hard ceiling: 6 lines for function/method headers, 1 line for struct fields.** A header that runs longer is usually doing the same job as an in-body novel — moved up to a "legal" location. Trim to: *what it does*, *what it returns*, and *the one constraint a caller must know about*. Drop incident history, design alternatives considered, and "Note on X" tangents — those belong in the commit message.

Struct fields are not docstring real estate. They get one short line — the name and the type already carry most of the meaning, and the surrounding type's header carries the rest. A 5-line explanation of *what this field means and how downstream code uses it* belongs in the type header or the consuming function's header, never on the field itself.

```go
// ❌ Multi-line struct field "what + why + downstream usage" novel
//
// Backend names the terraform component that terminates the backend tier — the
// component whose apply provisions the remote-state store every other component
// will use. When set, this component plus every terraform component declared before
// it in the TerraformComponents list form the backend tier; that tier is brought up
// with local state and migrated to the configured terraform.backend.* on every
// bootstrap, and torn down with state pulled back to local on full destroy. When
// empty, the blueprint has no in-blueprint backend tier and every component uses
// the configured backend directly (the "external backend" case).
Backend string `yaml:"backend,omitempty"`

// ✅ One line — name carries the meaning, type header carries the model
// Backend names the terraform component that terminates the backend tier.
Backend string `yaml:"backend,omitempty"`
```

The same hard ceiling applies to constants, package-level variables, and type aliases. If you cannot say what it is in one line, the name is wrong.

## Error classification

**Detect error categories with `errors.As` / `errors.Is` against typed errors. Do not use `strings.Contains(err.Error(), ...)` to classify errors.**

Error text is part of the surface, not the contract. It varies between library versions, between providers (ghcr, ECR, ACR, GAR all phrase the "denied" case differently), and gets reshaped every time an upstream layer wraps with `fmt.Errorf("...: %w", err)`. Substring matching on those strings produces three failure modes that have already bitten this codebase:

1. **False negatives.** A new library version or a different provider returns a string your pattern list does not match, and an auth failure silently looks like a generic error to the caller.
2. **False positives.** Generic patterns like `"POST https://"` or `"blobs/uploads"` flag *any* push error as an auth error, even when the real cause is a network reset, a malformed manifest, or a 5xx from the registry.
3. **Pattern-list drift.** Each new failure mode adds another string to the list. The list grows. It is never audited. Some entries become dead code, others contradict each other.

The replacement is structural. Almost every Go library that produces categorized errors exposes them as typed errors — use those:

```go
// ❌ Sketchy substring matching
if strings.Contains(err.Error(), "UNAUTHORIZED") ||
    strings.Contains(err.Error(), "DENIED") ||
    strings.Contains(err.Error(), "POST https://") { // false positive: matches any push error
    // ...
}

// ✅ Typed check via errors.As — works through wrapping, version-stable
var tErr *transport.Error
if errors.As(err, &tErr) {
    if tErr.StatusCode == http.StatusUnauthorized || tErr.StatusCode == http.StatusForbidden {
        // ...
    }
    for _, d := range tErr.Errors {
        if d.Code == transport.UnauthorizedErrorCode || d.Code == transport.DeniedErrorCode {
            // ...
        }
    }
}
```

The same rule applies to sentinel errors (`errors.Is(err, io.EOF)`, `errors.Is(err, context.DeadlineExceeded)`), to typed wrappers (`*os.PathError`, `*net.OpError`), and to custom errors defined in this codebase.

### If a library does not expose a typed error

Define your own typed error at the boundary where the library result enters Windsor code, and wrap the raw error with `%w`. From that point on, every downstream check is `errors.As` / `errors.Is` against the typed wrapper — never substring matching on the original library text. Example:

```go
// At the package boundary that calls the external tool:
type ExitError struct { Code int; Err error }
func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

if exitErr, ok := raw.(*exec.ExitError); ok {
    return &ExitError{Code: exitErr.ExitCode(), Err: exitErr}
}
```

Downstream callers then do `errors.As(err, &*ExitError)` — no substring matching anywhere in the chain.

The rule has no fallback clause. If you find yourself reaching for `strings.Contains(err.Error(), ...)`, the fix is to add a typed wrapper at the boundary, not to add a substring pattern.

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
- No `strings.Contains(err.Error(), ...)` (or equivalent) for error classification. Use `errors.As` / `errors.Is` against typed errors; if a library lacks one, define a typed wrapper at the boundary.
