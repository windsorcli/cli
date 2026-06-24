---
title: "windsor apply kustomize"
description: "Apply Flux kustomization(s) to the cluster."
---
# windsor apply kustomize

```sh
windsor apply kustomize [name] [flags]
```

Apply a single Flux kustomization to the cluster by name, or all kustomizations when no argument is given.

When a name is supplied with --wait, the wait scope is narrowed to only that kustomization.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--wait` | `false` | Wait for kustomization resources to be ready. |

## Examples

```sh
# Apply all kustomizations
windsor apply kustomize

# Apply just the dns kustomization
windsor apply kustomize dns

# Apply and wait for one kustomization to be ready
windsor apply kustomize dns --wait
```

## See also

- [`apply`](apply.md), [`plan kustomize`](plan-kustomize.md), [`destroy kustomize`](destroy-kustomize.md)
- Source: [cmd/apply.go](https://github.com/windsorcli/cli/blob/main/cmd/apply.go)
