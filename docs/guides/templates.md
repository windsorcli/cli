---
title: "Blueprint Templates"
description: "How _template/ + facets compose context-specific blueprints."
---
# Blueprint templates

The `contexts/_template/` directory is the foundation for every context in the project. It pairs a base `blueprint.yaml` with a directory of **facets** — modular, conditional fragments that contribute terraform components, kustomizations, configuration values, and substitutions based on the active context's configuration.

When a context is initialized, Windsor loads `_template/blueprint.yaml`, evaluates each facet against the context's values, and merges the results into a final composed blueprint. Run `windsor show blueprint` to see the result.

## Directory layout

```
contexts/
└── _template/
    ├── blueprint.yaml      # base blueprint
    ├── schema.yaml         # JSON Schema for values.yaml validation (optional)
    ├── metadata.yaml       # CLI version requirement (optional)
    └── facets/
        ├── config-cluster.yaml
        ├── platform-base.yaml
        ├── platform-aws.yaml
        ├── option-observability.yaml
        ├── addon-bookinfo.yaml
        └── ...
```

Files at any depth under `facets/` are loaded.

## Base blueprint

`_template/blueprint.yaml` is always loaded and merged in full. It defines repository, sources, and any unconditional terraform / kustomize components that all contexts share.

```yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
  description: Base blueprint for all contexts
sources:
  - name: core
    url: oci://ghcr.io/windsorcli/core:v0.5.0
terraform:
  - source: core
    path: cluster/talos
kustomize:
  - name: ingress
    path: ingress/base
    source: core
```

## Schema and metadata

`_template/schema.yaml` validates the user's `values.yaml` and supplies defaults for missing keys:

```yaml
$schema: https://windsorcli.dev/schema/2026-02/schema
type: object
properties:
  platform:
    type: string
    default: "none"
    enum: ["none", "metal", "docker", "aws", "azure", "gcp"]
  observability:
    type: object
    properties:
      enabled:
        type: boolean
        default: false
    additionalProperties: false
additionalProperties: false
```

Windsor implements a subset of JSON Schema Draft 2020-12 — see [Schema Reference](../reference/schema.md). Both `https://windsorcli.dev/schema/2026-02/schema` and `https://json-schema.org/draft/2020-12/schema` are accepted as `$schema` values.

`_template/metadata.yaml` declares the CLI version required to load the blueprint:

```yaml
cliVersion: ">=0.9.0"
```

Constraint forms: `>=0.9.0`, `~0.9.0`, `>=0.9.0 <0.10.0`. Mismatches abort blueprint loading with a clear error.

## Facets

A facet is a YAML document that contributes to the composed blueprint when its `when` expression evaluates to true. Each facet can carry:

- `config:` — named config blocks injected into the expression scope.
- `terraform:` — conditional terraform components.
- `kustomize:` — conditional kustomizations.
- `substitutions:` — common substitutions merged into `values-common`.

```yaml
kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: platform-aws
  description: AWS platform components

when: platform == 'aws'

config:
- name: cluster
  value:
    region: ${aws.region ?? "us-east-1"}

terraform:
- path: network/vpc
  source: core
  inputs:
    cidr: ${network.cidr_block ?? "10.0.0.0/16"}
  strategy: merge

kustomize:
- name: cert-manager
  path: cert-manager
  source: core
  strategy: merge

substitutions:
  REGION: ${cluster.region}
```

The `when:` expression sees user values, resolved config blocks from earlier facets, and the injected `repository` block (`repository.name`, `repository.url`, `repository.ref.*`).

## Ordinals

Facets are processed by **ordinal** (ascending), then by `metadata.name` for tiebreaks. Higher ordinal means higher precedence — later-processed facets win on conflict.

When a facet doesn't set `ordinal:` explicitly, the loader derives it from the file basename:

| Prefix | Ordinal |
|--------|---------|
| `config-*` | 100 |
| `provider-base*` / `platform-base*` | 199 |
| `provider-*` / `platform-*` | 200 |
| `option-*` / `options-*` | 300 |
| `addon-*` / `addons-*` | 400 |
| anything else | 0 |

The intent: configuration blocks load first; platform base before platform-specific; options after platforms; addons last so they can override platform defaults.

`ordinal:` can be set at four levels:

| Level | Where | Effect |
|-------|-------|--------|
| Facet | top of facet | Default for everything in the facet. |
| Config block | per `config:` entry | Overrides facet ordinal for this block. |
| Terraform component | per `terraform:` entry | Overrides facet ordinal for this component. |
| Kustomization | per `kustomize:` entry | Overrides facet ordinal for this kustomization. |

The legacy `priority:` field has been removed — use `ordinal:` everywhere.

## Strategies

Each terraform component, kustomization, and config block declares a `strategy:` for how it merges with existing entries of the same name:

| Strategy | Effect |
|----------|--------|
| `merge` *(default)* | Deep-merge fields onto matching existing entry. |
| `replace` | Wholly replace the matching existing entry. |
| `remove` | Strip non-index fields from the matching entry. Always applied last. |

Components match by Path + Source. Kustomizations match by Name. Config blocks match by Name.

When ordinals match, strategy precedence is `remove` > `replace` > `merge`.

## Config blocks

`config:` introduces values into the expression scope so terraform inputs and kustomize substitutions can reference them by name. Every entry has a required `value:` field — the canonical container for the block's payload — and is exposed at scope root under its `name`.

```yaml
config:
- name: talos
  value:
    controlplanes:
      cpu: ${cluster.controlplanes.cpu ?? 4}
    image: factory.talos.dev/installer/${talos.image_id}:v1.12.2

terraform:
- path: cluster/talos
  inputs:
    cpu: ${talos.controlplanes.cpu}
    image: ${talos.image}
```

When `value:` is a map, expressions reference `<name>.<key>`. When `value:` is a scalar or list, expressions reference `<name>` directly. The single `value:` field replaces the older convention of accepting arbitrary top-level keys; consolidating on `value` made the merge semantics unambiguous.

Config-block expressions are evaluated **once per round, after all same-name blocks are merged**, so an expression always sees the final merged value for its block.

## Substitutions and deferred values

A facet's `substitutions:` is merged into the `values-common` ConfigMap that ships with every kustomization (see [Kustomize](kustomize.md#substitutions)).

Substitution values may use expressions, including `terraform_output()` for cross-component output references. When an expression resolves at compose time, the result is materialized as a plain string. When an expression depends on a not-yet-applied component (`terraform_output("cluster", "endpoint")` before `cluster` has a state file), it is **deferred** and re-evaluated on the next compose pass.

```yaml
substitutions:
  CLUSTER_ENDPOINT: ${terraform_output("cluster", "endpoint") ?? "https://localhost:6443"}
  REGISTRY_HOST: ${docker.registry_url}
```

The `?? <fallback>` operator lets a deferred reference still produce a planable value before the upstream is applied, then resolve to the real output afterwards. See [Facets Reference](../reference/facets.md) for the full expression helper list.

## Composition order

When Windsor composes the final blueprint:

1. **OCI sources with `deploy: true`** (the default for OCI sources) have their components merged. Sources with `deploy: false` are index-only — their components aren't merged but components elsewhere can reference them via `source: <name>`. Non-OCI sources (git URLs) are always index-only.
2. **Base template** — `_template/blueprint.yaml` merges in full.
3. **Facets** — processed in ordinal order, with strategies and `when` expressions applied.
4. **User blueprint** — `contexts/<name>/blueprint.yaml` overrides without filtering. Anything from earlier layers stays unless the user blueprint's entry uses `enabled: false` or a remove strategy via a facet.

## File resolution

Files referenced from facets via `jsonnet()` or `file()` resolve relative to the facet file location. From `_template/facets/aws.yaml`:

- `file("config.jsonnet")` → `_template/facets/config.jsonnet`
- `file("../configs/config.jsonnet")` → `_template/configs/config.jsonnet`

Paths work for both the local filesystem and in-memory template data unpacked from OCI artifacts.

## See also

- [Reference: Facets](../reference/facets.md) — full schema and expression helpers
- [Reference: Blueprint](../reference/blueprint.md) — composed blueprint shape
- [Sharing Blueprints](sharing.md) — packaging templates as OCI artifacts
- [Kustomize](kustomize.md#substitutions) — how `substitutions` flow through to Flux
- [Explain](explain.md) — `windsor explain <path>` traces a value back to the contributing facet
