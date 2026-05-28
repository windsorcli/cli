---
title: "windsor upgrade cluster"
description: "Upgrade cluster nodes in parallel."
---
# windsor upgrade cluster

```sh
windsor upgrade cluster [flags]
```

Initiate a Talos upgrade on the named nodes in parallel. Returns once the upgrade requests are accepted; nodes reboot asynchronously.

Use 'windsor check node-health --wait-for-reboot' afterward to verify each node comes back healthy.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--image` | `""` | Talos image to upgrade to. Required. |
| `--nodes` | `[]` | Node addresses to upgrade. Required. |

## Examples

```sh
# Upgrade all controlplane nodes in parallel
windsor upgrade cluster \
  --nodes=10.0.0.5,10.0.0.6,10.0.0.7 \
  --image=ghcr.io/siderolabs/installer:v1.13.0
```

## See also

- [`upgrade node`](upgrade-node.md)
- [`check node-health`](check-node-health.md)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
