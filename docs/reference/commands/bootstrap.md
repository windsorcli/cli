---
title: "windsor bootstrap"
description: "Bootstrap a fresh environment end-to-end."
---

```sh
windsor bootstrap [context] [flags]
```

First-run setup for a context: applies Terraform, installs the Flux blueprint, migrates state to the configured remote backend, and waits for kustomizations. Bootstrap fails when a kustomization fails: a BuildFailed or ArtifactFailed condition, or a failed HelmRelease. Bootstrap also fails when a kustomization is still in progress at the end of the wait. If the blueprint declares post-run notes, bootstrap gives a warning for a kustomization in progress, and does not fail.

When the blueprint declares a backend Terraform component, bootstrap runs a two-phase apply to resolve the chicken-and-egg case where the remote backend (S3 bucket, DynamoDB table, etc.) lives in infrastructure Terraform must create first:

    1. Override terraform.backend.type to 'local' in memory and apply only the backend component.
    2. Restore the configured backend type and migrate the backend component's state.
    3. Subsequent components init directly against the remote backend.

When no backend component exists, bootstrap falls through to a single apply against whatever backend is configured.

Re-running on an existing context prompts for confirmation. Pass --yes (or -y) to skip the prompt in CI.

Pass --to to stop after one terraform component or kustomization, for example to do a manual step. Bootstrap then prints the command that continues. Run 'windsor bootstrap --continue' to skip the finished terraform components and run the rest, with no prompt. A stopped bootstrap writes no version marker. The backend components run as one unit, so --to accepts only the last of them.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--blueprint` | `""` | Blueprint OCI reference. Accepts oci://host/org/repo:tag, host/org/repo:tag, or org/repo:tag (host defaults to ghcr.io). Tag is required. |
| `--continue` | `false` | Continue a run that --to stopped, and skip the terraform components that it finished. |
| `--platform` | `""` | Target platform: none, docker, incus, metal, hetzner, aws, azure, gcp, hyperv, vsphere, proxmox. |
| `--set` | `[]` | Override config values, e.g. --set dns.enabled=false. May be repeated. |
| `--to` | `""` | Stop after this terraform component or kustomization. Use terraform/<id> or kustomize/<name> when a name matches both. |
| `-y`, `--yes` | `false` | Skip all confirmation prompts. |

## Examples

```sh
# Bootstrap a new staging context on AWS
windsor bootstrap staging --platform=aws --blueprint=ghcr.io/myorg/blueprint:v1.0.0

# Re-bootstrap an existing context, scripted (skip the prompt)
windsor bootstrap prod --yes

# Stop after the cluster component, then continue
windsor bootstrap --to cluster
windsor bootstrap --continue
```

## See also

- [`init`](init.md), [`apply`](apply.md), [`destroy`](destroy.md)
- [Global flags](../global-flags.md) — `--no-cache`, `--lock-timeout`, `--verbose`
- Source: [cmd/bootstrap.go](https://github.com/windsorcli/cli/blob/main/cmd/bootstrap.go)
