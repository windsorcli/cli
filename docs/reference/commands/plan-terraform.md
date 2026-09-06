---
title: "windsor plan terraform"
description: "Plan Terraform changes."
---
# windsor plan terraform

```sh
windsor plan terraform [component] [flags]
```

Stream 'terraform init' and 'terraform plan' for a specific component, or all components when no argument is given. Inherits --summary, --json, and --no-color from the parent 'plan' command.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--output` | `""` | Set to 'json-plan' to print the plan document from 'terraform show -json', instead of the progress log. |

## Examples

```sh
# Stream the plan for one component
windsor plan terraform cluster

# Compact summary across all components
windsor plan terraform --summary

# Machine-readable JSON of all component plans
windsor plan terraform --json

# Full plan document for a security scanner (one component per run)
windsor plan terraform cluster --output=json-plan | checkov -f -

# One compact plan document per line, one line per component
windsor plan terraform --output=json-plan
```

## See also

- [`plan`](plan.md), [`apply terraform`](apply-terraform.md), [`destroy terraform`](destroy-terraform.md)
- Source: [cmd/plan.go](https://github.com/windsorcli/cli/blob/main/cmd/plan.go)
