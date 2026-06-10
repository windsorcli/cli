---
title: "windsor bootstrap"
description: "Bootstrap a fresh environment end-to-end."
---
# windsor bootstrap

```sh
windsor bootstrap [context] [flags]
```

First-run setup for a context: applies Terraform, installs the Flux blueprint, migrates state to the configured remote backend, and waits for kustomizations.

When the blueprint declares a backend Terraform component, bootstrap runs a two-phase apply to resolve the chicken-and-egg case where the remote backend (S3 bucket, DynamoDB table, etc.) lives in infrastructure Terraform must create first:

    1. Override terraform.backend.type to 'local' in memory and apply only the backend component.
    2. Restore the configured backend type and migrate the backend component's state.
    3. Subsequent components init directly against the remote backend.

When no backend component exists, bootstrap falls through to a single apply against whatever backend is configured.

Re-running on an existing context prompts for confirmation. Pass --yes (or -y) to skip the prompt in CI.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--blueprint` | `""` | Blueprint OCI reference. Accepts oci://host/org/repo:tag, host/org/repo:tag, or org/repo:tag (host defaults to ghcr.io). Tag is required. |
| `--platform` | `""` | Target platform: none, metal, docker, aws, azure, gcp, hyperv. |
| `--set` | `[]` | Override config values, e.g. --set dns.enabled=false. May be repeated. |
| `-y`, `--yes` | `false` | Skip all confirmation prompts. |

## Examples

```sh
# Bootstrap a new staging context on AWS
windsor bootstrap staging --platform=aws --blueprint=ghcr.io/myorg/blueprint:v1.0.0

# Re-bootstrap an existing context, scripted (skip the prompt)
windsor bootstrap prod --yes
```

## See also

- [`init`](/reference/cli/commands/init), [`apply`](/reference/cli/commands/apply), [`destroy`](/reference/cli/commands/destroy)
- Source: [cmd/bootstrap.go](https://github.com/windsorcli/cli/blob/main/cmd/bootstrap.go)
