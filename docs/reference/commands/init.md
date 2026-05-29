---
title: "windsor init"
description: "Scaffold or re-initialize a Windsor context."
---
# windsor init

```sh
windsor init [context] [flags]
```

Scaffold a Windsor project. Writes windsor.yaml at the project root if missing, creates contexts/<context>/, and adds the current directory to the trusted-folders list. Re-running on an existing context updates configuration; pass --reset to overwrite generated files and clean .terraform.

If no context is given, the current context is used; if none is set, 'local' is used.

The directory must be a git repository — init refuses to run in an empty or non-git directory to prevent silently scaffolding against $HOME.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--aws-endpoint-url` | `""` | AWS endpoint URL. |
| `--aws-profile` | `""` | AWS profile name. |
| `--backend` | `""` | Terraform backend type. |
| `--blueprint` | `""` | Blueprint OCI reference or local path. |
| `--docker` | `false` | Enable Docker. |
| `--endpoint` | `""` | Kubernetes API endpoint. |
| `--git-livereload` | `false` | Enable git livereload. |
| `--platform` | `""` | Target platform: none, metal, docker, aws, azure, gcp, hyperv. |
| `--reset` | `false` | Overwrite existing files and clean .terraform. |
| `--set` | `[]` | Override config values, e.g. --set dns.enabled=false. May be repeated. |
| `--vm-arch` | `""` | CPU architecture for the workstation VM. |
| `--vm-cpu` | `0` | CPU count for the workstation VM. |
| `--vm-disk` | `0` | Disk size for the workstation VM (GB). |
| `--vm-driver` | `""` | VM driver: colima, colima-incus, docker-desktop, docker. |
| `--vm-memory` | `0` | Memory for the workstation VM (GB). |

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

- [`up`](up.md), [`apply`](apply.md), [`bootstrap`](bootstrap.md)
- Source: [cmd/init.go](https://github.com/windsorcli/cli/blob/main/cmd/init.go)
