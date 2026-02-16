---
title: "Facets"
description: "Reference for conditional blueprint fragments"
---
# Facets

Facets are conditional blueprint fragments that enable modular composition of blueprints based on user configuration values. They allow you to conditionally include Terraform components and Kustomizations in your blueprint based on expressions evaluated against your `windsor.yaml` and `values.yaml` configurations.

## Facet Definition

A Facet is defined in a YAML file, typically located in `_template/facets/` directory of a blueprint:

```yaml
kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-facet
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
| `kind`      | `string`                          | Must be `Facet`.                                                          |
| `apiVersion`| `string`                          | Must be `blueprints.windsorcli.dev/v1alpha1`.                               |
| `metadata`  | `Metadata`                        | Facet metadata including name and description.                            |
| `ordinal`   | `integer` (optional)              | Order in which this facet is applied relative to others. Higher ordinal = higher precedence when merging. When omitted, derived from the facet file basename (see [Default ordinal from filename](#default-ordinal-from-filename)). |
| `when`      | `string`                          | Expression that determines if the facet should be applied. Evaluated against configuration values. If empty or evaluates to `true`, the facet is applied. |
| `config`    | `[]ConfigBlock`                   | Named configuration blocks evaluated in blueprint context; referenced as `<name>.<key>` (e.g. `talos.controlplanes`) from terraform inputs and kustomize substitutions, same style as context (`cluster.*`, `network.*`). |
| `terraform` | `[]ConditionalTerraformComponent` | Terraform components to include when the facet matches.                  |
| `kustomize` | `[]ConditionalKustomization`       | Kustomizations to include when the facet matches.                         |

Facets inherit Repository and Sources from the base blueprint they are merged into.

### Default ordinal from filename

When a facet does not set `ordinal` in YAML, it is derived from the facet file **basename** (e.g. `config-cluster.yaml`, `provider-base.yaml`). This determines processing order: facets are sorted by ordinal ascending, then by `metadata.name` for tiebreak. Higher ordinal means higher precedence when merging (processed later, wins on conflict).

| Condition | Ordinal |
|-----------|--------|
| `config-*` | 100 |
| `provider-*` or `platform-*` with `-base` in name (e.g. `provider-base.yaml`, `platform-base.yaml`) | 199 |
| `provider-*` or `platform-*` (others) | 200 |
| `options-*` | 300 |
| `addons-*` | 400 |
| (no match) | 0 |

Terraform and kustomize components can set an optional `ordinal` to override the facet ordinal for that component's merge precedence.

## Config

Facets can define **config** blocks to build generic configuration (e.g. Talos node lists, patch vars) in-facet instead of external YAML files. Config is **merged globally** across all active facets in processing order (by facet ordinal, then by facet name). Merging is **deep**: same config block name merges block bodies recursively (later keys overwrite). At evaluation time, facet config is merged into the **same scope** as context (schema / values from `windsor.yaml` and `values.yaml`): one canonical root. So `talos.controlplanes` and `cluster.controlplanes` are siblings; expressions see both. When a key exists in both context and facet config, **facet config wins**—so blueprint authors can intentionally override or transform input values (e.g. a config block that derives or rewrites `cluster` for downstream use). Use distinct block names (e.g. `talos`, `gitops`) to add derived values; use the same name as a context key only when you want to override or transform that value for the blueprint.

`config` is a list of named blocks. Each block has:

- **name** – Exposed at scope root; expressions use `name.key` (e.g. `talos.controlplanes`).
- **when** (optional) – Expression; if present and false, the block is skipped.
- **body** – Remaining keys (e.g. `controlplanes`, `workers`, `patchVars`); values may contain `${}` expressions.

Config blocks are evaluated in blueprint context only (no facet config in scope). References from terraform or kustomize use `<name>.<key>`.

```yaml
config:
  - name: talos
    when: provider == 'incus' || provider == 'docker'
    controlplanes: ${cluster.controlplanes}
    workers: ${cluster.workers}
    patchVars:
      certSANs: ${cluster.apiServer.certSANs}
      poolPath: ${cluster.storage.poolPath}

terraform:
  - name: cluster
    path: cluster
    inputs:
      controlplanes: ${talos.controlplanes}
      workers: ${talos.workers}
      common_config_patches: ${yamlString("../configs/talos/common-patch.yaml", talos.patchVars)}
```

Evaluation order: facets are processed by ordinal (ascending), then by name. First, each facet's config block **structure** (name, when, body keys) is merged into a global scope (same block name merges block bodies recursively); config body expressions are **not** evaluated yet. After all facets are merged, config body expressions are evaluated once in blueprint context (very late). Then terraform and kustomize components are collected; their inputs and substitutions can reference the evaluated `<name>.<key>` (e.g. `talos.controlplanes`).

## Conditional Logic

Facets use the [Go expr library](https://github.com/expr-lang/expr) for expression evaluation. Expressions are evaluated against your configuration values from `windsor.yaml` and `values.yaml`.

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
| `when`     | `string`            | Expression that determines if this component should be applied. If empty, the component is always applied when the parent facet matches. |
| `ordinal`  | `integer` (optional)| Overrides the facet ordinal for this component's merge precedence. When omitted, the facet's ordinal is used. Higher ordinal wins when merging. |
| `strategy` | `string`            | Merge strategy: `merge` (default), `replace`, or `remove`. Only available in facets.  |

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
| `when`         | `string`            | Expression that determines if this kustomization should be applied. If empty, the kustomization is always applied when the parent facet matches. |
| `ordinal`      | `integer` (optional) | Overrides the facet ordinal for this kustomization's merge precedence. When omitted, the facet's ordinal is used. Higher ordinal wins when merging. |
| `strategy`     | `string`            | Merge strategy: `merge` (default), `replace`, or `remove`. Only available in facets.  |

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

Facets support two file loading functions for dynamic configuration:

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
- Paths are relative to the facet file location

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
- Paths are relative to the facet file location

### Path Resolution

Both functions resolve paths relative to the facet file location:
- Facet at `_template/facets/aws.yaml` can reference `config.jsonnet` in the same directory
- Use `../configs/config.jsonnet` for files in parent directories
- Paths work with both local filesystem and in-memory template data (from archives)

## Facet Loading

Facets are automatically loaded from:
- `_template/facets/*.yaml` - Individual facet files
- `_template/facets/**/*.yaml` - Nested facet directories

Facets are processed in alphabetical order by name, then merged into the base blueprint.

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

