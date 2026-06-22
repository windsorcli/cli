---
title: "windsor upgrade"
description: "Upgrade the blueprint, pruning kustomizations it no longer declares."
---
# windsor upgrade

```sh
windsor upgrade
```

Apply terraform and the Flux blueprint, wait for kustomizations to be ready, then prune any kustomizations this context no longer declares. Pruning runs only after a successful wait, so resources are never deleted before the desired set has reconciled.

Use the 'cluster' or 'node' subcommand to upgrade Talos nodes instead.

## Subcommands

- [`windsor upgrade cluster`](upgrade-cluster.md) — Upgrade cluster nodes in parallel.
- [`windsor upgrade node`](upgrade-node.md) — Upgrade a single cluster node and wait for it to rejoin.

## Examples

```sh
# Upgrade the blueprint and prune orphaned kustomizations
windsor upgrade

# Upgrade Talos nodes in parallel (see 'upgrade cluster')
windsor upgrade cluster --nodes=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0
```

## See also

- [`apply`](apply.md), [`bootstrap`](bootstrap.md), [`plan`](plan.md)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
