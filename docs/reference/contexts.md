---
title: "Contexts directory"
description: "On-disk layout of contexts/ at the project root."
---
# Contexts directory

`contexts/` lives at the project root. It holds one subdirectory per
context (`contexts/<name>/`) plus a shared `contexts/_template/`
directory that supplies the blueprint template the runtime expands for
every context.

## Layout

```
contexts/
├── _template/                              shared blueprint template
│   ├── blueprint.yaml                      component + kustomization definitions
│   ├── metadata.yaml                       artifact metadata (name, version, description)
│   ├── schema.yaml                         JSON Schema for context input values
│   ├── tests/<name>.test.yaml              composition-test fixtures
│   ├── terraform/<path>/                   local Terraform modules
│   └── kustomize/<path>/                   kustomization bases and patches
└── <context-name>/                         per-context configuration and credentials
    ├── blueprint.yaml                      referential blueprint for this context
    ├── values.yaml                         user-set values that feed the schema
    ├── terraform/<component-id>.tfvars     user-edited Terraform variable overrides
    ├── terraform/<component-id>.tfvars.json  JSON variant of the above
    ├── terraform/backend.tfvars            optional Terraform backend overrides
    ├── .aws/{config,credentials}           AWS CLI config + credentials, scoped to this context
    ├── .azure/                             Azure CLI config, scoped to this context
    ├── .gcp/gcloud/                        gcloud CLI config, scoped to this context
    ├── .gcp/service-accounts/default.json  default GCP service-account key
    ├── .kube/config                        kubectl kubeconfig
    ├── .talos/config                       Talos cluster config (cluster.driver=talos)
    └── .omni/config                        Omni cluster config (platform=omni)
```

## `_template/`

| Path | Type | Description |
|------|------|-------------|
| `blueprint.yaml` | YAML | Blueprint definition: kind, apiVersion, metadata, terraform components, kustomizations. See [Blueprint reference](blueprint.md). |
| `metadata.yaml` | YAML | Artifact metadata used by `windsor bundle` and `windsor push`. See [Metadata reference](metadata.md). |
| `schema.yaml` | YAML (JSON Schema 2020-12) | Validates the merged values object before render. Also supplies the defaults shown by `windsor values`. |
| `tests/<name>.test.yaml` | YAML | Composition tests run by `windsor test`. See [Testing reference](testing.md). |
| `terraform/<path>/` | Directory | Local Terraform modules referenced by blueprint components. |
| `kustomize/<path>/` | Directory | Kustomization bases and patches referenced by blueprint kustomizations. |

The same template is reused across every context. Per-context inputs
live under `contexts/<context-name>/`.

## `<context-name>/`

| Path | Type | Description |
|------|------|-------------|
| `blueprint.yaml` | YAML | Per-context blueprint metadata. Written by `windsor init`; references the template rather than expanding it. |
| `values.yaml` | YAML | User-supplied values for the schema. Read on every command; written by `windsor set` and `SaveConfig`. |
| `terraform/<component-id>.tfvars` | HCL | Operator-authored variable overrides for a Terraform component. Read at plan/apply time. |
| `terraform/<component-id>.tfvars.json` | JSON | JSON-formatted alternative to `.tfvars`. |
| `terraform/backend.tfvars` | HCL | Optional overrides applied to the Terraform backend init. |
| `.aws/config`, `.aws/credentials` | INI | AWS CLI files; the env hook sets `AWS_CONFIG_FILE` and `AWS_SHARED_CREDENTIALS_FILE` so `aws` commands inside the windsor shell write here instead of `~/.aws/`. |
| `.azure/` | Directory | Azure CLI config; `AZURE_CONFIG_DIR` points `az` here. |
| `.gcp/gcloud/` | Directory | gcloud CLI config; `CLOUDSDK_CONFIG` points `gcloud` here. |
| `.gcp/service-accounts/default.json` | JSON | Default GCP service-account key path; overridable via `gcp.credentials_path`. |
| `.kube/config` | YAML | Kubeconfig; `KUBECONFIG` points `kubectl` here. |
| `.talos/config` | YAML | Talos config; `TALOSCONFIG` points `talosctl` here. Present when the blueprint's cluster driver is `talos`. |
| `.omni/config` | YAML | Omni config; `OMNICONFIG` points `omnictl` here. Present when `platform: omni`. |

Hidden subdirectories (`.aws/`, `.azure/`, `.gcp/`, `.kube/`,
`.talos/`, `.omni/`) keep CLI state scoped to the context so that tools
invoked through the windsor shell never touch the operator's global
config under `~/`.

## Reset behaviour

`windsor init <context> --reset` removes the per-context credential and
config directories (`.aws/`, `.gcp/`, `.kube/`, `.talos/`, `.omni/`)
along with the runtime caches under `.windsor/contexts/<name>/`.
`.azure/` is **not** cleaned — operators wiping Azure credentials must
remove that directory by hand. User-authored files (`blueprint.yaml`,
`values.yaml`, hand-edited `terraform/*.tfvars`,
`terraform/backend.tfvars`) are preserved.

## See also

- [Windsor state directory](windsor-dir.md) — system-managed `.windsor/` layout
- [Blueprint reference](blueprint.md), [Configuration reference](configuration.md)
- [Metadata reference](metadata.md), [Testing reference](testing.md)
- [`init`](commands/init.md), [`set`](commands/set.md), [`get`](commands/get.md), [`bootstrap`](commands/bootstrap.md)
