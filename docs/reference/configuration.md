---
title: "Configuration"
description: "Operator-authored windsor.yaml schema."
---
# Configuration

Operator-authored windsor.yaml schema. Lives at the project root and
declares the per-context settings every Windsor command reads. The system
also writes ephemeral state (workstation, computed defaults) into
.windsor/contexts/<name>/workstation.yaml at runtime — that file is
system-managed and not covered here.

## Fields

| Field | Type | Description |
|------|------|-------------|
| `version` | `string` | Config schema version. Currently 'v1alpha1'. **(required)** |
| `contexts` | `object` | Map of context name to per-context configuration. Most projects have 'local' (workstation context) plus one or more deployment contexts (staging, prod, etc.). **(required)** |
| `terraform` | `object` | Root-level Terraform settings shared across every context. |
| `toolsManager` | `string` | Name of the tool manager whose configuration governs binary versions for this project. Currently the only supported value is 'aqua'. |

## terraform

| Field | Type | Description |
|------|------|-------------|
| `driver` | `string` | Terraform driver to use ('terraform' or 'tofu'). |

## Examples

```yaml
version: v1alpha1
contexts:
  local:
    dns:
      domain: local.test
    docker:
      enabled: true
    platform: docker
  staging:
    aws:
      profile: staging
      region: us-east-1
    cluster:
      driver: talos
      enabled: true
      endpoint: https://staging-cp.example.test:6443
    platform: aws
    terraform:
      backend:
        bucket: my-org-tf-state
        region: us-east-1
        type: s3
      enabled: true
```

## See also

- [Contexts reference](contexts.md) — context layout, on-disk files, and lifecycle
- [Blueprint reference](blueprint.md), [Facets reference](facets.md)
- [`init`](commands/init.md), [`show values`](commands/show-values.md), [`get contexts`](commands/get-contexts.md)
- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle), [Contexts guide](https://www.windsorcli.dev/docs/cli/contexts)
- Source schema: [pkg/runtime/config/schemas/artifacts/configuration.yaml](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schemas/artifacts/configuration.yaml)
