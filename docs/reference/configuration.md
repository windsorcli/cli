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
| `contexts` | `map<object>` | Map of context name to per-context configuration. Most projects have 'local' (workstation context) plus one or more deployment contexts (staging, prod, etc.). **(required)** |
| `terraform` | `object` | Root-level Terraform settings shared across every context. |
| `toolsManager` | `string` | Name of the tool manager whose configuration governs binary versions for this project. Currently the only supported value is 'aqua'. |

## contexts{}

| Field | Type | Description |
|------|------|-------------|
| `aws` | `object` | AWS integration. Activates whenever this block is present (or when platform is 'aws'); there is no separate 'enabled' flag. |
| `azure` | `object` | Azure integration. |
| `cluster` | `object` | Kubernetes cluster configuration. Node-group sub-types (NodeGroupConfig) are authored in api/v1alpha1/cluster/cluster_config.go; expansion to full field detail is a planned follow-up. |
| `dns` | `object` | DNS configuration. |
| `docker` | `object` | Docker / container-registry configuration. |
| `environment` | `map<string>` | Static environment variables exported into every command run in this context. Plain string-to-string map; no expression evaluation. |
| `gcp` | `object` | GCP integration. |
| `git` | `object` | Git / livereload configuration. |
| `id` | `string` | Stable identifier for the context, distinct from its map key. Used for cross-context references where the key may change. |
| `network` | `object` | Cluster network configuration. |
| `platform` | `string` | Target deployment platform. Selects platform-specific facets and drives backend type inference. One of: `none`, `docker`, `incus`, `metal`, `aws`, `azure`, `gcp`, `hyperv`. |
| `provider` | `string` | Deprecated alias for 'platform'. New configs should use 'platform'; the loader still reads 'provider' for backwards compatibility. |
| `secrets` | `object` | Secrets provider configuration. Currently 1Password is the only supported provider. |
| `terraform` | `object` | Per-context Terraform settings (state backend, lock policy, timeout). The runtime-validator sub-types (BackendConfig, LockConfig) are authored in api/v1alpha1/terraform/terraform_config.go; expansion to full field detail is a planned follow-up. |
| `vm` | `object` | Workstation VM settings. Applies to colima / colima-incus / docker- desktop driver choices; ignored when the workstation runs directly on Docker without a VM. |

### contexts{}.aws

| Field | Type | Description |
|------|------|-------------|
| `endpoint_url` | `string` | Custom AWS endpoint URL (e.g. when targeting Localstack). |
| `localstack` | `object` | Configuration for Localstack, a local AWS cloud emulator. |
| `mwaa_endpoint` | `string` | Endpoint for Managed Workflows for Apache Airflow (MWAA). |
| `profile` | `string` | AWS CLI profile to use for authentication. |
| `region` | `string` | AWS region. Exported to downstream tools as AWS_REGION. |
| `s3_hostname` | `string` | Custom hostname for the S3 service. |

#### contexts{}.aws.localstack

| Field | Type | Description |
|------|------|-------------|
| `enabled` | `boolean` | Whether to enable Localstack for this context. |
| `services` | `array<string>` | AWS services Localstack should emulate. |

### contexts{}.azure

| Field | Type | Description |
|------|------|-------------|
| `environment` | `string` | Azure environment name (AzurePublicCloud, AzureChinaCloud, etc.). |
| `kubelogin_mode` | `string` | kubelogin auth mode (e.g. 'azurecli', 'workloadidentity'). |
| `subscription_id` | `string` | Azure subscription ID for API calls. |
| `tenant_id` | `string` | Azure tenant ID for authentication. |

### contexts{}.cluster

| Field | Type | Description |
|------|------|-------------|
| `controlplanes` | `object` | Controlplane node group settings (count, cpu, memory, image, per- node overrides). See api/v1alpha1/cluster/cluster_config.go for full NodeGroupConfig fields. |
| `driver` | `string` | Cluster driver name (e.g. 'talos'). |
| `enabled` | `boolean` | Whether the cluster integration is active for this context. |
| `endpoint` | `string` | Kubernetes API endpoint URL. |
| `image` | `string` | Default node image (typically a Talos image reference). |
| `workers` | `object` | Worker node group settings. Same shape as controlplanes; see api/v1alpha1/cluster/cluster_config.go. |

### contexts{}.dns

| Field | Type | Description |
|------|------|-------------|
| `address` | `string` | DNS server address that downstream services should resolve against. |
| `domain` | `string` | Primary DNS domain for the context (e.g. 'example.test'). |
| `forward` | `array<string>` | Upstream DNS servers the cluster's CoreDNS forwards unresolved queries to. |
| `records` | `array<string>` | Static DNS records to register in the cluster's CoreDNS. |

### contexts{}.docker

| Field | Type | Description |
|------|------|-------------|
| `enabled` | `boolean` | Whether to start Docker-managed services (DNS, git livereload, registry proxies) on workstation up. |
| `registries` | `map<object>` | Map of registry alias to local-proxy configuration. The runtime creates a registry proxy container per entry when the workstation is enabled. |
| `registry_url` | `string` | Primary container registry URL. |

#### contexts{}.docker.registries{}

| Field | Type | Description |
|------|------|-------------|
| `hostname` | `string` | Local hostname for the proxy container. |
| `hostport` | `integer` | Local port for the proxy container. |
| `local` | `string` | Local proxy URL. |
| `remote` | `string` | Remote (upstream) registry URL. |

### contexts{}.gcp

| Field | Type | Description |
|------|------|-------------|
| `credentials_path` | `string` | Filesystem path to a GCP service-account credentials JSON file. |
| `enabled` | `boolean` | Whether to activate the GCP integration. |
| `project_id` | `string` | GCP project ID for API calls. |
| `quota_project` | `string` | Project to bill quota usage against. |

### contexts{}.git

| Field | Type | Description |
|------|------|-------------|
| `livereload` | `object` | Git livereload service settings. When 'enabled', the workstation runs a container that mirrors local changes into the cluster's gitops source for fast iteration. |

#### contexts{}.git.livereload

| Field | Type | Description |
|------|------|-------------|
| `enabled` | `boolean` | Whether to run the git livereload container. |
| `image` | `string` | Container image used for the livereload service. |
| `password` | `string` | Basic-auth password for the livereload endpoint. |
| `rsync_exclude` | `string` | Rsync exclude pattern. |
| `rsync_include` | `string` | Rsync include pattern for the mirrored directory. |
| `rsync_protect` | `string` | Rsync protect pattern (paths preserved during sync). |
| `username` | `string` | Basic-auth username for the livereload endpoint. |
| `verify_ssl` | `boolean` | Whether to verify TLS certificates on the webhook URL. |
| `webhook_url` | `string` | Webhook URL the livereload container pings on commit. |

### contexts{}.network

| Field | Type | Description |
|------|------|-------------|
| `cidr_block` | `string` | CIDR block for the cluster network. |
| `loadbalancer_ips` | `object` | IP address range to reserve for load-balancer services. |

#### contexts{}.network.loadbalancer_ips

| Field | Type | Description |
|------|------|-------------|
| `end` | `string` | Last IP in the range (inclusive). |
| `start` | `string` | First IP in the range (inclusive). |

### contexts{}.secrets

| Field | Type | Description |
|------|------|-------------|
| `onepassword` | `object` |  |

#### contexts{}.secrets.onepassword

| Field | Type | Description |
|------|------|-------------|
| `vaults` | `map<object>` | Map of vault alias to vault configuration. The alias is used in 'op://<alias>/...' references throughout other configs. |

#### contexts{}.secrets.onepassword.vaults{}

| Field | Type | Description |
|------|------|-------------|
| `id` | `string` | 1Password vault ID. |
| `name` | `string` | Human-readable vault name (defaults to the map key). |
| `url` | `string` | 1Password instance URL. |

### contexts{}.terraform

| Field | Type | Description |
|------|------|-------------|
| `backend` | `object` | State backend configuration (type plus per-type fields). See api/v1alpha1/terraform/terraform_config.go for the full BackendConfig field set (s3, azurerm, kubernetes, local, oss). |
| `enabled` | `boolean` | Whether terraform components are applied for this context. |
| `lock` | `object` | State-lock policy (timeout). See api/v1alpha1/terraform/terraform_config.go for LockConfig fields. |

### contexts{}.vm

| Field | Type | Description |
|------|------|-------------|
| `address` | `string` | VM address used for SSH and workstation networking. |
| `arch` | `string` | CPU architecture for the VM. One of: `amd64`, `arm64`. |
| `cpu` | `integer` | Virtual CPU count. |
| `disk` | `integer` | Disk size in GB. |
| `driver` | `string` | VM driver. One of 'colima', 'colima-incus', 'docker-desktop', 'docker'. |
| `memory` | `integer` | Memory in GB. |
| `runtime` | `string` | Container runtime inside the VM (typically 'docker'). |

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
        s3:
          bucket: my-org-tf-state
          region: us-east-1
        type: s3
      enabled: true
```

## See also

- [Contexts reference](/reference/cli/contexts) — context layout, on-disk files, and lifecycle
- [Blueprint reference](/reference/cli/blueprint), [Facets reference](/reference/cli/facets)
- [`init`](/reference/cli/commands/init), [`show values`](/reference/cli/commands/show-values), [`get contexts`](/reference/cli/commands/get-contexts)
- [Lifecycle guide](/contexts/lifecycle), [Contexts guide](/contexts/overview)
- Source schema: [pkg/runtime/config/schemas/artifacts/configuration.yaml](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schemas/artifacts/configuration.yaml)
