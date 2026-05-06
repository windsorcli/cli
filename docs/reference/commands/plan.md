---
title: "windsor plan"
description: "Preview terraform and Flux changes."
---
# windsor plan

```sh
windsor plan [component] [flags]
windsor plan terraform [project] [flags]
windsor plan kustomize [component] [flags]
```

Preview pending changes across Terraform components and Flux kustomizations without applying them.

- With no argument, prints a compact summary across all components. Components that have never been applied show as `(new)` so you can distinguish first-time creates from updates.
- With a component name, streams the full plan for every layer that contains that component.
- Subcommands restrict to one layer.

## Flags (persistent across `plan` subcommands)

| Flag | Default | Description |
|------|---------|-------------|
| `--summary` | `false` | Print a compact summary table instead of streaming output. |
| `--json` | `false` | Output as JSON. Streams full plan JSON on subcommands; emits the summary as JSON on root `plan`. |
| `--no-color` | `false` | Disable color output. |

## plan terraform

```sh
windsor plan terraform [project]
```

Stream `terraform init` and `terraform plan` for one component, or every component when no argument is given. Aliases: `tf`.

## plan kustomize

```sh
windsor plan kustomize [component]
```

Stream `flux diff` for one kustomization, or every kustomization when no argument is given. Aliases: `k8s`.

## Examples

```sh
# Compact summary across both layers
windsor plan

# Full streaming plan for one component
windsor plan cluster

# JSON-formatted summary, suitable for CI parsing
windsor plan --summary --json

# Just terraform, just one component
windsor plan terraform cluster
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [`apply`](apply.md), [`show`](show.md), [`explain`](explain.md)
- Source: [cmd/plan.go](https://github.com/windsorcli/cli/blob/main/cmd/plan.go)
