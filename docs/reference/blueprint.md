---
title: "Blueprint"
description: "Windsor blueprints reference collections of Kustomize and Terraform resources"
---
# Blueprint
The blueprint stores references and configuration specific to a context. It's configured in a `blueprint.yaml` file located in your context's configuration folder, such as `contexts/local/blueprint.yaml`.

When you run `windsor init local`, a default local blueprint file is created. The sections in this file are outlined below.

```yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata: # ...
repository: #...
sources: #...
terraform: #...
kustomize: #...
```

| Field       | Type                   | Description                                                                 |
|-------------|------------------------|-----------------------------------------------------------------------------|
| `kind`      | `string`               | Specifies the blueprint type, adhering to Kubernetes conventions.           |
| `apiVersion`| `string`               | Indicates the API schema version of the blueprint.                          |
| `metadata`  | `Metadata`             | Contains core information about the blueprint, such as identity and authors.|
| `repository`| `Repository`           | Provides details about the source repository of the blueprint.              |
| `sources`   | `[]Source`             | Lists external resources referenced by the blueprint.                       |
| `terraform` | `[]TerraformComponent` | Includes Terraform modules within the blueprint.                            |
| `kustomize` | `[]Kustomization`      | Contains Kustomization configurations in the blueprint.                     |

### Metadata
Core information about the blueprint, including its identity and authors.

```yaml
metadata:
  name: local
  description: Builds a local cloud environment
  authors:
    - "@rmvangun"
    - "@tvangundy"
```

| Field         | Type     | Description                                      |
|---------------|----------|--------------------------------------------------|
| `name`        | `string` | The blueprint's unique identifier.               |
| `description` | `string` | A brief overview of the blueprint.               |
| `authors`     | `[]string` | Creators or maintainers of the blueprint.      |

### Repository
Details the source repository of the blueprint.

```yaml
repository:
  url: https://github.com/sample-org/blueprint
  ref:
    branch: main
  secretName: git-creds
```

| Field        | Type        | Description                                           |
|--------------|-------------|-------------------------------------------------------|
| `url`        | `string`    | The repository location.                              |
| `ref`        | `Reference` | Details the branch, tag, or commit to use.            |
| `secretName` | `string`    | The name of the k8s secret containing git credentials.|

### Source
A dependency from which Terraform and Kustomize components may be sourced

```yaml
sources:
  - name: core
    url: github.com/windsorcli/core
    ref:
      tag: v0.3.0
  - name: oci-source
    url: oci://ghcr.io/windsorcli/core:v0.3.0
    # No ref needed for OCI - version is in the URL
```

| Field        | Type       | Description                                      |
|--------------|------------|--------------------------------------------------|
| `name`       | `string`   | Identifies the source.                           |
| `url`        | `string`   | The source location. Supports Git URLs and OCI URLs (oci://registry/repo:tag). |
| `ref`        | `Reference`| Details the branch, tag, or commit to use. Not needed for OCI URLs with embedded tags. |
| `secretName` | `string`   | The secret for source access.                    |

**Note:** For OCI sources, the URL should include the tag/version directly (e.g., `oci://registry.example.com/repo:v1.0.0`). The `ref` field is optional for OCI sources when the tag is specified in the URL.

### Reference
A reference to a specific git state or version

```yaml
reference:
  branch: main
  tag: v1.0.0
  name: refs/heads/main
  commit: 1a2b3c4d5e6f7g8h9i0j
```

| Field   | Type   | Description                                      |
|---------|--------|--------------------------------------------------|
| `branch`| `string` | Branch to use.                                 |
| `tag`   | `string` | Tag to use.                                    |
| `name`  | `string` | Name of the reference.                         |
| `commit`| `string` | Commit hash to use.                            |

### TerraformComponent
A local or remote reference to a Terraform module or "component" of the blueprint.

```yaml
terraform:
  # A Terraform module defined in the "core" repository source
  - source: core
    path: cluster/talos
  
  # A Terraform module defined within the current blueprint source
  - path: apps/my-infra
```

| Field      | Type                             | Description                                      |
|------------|----------------------------------|--------------------------------------------------|
| `source`   | `string`                         | Source of the Terraform module. Must be included in the list of sources. |
| `path`     | `string`                         | Path of the Terraform module relative to the `terraform/` folder.                    |
| `values`   | `map[string]any`         | Configuration values for the module.             |
| `variables`| `map[string]TerraformVariable`   | Input variables for the module.                  |

### Kustomization
For more information on Flux Kustomizations, which are sets of resources and configurations applied to a Kubernetes cluster, visit [Flux Kustomizations Documentation](https://fluxcd.io/flux/components/kustomize/kustomizations/). Most parameters are not necessary to define.

```yaml
kustomize:
  # A reference to a csi driver from the "core" source that implements longhorn
  - name: system-csi
    source: core
    path: csi 
    components:
      - longhorn

  # A reference to a local folder containing kubernetes manifests outlining "my-app"
  - name: my-app
    dependsOn:
      - system-csi
    path: apps/my-app
```


| Field         | Type                | Description                                      |
|---------------|---------------------|--------------------------------------------------|
| `name`        | `string`            | Name of the kustomization.                       |
| `path`        | `string`            | Path of the kustomization.                       |
| `source`      | `string`            | Source of the kustomization.                     |
| `dependsOn`   | `[]string`          | Dependencies of this kustomization.              |
| `interval`    | `*metav1.Duration`  | Interval for applying the kustomization.         |
| `retryInterval`| `*metav1.Duration` | Retry interval for a failed kustomization.       |
| `timeout`     | `*metav1.Duration`  | Timeout for the kustomization to complete.       |
| `patches`     | `[]kustomize.Patch` | Patches to apply to the kustomization.           |
| `wait`        | `*bool`             | Wait for the kustomization to be fully applied.  |
| `force`       | `*bool`             | Force apply the kustomization.                   |
| `components`  | `[]string`          | Components to include in the kustomization.      |

## Cluster Variables
When running `windsor install`, Kubernetes resources are applied. These resources include a configmap that introduces [post-build variables](https://fluxcd.io/flux/components/kustomize/kustomizations/#post-build-variable-substitution) into the Kubernetes manifests. These variables are outlined as follows:

| Key                     | Description                                                        |
|-------------------------|--------------------------------------------------------------------|
| `CONTEXT`               | Specifies the context name, e.g., local.                           |
| `DOMAIN`                | The domain used for subdomain registration, e.g., test.            |
| `LOADBALANCER_IP_END`   | The final IP in the range for load balancer assignments.           |
| `LOADBALANCER_IP_RANGE` | Complete range of load balancer IPs, e.g., 10.5.1.1-10.5.1.10.     |
| `LOADBALANCER_IP_START` | The initial IP in the range for load balancer assignments.         |
| `LOCAL_VOLUME_PATH`     | Directory path for local volume storage, e.g., /var/local.         |
| `REGISTRY_URL`          | Base URL for the container image registry, e.g., registry.test.    |

## Example: Local Blueprint
When you run `windsor init local`, a default local blueprint is generated:

```yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: local
  description: This blueprint configures core for running on docker desktop
repository:
  url: http://git.test/git/core
  ref:
    branch: main
  secretName: flux-system
sources:
- name: core
  url: github.com/windsorcli/core
  ref:
    branch: main
- name: oci-source
  url: oci://ghcr.io/windsorcli/core:v0.3.0
terraform:
- path: cluster/talos
- path: gitops/flux
kustomize:
- name: policy-base
  path: policy/base
  components:
  - kyverno
- name: policy-resources
  path: policy/resources
  dependsOn:
  - policy-base
- name: csi
  path: csi
  dependsOn:
  - policy-resources
  components:
  - openebs
  - openebs/dynamic-localpv
- name: ingress
  path: ingress/base
  dependsOn:
  - pki-resources
  force: true
  components:
  - nginx
  - nginx/nodeport
  - nginx/coredns
  - nginx/flux-webhook
  - nginx/web
- name: pki-base
  path: pki/base
  dependsOn:
  - policy-resources
  force: true
  components:
  - cert-manager
  - trust-manager
- name: pki-resources
  path: pki/resources
  dependsOn:
  - pki-base
  force: true
  components:
  - private-issuer/ca
  - public-issuer/selfsigned
- name: dns
  path: dns
  dependsOn:
  - ingress
  - pki-base
  force: true
  components:
  - coredns
  - coredns/etcd
  - external-dns
  - external-dns/localhost
  - external-dns/coredns
  - external-dns/ingress
- name: gitops
  path: gitops/flux
  dependsOn:
  - ingress
  force: true
  components:
  - webhook
- name: demo
  path: demo
  dependsOn:
  - ingress
  force: true
  components:
  - bookinfo
  - bookinfo/ingress
```

<div>
  {{ footer('Terraform', '../../guides/terraform/index.html', 'Configuration', '../configuration/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../security/terraform/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../configuration/index.html'; 
  });
</script>
