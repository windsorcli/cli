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

If terraform reports resources protected by 'lifecycle { prevent_destroy = true }', destroy warns up front so the operator knows the destroy may halt partway through. Resources whose state is empty are skipped with a warning naming any potentially orphaned cloud resources.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--confirm` | `""` | Context or component name to confirm destruction. Must match the prompt token exactly; mismatches abort. |

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
```

## See also

- [`apply`](apply.md), [`down`](down.md), [`plan`](plan.md)
- Source: [cmd/destroy.go](https://github.com/windsorcli/cli/blob/main/cmd/destroy.go)
