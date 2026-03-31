---
name: windsor-architecture-patterns
description: Enforce Windsor CLI architecture boundaries and dependency wiring patterns. Use when adding features or refactoring across cmd, runtime, evaluator, secrets, terraform, or composer packages, especially when deciding ownership of logic and data flow between layers.
---

# Windsor Architecture Patterns

## Apply when
- Changes touch more than one architectural layer (`cmd`, `pkg/runtime`, `pkg/composer`, API/config docs).
- Ownership is unclear between evaluator/secrets/terraform/runtime.
- New behavior could introduce cross-layer coupling.

## Do not use when
- The change is isolated to formatting, naming, or narrow local bug fixes within one file.
- The task is only writing tests with no architecture decisions.

## Canonical boundaries
- `cmd/*`: parse flags, instantiate runtime/composer, call orchestration methods, return user-facing errors.
- `pkg/runtime/runtime.go`: lifecycle orchestration and dependency wiring.
- `pkg/runtime/evaluator/*`: provider-agnostic expression evaluation and helper registration only.
- `pkg/runtime/secrets/*`: provider adapters and secret expression behavior.
- `pkg/runtime/terraform/*`: terraform-specific parsing, env assembly, and provider policy decisions.
- `pkg/composer/*`: blueprint load/process/compose/write pipeline and terraform module preparation.

## Boundary policy source of truth
- Use `.cursor/skills/windsor-runtime-boundaries/SKILL.md` as the canonical ownership/constraint/checklist policy for runtime, evaluator, secrets, and terraform.
- This skill defines cross-layer placement and dependency wiring decisions; do not restate or override runtime-boundary rules here.
- Additional ownership guidance in this skill is limited to:
  - `pkg/composer/*` composition pipeline ownership.
  - `cmd/*` CLI UX/policy ownership.

## Proven patterns in this repo
- Constructor injection with required dependency checks and panic-on-missing prerequisites.
  - `pkg/runtime/runtime.go`
  - `pkg/runtime/evaluator/evaluator.go`
  - `pkg/composer/blueprint/handler.go`
  - `pkg/composer/terraform/module_resolver.go`
- Boundary companion files for isolated sub-concerns inside a package.
  - `pkg/runtime/terraform/provider_sensitive_inputs.go`
- Public interfaces + concrete internal implementations per package.
  - `pkg/runtime/evaluator/evaluator.go`
  - `pkg/runtime/secrets/secrets_provider.go`
  - `pkg/composer/blueprint/handler.go`
- Shims wrappers for external/system interactions to keep logic testable.
  - `pkg/runtime/*/shims.go`
- Explicit lifecycle sequencing in orchestration entrypoints.
  - `cmd/env.go`
  - `pkg/runtime/runtime.go`

## Required decision checklist before coding
1. Identify the layer that owns the behavior.
2. Confirm whether this is orchestration, evaluation, provider adaptation, or terraform/composer-specific policy.
3. Place new logic in the owning package; inject dependencies instead of reaching across layers.
4. If adding a new sub-concern in a large package, isolate it in a boundary companion file.
5. Add or update tests in the same ownership layer; include at least one boundary-focused case.

## Change map requirement for cross-cutting work
When work spans multiple subsystems, produce and follow this map before implementation:
- subsystem
- reason for change
- risk
- verification target

## Validation pass before completion
- Boundaries remain intact and responsibilities are still single-owner.
- Lifecycle side effects remain centralized in orchestrators.
- No new global side effects were introduced.
- Tests validate boundary behavior, not only happy-path output.
