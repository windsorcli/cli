---
title: "windsor destroy terraform"
description: "Destroy Terraform component(s)."
---
# windsor destroy terraform

```sh
windsor destroy terraform [component]
```

Destroy a specific Terraform component, or all components when no argument is given. Inherits --confirm from the parent 'destroy' command.

## Examples

```sh
# Destroy a single terraform component
windsor destroy terraform cluster --confirm=cluster

# Destroy every terraform component in the current context
windsor destroy terraform --confirm=local
```

## See also

- [`destroy`](destroy.md), [`apply terraform`](apply-terraform.md), [`plan terraform`](plan-terraform.md)
- Source: [cmd/destroy.go](https://github.com/windsorcli/cli/blob/main/cmd/destroy.go)
