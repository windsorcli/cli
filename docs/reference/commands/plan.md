---
title: "windsor plan"
description: "Preview terraform and Flux changes."
---
# windsor plan

```sh
windsor plan [component]
```

Preview pending changes across Terraform components and Flux kustomizations without applying them.

With no argument, prints a compact summary across all components. Components that have never been applied show as '(new)' so you can distinguish first-time creates from updates.

With a component name, runs a full streaming plan for every layer (Terraform and/or Kustomize) that contains that component. Use a subcommand to restrict to a single layer.

The --summary, --json, and --no-color flags are persistent and apply to all subcommands.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON. Streams full plan JSON on subcommands; emits the summary as JSON on root 'plan'. |
| `--no-color` | `false` | Disable color output. |
| `--summary` | `false` | Show a compact summary table instead of streaming output. |

## Subcommands

- [`windsor plan kustomize`](/reference/cli/commands/plan-kustomize) — Plan Flux kustomization changes.
- [`windsor plan terraform`](/reference/cli/commands/plan-terraform) — Plan Terraform changes.

## Examples

```sh
# Compact summary across both layers
windsor plan

# Full streaming plan for one component (both layers)
windsor plan cluster

# JSON-formatted summary, suitable for CI parsing
windsor plan --summary --json

# Just terraform, just one component
windsor plan terraform cluster
```

## See also

- [`apply`](/reference/cli/commands/apply), [`show`](/reference/cli/commands/show), [`explain`](/reference/cli/commands/explain)
- Source: [cmd/plan.go](https://github.com/windsorcli/cli/blob/main/cmd/plan.go)
