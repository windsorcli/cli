---
title: "windsor upgrade"
description: "Upgrade cluster components."
---
# windsor upgrade

```sh
windsor upgrade cluster --nodes=<addr,...> --image=<image>
windsor upgrade node --node=<addr> --image=<image> [--timeout=<dur>]
```

Upgrade cluster components. Currently supports Talos node upgrades.

## upgrade cluster

Initiate a Talos upgrade on the named nodes. Returns once the upgrade requests are accepted; nodes reboot asynchronously. Use [`check node-health`](check.md#check-node-health) to verify they come back.

| Flag | Default | Description |
|------|---------|-------------|
| `--nodes` | `[]` | Node addresses to upgrade. **Required.** |
| `--image` | `""` | Talos image to upgrade to. **Required.** |

## upgrade node

Send an upgrade to a single Talos node, wait for it to reboot, then verify it is healthy. Suitable for rolling upgrades one node at a time.

| Flag | Default | Description |
|------|---------|-------------|
| `--node` | `""` | Node IP address to upgrade. **Required.** |
| `--image` | `""` | Talos image to upgrade to. **Required.** |
| `--timeout` | `10m` | Overall timeout. |

## Examples

```sh
# Upgrade all controlplane nodes in parallel
windsor upgrade cluster \
  --nodes=10.0.0.5,10.0.0.6,10.0.0.7 \
  --image=ghcr.io/siderolabs/installer:v1.13.0

# Roll one node at a time, blocking until each is healthy
windsor upgrade node --node=10.0.0.5 --image=ghcr.io/siderolabs/installer:v1.13.0
```

## See also

- [`check node-health`](check.md#check-node-health)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
