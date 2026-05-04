---
title: "windsor bootstrap"
description: "Bootstrap a fresh environment end-to-end."
---
# windsor bootstrap

```sh
windsor bootstrap [context] [flags]
```

Configure the project, apply Terraform with local state first, migrate to the configured remote backend, then install the blueprint and wait for kustomizations. The two-phase apply solves the chicken-and-egg case where the configured remote backend (S3 bucket, DynamoDB table, etc.) lives in infrastructure Terraform must create first.

When the blueprint declares a `backend` Terraform component:

1. Override `terraform.backend.type` to `local` in memory and apply only the backend component.
2. Restore the configured backend type and migrate the backend component's state.
3. Subsequent components init directly against the remote backend.

When no `backend` component exists, bootstrap falls through to a single apply against whatever backend is configured.

Re-running on an existing context prompts for confirmation; pass `--yes` (or `-y`) to skip in CI.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--platform` | `""` | Target platform: `none`, `metal`, `docker`, `aws`, `azure`, `gcp`. |
| `--blueprint` | `""` | Blueprint OCI reference. |
| `--set` | `[]` | Override config values, e.g. `--set dns.enabled=false`. |
| `-y`, `--yes` | `false` | Skip all confirmation prompts. |

## Examples

```sh
# Bootstrap a new staging context on AWS
windsor bootstrap staging --platform=aws --blueprint=ghcr.io/myorg/blueprint:v1.0.0

# Re-bootstrap an existing context, scripted
windsor bootstrap prod --yes
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [Terraform guide](https://www.windsorcli.dev/docs/components/terraform)
- [`init`](init.md), [`apply`](apply.md), [`destroy`](destroy.md)
- Source: [cmd/bootstrap.go](https://github.com/windsorcli/cli/blob/main/cmd/bootstrap.go)
