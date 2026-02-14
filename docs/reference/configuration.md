---
title: "Configuration"
description: "Configuration settings for the Windsor workstation using the windsor.yaml file."
---
# Configuration
A `windsor.yaml` file should exist at the base of your project, and will be created for you on running `windsor init`.

The majority of options available in the Windsor config relate to the local cloud setup. Additionally, each context configured in `windsor.yaml` allows for configuring details about cloud service providers or Kubernetes cluster drivers.

The configuration details are outlined as follows.

```yaml
version: v1alpha1
contexts: #...
```

| Field     | Type                   | Description                           |
|-----------|------------------------|---------------------------------------|
| `version` | `string`               | Specifies the configuration version.  |
| `contexts`| `map[string]Context`   | A map of Context configurations.      |

## Context
The context sections configure details related to each context. These configurations include cloud service providers, Kubernetes cluster drivers, and a variety of configurations involving the local cloud virtualization. Further details about these sub-configurations follow.

### Default Values

Windsor applies default values for various configuration options when they are not explicitly specified:

- **Cluster**: `cluster.enabled` defaults to `true` for all contexts. This ensures that cluster resources are available by default.
- **Terraform**: `terraform.enabled` defaults to `true` with a `local` backend.
- **Docker**: Docker is enabled by default with common registry configurations.
- **Provider**: Defaults to `"none"` for non-dev contexts, `"docker"` for dev contexts (or `"incus"` when using colima-incus vm-driver).

These defaults ensure that basic functionality is available without requiring explicit configuration. You can override any default by explicitly setting the value in your `windsor.yaml` file.

### AWS
Configuration details specific to the AWS cloud provider. Additionally, configures a Localstack service to simulate AWS resources locally.

```yaml
aws:
  enabled: true
  aws_endpoint_url: http://aws.test:4566
  aws_profile: local
  localstack: #...
```

| Field            | Type     | Description                                                                 |
|------------------|----------|-----------------------------------------------------------------------------|
| `enabled`        | `bool`   | Indicates whether AWS integration is enabled.                               |
| `aws_endpoint_url` | `string` | Specifies the custom endpoint URL for AWS services.                        |
| `aws_profile`    | `string` | Defines the AWS CLI profile to use for authentication.                      |
| `s3_hostname`    | `string` | Sets the custom hostname for the S3 service.                                |
| `mwaa_endpoint`  | `string` | Specifies the endpoint for Managed Workflows for Apache Airflow.            |
| `localstack`     | `LocalstackConfig` | Contains the configuration for Localstack, a local AWS cloud emulator. |

#### Localstack
Configures details specific to the Localstack service container. This service is available at `aws.test:4566`.

```yaml
aws:
  localstack:
    enabled: true
    services:
      - iam
      - kms
      - s3
      - dynamodb
```

| Field     | Type       | Description                                                      |
|-----------|------------|------------------------------------------------------------------|
| `enabled` | `bool`     | Indicates whether Localstack is enabled to emulate AWS services.|
| `services`| `[]string` | Lists the AWS services to be emulated by Localstack. For more details, see [Localstack AWS Feature Coverage](https://docs.localstack.cloud/user-guide/aws/feature-coverage/). |

### Cluster
Configures details specific to the local Kubernetes cluster. These nodes are available at `worker-{1..n}.test` and `controlplane-{1..n}.test`.

```yaml
cluster:
  enabled: true
  driver: talos
  controlplanes:
    count: 1
    cpu: 2
    memory: 2
  workers:
    count: 1
    cpu: 4
    memory: 4
    hostports:
      - 80:30080/tcp
      - 443:30443/tcp
      - 9292:30292/tcp
      - 8053:30053/udp
    volumes:
      - ${project_root}/.volumes:/var/mnt/local
```

Use `${project_root}` for volume sources; it is templated into Terraform and, when using the internal workstation (docker-compose), rewritten to `${WINDSOR_PROJECT_ROOT}` at compose generation time.

| Field                        | Type       | Description                                                        |
|------------------------------|------------|--------------------------------------------------------------------|
| `enabled`                    | `bool`     | Specifies whether the cluster is active.                           |
| `driver`                     | `string`   | Specifies the cluster driver. Currently, only 'talos' is supported.|
| `controlplanes`              | `struct`   | Configuration for control plane nodes.                             |
| `controlplanes.count`        | `int`      | Number of control plane nodes.                                     |
| `controlplanes.cpu`          | `int`      | CPU resources per control plane node.                              |
| `controlplanes.memory`       | `int`      | Memory resources per control plane node.                           |
| `controlplanes.hostports`    | `[]string` | Nodeports to forward to the host machine.                          |
| `controlplanes.volumes`      | `[]string` | Volume maps for mounting node volumes onto the host.               |
| `workers`                    | `struct`   | Configuration for worker nodes.                                    |
| `workers.count`              | `int`      | Number of worker nodes.                                            |
| `workers.cpu`                | `int`      | CPU resources per worker node.                                     |
| `workers.memory`             | `int`      | Memory resources per worker node.                                  |
| `workers.hostports`          | `[]string` | Nodeports to forward to the host machine.                          |
| `workers.volumes`            | `[]string` | Volume maps for mounting node volumes onto the host.               |

### Network
Configures details related to local networking. This includes both the CIDR block range used by the network, as well as the range of IPs to be used when acquiring load balancer IP addresses.

```yaml
network:
  cidr_block: 10.5.0.0/16
  loadbalancer_ips:
    start: 10.5.1.1
    end: 10.5.1.10
```

### DNS
Configures details related to the local DNS service. The service is available at `dns.test:53`. Presently, the local DNS server runs CoreDNS.

```yaml
dns:
  enabled: true
  domain: test
  records:
    - 127.0.0.1 flux-webhook.test
```

| Field     | Type       | Description                                                |
|-----------|------------|------------------------------------------------------------|
| `enabled` | `bool`     | Specifies if the DNS service is active.                    |
| `domain`  | `string`   | Defines the domain name used by the DNS service.           |
| `address` | `string`   | Custom address for the DNS service, overriding the default.|
| `records` | `[]string` | Additional DNS records to include in the Corefile.         |

### Docker
Configures details related to using Docker locally.

```yaml
docker:
  enabled: true
  registries: #...
```

| Field          | Type                       | Description                                                                 |
|----------------|----------------------------|-----------------------------------------------------------------------------|
| `enabled`      | `bool`                     | Indicates whether the Docker service is enabled.                            |
| `registries`   | `map[string]RegistryConfig`| Configuration for Docker registries, mapping registry names to their config.|

#### RegistryConfig
Configures details related to local Docker registries and registry mirrors.

```yaml
docker:
  registries:
    # Mirrors ghcr.io locally as ghcr.io
    ghcr.io:
      remote: https://ghcr.io

    # Mirrors registry-1.docker.io as docker.io locally
    registry-1.docker.io:
      remote: https://registry-1.docker.io
      local: https://docker.io

    # A generic local registry used while developing
    registry.test: {}
```

| Field      | Type     | Description                                      |
|------------|----------|--------------------------------------------------|
| `remote`   | `string` | URL of the remote registry to mirror.            |
| `local`    | `string` | Local URL where the registry is mirrored.        |
| `hostname` | `string` | Hostname used for accessing the registry.        |

### Git

#### Livereload
Configures details related to the local git livereload server

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
    image: ghcr.io/windsorcli/git-livereload-server:v0.2.1
```

| Field          | Type     | Description                                                  |
|----------------|----------|--------------------------------------------------------------|
| `enabled`      | `bool`   | Indicates whether the livereload feature is enabled.         |
| `rsync_include`| `string` | Comma-separated list of patterns to include in rsync. When set, only files matching these patterns will be synchronized. |
| `rsync_exclude`| `string` | Comma-separated list of patterns to exclude from rsync.      |
| `rsync_protect`| `string` | Specifies files or directories to protect during rsync.      |
| `username`     | `string` | Username for authentication with the livereload server.      |
| `password`     | `string` | Password for authentication with the livereload server.      |
| `webhook_url`  | `string` | URL for the webhook to trigger livereload actions.           |
| `verify_ssl`   | `bool`   | Determines if SSL verification is required for connections.  |
| `image`        | `string` | Docker image used for the livereload server.                 |

### Environment Variables

You can specify environment variables in your `windsor.yaml` configuration under the `environment` key. These variables are custom key-value pairs that the CLI makes available at runtime using a custom environment printer.

**Example:**

```yaml
environment:
  API_KEY: {% raw %}${{ op.my.api-key }}{% endraw %}
  DEBUG: "true"
  CUSTOM_VAR: "some-value"
```

You can also reference secrets, outlined in the next section.

### Secrets

The Windsor CLI supports integrating multiple secrets providers, enabling secure management of sensitive values in your project configurations. Within a context in your windsor.yaml file, you can define a secrets block to configure providers such as 1Password and SOPS.

#### 1Password CLI

Configure 1Password secrets by adding a onepassword block under the secrets key. For example:

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

| Field          | Type   | Description                                         |
|----------------|--------|-----------------------------------------------------|
| onepassword    | object | Configuration for the 1Password secrets provider.   |

**onepassword.vaults.&lt;key&gt;**

| Field       | Type   | Description                                           |
|-------------|--------|-------------------------------------------------------|
| url         | string | The endpoint URL for the 1Password service.           |
| name        | string | The name of the vault from which secrets are retrieved. |

## Terraform
Configures details related to working with Terraform in the context

```
terraform:
  enabled: true
  backend: s3
```

| Field    | Type     | Description                                      |
|----------|----------|--------------------------------------------------|
| `enabled`| `bool`   | Indicates whether the Terraform feature is enabled. |
| `backend`| `string` | Specifies the backend type used for Terraform state management. |

## VM
Configures details related to configuring the local cloud virtualization.

```
vm:
  driver: colima
  cpu: 8
  disk: 60
  memory: 8
```

| Field    | Type     | Description                                                                 |
|----------|----------|-----------------------------------------------------------------------------|
| `driver` | `string` | Specifies the virtualization driver to use. Options include "colima", "docker-desktop", or "docker". |
| `cpu`    | `int`    | Number of CPU cores allocated to the VM. Defaults to half of the system's CPU cores. |
| `disk`   | `int`    | Disk space allocated to the VM in GB. Defaults to half of the system's memory. |
| `memory` | `int`    | Memory allocated to the VM in GB. Defaults to 60GB. |

## Example: Local Context
This is the default local `windsor.yaml` file created when running `windsor init local`:

```
version: v1alpha1
contexts:
  local:
    docker:
      enabled: true
      registries:
        gcr.io:
          remote: https://gcr.io
        ghcr.io:
          remote: https://ghcr.io
        quay.io:
          remote: https://quay.io
        registry-1.docker.io:
          remote: https://registry-1.docker.io
          local: https://docker.io
        registry.k8s.io:
          remote: https://registry.k8s.io
        registry.test: {}
    git:
      livereload:
        enabled: true
        rsync_include: ""
        rsync_exclude: .windsor,.terraform,data,.volumes,.venv
        rsync_protect: flux-system
        username: local
        password: local
        webhook_url: http://worker-1.test:30292/hook/5dc88e45e809fb0872b749c0969067e2c1fd142e17aed07573fad20553cc0c59
        verify_ssl: false
        image: ghcr.io/windsorcli/git-livereload:v0.1.1
    terraform:
      enabled: true
      backend: local
    vm:
      driver: docker-desktop
    cluster:
      enabled: true
      driver: talos
      controlplanes:
        count: 1
        cpu: 2
        memory: 2
      workers:
        count: 1
        cpu: 4
        memory: 4
        hostports:
        - 80:30080/tcp
        - 443:30443/tcp
        - 9292:30292/tcp
        - 8053:30053/udp
        volumes:
        - ${project_root}/.volumes:/var/mnt/local
    network:
      cidr_block: 10.5.0.0/16
    dns:
      enabled: true
      domain: test
      forward:
      - 10.5.0.1:8053
```

<div>
  {{ footer('Blueprint', '../../reference/blueprint/index.html', 'Contexts', '../../reference/contexts/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../reference/blueprint/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../reference/contexts/index.html'; 
  });
</script>
