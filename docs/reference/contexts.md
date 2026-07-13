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
    ├── .env                                git-ignored operator env vars (e.g. provider credentials)
    ├── terraform/<component-id>.tfvars     user-edited Terraform variable overrides
    ├── terraform/<component-id>.tfvars.json  JSON variant of the above
    ├── terraform/backend.tfvars            optional Terraform backend overrides
    ├── terraform/.env                      git-ignored, Terraform-only env vars
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
| `terraform/.env` | dotenv | Terraform-only operator env vars, auto-git-ignored. See [Terraform-scoped `.env`](#terraform-scoped-env) below. |
| `.env` | dotenv | Operator-supplied environment variables, auto-git-ignored. See [`.env` files](#env-files) below. |
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

## `.env` files

`contexts/<context-name>/.env` is a per-context, git-ignored dotenv file for
provider environment variables that windsor has no native config for — Hyper-V
(`HYPERV_USER`, `HYPERV_PASSWORD`, `HYPERV_HOST`, …) and vSphere
(`VSPHERE_USER`, `VSPHERE_PASSWORD`, `VSPHERE_SERVER`, …) are the motivating
cases. Use it for credentials/local values; use the `environment:` key under
`contexts.<name>` in `windsor.yaml` for declarative, version-controlled values
shared with the team (see [Configuration reference](configuration.md)).

- Standard `KEY=VALUE` lines, one per line; `#` starts a comment.
- Values may reference secrets, e.g. `HYPERV_PASSWORD=${secret("op://vault/item/field")}`.
- Loaded on every `windsor env` / `windsor exec` / shell hook invocation, with
  the **lowest precedence** of any environment source — `windsor.yaml`'s
  `environment:` key and every provider-specific printer (AWS, Azure, GCP,
  Terraform, …) override a same-named `.env` key.
- Windsor warns (without failing) if the file is readable by group or other;
  restrict it to the owner (`chmod 600`).

### Committing `.env` anyway

A team that wants to share `.env` (or `terraform/.env`) through git — for
example, a file whose values are all `secret(...)` references rather than
raw credentials — can add a negation line to the end of
`contexts/<context-name>/.gitignore`:

```
!.env
```

Don't delete the `.env` line itself to do this. Windsor re-adds any missing
line from its managed set on every `windsor init`/`up` (so upgrading to a
newer CLI still protects contexts created by an older one), and deleting the
line would just have it silently reappear. Appending `!.env` after it is
stable: the line is still present, so Windsor leaves the file alone, and
git's own last-match-wins rule un-ignores the file anyway.

## Terraform-scoped `.env`

`contexts/<context-name>/terraform/.env` is a second, narrower dotenv file
for variables that are only ever relevant to Terraform — most commonly
provider credentials read directly from the environment, like Hyper-V or
vSphere. Same format as the general `.env` above, including `secret(...)`
support.

The difference is *when* it loads. The general `.env` loads on every
`windsor env` / shell hook invocation; `terraform/.env` loads only when
Windsor is actually doing Terraform work — either the operator is `cd`'d
into a `terraform/<component>/` directory, or a command like `windsor up`
is running. A plain shell prompt elsewhere never touches it, so it never
pays secret-resolution cost for content that isn't relevant right now. Move
a variable here instead of the general `.env` once you notice it's
Terraform-only and secret-resolution heavy enough to matter for prompt
latency.

## See also

- [Windsor state directory](windsor-dir.md) — system-managed `.windsor/` layout
- [Blueprint reference](blueprint.md), [Configuration reference](configuration.md)
- [Metadata reference](metadata.md), [Testing reference](testing.md)
- [`init`](commands/init.md), [`set`](commands/set.md), [`get`](commands/get.md), [`bootstrap`](commands/bootstrap.md)
