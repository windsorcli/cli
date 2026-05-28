---
title: "windsor up"
description: "Bring up the local workstation environment."
---
# windsor up

```sh
windsor up [flags]
```

Start the workstation VM, run Terraform components, then install the Flux blueprint. Workstation contexts only — for non-workstation contexts, use 'windsor apply'. If the current context has no workstation, up exits with a hint and does no work.

Returns once the install request has been issued. Pass --wait to block until kustomizations report ready.

If any host-side network or DNS configuration was deferred (because it requires sudo / elevation), up prints a follow-up command at the end so the operator knows what to run next.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--blueprint` | `""` | Blueprint OCI reference or local path. |
| `--platform` | `""` | Target platform: none, metal, docker, aws, azure, gcp, hyperv. |
| `--set` | `[]` | Override config values, e.g. --set dns.enabled=false. May be repeated. |
| `--vm-driver` | `""` | VM driver: colima, colima-incus, docker-desktop, docker. |
| `--wait` | `false` | Wait for kustomization resources to be ready. |

## Examples

```sh
# Bring up the workstation and wait for everything to be ready
windsor up --wait

# Override an inline config value at startup
windsor up --set cluster.workers.count=3

# Initialize and bring up with a specific blueprint
windsor up --blueprint=ghcr.io/myorg/blueprint:v1.0.0
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [Workstation overview](https://www.windsorcli.dev/docs/workstation/overview)
- [`down`](down.md), [`apply`](apply.md), [`destroy`](destroy.md)
- Source: [cmd/up.go](https://github.com/windsorcli/cli/blob/main/cmd/up.go)
