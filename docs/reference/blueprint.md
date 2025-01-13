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
```

| Field        | Type       | Description                                      |
|--------------|------------|--------------------------------------------------|
| `name`       | `string`   | Identifies the source.                           |
| `url`        | `string`   | The source location.                             |
| `ref`        | `Reference`| Details the branch, tag, or commit to use.       |
| `secretName` | `string`   | The secret for source access.                    |

### Reference
A reference to a specific git state or version

```yaml
reference:
  branch: main
  tag: v1.0.0
  semver: "^1.0.0"
  name: refs/heads/main
  commit: 1a2b3c4d5e6f7g8h9i0j
```

| Field   | Type   | Description                                      |
|---------|--------|--------------------------------------------------|
| `branch`| `string` | Branch to use.                                 |
| `tag`   | `string` | Tag to use.                                    |
| `semver`| `string` | SemVer to use.                                 |
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
| `values`   | `map[string]interface{}`         | Configuration values for the module.             |
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

## Example: Local Blueprint
When you run `windsor init local`, a default local blueprint is generated:

```yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: local
  description: This blueprint outlines resources in the local context
repository:
  url: http://git.test/git/my-app # Points to the local git livereload server
  ref:
    branch: main
  secretName: flux-system
sources:
- name: core
  url: github.com/windsorcli/core
  ref:
    tag: v0.3.0
terraform:
- source: core 
  path: cluster/talos # Bootstraps a local talos cluster
- source: core
  path: gitops/flux # Bootstraps flux gitops controllers
kustomize:
- name: local
  path: "" # Begins reflecting k8s manifests from the kustomize/ folder
```
