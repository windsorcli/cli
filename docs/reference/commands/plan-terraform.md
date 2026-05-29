---
title: "windsor plan terraform"
description: "Plan Terraform changes."
---
# windsor plan terraform

```sh
windsor plan terraform [component]
```

Stream 'terraform init' and 'terraform plan' for a specific component, or all components when no argument is given. Inherits --summary, --json, and --no-color from the parent 'plan' command.

## Examples

```sh
# Stream the plan for one component
windsor plan terraform cluster

# Compact summary across all components
windsor plan terraform --summary

# Machine-readable JSON of all component plans
windsor plan terraform --json
```

## See also

- [`plan`](plan.md), [`apply terraform`](apply-terraform.md), [`destroy terraform`](destroy-terraform.md)
- Source: [cmd/plan.go](https://github.com/windsorcli/cli/blob/main/cmd/plan.go)
