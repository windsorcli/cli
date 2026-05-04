---
title: "Configuration"
description: "Reference for windsor.yaml, per-context values.yaml, and the schema fields each accepts."
---
# Configuration

Configuration spans three files:

| File | Authored by | Purpose |
|------|-------------|---------|
| `windsor.yaml` (project root) | user | Project-level config: schema version, optional `terraform.driver` (opentofu), optional `contexts:` map. |
| `contexts/<name>/values.yaml` | user | Per-context values (cluster topology, AWS/Azure/GCP, DNS, env vars, secrets). |
| `.windsor/contexts/<name>/workstation.yaml` | Windsor | Ephemeral, system-managed: `platform`, `workstation.runtime`, `workstation.arch`, `workstation.dns.address`. Do not commit. |

After `windsor init local`, the root `windsor.yaml` is just `version: v1alpha1`; everything else lives in `contexts/local/values.yaml`. The `contexts.<name>.<...>` form in `windsor.yaml` is also accepted for compatibility but is no longer the canonical layout.

## Root configuration

```yaml
# windsor.yaml
version: v1alpha1

# Optional: switch the terraform driver (defaults to terraform; experimental opentofu support)
terraform:
  driver: opentofu     # terraform | opentofu (alias: tofu)
```

| Field | Type | Description |
|-------|------|-------------|
| `version` | `string` | Schema version. Currently `v1alpha1`. |
| `toolsManager` | `string` | Optional. Override the tools manager (default: `aqua`). |
| `terraform.driver` | `string` | Optional. Override the Terraform CLI to use (`terraform` or `opentofu`). When unset, Windsor auto-detects: prefer `terraform`, fall back to `tofu`. OpenTofu support is **experimental**. |
| `contexts` | `map[string]Context` | Optional. Per-context configurations. Equivalent to authoring under `contexts/<name>/values.yaml`. |

## Context fields

Every key below can appear at the top of a per-context `values.yaml`, or under `contexts.<name>.<key>` in `windsor.yaml`.

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | Stable context id, written by Windsor on `init`. Surfaces as `WINDSOR_CONTEXT_ID`. |
| `platform` | `string` | Target platform: `none`, `metal`, `docker`, `incus`, `aws`, `azure`, `gcp`. Drives backend defaults and which terraform components apply. |
| `provider` | `string` | **Deprecated.** Use `platform` instead. |
| `dev` | `bool` | When `true` (default in `local` context), enables dev features such as `git-livereload`. |
| `environment` | `map[string]string` | Custom env vars exported into the shell — see [Environment variables](#environment-variables). |
| `secrets` | `SecretsConfig` | Secrets provider configuration — see [Secrets](#secrets). |
| `aws`, `azure`, `gcp` | per-provider | Cloud provider integration. Presence of the block (or `platform: <name>`) activates the corresponding env injection. |
| `docker` | `DockerConfig` | Docker registries for the workstation. |
| `git` | `GitConfig` | git-livereload configuration. |
| `terraform` | `TerraformConfig` | Per-context Terraform settings (backend). |
| `vm` | `VMConfig` | **Deprecated** — see [VM (deprecated)](#vm-deprecated). |
| `cluster` | `ClusterConfig` | Kubernetes cluster topology. |
| `network` | `NetworkConfig` | Local CIDR + load balancer IP range. |
| `dns` | `DNSConfig` | Local DNS service configuration. |

## Platform

`platform` selects backends, which terraform components apply, and which environment is injected.

```yaml
platform: aws       # none | metal | docker | incus | aws | azure | gcp
```

Backend defaults applied at `init` / `bootstrap` / `up` (when `terraform.backend.type` is unset):

| Platform | Default backend |
|----------|-----------------|
| `aws` | `s3` |
| `azure` | `azurerm` |
| `metal`, `docker`, `incus` | `kubernetes` |
| `gcp`, `none`, unset | not defaulted (effectively `local`) |

## AWS

AWS integration activates when the context has `platform: aws` or any `aws:` block.

```yaml
aws:
  region: us-east-2
  profile: company-prod        # default: context name
  endpoint_url: ...
  s3_hostname: ...
  mwaa_endpoint: ...
```

| Field | Type | Description |
|-------|------|-------------|
| `region` | `string` | Emitted as `AWS_REGION`. |
| `profile` | `string` | Emitted as `AWS_PROFILE`. Defaults to the context name. |
| `endpoint_url` | `string` | Custom AWS endpoint URL. Emitted as `AWS_ENDPOINT_URL`. |
| `s3_hostname` | `string` | Custom S3 hostname. |
| `mwaa_endpoint` | `string` | Endpoint for Managed Workflows for Apache Airflow. |

See [Environment](environment.md#aws) for what gets exported to your shell.

## Azure

Azure integration activates when the context has `platform: azure` or any `azure:` block.

```yaml
azure:
  subscription_id: 00000000-0000-0000-0000-000000000000
  tenant_id: 11111111-1111-1111-1111-111111111111
  environment: public           # public | usgovernment | china | german
  kubelogin_mode: azurecli      # override only when needed
```

| Field | Type | Description |
|-------|------|-------------|
| `subscription_id` | `string` | Emitted as `ARM_SUBSCRIPTION_ID`. |
| `tenant_id` | `string` | Emitted as `ARM_TENANT_ID`. |
| `environment` | `string` | Azure cloud environment. Emitted as `ARM_ENVIRONMENT`. |
| `kubelogin_mode` | `string` | Override `TF_VAR_kubelogin_mode` for AKS kubeconfigs. The only path that handles managed identity (no env signal exists for MI). |

## GCP

GCP integration activates when the context has `platform: gcp` or any `gcp:` block.

```yaml
gcp:
  project_id: my-project
  quota_project: my-quota-project
  credentials_path: /path/to/service-account.json
```

| Field | Type | Description |
|-------|------|-------------|
| `project_id` | `string` | Emitted as `GOOGLE_CLOUD_PROJECT` and `GCLOUD_PROJECT`. |
| `quota_project` | `string` | Emitted as `GOOGLE_CLOUD_QUOTA_PROJECT`. |
| `credentials_path` | `string` | Path to a service account key file. Emitted as `GOOGLE_APPLICATION_CREDENTIALS`. |
| `enabled` | `bool` | Optional explicit toggle. Presence of the block activates GCP without it. |

## Cluster

```yaml
cluster:
  driver: talos
  controlplanes:
    count: 1
    cpu: 4
    memory: 4
    schedulable: true            # auto-true when count == 1 && workers.count == 0
  workers:
    count: 1
    cpu: 4
    memory: 8
    hostports:
      - 80:30080/tcp
      - 443:30443/tcp
    volumes:
      - ${project_root}/.volumes:/var/mnt/local
```

| Field | Type | Description |
|-------|------|-------------|
| `driver` | `string` | Cluster driver. Currently only `talos` is supported. |
| `endpoint` | `string` | Override the cluster API endpoint. Defaults to a local URL when unset. |
| `image` | `string` | Override the cluster node image. |
| `controlplanes.count` | `int` | Number of control plane nodes. Default `1`. |
| `controlplanes.cpu` | `int` | CPU per controlplane. Default 8 when schedulable, 4 when dedicated. |
| `controlplanes.memory` | `int` | Memory (GB) per controlplane. Default 12 schedulable, 4 dedicated. |
| `controlplanes.schedulable` | `bool` | Whether controlplanes also run workloads. Auto-true when `count == 1 && workers.count == 0`. |
| `controlplanes.hostports` | `[]string` | Container-to-host port mappings (`docker-desktop` / `docker` only). |
| `controlplanes.volumes` | `[]string` | Bind-mounts on the node filesystem. |
| `workers.count` | `int` | Number of worker nodes. Default `0` (single-node cluster). |
| `workers.cpu` | `int` | CPU per worker. Default `4`. |
| `workers.memory` | `int` | Memory (GB) per worker. Default `8`. |
| `workers.hostports` | `[]string` | As above. |
| `workers.volumes` | `[]string` | As above. |

`${project_root}` is templated into Terraform; under Colima it resolves to the in-VM path of the project, under `docker` / `docker-desktop` to the host path.

## Network

```yaml
network:
  cidr_block: 10.5.0.0/16
  loadbalancer_ips:
    start: 10.5.1.1
    end: 10.5.1.10
```

| Field | Type | Description |
|-------|------|-------------|
| `cidr_block` | `string` | Workstation network CIDR. |
| `loadbalancer_ips.start` | `string` | First IP in the LB range. |
| `loadbalancer_ips.end` | `string` | Last IP in the LB range. |

## DNS

```yaml
dns:
  enabled: true
  domain: test
  forward:
    - 10.5.0.1:8053
  records:
    - "127.0.0.1 flux-webhook.test"
```

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | `bool` | Whether the local DNS service is active. Default `true`. |
| `domain` | `string` | DNS domain (default `test`). |
| `address` | `string` | Override the DNS service IP. |
| `forward` | `[]string` | Upstream resolvers to forward unmatched queries to. |
| `records` | `[]string` | Additional CoreDNS records. |

## Docker

Workstation registries and Docker config. The internal-workstation toggle (`docker.enabled`) is deprecated; whether a workstation is brought up is now driven by `platform` and `workstation.runtime`.

```yaml
docker:
  registries:
    ghcr.io:
      remote: https://ghcr.io
    registry-1.docker.io:
      remote: https://registry-1.docker.io
      local: https://docker.io
    registry.test: {}
```

| Field | Type | Description |
|-------|------|-------------|
| `registries` | `map[string]RegistryConfig` | Registry mirrors keyed by upstream hostname. |
| `enabled` | `bool` | **Deprecated** — no-op in v0.9. |

### RegistryConfig

| Field | Type | Description |
|-------|------|-------------|
| `remote` | `string` | URL of the upstream registry to mirror. |
| `local` | `string` | Optional local-facing URL (rare; used when local hostname differs from `remote`). |
| `hostname` | `string` | Optional hostname override. |

## Terraform

Per-context Terraform settings. The root-level `terraform.driver` is documented above.

```yaml
terraform:
  enabled: true
  backend:
    type: s3              # local | s3 | kubernetes | azurerm
    s3:
      bucket: my-tf-state
      key: contexts/staging
      region: us-east-2
```

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | `*bool` | Whether the Terraform layer is active. Default `true`. |
| `backend.type` | `string` | Backend type: `local`, `s3`, `kubernetes`, `azurerm`. Default depends on `platform` (see [Platform](#platform)). |
| `backend.local` | `LocalBackend` | Local backend overrides (e.g. `path`). |
| `backend.s3` | `S3Backend` | S3 backend config (`bucket`, `key`, `region`, optional credentials, optional `dynamodb_table`, optional `use_lockfile`, etc.). |
| `backend.kubernetes` | `KubernetesBackend` | Kubernetes backend config (`secret_suffix`, `namespace`, etc.). State per component lives in a Secret. |
| `backend.azurerm` | `AzureRMBackend` | AzureRM backend config (`storage_account_name`, `container_name`, etc.). |

Windsor writes a `backend_override.tf` next to each generated module shim under `.windsor/contexts/<name>/terraform/<component>/` so module sources don't need a hard-coded `backend` block.

## VM (deprecated)

The top-level `vm:` block is **deprecated** in v0.9 and may be removed in a future release. The keys still parse for compatibility but should not be authored in new configs:

- VM size is sized automatically from `cluster.controlplanes.cpu` / `memory` + `cluster.workers.cpu` / `memory` (see [Cluster](#cluster)).
- For one-off overrides, pass `--vm-cpu`, `--vm-memory`, `--vm-disk` to `windsor init`; Windsor records them under the system-managed `workstation.yaml`.
- The driver is set with `--vm-driver` on `init`, which writes `workstation.runtime`.

## Workstation (system-managed)

`.windsor/contexts/<name>/workstation.yaml` is **system-managed** — Windsor writes it during `init` / `up` based on `--vm-driver`, `--platform`, and the host architecture. Do not commit it.

```yaml
platform: docker
workstation:
  arch: arm64
  dns:
    address: 127.0.0.1
  runtime: docker-desktop
```

| Key | Meaning |
|-----|---------|
| `platform` | Same `platform` as in the context (re-emitted in workstation state). |
| `workstation.runtime` | `docker-desktop`, `colima`, or `docker`. |
| `workstation.arch` | VM architecture. |
| `workstation.address` | VM IP after boot. |
| `workstation.dns.address` | DNS service IP that the host resolver should be pointed at. |

## Environment variables

Custom env vars surfaced to the shell via `windsor env`:

```yaml
environment:
  API_KEY: ${{ op.my.api-key }}
  DEBUG: "true"
  CUSTOM_VAR: "some-value"
```

Values may reference [secrets](#secrets) using `${{ <provider>.<path> }}` notation.

## Secrets

```yaml
secrets:
  onepassword:
    vaults:
      personal:
        url: my.1password.com
        name: "Personal"
      development:
        url: my-company.1password.com
        name: "Development"
```

| Field | Type | Description |
|-------|------|-------------|
| `onepassword` | `OnePasswordConfig` | 1Password CLI vault configuration. |

**`onepassword.vaults.<key>`**

| Field | Type | Description |
|-------|------|-------------|
| `url` | `string` | 1Password account URL. |
| `name` | `string` | Vault name. |

SOPS does not require explicit configuration — Windsor detects `secrets.enc.yaml` files under each context and decrypts them transparently.

## Git

### Livereload

```yaml
git:
  livereload:
    enabled: true
    rsync_include: ""
    rsync_exclude: .windsor,.terraform,data,.volumes,.venv
    rsync_protect: flux-system
    username: local
    password: local
    webhook_url: http://flux-webhook.local.test
    verify_ssl: false
    image: ghcr.io/windsorcli/git-livereload:v0.2.1
```

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | `bool` | Whether to run git-livereload. |
| `rsync_include` | `string` | Comma-separated include patterns. |
| `rsync_exclude` | `string` | Comma-separated exclude patterns. |
| `rsync_protect` | `string` | Files/directories to protect during rsync. |
| `username` | `string` | HTTP-basic username for the served git endpoint. |
| `password` | `string` | HTTP-basic password. |
| `webhook_url` | `string` | URL to POST after each filesystem change (typically the Flux webhook receiver). |
| `verify_ssl` | `bool` | SSL verification for the webhook. |
| `image` | `string` | Container image for the livereload server. |

## Example: a fresh `windsor init local`

The two files Windsor creates:

```yaml
# windsor.yaml
version: v1alpha1
```

```yaml
# contexts/local/values.yaml
dev: true
dns:
  domain: test
id: w2g5rk7d
```

Plus the system-managed:

```yaml
# .windsor/contexts/local/workstation.yaml
platform: docker
workstation:
  arch: arm64
  dns:
    address: 127.0.0.1
  runtime: docker-desktop
```

Everything else (cluster topology, registries, terraform backend) takes platform-driven defaults until you override.
