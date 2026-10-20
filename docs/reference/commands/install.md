---
title: "windsor install"
description: "Install the blueprint's Flux kustomizations."
---

```sh
windsor install
```

Apply only the Flux kustomizations to the cluster, skipping Terraform. Use this when Terraform has already been applied separately (e.g. by another tool or pipeline) and you only want to hand the cluster off to Flux.

For most workflows, prefer 'windsor apply', which runs Terraform and Flux in the right order.

Waits until every kustomization is Ready, and fails when one does not become Ready.

## Examples

```sh
# Install kustomizations
windsor install
```

## See also

- [`apply`](apply.md), [`apply kustomize`](apply-kustomize.md)
- Source: [cmd/install.go](https://github.com/windsorcli/cli/blob/main/cmd/install.go)
