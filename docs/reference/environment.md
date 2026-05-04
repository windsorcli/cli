---
title: "Environment"
description: "Catalog of environment variables Windsor manages, organized by tool."
---
# Environment

Catalog of environment variables Windsor manages. Variables are emitted by `windsor env` whenever the current directory is trusted; the shell hook re-emits them on every prompt and updates `WINDSOR_MANAGED_ENV` so stale variables get unset on context switch. See [Environment Injection](https://www.windsorcli.dev/docs/cli/environment-injection) for the conceptual overview.

## Project mode and global mode

Windsor walks up from the current directory looking for `windsor.yaml`. When found, the shell is in **project mode** and that directory is the project root. When no `windsor.yaml` is found, Windsor falls back to `~/.config/windsor` as the effective project root and runs in **global mode**.

Some context-scoped variables suppress in global mode so Windsor does not override operator-level configuration:

| Variable / file | Project mode | Global mode |
|---|---|---|
| `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE` | scoped to context's `.aws/` | not emitted (defers to `~/.aws/`) |
| `AZURE_CONFIG_DIR`, `AZURE_CORE_LOGIN_EXPERIENCE_V2` | scoped to context's `.azure/` | not emitted |
| `CLOUDSDK_CONFIG`, `GOOGLE_APPLICATION_CREDENTIALS` (default path) | scoped to context's `.gcp/gcloud/` | not emitted (defers to ambient gcloud) |
| `AWS_PROFILE`, `ARM_*`, `GOOGLE_CLOUD_PROJECT`, `TF_VAR_kubelogin_mode` | emitted | emitted |
| `KUBECONFIG`, `TALOSCONFIG` | emitted | emitted |
| `DOCKER_HOST`, `DOCKER_CONFIG` | emitted | not emitted (no workstation in global mode) |

The principle: variables that describe **which** account/cluster/project the context targets flow in both modes. Variables that would redirect tools to context-local **credential or config files** flow only in project mode.

## --hook mode

The `--hook` flag puts `env` in non-fatal mode: warnings are suppressed and errors exit 0 so a misconfigured project never breaks your prompt. Run `windsor env` without `--hook` to see the full output and any errors.

## Windsor core

Always emitted in both modes (subject to trust):

| Variable | Description |
|---|---|
| `WINDSOR_CONTEXT` | Current context name. |
| `WINDSOR_CONTEXT_ID` | Stable per-context identifier (set in `values.yaml`). |
| `WINDSOR_PROJECT_ROOT` | Project root, or `~/.config/windsor` in global mode. |
| `WINDSOR_SESSION_TOKEN` | Identifier for the current shell's reset state. |
| `WINDSOR_MANAGED_ENV` | Comma-separated list of variables Windsor will unset on context switch. |
| `WINDSOR_MANAGED_ALIAS` | Comma-separated list of aliases Windsor manages. |
| `BUILD_ID` | Optional artifact build identifier (set when `windsor build-id` has been run). |

## AWS

AWS integration activates when the context has `platform: aws` or any `aws:` block.

```yaml
aws:
  region: us-east-2
  profile: company-prod      # default: context name
  endpoint_url: ...           # optional
```

| Variable | Default | Configured by |
|---|---|---|
| `AWS_CONFIG_FILE` | `contexts/<name>/.aws/config` (project mode only) | — |
| `AWS_SHARED_CREDENTIALS_FILE` | `contexts/<name>/.aws/credentials` (project mode only) | — |
| `AWS_PROFILE` | context name | `aws.profile` |
| `AWS_REGION` | profile's `region =` line | `aws.region` |
| `AWS_ENDPOINT_URL` | system default | `aws.endpoint_url` |
| `S3_HOSTNAME` | system default | `aws.s3_hostname` |
| `MWAA_ENDPOINT` | system default | `aws.mwaa_endpoint` |

Pointing `AWS_CONFIG_FILE` and `AWS_SHARED_CREDENTIALS_FILE` at the context directory means `aws configure sso --profile <context>` writes into the context, not into the operator's `~/.aws/`. `AWS_PROFILE` defaults to the context name so the same profile names line up across `aws configure sso` and `aws sso login`.

## Azure

Azure integration activates when the context has `platform: azure` or any `azure:` block.

```yaml
azure:
  subscription_id: 00000000-0000-0000-0000-000000000000
  tenant_id: 11111111-1111-1111-1111-111111111111
  environment: public           # public | usgovernment | china | german
  kubelogin_mode: azurecli      # override only when needed
```

| Variable | Source | Notes |
|---|---|---|
| `AZURE_CONFIG_DIR` | `contexts/<name>/.azure/` | Project mode only. |
| `AZURE_CORE_LOGIN_EXPERIENCE_V2` | `false` | Project mode only. Pins to V1 login UX. |
| `ARM_SUBSCRIPTION_ID` | `azure.subscription_id` | Both modes. |
| `ARM_TENANT_ID` | `azure.tenant_id` | Both modes. |
| `ARM_ENVIRONMENT` | `azure.environment` | Both modes. |
| `TF_VAR_kubelogin_mode` | resolved (see below) | Both modes. |

`TF_VAR_kubelogin_mode` is resolved in this order:

1. `azure.kubelogin_mode` if set in config (the only path that handles managed identity, since MI has no env signal).
2. `workloadidentity` if `AZURE_FEDERATED_TOKEN_FILE` is set.
3. `spn` if `AZURE_CLIENT_SECRET` or `AZURE_CLIENT_CERTIFICATE_PATH` is set.
4. `azurecli` otherwise (default for laptop development).

## GCP

GCP integration activates when the context has `platform: gcp` or any `gcp:` block.

```yaml
gcp:
  project_id: my-project
  quota_project: my-quota-project
  credentials_path: /path/to/service-account.json   # optional
```

| Variable | Source | Notes |
|---|---|---|
| `CLOUDSDK_CONFIG` | `contexts/<name>/.gcp/gcloud/` | Project mode only. |
| `GOOGLE_APPLICATION_CREDENTIALS` | `gcp.credentials_path` if set; else `contexts/<name>/.gcp/service-accounts/default.json` (project mode only when not set) | Skipped if already set in the parent shell. |
| `GOOGLE_CLOUD_PROJECT` | `gcp.project_id` | Both modes. |
| `GCLOUD_PROJECT` | `gcp.project_id` | Both modes. |
| `GOOGLE_CLOUD_QUOTA_PROJECT` | `gcp.quota_project` | Both modes. |

## Docker and virtualization

`DOCKER_HOST` tracks the configured `workstation.runtime`:

| Runtime | `DOCKER_HOST` |
|---|---|
| `colima` | `unix://$HOME/.colima/windsor-<context>/docker.sock` |
| `docker-desktop` (macOS/Linux) | `unix://$HOME/.docker/run/docker.sock` |
| `docker-desktop` (Windows) | `npipe:////./pipe/docker_engine` |
| `docker` | `unix:///var/run/docker.sock` |

`DOCKER_CONFIG` points at `~/.config/windsor/docker` so docker CLI logins are scoped per operator, not per context.

| Variable | Description |
|---|---|
| `DOCKER_HOST` | See table above. |
| `DOCKER_CONFIG` | `~/.config/windsor/docker`. |
| `REGISTRY_URL` | Active workstation registry URL when registries are configured. |
| `INCUS_SOCKET` | Path to the Incus socket when `platform: incus`. |

## Kubernetes and Talos

Workstation contexts and any context with a Flux blueprint emit:

| Variable | Path |
|---|---|
| `KUBECONFIG` | `contexts/<name>/.kube/config` (or `~/.config/windsor/contexts/<name>/.kube/config` in global mode) |
| `KUBE_CONFIG_PATH` | same as `KUBECONFIG` (Terraform's kubernetes provider reads this). |
| `K8S_AUTH_KUBECONFIG` | same as `KUBECONFIG` (Ansible's k8s collection reads this). |
| `FLUX_SYSTEM_NAMESPACE` | `gitops.namespace` config, default `system-gitops`. |
| `TALOSCONFIG` | `contexts/<name>/.talos/config`. Project mode only. |
| `OMNICONFIG` | Path to the Omni client config when configured. |
| `PV_<NAMESPACE>_<NAME>` | Local filesystem path corresponding to a PVC of that name (workstation contexts with persistent volume mounts). |

`PV_*` variables are populated by inspecting the cluster's PVC list and matching against `contexts/<name>/.volumes/pvc-*` directories. If the API isn't reachable, they're skipped silently.

## Terraform

When the current directory is inside a generated Terraform module shim (`.windsor/contexts/<name>/terraform/<component>/`), Windsor adds:

- `TF_DATA_DIR` and `TF_CLI_ARGS_*` (init/plan/apply/refresh/import/destroy) targeting the right backend, plan file, and tfvars.
- `TF_VAR_context`, `TF_VAR_context_id`, `TF_VAR_context_path`, `TF_VAR_project_root`, `TF_VAR_os_type`, `TF_VAR_operation`.
- `TF_VAR_<input>` for each evaluated component input.

See the [Terraform guide](../guides/terraform.md) for the full lifecycle.

## See also

- [`windsor env`](commands/env.md), [`windsor hook`](commands/hook.md)
- [Environment Injection](https://www.windsorcli.dev/docs/cli/environment-injection) — conceptual overview
- [Trusted Folders](https://www.windsorcli.dev/docs/cli/trusted-folders) — how Windsor decides a project is safe
