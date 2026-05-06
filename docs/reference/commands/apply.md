---
title: "windsor apply"
description: "Apply terraform and install the blueprint."
---
# windsor apply

```sh
windsor apply [flags]
windsor apply terraform <component> [flags]
windsor apply kustomize [name] [flags]
```

Run Terraform components, then install the Flux blueprint. Use the `terraform` or `kustomize` subcommand to scope to a single layer. For workstation contexts, prefer [`up`](up.md) — it does the same work plus VM management.

## apply

Apply both layers (terraform → kustomize). Pass `--wait` to block until kustomizations report ready.

| Flag | Default | Description |
|------|---------|-------------|
| `--wait` | `false` | Wait for kustomization resources to be ready. |

## apply terraform

```sh
windsor apply terraform <component>
```

Run terraform apply for a single component. Aliases: `tf`. The `<component>` argument is required and must match a terraform component declared in the blueprint.

## apply kustomize

```sh
windsor apply kustomize [name] [--wait]
```

Apply a single Flux kustomization by name, or every kustomization when no argument is given. Accepts its own `--wait` flag (`apply terraform` does not).

## Examples

```sh
# Apply everything and block until ready
windsor apply --wait

# Apply only the cluster terraform component
windsor apply terraform cluster

# Apply just the dns kustomization
windsor apply kustomize dns
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [`plan`](plan.md), [`destroy`](destroy.md), [`up`](up.md), [`bootstrap`](bootstrap.md)
- Source: [cmd/apply.go](https://github.com/windsorcli/cli/blob/main/cmd/apply.go)
