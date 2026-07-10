---
title: "windsor apply"
description: "Apply terraform and install the blueprint."
---
# windsor apply

```sh
windsor apply [flags]
```

Run Terraform components, then install the Flux blueprint. Use the 'terraform' or 'kustomize' subcommand to scope to a single layer.

For workstation contexts, prefer 'windsor up' — it does the same work plus VM management.

Pass --wait to block until kustomizations report ready. Pass --prune to also remove kustomizations the blueprint no longer declares, once the new set is Ready.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--prune` | `false` | Remove kustomizations the blueprint no longer declares. |
| `--wait` | `false` | Wait for kustomization resources to be ready. |

## Subcommands

- [`windsor apply kustomize`](apply-kustomize.md) — Apply Flux kustomization(s) to the cluster.
- [`windsor apply terraform`](apply-terraform.md) — Apply Terraform changes for a single component.

## Examples

```sh
# Apply everything and block until ready
windsor apply --wait

# Apply and remove kustomizations no longer declared
windsor apply --prune

# Apply only the cluster terraform component
windsor apply terraform cluster

# Apply just the dns kustomization
windsor apply kustomize dns
```

## See also

- [`plan`](plan.md), [`destroy`](destroy.md), [`up`](up.md), [`bootstrap`](bootstrap.md)
- Source: [cmd/apply.go](https://github.com/windsorcli/cli/blob/main/cmd/apply.go)
