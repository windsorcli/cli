---
name: architecture
description: Enforce Windsor CLI architecture boundaries and separation of concerns. Use when changes touch more than one layer (cmd, runtime, evaluator, secrets, provisioner, composer, workstation), or when ownership of logic is unclear.
---

# Windsor Architecture

## Apply when
- Changes touch more than one architectural layer.
- Ownership is unclear between evaluator/secrets/runtime/provisioner/composer/workstation — or between the three packages named "terraform" (see below).
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
| Runtime Terraform | `pkg/runtime/terraform/*` | Terraform metadata introspection, env assembly, provider policy |
| Composer | `pkg/composer/*` | Blueprint load/process/compose/write pipeline, source/module resolution |
| Provisioner | `pkg/provisioner/*` | Infrastructure lifecycle: terraform execution, Kubernetes/Flux management, cluster clients, stack locking |
| Workstation | `pkg/workstation/*` | Local VM and network provisioning (colima, docker-desktop, host networking) |

### Three packages named "terraform" — do not conflate

- `pkg/runtime/terraform/*` — metadata introspection, env var assembly, provider policy. No execution.
- `pkg/composer/terraform/*` — resolves terraform module sources (OCI/git) during blueprint composition. No execution.
- `pkg/provisioner/terraform/*` (`TerraformStack`) — the only one that actually runs `terraform init/plan/apply/destroy`.

If a change needs to run terraform or change how/when it runs, it belongs in `pkg/provisioner/terraform`. If it needs to read terraform config/state shape without executing, it belongs in `pkg/runtime/terraform`. If it's about finding/fetching module source code before composition, it belongs in `pkg/composer/terraform`.

## Ownership rules

- **Runtime orchestrates.** Coordinates initialization order and lifecycle transitions. Does not make provider-specific decisions.
- **Evaluator evaluates.** Must not hardcode provider-specific branches.
- **Secrets resolves.** Handles provider-specific retrieval. Does not orchestrate lifecycle.
- **Runtime terraform owns** terraform metadata and env var decisions — not execution.
- **Composer blueprint handler owns** blueprint pipelines and source/template composition.
- **Provisioner owns** actually running infrastructure operations: `TerraformStack` (terraform exec), `KubernetesManager` (cluster/Flux operations), `ClusterClient` (cluster lifecycle), stack locking. The `Provisioner` struct in `pkg/provisioner/provisioner.go` is the orchestration entrypoint cmd/ calls into for apply/destroy/bootstrap.
- **Workstation owns** local VM/network setup — colima, docker-desktop, host DNS/routes. Does not touch cloud or cluster infrastructure.
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
