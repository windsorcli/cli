---
title: "Contexts"
description: "Reference for context layout, on-disk files, and the directories Windsor materializes per context."
---
# Contexts

A context is a group of configuration values specific to one deployment target in a Windsor project. Each context has user-authored files under `contexts/<name>/` and a system-managed working area under `.windsor/contexts/<name>/`.

## Layout

```text
contexts/
└── local/
    ├── values.yaml              # user-authored context values
    ├── blueprint.yaml           # user-authored blueprint overrides (optional)
    ├── patches/                 # context-specific Kustomize patches (optional)
    │   └── <kustomization>/
    │       └── *.yaml
    ├── terraform/               # OPTIONAL: hand-authored .tfvars overrides
    │   └── <component>.tfvars
    ├── .aws/                    # generated: AWS config + credentials (project mode only)
    ├── .azure/                  # generated: Azure config (project mode only)
    ├── .gcp/                    # generated: gcloud config (project mode only)
    ├── .kube/
    │   └── config               # generated: kubeconfig
    └── .talos/
        └── config               # generated: talosconfig
.windsor/
└── contexts/
    └── local/
        ├── workstation.yaml             # system-managed: platform, runtime, arch, dns
        ├── terraform/<component>/       # generated module shim
        │   ├── main.tf · variables.tf · outputs.tf
        │   ├── backend_override.tf
        │   └── terraform.tfvars
        └── .terraform/<component>/      # provider plugins, modules, tfstate (local backend)
            └── terraform.tfstate
```

User-authored files are versioned. The system-managed files (`.windsor/`) and the generated credential dirs (`.aws/`, `.azure/`, `.gcp/`, `.kube/`, `.talos/`) should not be checked in.

## User-authored files

### `values.yaml`

The primary context-level configuration. After `windsor init <context>`, context-specific defaults are written here. Values in this file override schema defaults and are available to facets for expression evaluation. See the [configuration reference](configuration.md) for the full schema.

```yaml
# contexts/local/values.yaml
dev: true
dns:
  domain: test
id: w2g5rk7d
cluster:
  controlplanes:
    cpu: 6
    memory: 8
```

`values.yaml` is:

- Loaded and merged with the schema defaults from `_template/schema.yaml`.
- Validated against that schema if present.
- Available to facet expressions and to terraform input evaluation.

### `blueprint.yaml`

Per-context overrides on top of the composed blueprint. Most contexts don't author this — the base `_template/blueprint.yaml` plus facets cover the common cases. Use it to add context-specific terraform components or kustomizations.

See the [blueprint reference](blueprint.md) for the schema.

### `patches/`

Strategic-merge or JSON 6902 patches applied to specific kustomizations. Files at `contexts/<name>/patches/<kustomization-name>/*.yaml` are auto-discovered and added to that kustomization's `patches`.

### `terraform/<component>.tfvars` (optional)

Hand-authored tfvars that override the per-component file Windsor generates at `.windsor/contexts/<name>/terraform/<component>/terraform.tfvars`. When present, Windsor consumes the user file instead. Useful for local one-off overrides without changing the blueprint.

## System-managed files

### `.windsor/contexts/<name>/workstation.yaml`

Ephemeral state Windsor writes during `init` / `up` based on `--vm-driver`, `--platform`, and host architecture. Holds `platform`, `workstation.runtime`, `workstation.arch`, `workstation.dns.address`, and the booted VM IP. Treat as ephemeral; do not commit. See the [configuration reference](configuration.md#workstation-system-managed) for the field set.

### `.windsor/contexts/<name>/terraform/<component>/`

The **module shim** Windsor materializes from blueprint sources. Contains the resolved `.tf` files, `backend_override.tf` (so the upstream module doesn't need to declare a backend), and the generated `terraform.tfvars`. Windsor invokes Terraform from here.

### `.windsor/contexts/<name>/.terraform/<component>/`

Terraform's working directory — provider plugins, downloaded modules, and (for the `local` backend) the `terraform.tfstate` file. Per-component, isolated within the context.

## Generated credential / kubeconfig dirs

| Directory | Purpose |
|-----------|---------|
| `contexts/<name>/.aws/` | `config` and `credentials` for the AWS CLI. Used in project mode (suppressed in global mode). |
| `contexts/<name>/.azure/` | `az` config directory. Project mode only. |
| `contexts/<name>/.gcp/` | `gcloud` config directory. Project mode only. |
| `contexts/<name>/.kube/config` | Kubeconfig the workstation cluster writes; exported as `KUBECONFIG` / `KUBE_CONFIG_PATH` / `K8S_AUTH_KUBECONFIG`. |
| `contexts/<name>/.talos/config` | Talos client config; exported as `TALOSCONFIG`. |

The cloud-provider directories let `aws configure sso`, `az login`, and `gcloud auth login` write their credentials into the context rather than into the operator's `~/.aws/`, `~/.azure/`, `~/.config/gcloud/`. See the [environment reference](environment.md) for the full env-var table.

## Context management

### Creating

```bash
windsor init <context-name>
```

Creates `contexts/<context-name>/` with `values.yaml` and a baseline `blueprint.yaml` (when needed), and adds the project root to your trusted-folders list. Per-context configuration lives in `contexts/<name>/values.yaml`; the root `windsor.yaml` only carries the schema version and any project-level overrides.

Contexts named `local` or starting with `local-` are workstation contexts: defaults flip toward local virtualization, and lifecycle uses [`up`](commands/up.md) / [`down`](commands/down.md). All other contexts use [`apply`](commands/apply.md) / [`destroy`](commands/destroy.md).

### Switching

```bash
windsor set context <context-name>
```

### Inspecting

```bash
windsor get context           # current context name
windsor get contexts          # list every context in the project
```

The current context is also exported as `WINDSOR_CONTEXT` (and `WINDSOR_CONTEXT_ID` for the stable id from `values.yaml`). See the [environment reference](environment.md#windsor-core).
