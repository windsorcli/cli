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
when: platform == 'aws'
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
| `option-*` or `options-*` | 300 |
| `addon-*` or `addons-*` | 400 |
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
    when: platform == 'incus' || platform == 'docker'
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

## Expressions

Windsor uses [expr-lang/expr](https://expr-lang.org/) for expression evaluation. Expressions appear in facet `when` clauses, in `inputs:` values, in kustomization `substitutions`, and in patch content. They are evaluated against the facet expression scope: the merged config (everything from `windsor.yaml` / `values.yaml`), facet `config:` blocks (exposed at scope root by name), and the injected `repository` block (`repository.name`, `repository.url`, `repository.ref.*`).

### Syntax

Expressions use `${...}` placeholders inside string fields, and bare expressions in `when:`. The full language definition is at [expr-lang.org/docs/language-definition](https://expr-lang.org/docs/language-definition). Common forms:

- **Equality**: `==`, `!=`
- **Logical**: `&&`, `||`, `!`
- **Arithmetic**: `+`, `-`, `*`, `/`, `%`
- **String literals**: single quotes — `'aws'`, `'talos'`
- **Booleans / numbers**: `true`, `false`, `1`, `2.5`
- **Member access**: `cluster.driver`, `cluster.workers.count`
- **Indexing**: `subnets[0].id`, `tags['env']`
- **Null coalescing**: `value ?? default`
- **Ternary**: `cond ? a : b`
- **Helper calls**: `jsonnet("path")`, `terraform_output("comp", "key")`

```yaml
when: platform == 'aws'
when: platform == 'aws' && cluster.driver == 'eks'
when: cluster.driver == 'eks' || cluster.driver == 'aks'
when: observability.enabled ?? false
```

### Helpers

Windsor registers these helpers on the expression engine.

| Helper | Signature | Description |
|--------|-----------|-------------|
| `terraform_output(component, key)` | `(string, string) → any` | Read another Terraform component's output. **Deferred** until that component has been applied; returns `nil` until then so `?? <fallback>` can supply a plan-time value. The bare path syntax `${terraform.<x>.outputs.<y>}` is not supported — use this helper. |
| `secret(provider, item, field)` | `(string, string, string) → string` | Resolve a secret from a configured provider (`sops`, 1Password `op`, etc.). Deferred — only evaluated when the surrounding flow demands a real value. Returns `<ERROR: ...>` on failure. |
| `jsonnet(path)` | `(string) → any` | Evaluate a Jsonnet file relative to the facet's directory. Result is any JSON-compatible type — typically a map. |
| `file(path)` | `(string) → string` | Read a file as a string, relative to the facet's directory. |
| `yaml(content_or_path)` | `(string) → any` | Parse YAML. The argument is either inline YAML content or a path (relative to the facet) to a YAML file. |
| `yaml(path, vars)` | `(string, any) → any` | Render a YAML template from a file with the given variables, then parse. |
| `yamlString(value)` | `(any) → string` | Marshal a value to a YAML string. |
| `yamlString(path, vars)` | `(string, any) → string` | Render a YAML template from a file with variables and return the rendered string (no parse). |
| `jsonString(value)` | `(any) → string` | Marshal a value to a JSON string. |
| `string(value)` | `(any) → string` | Coerce a value to a string (`nil` becomes `""`). |
| `split(s, sep)` | `(string, string) → []string` | Split a string by separator. |
| `cidrhost(cidr, hostnum)` | `(string, int) → string` | Return the IP at index `hostnum` within `cidr`. |
| `cidrsubnet(cidr, newbits, netnum)` | `(string, int, int) → string` | Return a single subnet within `cidr`. |
| `cidrsubnets(cidr, newbits...)` | `(string, ...int) → []string` | Return multiple subnets at once. |
| `cidrnetmask(cidr)` | `(string) → string` | Return the netmask for `cidr`. |

Paths passed to `file()`, `jsonnet()`, `yaml()`, and `yamlString()` resolve relative to the facet file location and work for both local templates and OCI artifacts.

### Examples

```yaml
inputs:
  cidr: ${network.cidr_block ?? "10.0.0.0/16"}
  api_endpoint: ${terraform_output("cluster", "endpoint")}
  mirrors: ${terraform_output("workstation", "registries") ?? {}}
  config: ${jsonnet("talos-config.jsonnet")}
  cert: ${file("certs/tls.crt")}
  worker_patches: ${yaml("../patches/worker.yaml")}
  cluster_id: "${context}-${string(cluster.id ?? 0)}"
  vpc_subnets: ${cidrsubnets(network.cidr_block, 4, 4, 8)}

substitutions:
  CLUSTER_ENDPOINT: ${terraform_output("cluster", "endpoint") ?? "https://localhost:6443"}
  DB_PASSWORD: ${secret("op", "db", "password")}
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

### Input evaluation

Input values are either literals or `${}` expressions. Plain strings, numbers, booleans, and structured values pass through as-is. Anything inside `${...}` is evaluated as an expression against the facet scope.

```yaml
inputs:
  count: 3
  enable_https: true
  endpoint: ${cluster.endpoint ?? "https://localhost:6443"}
  workers: ${cluster.workers.count * 2}
  config: ${jsonnet("config.jsonnet")}
  output: ${terraform_output("network", "vpc_id")}
```

See [Helpers](#helpers) above for the full helper catalog.

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

## Facet loading

Facets are loaded automatically from:

- `_template/facets/*.yaml` — top-level facet files.
- `_template/facets/**/*.yaml` — nested facet directories.

Facets are processed by **ordinal** (ascending), then by `metadata.name` (tiebreak). Higher ordinal means higher precedence — later-processed facets win on conflict. See [Default ordinal from filename](#default-ordinal-from-filename) for the basename-derived defaults.
