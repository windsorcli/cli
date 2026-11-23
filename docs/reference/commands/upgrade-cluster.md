---
title: "windsor upgrade cluster"
description: "Upgrade several cluster nodes."
---

```sh
windsor upgrade cluster [flags]
```

Upgrade the named Talos nodes. Control planes upgrade one at a time, each until it is healthy, and the command stops at the first failure. Workers upgrade together after the control planes. The command returns when the worker requests are accepted; workers reboot asynchronously.

Use 'windsor check node-health --wait-for-reboot' afterward to verify the workers come back healthy.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--image` | `""` | Talos image to upgrade to. Required. |
| `--nodes` | `[]` | Node addresses to upgrade. Required. |
| `--reboot-mode` | `default` | Reboot mode: "default" (kexec, fast) or "powercycle" (full ACPI reset). Use powercycle on platforms where kexec doesn't reliably register as offline (e.g. nested virtualization). |

## Examples

```sh
# Upgrade three control planes one after another
windsor upgrade cluster \
  --nodes=10.0.0.5,10.0.0.6,10.0.0.7 \
  --image=ghcr.io/siderolabs/installer:v1.13.0
```

## See also

- [`upgrade node`](upgrade-node.md)
- [`check node-health`](check-node-health.md)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
