---
title: "Blueprint"
description: "Top-level blueprint definition."
---
# Blueprint

Top-level blueprint definition. Lives at contexts/<context>/blueprint.yaml and
declares the Terraform components and Flux kustomizations that windsor will
apply for that context, plus the sources those components come from and the
variable substitutions shared across them.

## Fields

| Field | Type | Description |
|------|------|-------------|
| `kind` | `string` | Resource kind, following Kubernetes conventions. Must be 'Blueprint'. **(required)** |
| `apiVersion` | `string` | API schema version. Must be 'blueprints.windsorcli.dev/v1alpha1'. **(required)** |
| `metadata` | `object` | Identity for the blueprint. **(required)** |
| `backend` | `string` | Names the terraform component that terminates the backend tier. When set, 'windsor bootstrap' applies the backend component (and every component declared before it) against local state first, then migrates state to the configured remote backend before applying the rest of the graph. |
| `configMaps` | `map<object>` | Standalone ConfigMaps to create. Each top-level key is a ConfigMap name; its map-of-string value is the .data payload. These are referenced by every kustomization in PostBuild substitution. |
| `crds` | `array<string>` | The flat list of vendored CRD references installed from the default/project source (e.g. 'cert-manager-1.16.2'), authored as a bare scalar list — the same form facets use. CRDs carried by an OCI source are not listed here: they ride with that source (see sources[].crds) and install in the background when it is install:true. The provisioner materializes this list into the 'crds' kustomization — pruning disabled, wait enabled — applied ahead of the kustomize: layer so every kustomization sees its CRDs Established first. |
| `flux` | `array<object>` | Flux Kustomizations included in the blueprint — the preferred spelling of 'kustomize:' (every entry compiles to a Flux Kustomization). 'kustomize:' remains accepted as a backward-compatible alias; both keys merge into one list. |
| `kustomize` | `array<object>` | Flux kustomizations included in the blueprint. Each entry maps to a Kustomization resource the provisioner applies to the cluster, in topologically sorted dependsOn order. Deprecated alias of 'flux:'; both keys are accepted and merge into the same list. |
| `repository` | `object` | Source repository this blueprint was bootstrapped from. |
| `sources` | `array<object>` | External resources referenced by the blueprint. Each source is an OCI blueprint artifact or a Git repository that contributes Terraform modules and/or kustomize bases consumable by the components below. |
| `substitutions` | `map<string>` | Blueprint-level substitutions injected into 'values-common' and made available to every kustomization via PostBuild substitution. Values may use expression syntax (e.g. '${dns.domain}') resolved against facet config blocks. |
| `terraform` | `array<object>` | Terraform components included in the blueprint, in declaration order. Components are reordered topologically by dependsOn at apply time. |

## metadata

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Blueprint's unique identifier within the project. **(required)** |
| `description` | `string` | One-line overview of what this blueprint provisions. |

## flux[]

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Identifier for the kustomization; referenced by dependsOn. **(required)** |
| `path` | `string` | Path within the source containing the kustomize base. **(required)** |
| `components` | `array<string>` | Kustomize components to compose into this kustomization. |
| `dependsOn` | `array<string>` | Names of kustomizations that must reconcile before this one. |
| `destroy` | `boolean / string` | Whether to delete this kustomization during 'windsor down' / 'windsor destroy'. Boolean or expression. Defaults to true. |
| `destroyOnly` | `boolean` | When true, the kustomization only runs during destroy. Useful for teardown-only resources (e.g. cleanup jobs). |
| `enabled` | `boolean / string` | Whether to include this kustomization in the final blueprint. Boolean or expression. Defaults to true. |
| `force` | `boolean` | Force-apply resources Flux would otherwise refuse to update. |
| `install` | `array<string>` | Components of the install (controller/operator) tier. Like components, each entry may be a '${...}' expression that prunes to empty. When install and/or resources are set, the entry expands into separate kustomizations sharing this path: '<name>-install' carrying these components and '<name>-resources' depending on it, so the controller is reconciled before the custom resources it admits. |
| `interval` | `string` | Reconciliation interval, expressed as a Go duration string (e.g. '5m', '1h'). Defaults to a mode-appropriate value chosen by the provisioner (short poll for pull mode, long fallback for push). |
| `namespace` | `string` | Namespace where the Flux Kustomization object itself lives. Defaults to the gitops namespace. DependsOn references always resolve in the gitops namespace; cross-namespace dependencies are not supported. |
| `patches` | `array<object>` | Strategic-merge or Flux-style patches applied to the kustomization. Each entry is either a 'path:' to a patch file relative to the kustomization, or a 'patch:' inline YAML body with an optional 'target:' selector (kind / name / namespace). |
| `prune` | `boolean` | Garbage-collect resources removed from the source. Defaults to true. |
| `resources` | `array<string>` | Components of the custom-resource tier. See install. |
| `retryInterval` | `string` | Duration to wait before retrying a failed reconciliation (e.g. '2m'). |
| `source` | `string` | Name of the source (from the sources list) that provides this kustomization. Defaults to the blueprint's primary source when unset. |
| `substitutions` | `map<string>` | PostBuild variable substitutions for this kustomization. Collected into a 'values-<name>' ConfigMap that Flux substitutes from. |
| `targetNamespace` | `string` | Populates spec.targetNamespace, instructing Flux to override the namespace of every resource reconciled by this kustomization. |
| `timeout` | `string` | Maximum duration for a single reconciliation attempt (e.g. '10m'). |
| `wait` | `boolean` | Wait for resources to settle before declaring reconciliation complete. |

### flux[].patches[]

| Field | Type | Description |
|------|------|-------------|
| `patch` | `string` | Inline patch body as YAML (Flux format). |
| `path` | `string` | Path to a patch file relative to the kustomization (blueprint format). |
| `target` | `object` | Selector for the patch (Flux format). Used with 'patch:'. |

## kustomize[]

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Identifier for the kustomization; referenced by dependsOn. **(required)** |
| `path` | `string` | Path within the source containing the kustomize base. **(required)** |
| `components` | `array<string>` | Kustomize components to compose into this kustomization. |
| `dependsOn` | `array<string>` | Names of kustomizations that must reconcile before this one. |
| `destroy` | `boolean / string` | Whether to delete this kustomization during 'windsor down' / 'windsor destroy'. Boolean or expression. Defaults to true. |
| `destroyOnly` | `boolean` | When true, the kustomization only runs during destroy. Useful for teardown-only resources (e.g. cleanup jobs). |
| `enabled` | `boolean / string` | Whether to include this kustomization in the final blueprint. Boolean or expression. Defaults to true. |
| `force` | `boolean` | Force-apply resources Flux would otherwise refuse to update. |
| `install` | `array<string>` | Components of the install (controller/operator) tier. Like components, each entry may be a '${...}' expression that prunes to empty. When install and/or resources are set, the entry expands into separate kustomizations sharing this path: '<name>-install' carrying these components and '<name>-resources' depending on it, so the controller is reconciled before the custom resources it admits. |
| `interval` | `string` | Reconciliation interval, expressed as a Go duration string (e.g. '5m', '1h'). Defaults to a mode-appropriate value chosen by the provisioner (short poll for pull mode, long fallback for push). |
| `namespace` | `string` | Namespace where the Flux Kustomization object itself lives. Defaults to the gitops namespace. DependsOn references always resolve in the gitops namespace; cross-namespace dependencies are not supported. |
| `patches` | `array<object>` | Strategic-merge or Flux-style patches applied to the kustomization. Each entry is either a 'path:' to a patch file relative to the kustomization, or a 'patch:' inline YAML body with an optional 'target:' selector (kind / name / namespace). |
| `prune` | `boolean` | Garbage-collect resources removed from the source. Defaults to true. |
| `resources` | `array<string>` | Components of the custom-resource tier. See install. |
| `retryInterval` | `string` | Duration to wait before retrying a failed reconciliation (e.g. '2m'). |
| `source` | `string` | Name of the source (from the sources list) that provides this kustomization. Defaults to the blueprint's primary source when unset. |
| `substitutions` | `map<string>` | PostBuild variable substitutions for this kustomization. Collected into a 'values-<name>' ConfigMap that Flux substitutes from. |
| `targetNamespace` | `string` | Populates spec.targetNamespace, instructing Flux to override the namespace of every resource reconciled by this kustomization. |
| `timeout` | `string` | Maximum duration for a single reconciliation attempt (e.g. '10m'). |
| `wait` | `boolean` | Wait for resources to settle before declaring reconciliation complete. |

### kustomize[].patches[]

| Field | Type | Description |
|------|------|-------------|
| `patch` | `string` | Inline patch body as YAML (Flux format). |
| `path` | `string` | Path to a patch file relative to the kustomization (blueprint format). |
| `target` | `object` | Selector for the patch (Flux format). Used with 'patch:'. |

## repository

| Field | Type | Description |
|------|------|-------------|
| `ref` | `object` | A specific version or state of a repository or source (one of branch / tag / semver / commit). |
| `secretName` | `string` | Name of a Flux secret holding credentials for the repository. |
| `url` | `string` | Repository URL. |

### repository.ref

| Field | Type | Description |
|------|------|-------------|
| `branch` | `string` | Branch to follow. |
| `commit` | `string` | Specific commit SHA. |
| `name` | `string` | Named reference. |
| `semver` | `string` | Semver constraint (e.g. '>=1.0.0'). |
| `tag` | `string` | Specific tag. |

## sources[]

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Identifier for the source; referenced by 'source:' on terraform / kustomize components. **(required)** |
| `crds` | `array<string>` | CRD references this source vendors at <source>/kustomize/crds/<ref>, populated by the composer from the source's included facets. When the source is install:true the provisioner installs them in the background as a 'crds-<name>' kustomization bound to this source, so the blueprint need not list them. |
| `install` | `boolean / string` | For OCI sources, whether to merge this source's components into the final blueprint. Accepts a boolean (true/false) or an expression (e.g. '${some.condition ?? true}') evaluated against facet config. Defaults to true. Has no effect on Git sources. |
| `pathPrefix` | `string` | Path prefix applied to the source. Defaults to 'terraform' when unset. |
| `ref` | `object` | A specific version or state of a repository or source (one of branch / tag / semver / commit). |
| `secretName` | `string` | Name of a Flux secret holding credentials for the source. |
| `url` | `string` | Source location. Accepts Git URLs and OCI URLs (oci://registry/repo:tag). |

### sources[].ref

| Field | Type | Description |
|------|------|-------------|
| `branch` | `string` | Branch to follow. |
| `commit` | `string` | Specific commit SHA. |
| `name` | `string` | Named reference. |
| `semver` | `string` | Semver constraint (e.g. '>=1.0.0'). |
| `tag` | `string` | Specific tag. |

## terraform[]

| Field | Type | Description |
|------|------|-------------|
| `path` | `string` | Path of the module within the source. **(required)** |
| `dependsOn` | `array<string>` | IDs of components that must apply before this one. |
| `destroy` | `boolean / string` | Whether to destroy this component during 'windsor down' / 'windsor destroy'. Boolean (true/false) or expression (e.g. '${cluster.destroy ?? true}'). Defaults to true. |
| `enabled` | `boolean / string` | Whether to include this component in the final blueprint. Boolean (true/false) or expression. Defaults to true. |
| `inputs` | `object` | Module input values. Plain values pass through as literals; values using '${expr}' syntax are evaluated against facet config at apply time. Used to generate tfvars and not written to the final context blueprint.yaml. |
| `name` | `string` | Optional name. When set, becomes the unique identifier for the component (used by dependsOn and context variables) instead of path. |
| `parallelism` | `integer / string` | Caps concurrent operations during 'terraform apply' / 'terraform destroy' for this component (the -parallelism flag). Integer or expression (e.g. '${cluster.parallelism ?? 10}'). |
| `source` | `string` | Name of the source (from the sources list) that provides this module. |

## Examples

```yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  description: Local workstation blueprint
  name: local
sources:
  - name: core
    url: oci://ghcr.io/windsorcli/core:v0.3.0
  - name: modules
    ref:
      branch: main
    url: github.com/org/terraform-modules
terraform:
  - inputs:
      cluster_name: local
    name: cluster
    path: cluster/talos
    source: core
  - dependsOn:
      - cluster
    name: dns
    path: dns/coredns
    source: core
kustomize:
  - name: flux-system
    path: flux-system
    source: core
  - dependsOn:
      - flux-system
    name: dns
    path: dns
    source: core
substitutions:
  external_domain: example.test
```

## See also

- [Facets reference](facets.md), [Metadata reference](metadata.md), [Schema reference](schema.md)
- [`apply`](commands/apply.md), [`up`](commands/up.md), [`bootstrap`](commands/bootstrap.md), [`destroy`](commands/destroy.md)
- [`show blueprint`](commands/show-blueprint.md), [`explain`](commands/explain.md)
- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle), [Sharing blueprints](https://www.windsorcli.dev/docs/blueprints/sharing)
- Source schema: [pkg/runtime/config/schemas/artifacts/blueprint.yaml](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schemas/artifacts/blueprint.yaml)
