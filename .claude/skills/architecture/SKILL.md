---
name: architecture
description: Enforce Windsor CLI architecture boundaries and separation of concerns. Use when changes touch more than one layer (cmd, runtime, evaluator, secrets, terraform, composer), or when ownership of logic is unclear.
---

# Windsor Architecture

## Apply when
- Changes touch more than one architectural layer.
- Ownership is unclear between evaluator/secrets/terraform/runtime.
- New behavior could introduce cross-layer coupling.

## Do not apply when
- The change is isolated to formatting, naming, or a narrow bug fix in one file.
- The task is only writing tests with no architecture decisions.

## Canonical layer boundaries

| Layer | Package | Owns |
|---|---|---|
| CLI | `cmd/*` | Flag parsing, runtime instantiation, user-facing errors |
| Runtime | `pkg/runtime/runtime.go` | Lifecycle orchestration and dependency wiring |
| Evaluator | `pkg/runtime/evaluator/*` | Provider-agnostic expression evaluation and helper registration |
| Secrets | `pkg/runtime/secrets/*` | Provider adapters and secret expression resolution |
| Terraform | `pkg/runtime/terraform/*` | Terraform metadata introspection, env assembly, provider policy |
| Composer | `pkg/composer/*` | Blueprint load/process/compose/write pipeline, module preparation |

## Ownership rules

- **Runtime orchestrates.** Coordinates initialization order and lifecycle transitions. Does not make provider-specific decisions.
- **Evaluator evaluates.** Must not hardcode provider-specific branches.
- **Secrets resolves.** Handles provider-specific retrieval. Does not orchestrate lifecycle.
- **Terraform provider owns** terraform metadata and env var decisions.
- **Composer blueprint handler owns** blueprint pipelines and source/template composition.
- **CLI command layer owns** UX/policy decisions (hook/non-hook error behavior, output format).

## Proven patterns

- Constructor injection with required dependency checks (`pkg/runtime/evaluator/evaluator.go`).
- Boundary companion files for isolated sub-concerns (`pkg/runtime/terraform/provider_sensitive_inputs.go`).
- Public interfaces + concrete internal implementations per package.
- Shims wrappers for external/system calls to keep logic testable (`pkg/runtime/*/shims.go`).
- Explicit lifecycle sequencing in orchestration entrypoints (`cmd/env.go`, `pkg/runtime/runtime.go`).

## Do not introduce

- Provider-specific syntax branching inside evaluator core.
- Hidden feature toggles via anonymous type assertions.
- Parsing/introspection logic mixed directly into orchestration methods.
- Arbitrary function dump files.
- Exposed mutable fields where explicit methods suffice.

## Decision checklist before coding

1. Identify the layer that owns the behavior.
2. Confirm whether this is orchestration, evaluation, provider adaptation, or policy.
3. Place new logic in the owning package; inject dependencies instead of reaching across layers.
4. If adding a sub-concern to a large package, isolate it in a boundary companion file.
5. Add or update tests in the same ownership layer with at least one boundary-focused case.

## Cross-cutting change map

For work spanning multiple subsystems, produce this map before writing any code:

| Subsystem | Reason for change | Risk | Verification target |
|---|---|---|---|
| ... | ... | ... | ... |

## Validation before completion

- Boundaries remain intact; responsibilities are still single-owner.
- Lifecycle side effects remain centralized in orchestrators.
- No new global side effects introduced.
- Tests validate boundary behavior, not only happy-path output.
