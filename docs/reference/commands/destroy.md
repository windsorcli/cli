---
title: "windsor destroy"
description: "Destroy live infrastructure."
---
# windsor destroy

```sh
windsor destroy [component]
```

Destroy live infrastructure. With no argument, removes every Flux kustomization, then every Terraform component. With a component name, destroys that component across both layers (Terraform and/or Kustomize).

Every form requires confirmation. Either type the context or component name at the prompt, or pass --confirm=<expected> to satisfy the gate non-interactively (CI-safe). The --confirm value must match the prompt token exactly; mismatches abort the operation.

If terraform reports resources protected by 'lifecycle { prevent_destroy = true }', destroy warns up front so the operator knows the destroy may halt partway through. It also warns when deletion_policy, deletion_protection, force_destroy, or skip_final_snapshot has drifted from state. Resources whose state is empty are skipped with a warning naming any potentially orphaned cloud resources.

If any component fails destroy-plan generation, destroy halts before the confirmation prompt and names the failed components, rather than offering to destroy a plan it cannot fully execute. This is distinct from --continue, which governs failures during execution, after confirmation.

The default behavior is to abort on the first per-component destroy failure. Pass --continue to keep going past failures and print a one-line summary at the end (windsor destroy: N destroyed, N no-op (empty state), N failed (...), terraform deferred). --continue applies to a layer-wide destroy only; it is refused with a component argument.

Terraform is skipped in two cases:
- A non-backend component is left un-destroyed.
- A Flux kustomization fails to delete while the cluster is still reachable. A live controller, such as Crossplane, may still be tearing down a cloud resource that terraform never tracked. Destroying the cluster now would orphan that resource.

Rerun 'windsor destroy --continue' after you resolve the failures. The next pass picks up where the last one stopped.

When terraform.backend.type is 'kubernetes', a full-cycle destroy (no argument) migrates every component's state to local before destroying anything, then destroys entirely against that local copy — the kubernetes backend stores state on the cluster the destroy is about to tear down, so reads pivot away from it up front rather than stranding mid-teardown once the cluster is gone. A single component can't be destroyed in isolation while it's one of the backend's components, since destroying it directly would orphan every other component's state; run a full 'windsor destroy' instead.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--confirm` | `""` | Context or component name to confirm destruction. Must match the prompt token exactly; mismatches abort. |
| `--continue` | `false` | Continue past per-component destroy failures and report a summary at the end. Layer-wide destroy only. Defers terraform when a kustomize failure leaves the cluster reachable, or when a non-backend component fails. |

## Subcommands

- [`windsor destroy kustomize`](destroy-kustomize.md) — Destroy Flux kustomization(s).
- [`windsor destroy terraform`](destroy-terraform.md) — Destroy Terraform component(s).

## Examples

```sh
# Destroy everything in the current context (interactive)
windsor destroy
# → prompts: Type "local" to confirm:

# Same, scripted
windsor destroy --confirm=local

# Destroy just the dns component (across both layers)
windsor destroy dns --confirm=dns

# Continue past per-component failures and converge by rerunning
windsor destroy --confirm=local --continue
```

## See also

- [`apply`](apply.md), [`down`](down.md), [`plan`](plan.md)
- Source: [cmd/destroy.go](https://github.com/windsorcli/cli/blob/main/cmd/destroy.go)
