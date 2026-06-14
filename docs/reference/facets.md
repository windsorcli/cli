---
title: "Facets"
description: "Conditional blueprint fragments that compose into a base blueprint."
---
# Facets

Conditional blueprint fragments that compose into a base blueprint. A facet
declares terraform components, kustomizations, configuration blocks, and
substitutions that are merged into the blueprint only when the facet's
'when' expression evaluates to true against the operator's configuration.
Facets live under contexts/_template/facets/ as one .yaml file per facet
and are composed in ordinal order (lower-priority first).

## Fields

| Field | Type | Description |
|------|------|-------------|
| `kind` | `string` | Resource kind, following Kubernetes conventions. Must be 'Facet'. **(required)** |
| `apiVersion` | `string` | API schema version. Must be 'blueprints.windsorcli.dev/v1alpha1'. **(required)** |
| `metadata` | `object` | Identity for the facet. **(required)** |
| `backend` | `string` | Contributes to Blueprint.backend (the terraform component that terminates the backend tier). The composer merges this across active facets by ordinal precedence; the highest-ordinal non-empty value wins. |
| `config` | `array<object>` | Named configuration blocks exposed at scope root. Terraform inputs and kustomize substitutions reference '<name>' (scalar/list values) or '<name>.<key>' (map values), e.g. 'talos.controlplanes'. |
| `crds` | `array<string>` | References into the vendored CRD catalog, version-pinned (e.g. 'cert-manager-1.16.2'). Each reference registers one deduped kustomization at 'kustomize/crds/<ref>' (pruning disabled, wait enabled) into the cluster-wide CRD layer. The composer orders that layer ahead of the whole stack — every non-CRD root gains a dependsOn on the layer, so the API surface is Established before any custom resource is applied — so facet authors never write a CRD dependsOn themselves. Entries may use '${...}' expressions that resolve against the facet scope and prune to empty — e.g. "${gateway.driver == 'envoy' ? 'envoy-gateway-1.7.1' : ''}" — so one facet can select different CRDs per driver. Each resolved reference must be a single path segment; values containing '/', '\', or '..' are rejected at composition time because they would escape the crds/ catalog directory. |
| `kustomize` | `array<object>` | Kustomizations contributed by this facet. Each entry extends the blueprint's Kustomization shape with conditional fields (when, strategy, ordinal, requires). |
| `ordinal` | `integer` | Merge precedence relative to other facets. Higher ordinal wins on conflict (processed later). When unset, the loader derives an ordinal from the file basename: 'config-*' = 100; 'provider-*' or 'platform-*' with '-base' in the name = 199; other 'provider-*' / 'platform-*' = 200; 'option-*' / 'options-*' = 300; 'addon-*' / 'addons-*' = 400; no match = 0. |
| `requires` | `array<object>` | Input-requirement blocks for the facet as a whole. When the facet is active and a block's optional 'when' holds, every path in that block must resolve to a present, non-empty value in the merged scope. Unsatisfied paths across every active facet are aggregated into a single user-facing error. |
| `substitutions` | `map<string>` | Top-level key/value pairs evaluated with facet scope and injected into 'values-common', making them available to every kustomization via PostBuild substitution. Values may use expression syntax (e.g. '${dns.domain}') resolved against active facet config blocks. |
| `terraform` | `array<object>` | Terraform components contributed by this facet. Each entry extends the blueprint's TerraformComponent shape with conditional fields (when, strategy, ordinal, requires). |
| `when` | `string` | Expression that gates the facet. Evaluated against merged configuration values; the facet is applied only when the expression yields true. Examples: "platform == 'aws'", "observability.enabled == true && observability.backend == 'quickwit'". Empty 'when' means the facet always applies. |

## metadata

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Facet's unique identifier within the template. **(required)** |
| `description` | `string` | One-line overview of what the facet contributes. |

## config[]

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Block identifier; exposed at scope root. **(required)** |
| `value` | `any` | The block's content. May be a scalar, list, or map. References from terraform.inputs and kustomize.substitutions use the block name for scalar/list, or '<name>.<key>' when value is a map. **(required)** |
| `ordinal` | `integer` | Per-block override of the facet's ordinal for merge precedence. Higher wins on conflict. When unset, the parent facet's ordinal is used. |
| `requires` | `array<object>` | Requirement blocks scoped to this config block. Evaluated only when the parent facet is active AND this block's 'when' holds; missing paths surface under the effective AND condition. |
| `strategy` | `string` | How this block merges with same-named blocks from other facets. 'merge' (default) deep-merges; 'replace' overwrites; 'remove' deletes the block from scope. One of: `merge`, `replace`, `remove`. |
| `when` | `string` | Optional expression that gates this block. Empty means the block is always evaluated when the parent facet is active. |

### config[].requires[]

| Field | Type | Description |
|------|------|-------------|
| `paths` | `array<string>` | Dotted scope keys whose values must be present and non-empty. Required; the parser rejects an empty list. **(required)** |
| `message` | `string` | Optional author-supplied context surfaced under this block's heading in the aggregated error. |
| `when` | `string` | Optional expression gating this requirement. Empty means the paths are required whenever the parent (facet, config block, or component) is active. |

## kustomize[]

| Field | Type | Description |
|------|------|-------------|
| `ordinal` | `integer` | Per-kustomization override of the facet's ordinal for merge precedence. |
| `requires` | `array<object>` | Requirement blocks scoped to this kustomization. Evaluated only when the parent facet is active and this kustomization's 'when' holds. |
| `strategy` | `string` | How this kustomization is merged into the blueprint. 'merge' (default) deep-merges with existing kustomizations matching the same Name; 'replace' overwrites; 'remove' deletes the matching existing kustomization's non-index fields. Remove operations are always applied last. One of: `merge`, `replace`, `remove`. |
| `when` | `string` | Expression that gates this kustomization. Empty means the kustomization is always applied when the parent facet matches. |

### kustomize[].requires[]

| Field | Type | Description |
|------|------|-------------|
| `paths` | `array<string>` | Dotted scope keys whose values must be present and non-empty. Required; the parser rejects an empty list. **(required)** |
| `message` | `string` | Optional author-supplied context surfaced under this block's heading in the aggregated error. |
| `when` | `string` | Optional expression gating this requirement. Empty means the paths are required whenever the parent (facet, config block, or component) is active. |

## requires[]

| Field | Type | Description |
|------|------|-------------|
| `paths` | `array<string>` | Dotted scope keys whose values must be present and non-empty. Required; the parser rejects an empty list. **(required)** |
| `message` | `string` | Optional author-supplied context surfaced under this block's heading in the aggregated error. |
| `when` | `string` | Optional expression gating this requirement. Empty means the paths are required whenever the parent (facet, config block, or component) is active. |

## terraform[]

| Field | Type | Description |
|------|------|-------------|
| `ordinal` | `integer` | Per-component override of the facet's ordinal for merge precedence. |
| `requires` | `array<object>` | Requirement blocks scoped to this component. Evaluated only when the parent facet is active and this component's 'when' holds. |
| `strategy` | `string` | How this component is merged into the blueprint. 'merge' (default) deep-merges with existing components matching the same Path and Source; 'replace' overwrites; 'remove' deletes the matching existing component's non-index fields. Remove operations are always applied last. One of: `merge`, `replace`, `remove`. |
| `when` | `string` | Expression that gates this component. Empty means the component is always applied when the parent facet matches. |

### terraform[].requires[]

| Field | Type | Description |
|------|------|-------------|
| `paths` | `array<string>` | Dotted scope keys whose values must be present and non-empty. Required; the parser rejects an empty list. **(required)** |
| `message` | `string` | Optional author-supplied context surfaced under this block's heading in the aggregated error. |
| `when` | `string` | Optional expression gating this requirement. Empty means the paths are required whenever the parent (facet, config block, or component) is active. |

## Examples

```yaml
kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  description: AWS platform components
  name: platform-aws
when: platform == 'aws'
config:
  - name: aws
    value:
      region: us-east-1
terraform:
  - inputs:
      cidr: ${aws.cidr ?? '10.0.0.0/16'}
    name: vpc
    path: aws/vpc
    source: core
kustomize:
  - name: aws-load-balancer-controller
    path: addons/aws-lb-controller
    source: core
```

```yaml
kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  description: Adds observability stack when enabled
  name: addon-observability
when: observability.enabled == true
requires:
  - message: Observability is enabled; choose a backend (quickwit, victoriametrics).
    paths:
      - observability.backend
kustomize:
  - name: observability
    path: addons/observability
    source: core
    when: observability.backend == 'quickwit'
```

## See also

- [Blueprint reference](blueprint.md) — for the inherited TerraformComponent and Kustomization fields
- [`apply`](commands/apply.md), [`up`](commands/up.md), [`explain`](commands/explain.md)
- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- Source schema: [pkg/runtime/config/schemas/artifacts/facets.yaml](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schemas/artifacts/facets.yaml)
