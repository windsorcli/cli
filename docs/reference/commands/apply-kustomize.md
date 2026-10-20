---
title: "windsor apply kustomize"
description: "Apply Flux kustomization(s) to the cluster."
---

```sh
windsor apply kustomize [name]
```

Apply a single Flux kustomization to the cluster by name, or all kustomizations when no argument is given.

Waits until the applied kustomizations are Ready, and fails when one does not become Ready.

## Examples

```sh
# Apply all kustomizations
windsor apply kustomize

# Apply just the dns kustomization
windsor apply kustomize dns

# Same, using the 'k8s' alias
windsor apply k8s dns
```

## See also

- [`apply`](apply.md), [`plan kustomize`](plan-kustomize.md), [`destroy kustomize`](destroy-kustomize.md)
- Source: [cmd/apply.go](https://github.com/windsorcli/cli/blob/main/cmd/apply.go)
