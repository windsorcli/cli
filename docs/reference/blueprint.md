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
| `configMaps`| `map[string]map[string]string` | Standalone ConfigMaps to be created, not tied to specific kustomizations. These ConfigMaps are referenced by all kustomizations in PostBuild substitution. |

For information about Features, see the [Features Reference](features.md). For schema validation, see the [Schema Reference](schema.md). For blueprint metadata, see the [Metadata Reference](metadata.md).

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
  - name: archive-source
    url: file://./archives/modules.tar.gz
    # file:// URLs point to local tar.gz archives containing terraform modules
    # The path within the archive (e.g., //terraform/modules) is automatically
    # constructed from the source's pathPrefix and component path during resolution
```

| Field        | Type       | Description                                      |
|--------------|------------|--------------------------------------------------|
| `name`       | `string`   | Identifies the source.                           |
| `url`        | `string`   | The source location. Supports Git URLs, OCI URLs (oci://registry/repo:tag), and file:// URLs for local archives. |
| `pathPrefix` | `string`   | Prefix to the source path. Defaults to `terraform` if not specified. |
| `ref`        | `Reference`| Details the branch, tag, or commit to use. Not needed for OCI URLs with embedded tags or file:// URLs. |
| `secretName` | `string`   | The secret for source access.                    |

**Note:** 
- For OCI sources, the URL should include the tag/version directly (e.g., `oci://registry.example.com/repo:v1.0.0`). The `ref` field is optional for OCI sources when the tag is specified in the URL.
- For file:// sources, the URL should be the path to a local `.tar.gz` archive file (relative to the blueprint.yaml directory or absolute), e.g., `file://./archives/modules.tar.gz`. The path within the archive (e.g., `terraform/cluster/talos`) is automatically constructed from the source's `pathPrefix` (defaults to `terraform`) and the component's `path` during resolution. The archive is automatically extracted and modules are made available for use in Terraform components.

### Reference
A reference to a specific git state or version

```yaml
reference:
  branch: main
  tag: v1.0.0
  semver: ~1.0.0
  name: refs/heads/main
  commit: 1a2b3c4d5e6f7g8h9i0j
```

| Field   | Type   | Description                                      |
|---------|--------|--------------------------------------------------|
| `branch`| `string` | Branch to use.                                 |
| `tag`   | `string` | Tag to use.                                    |
| `semver`| `string` | Semantic version constraint to use (e.g., `~1.0.0`, `>=1.0.0`). |
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
    parallelism: 5
  
  # A Terraform module from a local archive source
  - source: archive-source
    path: cluster/talos
```

| Field      | Type                             | Description                                      |
|------------|----------------------------------|--------------------------------------------------|
| `source`   | `string`                         | Source of the Terraform module. Must be included in the list of sources. Supports Git repositories, OCI artifacts, and file:// archive URLs. |
| `path`     | `string`                         | Path of the Terraform module relative to the `terraform/` folder (for Git/OCI sources) or within the archive (for file:// sources).                    |
| `inputs`   | `map[string]any`                  | Configuration values for the module. These values can be expressions using `${}` syntax (e.g., `${cluster.name}`) or literals. Values with `${}` are evaluated as expressions, plain values are passed through as literals. These are used for generating tfvars files and are not written to the final context blueprint.yaml. |
| `dependsOn`| `[]string`                       | Dependencies of this terraform component.        |
| `destroy`  | `*bool`                          | Determines if the component should be destroyed during down operations. Defaults to true if not specified. |
| `parallelism`| `int`                         | Limits the number of concurrent operations as Terraform walks the graph. Corresponds to the `-parallelism` flag. |

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
| `patches`     | `[]BlueprintPatch` | Patches to apply to the kustomization. Supports both blueprint format (path) and Flux format (patch + target). |
| `wait`        | `*bool`             | Wait for the kustomization to be fully applied.  |
| `force`       | `*bool`             | Force apply the kustomization.                   |
| `prune`       | `*bool`             | Enable garbage collection of resources that are no longer present in the source. |
| `components`  | `[]string`          | Components to include in the kustomization.      |
| `cleanup`     | `[]string`          | Resources to clean up after the kustomization is applied. |
| `destroy`     | `*bool`             | Determines if the kustomization should be destroyed during down operations. Defaults to true if not specified. |
| `substitutions` | `map[string]string` | Values for post-build variable replacement, collected and stored in ConfigMaps for use by Flux postBuild substitution. All values are converted to strings. These are used for generating ConfigMaps and are not written to the final context blueprint.yaml. |

#### Patches

Patches are provided via Features, not directly in blueprint definitions. See the [Features Reference](features.md#kustomization-patches) for details on how to define patches in features.

## Cluster Variables
When running `windsor install`, Kubernetes resources are applied. These resources include a configmap that introduces [post-build variables](https://fluxcd.io/flux/components/kustomize/kustomizations/#post-build-variable-substitution) into the Kubernetes manifests. These variables are outlined as follows:

| Key                     | Description                                                        |
|-------------------------|--------------------------------------------------------------------|
| `BUILD_ID`              | Build identifier for artifact tagging, generated by `windsor build-id`. Format: YYMMDD.RANDOM.# |
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
  {{ footer('Securing Secrets', '../../security/secrets/index.html', 'Configuration', '../configuration/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../security/secrets/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../configuration/index.html'; 
  });
</script>
