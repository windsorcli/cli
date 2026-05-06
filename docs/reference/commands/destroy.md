---
title: "windsor destroy"
description: "Destroy live infrastructure."
---
# windsor destroy

```sh
windsor destroy [component] [flags]
windsor destroy terraform [component] [flags]
windsor destroy kustomize [name] [flags]
```

Destroy live infrastructure. With no argument, removes every Flux kustomization, then every Terraform component. With a component name, destroys that component across both layers (Terraform and/or Kustomize).

Every form requires confirmation. Either type the context or component name at the prompt, or pass `--confirm=<expected>` to satisfy the gate non-interactively (CI-safe).

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--confirm` | `""` | Context or component name to confirm destruction (skips the prompt). |

The `--confirm` flag is persistent and applies to all subcommands. It must match the expected confirmation token *exactly*; mismatches abort the operation.

## destroy terraform

```sh
windsor destroy terraform [component] [--confirm=<token>]
```

Destroy one terraform component, or every component when no argument is given. Aliases: `tf`.

## destroy kustomize

```sh
windsor destroy kustomize [name] [--confirm=<token>]
```

Delete one Flux kustomization, or every kustomization when no argument is given. Aliases: `k8s`.

## Examples

```sh
# Destroy everything in the local context (interactive)
windsor destroy
# → prompts: Type "local" to confirm:

# Same, scripted
windsor destroy --confirm=local

# Destroy just the dns component (across both layers)
windsor destroy dns --confirm=dns

# Destroy just the cluster terraform component
windsor destroy terraform cluster --confirm=cluster
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [`apply`](apply.md), [`down`](down.md), [`plan`](plan.md)
- Source: [cmd/destroy.go](https://github.com/windsorcli/cli/blob/main/cmd/destroy.go)
