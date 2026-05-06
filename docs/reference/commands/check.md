---
title: "windsor check"
description: "Verify required tools are installed."
---
# windsor check

```sh
windsor check
windsor check node-health [flags]
```

Verify that required tools are installed at the expected versions and that cloud credentials resolve.

## check

Runs the standard preflight in two passes: tool version checks for local CLIs (`terraform`, `kubectl`, `talosctl`, etc.), then a credential check for the platform configured on the current context (e.g. `aws sts get-caller-identity` for `platform: aws`). Fails fast if a required tool is missing at the wrong version or if credentials don't resolve.

## check node-health

Probe one or more cluster nodes for readiness. Useful after `windsor upgrade` or for routine monitoring.

| Flag | Default | Description |
|------|---------|-------------|
| `--nodes` | `[]` | Node addresses to check. |
| `--timeout` | `5m` | Maximum time to wait for nodes to be ready. |
| `--version` | `""` | Expected Talos version. Reports a mismatch if set. |
| `--k8s-endpoint` | `""` | Probe the Kubernetes API at this URL (or pass without value to use the configured endpoint). |
| `--ready` | `false` | Check Kubernetes node readiness in addition to Talos. |
| `--skip-services` | `[]` | Service names to ignore (e.g. `dashboard`). |
| `--wait-for-reboot` | `false` | Poll until the Talos API goes offline (reboot started), then wait for it to come back. |

At least one of `--nodes` or `--k8s-endpoint` must be set.

## Examples

```sh
# Verify the toolchain
windsor check

# Health-check one node, polling through a reboot
windsor check node-health --nodes=10.0.0.5 --wait-for-reboot

# Verify all nodes report Ready
windsor check node-health --k8s-endpoint --ready
```

## See also

- [Getting started](https://www.windsorcli.dev/docs/cli/getting-started)
- [`upgrade`](upgrade.md), [`up`](up.md)
- Source: [cmd/check.go](https://github.com/windsorcli/cli/blob/main/cmd/check.go)
