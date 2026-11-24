---
title: "windsor upgrade cluster"
description: "Upgrade several cluster nodes."
---

```sh
windsor upgrade cluster [flags]
```

Upgrade the named Talos nodes. Nodes upgrade one at a time, control planes first. Each node must be healthy before the next starts, and the command stops at the first failure.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--image` | `""` | Talos image to upgrade to. Required. |
| `--nodes` | `[]` | Node addresses to upgrade. Required. |
| `--offline-timeout` | `0s` | Timeout for each node to confirm its reboot. Default 3m, or the value of --timeout when only --timeout is set. |
| `--reboot-mode` | `default` | Reboot mode: "default" (kexec, fast) or "powercycle" (full ACPI reset). Use powercycle on platforms where kexec doesn't reliably register as offline (e.g. nested virtualization). |
| `--skip-if-current` | `false` | Skip each node that already runs the image version. If a version cannot be read, upgrade that node as usual. |
| `--timeout` | `0s` | Timeout for the upgrade of each node, including the offline wait. Default 10m. |

## Examples

```sh
# Upgrade three control planes in turn
windsor upgrade cluster \
  --nodes=10.0.0.5,10.0.0.6,10.0.0.7 \
  --image=ghcr.io/siderolabs/installer:v1.13.0

# Resume after a failure without rebooting nodes that already run the image
windsor upgrade cluster \
  --nodes=10.0.0.5,10.0.0.6,10.0.0.7 \
  --image=ghcr.io/siderolabs/installer:v1.13.0 --skip-if-current
```

## See also

- [`upgrade node`](upgrade-node.md)
- [`check node-health`](check-node-health.md)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
