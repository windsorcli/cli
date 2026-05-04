---
title: "windsor up"
description: "Bring up the local workstation environment."
---
# windsor up

```sh
windsor up [flags]
```

Start the workstation VM, run Terraform components, then install the Flux blueprint. Workstation contexts only — for non-workstation contexts, use [`apply`](apply.md). Returns once the install request has been issued; pass `--wait` to block until kustomizations report ready.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--wait` | `false` | Wait for kustomization resources to be ready. |
| `--vm-driver` | `""` | VM driver: `colima`, `colima-incus`, `docker-desktop`, `docker`. |
| `--platform` | `""` | Target platform: `none`, `metal`, `docker`, `aws`, `azure`, `gcp`. |
| `--blueprint` | `""` | Blueprint OCI reference or local path. |
| `--set` | `[]` | Override config values, e.g. `--set dns.enabled=false`. |
| `--install` | — | Deprecated. No-op, kept for backwards compatibility. |

## Examples

```sh
# Bring up the workstation and wait for everything to be ready
windsor up --wait

# Override an inline config value at startup
windsor up --set cluster.workers.count=3
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [Local Workstation guide](../../guides/local-workstation.md)
- [`down`](down.md), [`apply`](apply.md), [`destroy`](destroy.md)
- Source: [cmd/up.go](https://github.com/windsorcli/cli/blob/main/cmd/up.go)
