---
title: "Environment Injection"
description: "How Windsor manages environment variables across contexts, projects, and global mode."
---
# Environment injection

Windsor manages environment variables for the tools it drives — `kubectl`, `terraform`, the cloud-provider CLIs, container runtimes — so that switching contexts or moving between projects updates your shell automatically. Conceptually it is similar to [direnv](https://github.com/direnv/direnv), but the variables track the active Windsor context rather than the current directory.

## How it works

`windsor hook <shell>` emits a snippet you install into your `~/.zshrc`, `~/.bashrc`, or PowerShell profile (see [Installation](../install.md)). On every prompt, the hook calls `windsor env --hook` and the shell evaluates the output. The variables Windsor manages are listed in `WINDSOR_MANAGED_ENV`; on context switch the hook unsets stale variables before applying the new set.

`windsor env` only emits variables when the current directory is **trusted**. Trust is recorded in `~/.config/windsor/.trusted` and added by `windsor init`. If a project hasn't been trusted, `windsor env` exits silently.

The `--hook` flag puts `env` in non-fatal mode: it suppresses warnings and exits 0 on errors so a misconfigured project never breaks your prompt. Run `windsor env` without `--hook` to see the full output and any errors.

## Project mode and global mode

Windsor finds the project root by walking up from the current directory looking for `windsor.yaml`. When found, the shell is in **project mode** and that directory is the project root.

When no `windsor.yaml` is found in the tree, Windsor falls back to `~/.config/windsor` as the effective project root and runs in **global mode**. Global mode lets cloud-provider CLIs and `kubectl` operate against a Windsor-managed context from any directory.

Some context-scoped variables are deliberately suppressed in global mode so Windsor doesn't override your operator-level config:

| Variable / file | Project mode | Global mode |
|---|---|---|
| `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE` | scoped to context's `.aws/` | not emitted (defers to `~/.aws/`) |
| `AZURE_CONFIG_DIR`, `AZURE_CORE_LOGIN_EXPERIENCE_V2` | scoped to context's `.azure/` | not emitted |
| `CLOUDSDK_CONFIG`, `GOOGLE_APPLICATION_CREDENTIALS` (default path) | scoped to context's `.gcp/gcloud/` | not emitted (defers to ambient gcloud) |
| `AWS_PROFILE`, `ARM_*`, `GOOGLE_CLOUD_PROJECT`, `TF_VAR_kubelogin_mode` | emitted | emitted |
| `KUBECONFIG`, `TALOSCONFIG` | emitted | emitted |
| `DOCKER_HOST`, `DOCKER_CONFIG` | emitted | not emitted (no workstation in global mode) |

The principle: variables that describe **which** account/cluster/project the context targets flow in both modes. Variables that would redirect tools to context-local **credential or config files** flow only in project mode.

## Sample output

A fresh `windsor init local` in a project directory:

```
$ windsor env
DOCKER_CONFIG=/Users/me/.config/windsor/docker
DOCKER_HOST=unix:///Users/me/.docker/run/docker.sock
FLUX_SYSTEM_NAMESPACE=system-gitops
K8S_AUTH_KUBECONFIG=/path/to/project/contexts/local/.kube/config
KUBECONFIG=/path/to/project/contexts/local/.kube/config
KUBE_CONFIG_PATH=/path/to/project/contexts/local/.kube/config
TALOSCONFIG=/path/to/project/contexts/local/.talos/config
WINDSOR_CONTEXT=local
WINDSOR_CONTEXT_ID=wrk3va8i
WINDSOR_MANAGED_ALIAS=
WINDSOR_MANAGED_ENV=DOCKER_HOST,DOCKER_CONFIG,KUBECONFIG,KUBE_CONFIG_PATH,K8S_AUTH_KUBECONFIG,FLUX_SYSTEM_NAMESPACE,TALOSCONFIG,WINDSOR_CONTEXT,WINDSOR_CONTEXT_ID,WINDSOR_PROJECT_ROOT,WINDSOR_SESSION_TOKEN,WINDSOR_MANAGED_ENV,WINDSOR_MANAGED_ALIAS
WINDSOR_PROJECT_ROOT=/path/to/project
WINDSOR_SESSION_TOKEN=ldC26Dp
```

Cloud-provider variables appear when the corresponding config block is present. Terraform variables appear when the current directory is inside a generated Terraform module shim under `.windsor/contexts/<name>/terraform/<component>/`.

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

AWS integration activates when the context has `platform: aws` or any `aws:` block. Configuration in `contexts/<name>/values.yaml`:

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

1. `azure.kubelogin_mode` if set in config (only path that handles managed identity, since MI has no env signal).
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

When the current directory is inside a generated Terraform module shim, Windsor adds:

- `TF_DATA_DIR` and `TF_CLI_ARGS_*` (init/plan/apply/refresh/import/destroy) targeting the right backend, plan file, and tfvars.
- `TF_VAR_context`, `TF_VAR_context_id`, `TF_VAR_context_path`, `TF_VAR_project_root`, `TF_VAR_os_type`, `TF_VAR_operation`.
- `TF_VAR_<input>` for each evaluated component input.

See [Terraform](terraform.md) for the full lifecycle.

## See also

- [`windsor env`](../reference/commands/env.md), [`windsor hook`](../reference/commands/hook.md)
- [Trusted Folders](../security/trusted-folders.md) — how Windsor decides a project is safe
- [Local Workstation](local-workstation.md) — `workstation.runtime` and Docker socket selection
