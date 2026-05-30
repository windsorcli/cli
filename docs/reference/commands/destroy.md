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

The default behavior is to abort on the first per-component destroy failure. Pass `--continue` to keep going past individual failures, collect them, and print a one-line summary at the end (`windsor destroy: N destroyed, N no-op (empty state), N failed (...), backend tier deferred`). When `--continue` leaves any non-tier component un-destroyed, the backend tier is **not** attempted — this prevents destroying the state store while other components still depend on it. Rerun `windsor destroy --continue` after resolving the underlying failures; the second pass picks up where the first left off and converges on a clean slate.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--confirm` | `""` | Context or component name to confirm destruction. Must match the prompt token exactly; mismatches abort. |
| `--continue` | `false` | Continue past per-component destroy failures and report a summary at the end. Backend tier is deferred when any non-tier component fails. |

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
