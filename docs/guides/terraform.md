---
title: "Terraform"
description: "How Windsor drives Terraform: components, generated tfvars, backend configuration, and the bootstrap workflow."
---
# Terraform

Windsor invokes Terraform on your behalf. You declare components in `blueprint.yaml`, Windsor generates per-component shims, materializes a `terraform.tfvars` for each, sets the right `TF_*` environment variables, and runs `init`, `plan`, `apply`, and `destroy` against the right module in the right order.

OpenTofu support is **experimental**. Set `terraform.driver: opentofu` in the root `windsor.yaml`, or let Windsor auto-detect from `$PATH` — it prefers `terraform`, falls back to `tofu`. Behavior may diverge from Terraform on edge cases; report issues you hit.

## Folder layout

Two trees matter. The first is your project source; the second is Windsor's working area.

```plaintext
contexts/
└── local/
    ├── blueprint.yaml          # the only thing required after `windsor init`
    ├── values.yaml             # context values consumed by facets
    └── terraform/              # OPTIONAL: hand-authored .tfvars overrides
        └── cluster/talos.tfvars
terraform/
└── database/postgres/
    ├── main.tf
    └── variables.tf
.windsor/
└── contexts/
    └── local/
        ├── terraform/<component>/  # generated module shim + terraform.tfvars
        │   ├── main.tf
        │   ├── variables.tf
        │   ├── outputs.tf
        │   ├── backend_override.tf
        │   └── terraform.tfvars
        └── .terraform/<component>/ # provider plugins, modules, tfstate (local backend)
            └── terraform.tfstate
```

Modules under `terraform/` are local to the project; reference them with a `path:` and no `source:`. Modules pulled from blueprint sources (OCI artifacts or git repositories) are unpacked into `.windsor/contexts/<name>/terraform/<component>/` as shims.

`contexts/<name>/terraform/<component>.tfvars` is an optional user override. When present, Windsor consumes it instead of the generated tfvars.

## Declaring components

Add a `terraform:` entry to `blueprint.yaml` for each module the context depends on:

```yaml
terraform:
- source: core
  path: cluster/talos
- source: core
  path: gitops/flux
  dependsOn:
    - cluster/talos
- path: example/my-app          # local module under terraform/
- name: backend                 # opt-in formal name (any unique slug)
  source: core
  path: aws/state
  inputs:
    bucket_name: ${cluster.name}-state
```

Components without a `source:` resolve to `terraform/<path>` in the project. Components with `source:` resolve into the named blueprint source. `inputs` accepts both literal values and `${...}` expressions evaluated at compose time. See [reference/blueprint.md](../reference/blueprint.md) for the full schema.

## Lifecycle commands

| Command | Effect |
|---------|--------|
| `windsor plan terraform [component]` | `init` + `plan` for one component, or all if omitted. `--summary` prints a table; otherwise streams full output. New components show `(new)`. |
| `windsor apply terraform <component>` | `init` + `plan` + `apply` for the named component. `<component>` is required. |
| `windsor apply` | Apply every component in dependency order, then install the Flux blueprint. |
| `windsor destroy terraform [component]` | Destroy one component, or all in reverse-topological order. `--confirm=<token>` skips the interactive prompt. |
| `windsor up` / `windsor down` | Workstation contexts only. `up` drives Terraform + Flux for the workstation; `down` stops the VM. |
| `windsor bootstrap` | First-run setup — see [Bootstrap](#bootstrap) below. |

When Windsor invokes Terraform from `apply`, it runs `terraform apply <plan-file>` against a saved plan, so Terraform never prompts and no `-auto-approve` is needed. When Windsor invokes Terraform from `destroy`, it passes `-auto-approve`; Windsor itself owns the confirmation gate (`--confirm` or interactive prompt) before that. If you run `terraform destroy` directly inside a `windsor env`-managed shell, Windsor leaves `TF_CLI_ARGS_destroy` empty so Terraform's own prompt fires.

## Variables and environment

Windsor materializes inputs to a generated `terraform.tfvars` next to each module shim and points Terraform at the right plan/apply/destroy args via `TF_CLI_ARGS_*`. You don't pass `-var-file` yourself.

The runtime injects these variables into Terraform invocations (visible in a `windsor env`-managed shell when your CWD is inside a terraform module dir):

| Variable | Meaning |
|----------|---------|
| `TF_VAR_context` | Current context name. |
| `TF_VAR_context_id` | Stable context id. |
| `TF_VAR_context_path` | Filesystem path to the current context. |
| `TF_VAR_project_root` | Filesystem path to the project root. |
| `TF_VAR_os_type` | `linux`, `darwin`, or `windows`. |
| `TF_VAR_operation` | `apply` during apply/up/bootstrap; `destroy` during destroy/down. Useful for components that branch on lifecycle phase. |

Per-component `TF_VAR_<input>` variables are also materialized from the component's evaluated `inputs:`.

Cloud-provider variables (`AWS_*`, `ARM_*`, `GOOGLE_*`) are derived from the active context's `aws` / `azure` / `gcp` config blocks. See [Environment reference](../reference/environment.md).

To inspect what Windsor exports for the current context:

```sh
windsor env
```

## Cross-component outputs

A component can read another component's outputs through the `terraform_output` expression helper inside facet expressions:

```yaml
terraform:
- name: cluster
  source: core
  path: cluster/talos
- name: app
  path: example/my-app
  dependsOn: [cluster]
  inputs:
    api_endpoint: ${terraform_output("cluster", "endpoint")}
    mirrors: ${terraform_output("workstation", "registries") ?? {}}
```

Windsor walks components in dependency order; when `terraform_output("cluster", "endpoint")` is evaluated for `app`, the `cluster` component's `terraform output -json` is read on the spot and the result becomes `TF_VAR_api_endpoint`. The `??` operator supplies a default when the upstream component has not been applied yet — this lets the dependent component still plan.

The bare path syntax `${terraform.<other>.outputs.<key>}` does not exist — only the `terraform_output()` helper is supported, and only inside facet expressions.

## Backend configuration

Configure the state backend in the context's `windsor.yaml`:

```yaml
terraform:
  backend:
    type: s3              # local | s3 | kubernetes | azurerm
    s3:
      bucket: my-tf-state
      key: contexts/staging
      region: us-east-2
```

Windsor writes a `backend_override.tf` next to each generated module shim that points at the configured backend, so the underlying terraform module doesn't need a hard-coded `backend` block. State is keyed per-component, isolated within a single context.

Default backend by `platform`, applied at `windsor init` / `windsor bootstrap` / `windsor up` time when no explicit backend is set:

| Platform | Default backend |
|----------|-----------------|
| `aws` | `s3` |
| `azure` | `azurerm` |
| `metal`, `docker`, `incus` | `kubernetes` (each component's state is stored as a Secret in the cluster) |
| `gcp`, `none`, unset | not defaulted (effectively `local`) |

Override at init time with `--backend`, or via `--set terraform.backend.type=...` on `bootstrap`, or by editing `windsor.yaml` directly.

## Bootstrap

`windsor bootstrap` handles the chicken-and-egg case where the configured remote backend lives in infrastructure that Terraform itself must create — for example, an S3 bucket for state, or an Azure Storage account.

When the blueprint declares a component named `backend`, bootstrap runs in two phases:

1. **Phase 1.** Override `terraform.backend.type` to `local` in memory and apply only the `backend` component. This materializes the remote state store (bucket, table, etc.).
2. **Phase 2.** Restore the configured backend type and `terraform init -migrate-state -force-copy` for the `backend` component, moving its state to remote. The on-disk `windsor.yaml` is never mutated.

The next `apply` / `up` initializes the rest of the components directly against the configured remote backend with no migration needed — they have not been applied yet.

When the blueprint has no `backend` component, bootstrap behaves like `apply --wait` against whatever backend is configured. It is also safe to run repeatedly; subsequent invocations detect the migrated backend and skip Phase 1.

```sh
windsor bootstrap                    # current context
windsor bootstrap staging            # switch context, then bootstrap
windsor bootstrap --platform aws --blueprint ghcr.io/org/blueprint:v1.2.0
```

## Workstation network callback

When the blueprint includes a Terraform component that represents the workstation itself (component id `workstation`), host/guest networking and DNS are deferred until after that component applies. A hook then configures host routes, guest networking, and DNS for the active platform (Colima, Docker, etc.) using the DNS address from the component's outputs when available.

This is workstation-context behavior only; non-workstation contexts skip the callback.

## See also

- [Lifecycle](https://www.windsorcli.dev/docs/cli/lifecycle) — phase-by-phase command map
- [Environment reference](../reference/environment.md) — full env table including cloud providers
- [Local Workstation](local-workstation.md) — workstation-specific terraform components
- [reference/blueprint.md](../reference/blueprint.md#terraformcomponent) — `TerraformComponent` schema
