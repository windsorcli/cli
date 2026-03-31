---
name: windsor-runtime-boundaries
description: Enforce runtime/evaluator/secrets/terraform separation-of-concerns boundaries. Use when implementing or reviewing changes in pkg/runtime, pkg/composer/terraform, and related command wiring.
---

# Windsor Runtime Boundaries

## Ownership rules
- Runtime orchestrates lifecycle and wiring.
- Evaluator is provider-agnostic and executes expressions.
- Secrets package resolves/provider-adapts secret references.
- Terraform provider assembles policy/output; metadata parsing lives in provider-scoped boundary files.

## Do not introduce
- Provider-specific syntax branching inside evaluator core.
- Hidden feature toggles via anonymous type assertions.
- Parsing/introspection logic mixed directly into orchestration paths.
- New files that are arbitrary function dumps.

## Preferred patterns
- Constructor-injected policy where possible.
- Named boundary companion files (example: `provider_sensitive_inputs.go`).
- Public API kept minimal; internal state private.
- Explicit method-based mutation instead of ad-hoc exported mutable fields.

## Review checklist
- Identify touched boundaries before edits.
- Verify each changed symbol belongs to the file/package boundary.
- Ensure lifecycle side effects are centralized.
- Confirm tests cover boundary behavior, not only happy-path output.

