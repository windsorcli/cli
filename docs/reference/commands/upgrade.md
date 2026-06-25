---
title: "windsor upgrade"
description: "Move sources to their latest version and reconcile the blueprint."
---
# windsor upgrade

```sh
windsor upgrade [flags]
```

With no arguments, move every declared OCI source to its latest stable version, then reconcile: apply terraform and the Flux blueprint, wait, and prune kustomizations this context no longer declares. Use --source name=url to move named sources to specific versions instead. Prunes run only after a successful wait and are gated by --yes.

Use the 'cluster' or 'node' subcommand to upgrade Talos nodes instead.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--allow-downgrade` | `false` | Permit moving a source to an older version. Reverts infrastructure declaratively; does NOT reverse application data. |
| `--source` | `[]` | Retarget a declared source to a new tagged URL (name=url); repeatable. Persisted to blueprint.yaml. |
| `--yes` | `false` | Proceed without confirmation when the upgrade would prune kustomizations. |

## Subcommands

- [`windsor upgrade cluster`](upgrade-cluster.md) — Upgrade cluster nodes in parallel.
- [`windsor upgrade node`](upgrade-node.md) — Upgrade a single cluster node and wait for it to rejoin.

## Examples

```sh
# Move all sources to their latest stable version and reconcile
windsor upgrade --yes

# Move a specific source to a specific version
windsor upgrade --source core=oci://ghcr.io/windsorcli/core:v0.6.0 --yes

# Upgrade Talos nodes in parallel (see 'upgrade cluster')
windsor upgrade cluster --nodes=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0
```

## See also

- [`apply`](apply.md), [`bootstrap`](bootstrap.md), [`plan`](plan.md)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
