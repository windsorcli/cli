---
title: "windsor apply terraform"
description: "Apply Terraform changes for a single component."
---
# windsor apply terraform

```sh
windsor apply terraform <component>
```

Run terraform apply for a single component. The <component> argument is required and must match a terraform component declared in the blueprint.

## Examples

```sh
# Apply the cluster component
windsor apply terraform cluster

# Same, using the 'tf' alias
windsor apply tf cluster
```

## See also

- [`apply`](apply.md), [`plan terraform`](plan-terraform.md), [`destroy terraform`](destroy-terraform.md)
- Source: [cmd/apply.go](https://github.com/windsorcli/cli/blob/main/cmd/apply.go)
