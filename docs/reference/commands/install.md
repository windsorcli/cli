---
title: "windsor install"
description: "Install the blueprint's Flux kustomizations."
---
# windsor install

```sh
windsor install [flags]
```

Apply only the Flux kustomizations to the cluster, skipping Terraform. Use this when Terraform has already been applied separately (e.g. by another tool or pipeline) and you only want to hand the cluster off to Flux.

For most workflows, prefer 'windsor apply', which runs Terraform and Flux in the right order.

Pass --wait to block until kustomizations report ready.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--wait` | `false` | Wait for kustomization resources to be ready. |

## Examples

```sh
# Install kustomizations and wait for them to settle
windsor install --wait
```

## See also

- [`apply`](/reference/cli/commands/apply), [`apply kustomize`](/reference/cli/commands/apply-kustomize)
- Source: [cmd/install.go](https://github.com/windsorcli/cli/blob/main/cmd/install.go)
