---
title: "windsor destroy kustomize"
description: "Destroy Flux kustomization(s)."
---
# windsor destroy kustomize

```sh
windsor destroy kustomize [name]
```

Delete a specific Flux kustomization from the cluster, or all kustomizations when no argument is given. Inherits --confirm from the parent 'destroy' command.

## Examples

```sh
# Delete a single kustomization
windsor destroy kustomize dns --confirm=dns

# Delete every kustomization in the current context
windsor destroy kustomize --confirm=local
```

## See also

- [`destroy`](destroy.md), [`apply kustomize`](apply-kustomize.md), [`plan kustomize`](plan-kustomize.md)
- Source: [cmd/destroy.go](https://github.com/windsorcli/cli/blob/main/cmd/destroy.go)
