---
title: "windsor upgrade node"
description: "Upgrade a single cluster node and wait for it to rejoin."
---
# windsor upgrade node

```sh
windsor upgrade node [flags]
```

Send an upgrade request to a single Talos node, wait for it to reboot, then verify it is healthy. Suitable for rolling upgrades one node at a time.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--image` | `""` | Talos image to upgrade to. Required. |
| `--node` | `""` | Node IP address to upgrade. Required. |
| `--offline-timeout` | `0s` | Timeout for the node to go offline after the upgrade request, before it's assumed rebooting. Raise this on slow-rebooting or nested-virtualized platforms. Default 3m. |
| `--timeout` | `0s` | Overall timeout for the whole upgrade, including the offline wait (see --offline-timeout). Default 10m. |

## Examples

```sh
# Roll one node, blocking until it is healthy
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0

# Same with a longer overall timeout for slow rebooters
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0 --timeout=20m

# Nested-virtualized platforms can take longer than 3m to go offline after the upgrade request
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0 --offline-timeout=8m
```

## See also

- [`upgrade cluster`](upgrade-cluster.md)
- [`check node-health`](check-node-health.md)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
