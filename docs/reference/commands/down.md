---
title: "windsor down"
description: "Stop the local workstation environment."
---

```sh
windsor down [flags]
```

Delete the workstation VM, stop the container runtimes, and clear the runtime caches (.terraform, .tfstate) and the cluster credentials (.kube, .talos). Hand-edited tfvars and cloud credentials stay. Workstation contexts only.

Without --destroy, down does not wait for the cluster to release resources outside the VM. Pass --destroy when the cluster manages such resources, for example cloud resources. Down then runs the same steps as 'windsor destroy' first, with the same plan and confirmation. If the destroy fails, down stops and keeps the workstation, so you can run it again.

If 'windsor configure network' installed host network or DNS configuration, down prints a follow-up command at the end to remove it.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--confirm` | `""` | Context name to confirm the destroy. Applies only with --destroy. |
| `--continue` | `false` | Continue past per-component destroy failures and report a summary. Applies only with --destroy. Down still stops before it deletes the workstation when a component fails. |
| `--destroy` | `false` | Destroy the live infrastructure of the context before the workstation goes. |

## Examples

```sh
# Delete the workstation environment
windsor down

# Destroy the infrastructure, then delete the workstation environment
windsor down --destroy

# Same, scripted
windsor down --destroy --confirm=local
```

## See also

- [`up`](up.md), [`destroy`](destroy.md), [`configure network`](configure-network.md)
- Source: [cmd/down.go](https://github.com/windsorcli/cli/blob/main/cmd/down.go)
