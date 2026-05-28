---
title: "windsor check node-health"
description: "Check the health of cluster nodes."
---
# windsor check node-health

```sh
windsor check node-health [flags]
```

Probe one or more cluster nodes for readiness. Useful after 'windsor upgrade' or for routine monitoring.

At least one of --nodes or --k8s-endpoint must be set.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--k8s-endpoint` | `""` | Probe the Kubernetes API at this URL, or pass without value to use the configured endpoint. |
| `--nodes` | `[]` | Node addresses to check. |
| `--ready` | `false` | Check Kubernetes node readiness in addition to Talos. |
| `--skip-services` | `[]` | Service names to ignore (e.g., dashboard). |
| `--timeout` | `0s` | Maximum time to wait for nodes to be ready. Default 5m. |
| `--version` | `""` | Expected Talos version. Reports a mismatch if set. |
| `--wait-for-reboot` | `false` | Poll until the Talos API goes offline (reboot started), then wait for it to come back. |

## Examples

```sh
# Health-check one node, polling through a reboot
windsor check node-health --nodes=10.0.0.5 --wait-for-reboot

# Verify all nodes report Ready via the configured Kubernetes endpoint
windsor check node-health --k8s-endpoint --ready

# Check a specific Talos version on a set of nodes
windsor check node-health --nodes=10.0.0.5,10.0.0.6 --version=v1.13.3
```

## See also

- [Getting started](https://www.windsorcli.dev/docs/cli/getting-started)
- [`upgrade`](upgrade.md), [`up`](up.md)
- Source: [cmd/check.go](https://github.com/windsorcli/cli/blob/main/cmd/check.go)
