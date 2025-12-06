---
title: "Features"
description: "Reference for conditional blueprint fragments"
---
# Features

Features are conditional blueprint fragments that enable modular composition of blueprints based on user configuration values. They allow you to conditionally include Terraform components and Kustomizations in your blueprint based on expressions evaluated against your `windsor.yaml` and `values.yaml` configurations.

## Feature Definition

A Feature is defined in a YAML file, typically located in `_template/features/` directory of a blueprint:

```yaml
kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
  description: AWS-specific infrastructure components
when: provider == 'aws'
terraform:
  - path: network/vpc
    source: core
    inputs:
      cidr: ${network.cidr_block ?? "10.0.0.0/16"}
      enable_nat: ${network.enable_nat ?? true}
    strategy: merge
kustomize:
  - name: ingress
    path: ingress/base
    source: core
    components:
      - nginx
      - nginx/nodeport
    substitutions:
      domain: ${dns.domain}
    strategy: merge
```

| Field       | Type                              | Description                                                                 |
|-------------|-----------------------------------|-----------------------------------------------------------------------------|
| `kind`      | `string`                          | Must be `Feature`.                                                          |
| `apiVersion`| `string`                          | Must be `blueprints.windsorcli.dev/v1alpha1`.                               |
| `metadata`  | `Metadata`                        | Feature metadata including name and description.                            |
| `when`      | `string`                          | Expression that determines if the feature should be applied. Evaluated against configuration values. If empty or evaluates to `true`, the feature is applied. |
| `terraform` | `[]ConditionalTerraformComponent` | Terraform components to include when the feature matches.                  |
| `kustomize` | `[]ConditionalKustomization`       | Kustomizations to include when the feature matches.                         |

Features inherit Repository and Sources from the base blueprint they are merged into.

## Conditional Logic

Features use the [Go expr library](https://github.com/expr-lang/expr) for expression evaluation. Expressions are evaluated against your configuration values from `windsor.yaml` and `values.yaml`.

### Expression Syntax

Expressions support:

- **Equality/inequality**: `==`, `!=`
- **Logical operators**: `&&`, `||`
- **Parentheses for grouping**: `(expression)`
- **Nested object access**: `provider`, `cluster.enabled`, `vm.driver`, `cluster.workers.count`
- **String literals**: Use single quotes: `'aws'`, `'talos'`, `'local'`
- **Boolean values**: `true`, `false`
- **Numeric values**: `1`, `2.5`
- **Null coalescing**: `value ?? default`
- **Functions**: `values()`, `jsonnet()`, `file()`

### Expression Examples

```yaml
# Simple equality check
when: provider == 'aws'

# Multiple conditions
when: provider == 'aws' && cluster.enabled == true

# Nested property access
when: cluster.enabled == true && cluster.driver == 'talos'

# Complex logical expressions
when: (provider == 'aws' || provider == 'azure') && cluster.enabled == true

# Null coalescing
when: observability.enabled ?? false
```

## ConditionalTerraformComponent

Extends `TerraformComponent` with conditional logic and merge strategy support.

| Field      | Type                | Description                                                                 |
|------------|---------------------|-----------------------------------------------------------------------------|
| `path`     | `string`            | Path of the Terraform module. Required.                                     |
| `source`   | `string`            | Source of the Terraform module. Optional.                                   |
| `inputs`   | `map[string]any`    | Configuration values for the module. Supports `${}` expressions.            |
| `dependsOn`| `[]string`          | Dependencies of this terraform component.                                    |
| `destroy`  | `*bool`             | Determines if the component should be destroyed during down operations.     |
| `parallelism`| `int`             | Limits the number of concurrent operations as Terraform walks the graph.     |
| `when`     | `string`            | Expression that determines if this component should be applied. If empty, the component is always applied when the parent feature matches. |
| `strategy` | `string`            | Merge strategy: `merge` (default) or `replace`. Only available in features.  |

### Merge Strategies

**Merge (default)**:
- Matches on `Path` and `Source`
- Deep merges `inputs` maps (overlay values override base values)
- Appends unique dependencies to `dependsOn`
- Updates `destroy` and `parallelism` if provided
- If no matching component exists, the component is appended

**Replace**:
- Matches on `Path` and `Source`
- Completely replaces the matching component with the new one
- All existing inputs, dependencies, and settings are discarded
- If no matching component exists, the component is appended

### Input Evaluation

Input values support:

- **Expressions**: Use `${}` syntax (e.g., `${cluster.workers.count}`, `${cluster.workers.count + 2}`)
- **String literals**: Plain strings are treated as literals
- **Other types**: Numbers, booleans, etc. are passed through as-is

Expressions support:
- Direct property access: `${provider}`
- Nested access: `${cluster.workers.count}`
- Arithmetic: `${cluster.workers.count * 2}`
- Null coalescing: `${cluster.endpoint ?? "https://localhost:6443"}`
- Functions: `${values(cluster.controlplanes.nodes)}`
- File loading: `${jsonnet("config.jsonnet")}`, `${file("key.pem")}`

## ConditionalKustomization

Extends `Kustomization` with conditional logic and merge strategy support.

| Field          | Type                | Description                                                                 |
|----------------|---------------------|-----------------------------------------------------------------------------|
| `name`         | `string`            | Name of the kustomization. Required.                                        |
| `path`         | `string`            | Path of the kustomization. Required.                                        |
| `source`       | `string`            | Source of the kustomization. Optional.                                      |
| `dependsOn`    | `[]string`          | Dependencies of this kustomization.                                          |
| `components`   | `[]string`          | Components to include in the kustomization.                                  |
| `patches`      | `[]BlueprintPatch`  | Patches to apply to the kustomization.                                      |
| `substitutions`| `map[string]string` | Values for post-build variable replacement. Supports `${}` expressions.      |
| `when`         | `string`            | Expression that determines if this kustomization should be applied. If empty, the kustomization is always applied when the parent feature matches. |
| `strategy`     | `string`            | Merge strategy: `merge` (default) or `replace`. Only available in features.  |

### Merge Strategies

**Merge (default)**:
- Matches on `Name`
- Appends unique components to the `components` array
- Appends unique dependencies to `dependsOn`
- Updates `path`, `source`, and `destroy` if provided
- Appends patches to the existing patches array
- If no matching kustomization exists, the kustomization is appended

**Replace**:
- Matches on `Name`
- Completely replaces the matching kustomization with the new one
- All existing components, dependencies, patches, and settings are discarded
- If no matching kustomization exists, the kustomization is appended

### Substitutions

Substitutions are:
- Converted to strings (as required by Flux post-build substitution)
- Stored in ConfigMaps
- Available in Kubernetes manifests via Flux's postBuild substitution
- Support `${}` expression interpolation

### Kustomization Patches

Patches support expression interpolation in the `patch` field content using `${}` syntax.

**Blueprint Format** (path-based):
```yaml
kustomize:
  - name: my-app
    path: apps/my-app
    patches:
      - path: patches/custom-patch.yaml
```

**Flux Format** (inline with target selector):
```yaml
kustomize:
  - name: my-app
    path: apps/my-app
    patches:
      - patch: |-
          - op: replace
            path: /spec/replicas
            value: ${replicas ?? 3}
        target:
          kind: Deployment
          name: my-app
          namespace: default
```

Patch content in the `patch` field supports expression interpolation:
- Direct property access: `${dns.domain}`
- Nested access: `${cluster.workers.count}`
- Null coalescing: `${replicas ?? 3}`
- String interpolation: `"${context}-config"`

When using the `merge` strategy (default), patches are appended to existing patches. When using the `replace` strategy, all existing patches are discarded and replaced with the new patches.

## File Loading Functions

Features support two file loading functions for dynamic configuration:

### jsonnet()

Loads and evaluates a Jsonnet file:

```yaml
terraform:
  - path: cluster/talos
    source: core
    inputs:
      config: ${jsonnet("talos-config.jsonnet")}
      worker_patches: ${jsonnet("talos-config.jsonnet").worker_config_patches}
```

- Takes a relative path to a `.jsonnet` file
- Evaluates the Jsonnet file with access to configuration values
- Returns the evaluated result (can be any JSON-compatible type)
- Paths are relative to the feature file location

### file()

Loads a file as a string:

```yaml
kustomize:
  - name: ingress
    path: ingress/base
    substitutions:
      tls_cert: ${file("certs/tls.crt")}
      tls_key: ${file("certs/tls.key")}
```

- Takes a relative path to any file
- Returns the file contents as a string
- Paths are relative to the feature file location

### Path Resolution

Both functions resolve paths relative to the feature file location:
- Feature at `_template/features/aws.yaml` can reference `config.jsonnet` in the same directory
- Use `../configs/config.jsonnet` for files in parent directories
- Paths work with both local filesystem and in-memory template data (from archives)

## Feature Loading

Features are automatically loaded from:
- `_template/features/*.yaml` - Individual feature files
- `_template/features/**/*.yaml` - Nested feature directories

Features are processed in alphabetical order by name, then merged into the base blueprint.

<div>
  {{ footer('Contexts', '../contexts/index.html', 'Schema', '../schema/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../contexts/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../schema/index.html'; 
  });
</script>

