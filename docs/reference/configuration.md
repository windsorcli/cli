---
title: "Configuration"
description: "Settings that the Windsor CLI reads from a context, in the v1alpha2 schema."
---

Settings that the Windsor CLI reads from a context, in the v1alpha2 schema.
Each context keeps its settings in `contexts/<name>/values.yaml`. The
blueprint schema in `contexts/_template/schema.yaml` can add more keys. The
CLI passes those keys to the blueprint. A blueprint schema can declare a key
from this page, but the key MUST keep the type that this page gives.

The root `windsor.yaml` holds project-wide settings and no per-context
values. A v1alpha2 root file MUST hold only these keys:

| Key | Type | Description |
|------|------|-------------|
| `version` | `string` | Config schema version. Set it to `v1alpha2`. |
| `environment` | `map<string>` | Environment variables that the CLI exports in every context. A key with the same name in the `environment` of a context wins. |
| `paths.base` | `string` | Directory that holds `contexts/`, relative to the project root. The default is the project root. The path MUST be relative. It MUST NOT contain `..` and MUST NOT be inside `.windsor`. |

A v1alpha2 root file MUST NOT sit beside a `contexts/<name>/windsor.yaml`.
Run `windsor migrate` to convert a v1alpha1 project, which keeps
per-context settings in `windsor.yaml` files.

```yaml
version: v1alpha2
environment:
  TF_LOG: WARN
```

## Fields

| Field | Type | Description |
|------|------|-------------|
| `aws` | `object` | AWS settings that the CLI exports and checks. |
| `azure` | `object` | Azure settings that the CLI exports and checks. |
| `cluster` | `object` | Kubernetes cluster settings. |
| `dev` | `boolean` | Treat the context as a local development context. When the key is unset, the CLI treats a context named 'local' or 'local-<name>' as a development context. |
| `dns` | `object` | DNS settings. |
| `docker` | `object` | Docker settings. |
| `environment` | `map<string>` | Environment variables that the CLI exports for the context. Values can contain `${...}` expressions, including `secret(...)` references. Keep local, git-ignored values in [`.env` files](contexts.md#env-files). |
| `gcp` | `object` | GCP settings that the CLI exports. |
| `gitops` | `object` | GitOps settings. |
| `id` | `string` | Stable identifier for the context, distinct from its directory name. The CLI generates one when it is empty. The ID starts with a lowercase letter and contains lowercase letters, digits, and single hyphens. It has 1 to 63 characters, the DNS label limit. |
| `network` | `object` | Cluster network settings. |
| `platform` | `string` | Platform the context deploys to. The platform selects the platform facets of the blueprint. When terraform.backend.type is unset, the platform sets the default backend: aws uses s3, azure uses azurerm, gcp uses gcs, none uses local, and every other platform uses kubernetes. One of: `none`, `docker`, `incus`, `metal`, `hetzner`, `aws`, `azure`, `gcp`, `hyperv`, `vsphere`, `proxmox`. |
| `registries` | `map<object>` | Registries, keyed by upstream registry host. The key '*' applies to each host with no entry. An entry with endpoint is a mirror: cluster nodes pull the images of its host from the mirror, and the CLI and Flux pull the oci:// blueprint sources of its host from the mirror. |
| `secrets` | `object` | Secret providers. The CLI supports SOPS and 1Password. |
| `terraform` | `object` | Terraform settings of the context. |
| `vsphere` | `object` | vCenter connection settings that the CLI exports. The password MUST come from secrets or from the environment, never from this file. |
| `workstation` | `object` | Local workstation settings. |

## aws

| Field | Type | Description |
|------|------|-------------|
| `endpoint_url` | `string` | Custom AWS endpoint URL. |
| `mwaa_endpoint` | `string` | Endpoint for Managed Workflows for Apache Airflow (MWAA). |
| `profile` | `string` | AWS CLI profile for authentication. |
| `region` | `string` | AWS region. The CLI exports it as AWS_REGION. |
| `s3_hostname` | `string` | Custom hostname for the S3 service. |

## azure

| Field | Type | Description |
|------|------|-------------|
| `environment` | `string` | Azure cloud name, such as AzurePublicCloud or AzureChinaCloud. |
| `kubelogin_mode` | `string` | kubelogin authentication mode, such as 'azurecli' or 'workloadidentity'. |
| `region` | `string` | Azure region. The CLI exports it as TF_VAR_region. |
| `subscription_id` | `string` | Azure subscription ID. |
| `tenant_id` | `string` | Azure tenant ID. |

## cluster

| Field | Type | Description |
|------|------|-------------|
| `driver` | `string` | Cluster driver, such as 'talos'. |

## dns

| Field | Type | Description |
|------|------|-------------|
| `domain` | `string` | Primary DNS domain of the context, such as 'example.test'. |
| `enabled` | `boolean` | Turn DNS on or off for the context. When it is not false and a workstation runtime is set, the default domain is 'test'. |

## docker

| Field | Type | Description |
|------|------|-------------|
| `enabled` | `boolean` | Start the Docker-managed workstation services. |
| `registries` | `map<object>` | Deprecated location of the registries map. The CLI reads it when the context does not set the top-level registries key, and gives a warning. Use the top-level registries key. |
| `registry_url` | `string` | Primary container registry URL. |

### docker.registries{}

| Field | Type | Description |
|------|------|-------------|
| `endpoint` | `string` |  |
| `hostport` | `integer` |  |
| `local` | `string` |  |
| `remote` | `string` |  |

## gcp

| Field | Type | Description |
|------|------|-------------|
| `credentials_path` | `string` | Path to a GCP service-account credentials JSON file. |
| `project_id` | `string` | GCP project ID. |
| `quota_project` | `string` | Project that the quota usage bills to. |

## gitops

| Field | Type | Description |
|------|------|-------------|
| `mode` | `string` | GitOps mode. The CLI uses pull for an empty or unknown value. One of: `pull`, `push`. |
| `namespace` | `string` | Namespace of the GitOps resources. The default is 'system-gitops'. |

## network

| Field | Type | Description |
|------|------|-------------|
| `cidr_block` | `string` | CIDR block of the cluster network. |
| `loadbalancer_ips` | `object` | IP address range for load-balancer services. |

### network.loadbalancer_ips

| Field | Type | Description |
|------|------|-------------|
| `end` | `string` | Last IP address of the range, included. |
| `start` | `string` | First IP address of the range, included. |

## registries{}

| Field | Type | Description |
|------|------|-------------|
| `endpoint` | `string` | Mirror of the upstream host, as an optional http or https scheme, a host, an optional port, and an optional path. The default scheme is https. |
| `hostport` | `integer` | Host port of a local registry. |
| `local` | `string` | Upstream host that the entry applies to. The default is the key. |
| `remote` | `string` | Upstream registry URL. A registry without one is a local registry. |

## secrets

| Field | Type | Description |
|------|------|-------------|
| `onepassword` | `object` |  |
| `sops` | `object` |  |

### secrets.onepassword

| Field | Type | Description |
|------|------|-------------|
| `vaults` | `map<object>` | 1Password vaults, keyed by the alias that 'op://<alias>/...' references use. |

#### secrets.onepassword.vaults{}

| Field | Type | Description |
|------|------|-------------|
| `id` | `string` | 1Password vault ID. |
| `name` | `string` | Vault name. The default is the map key. |
| `url` | `string` | 1Password instance URL. |

### secrets.sops

| Field | Type | Description |
|------|------|-------------|
| `enabled` | `boolean` | Decrypt SOPS-encrypted secrets for the context. The CLI then resolves 'sops.<key>' references from the 'secrets*.yaml' files of the context. |

## terraform

| Field | Type | Description |
|------|------|-------------|
| `backend` | `object` | State backend. Windsor passes each field of the selected backend block to terraform init. When the type changes on a context that has state, windsor plan names the move. windsor apply, up, and bootstrap then move the state of each component and verify each copy. |
| `enabled` | `boolean` | Apply the terraform components of the blueprint for this context. |
| `lock` | `object` |  |

### terraform.backend

| Field | Type | Description |
|------|------|-------------|
| `auto_prefix` | `boolean` | Add <context>-<id>/ after prefix in the state key, so contexts that share a backend keep separate state. |
| `azurerm` | `object` |  |
| `gcs` | `object` |  |
| `kubernetes` | `object` |  |
| `prefix` | `string` | Prefix of the state key of each component. |
| `s3` | `object` |  |
| `type` | `string` | Backend type, such as s3, azurerm, gcs, or kubernetes. |

#### terraform.backend.azurerm

| Field | Type | Description |
|------|------|-------------|
| `container_name` | `string` |  |
| `storage_account_name` | `string` |  |

#### terraform.backend.gcs

| Field | Type | Description |
|------|------|-------------|
| `bucket` | `string` |  |

#### terraform.backend.s3

| Field | Type | Description |
|------|------|-------------|
| `bucket` | `string` |  |

### terraform.lock

| Field | Type | Description |
|------|------|-------------|
| `timeout` | `string` | How long terraform waits for its own state lock, as a Go duration such as '30s' or '5m'. The default is '5m'. Windsor passes it as -lock-timeout to each terraform subcommand that touches state. For the Windsor stack lock, see the global --lock-timeout flag. |

## vsphere

| Field | Type | Description |
|------|------|-------------|
| `insecure` | `boolean` | Do not verify the TLS certificate of vCenter. The CLI exports it as VSPHERE_ALLOW_UNVERIFIED_SSL. |
| `server` | `string` | vCenter server hostname or IP address. The CLI exports it as VSPHERE_SERVER. |
| `user` | `string` | vCenter username. The CLI exports it as VSPHERE_USER. |

## workstation

| Field | Type | Description |
|------|------|-------------|
| `address` | `string` | Workstation address. The CLI uses it for SSH to configure workstation networking. |
| `arch` | `string` | CPU architecture. The default is the host architecture. One of: `amd64`, `arm64`. |
| `dns` | `object` |  |
| `runtime` | `string` | Host runtime for local clusters. colima and docker-desktop run a VM. docker uses the Docker engine directly. One of: `colima`, `docker-desktop`, `docker`. |

### workstation.dns

| Field | Type | Description |
|------|------|-------------|
| `address` | `string` | DNS server address that the workstation infrastructure supplies. |

## Examples

```yaml
platform: aws
aws:
  profile: staging
  region: us-east-1
cluster:
  driver: talos
terraform:
  backend:
    s3:
      bucket: my-org-tf-state
    type: s3
  enabled: true
```

## See also

- [Contexts reference](contexts.md) — context layout, on-disk files, and lifecycle
- [Blueprint reference](blueprint.md), [Facets reference](facets.md)
- [`init`](commands/init.md), [`migrate`](commands/migrate.md), [`show values`](commands/show-values.md), [`get contexts`](commands/get-contexts.md)
- [Lifecycle guide](https://www.windsorcli.dev/contexts/lifecycle), [Contexts guide](https://www.windsorcli.dev/contexts/overview)
- Source schema: [pkg/runtime/config/schemas/artifacts/configuration.yaml](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schemas/artifacts/configuration.yaml)
