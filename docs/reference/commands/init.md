---
title: "windsor init"
description: "Create or re-initialize a context."
---
# windsor init

```sh
windsor init [context] [flags]
```

Scaffold a Windsor project. Writes `windsor.yaml` at the project root if missing, creates `contexts/<context>/`, and adds the current directory to the trusted-folders list. Re-running on an existing context updates configuration; pass `--reset` to overwrite generated files and clean `.terraform`.

If no context is given, the current context is used; if none is set, `local` is used.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--reset` | `false` | Overwrite existing files and clean `.terraform`. |
| `--platform` | `""` | Target platform: `none`, `metal`, `docker`, `aws`, `azure`, `gcp`. |
| `--blueprint` | `""` | Blueprint OCI reference or local path. |
| `--backend` | `""` | Terraform backend type. |
| `--endpoint` | `""` | Kubernetes API endpoint. |
| `--vm-driver` | `""` | VM driver: `colima`, `colima-incus`, `docker-desktop`, `docker`. |
| `--vm-cpu` | `0` | CPU count for the workstation VM. |
| `--vm-memory` | `0` | Memory for the workstation VM (GB). |
| `--vm-disk` | `0` | Disk size for the workstation VM (GB). |
| `--vm-arch` | `""` | CPU architecture for the workstation VM. |
| `--docker` | `false` | Enable Docker. |
| `--git-livereload` | `false` | Enable git livereload. |
| `--aws-profile` | `""` | AWS profile name. |
| `--aws-endpoint-url` | `""` | AWS endpoint URL. |
| `--set` | `[]` | Override config values, e.g. `--set dns.enabled=false`. May be repeated. |
| `--provider` | `""` | Deprecated: use `--platform`. |

## Examples

```sh
# Scaffold a local context with the docker VM driver
windsor init local --vm-driver=docker

# Re-initialize and overwrite generated files
windsor init local --reset

# Initialize an AWS staging context
windsor init staging --platform=aws --aws-profile=staging
```

## See also

- [Lifecycle guide](../../guides/lifecycle.md)
- [Contexts reference](../contexts.md)
- [`up`](up.md), [`apply`](apply.md), [`bootstrap`](bootstrap.md)
- Source: [cmd/init.go](https://github.com/windsorcli/cli/blob/main/cmd/init.go)
