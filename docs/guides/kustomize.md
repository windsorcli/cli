---
title: "Kustomize"
description: "How Windsor composes Flux Kustomizations from blueprints, including substitutions, patches, and destroy-only hooks."
---
# Kustomize

The kustomize layer is the second half of a blueprint, applied after Terraform. Each entry under `kustomize:` in `blueprint.yaml` becomes a Flux [`Kustomization`](https://fluxcd.io/flux/components/kustomize/kustomizations/) resource that points at a path in a blueprint source. Flux then reconciles the resources at that path onto the cluster.

`windsor apply` (or `windsor up` for workstation contexts) installs every kustomization in dependency order. `windsor destroy` removes them all in reverse-topological order. Use `windsor apply kustomize <name>` or `windsor destroy kustomize <name>` to target a single kustomization.

## Folder layout

Kustomizations live in source repositories, not in the project tree. A typical OCI blueprint exposes them under a `kustomize/` directory; local kustomizations sit under `kustomize/` in the project root:

```plaintext
contexts/
└── local/
    ├── blueprint.yaml
    └── patches/
        └── my-app/
            └── increase-replicas.yaml
kustomize/
└── my-app/
    ├── prometheus/
    │   ├── kustomization.yaml
    │   └── service-monitor.yaml
    ├── kustomization.yaml
    ├── deployment.yaml
    └── service.yaml
```

In this example `my-app` is a local app with a Prometheus [Kustomize component](https://kubectl.docs.kubernetes.io/guides/config_management/components/). The blueprint references it without a `source:`:

```yaml
kustomize:
- name: my-app
  path: my-app
  components:
    - prometheus
```

To pull a kustomization from a remote source, name the source and let Windsor resolve the path inside it:

```yaml
kustomize:
- name: csi
  source: core
  path: csi
  components:
    - longhorn
```

## Kustomization fields

The full schema is in [reference/blueprint.md](../reference/blueprint.md#kustomization). The most-used fields:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Required. Becomes the Flux Kustomization name and `values-<name>` ConfigMap name. |
| `path` | string | Path inside the source. |
| `source` | string | Blueprint source name, defaults to the blueprint's own repository. |
| `dependsOn` | `[]string` | Names of other kustomizations this one depends on. Resolved in the gitops namespace. |
| `components` | `[]string` | Kustomize components to layer in. |
| `interval`, `retryInterval`, `timeout` | duration | Reconciliation cadence. |
| `wait`, `force`, `prune` | `*bool` | Standard Flux flags. |
| `substitutions` | `map[string]string` | Per-kustomization `postBuild.substitute` values, materialized as `values-<name>` ConfigMap. |
| `patches` | `[]Patch` | Strategic-merge or JSON 6902 patches. |
| `namespace` | string | Override where the Kustomization object itself lives. Defaults to the gitops namespace. |
| `targetNamespace` | string | Sets `spec.targetNamespace` so Flux rewrites the namespace of every resource it reconciles. |
| `destroy` | bool/expression | Whether to remove on destroy. Defaults to `true`. Supports facet expressions. |
| `destroyOnly` | `*bool` | Hook for teardown work — final backups, snapshot/archive jobs, deregistrations. Skipped during apply/up and only applied during destroy. See [`destroyOnly`](#destroyonly) below. |
| `enabled` | bool/expression | Whether to include the kustomization at all. Defaults to `true`. |

### Namespace vs. target namespace

These two fields control different things and are usually left unset.

- **`targetNamespace`** sets `spec.targetNamespace` on the Flux Kustomization. Flux rewrites every reconciled resource into that namespace. Use it when the same kustomization layout serves multiple deployment namespaces — for example, the same `apps/my-app` path deployed into `staging` in one context and `production` in another.

- **`namespace`** controls where the Flux Kustomization *object itself* lives — the namespace of the `Kustomization` CR, not the namespace of the resources it reconciles. Defaults to the gitops namespace (`system-gitops`). Rarely needed; setting it also breaks `dependsOn` references, which always resolve in the gitops namespace.

```yaml
kustomize:
- name: my-app
  source: core
  path: apps/my-app
  targetNamespace: production    # reconciled resources land in `production`
```

### `destroyOnly`

A `destroyOnly` kustomization sits in the blueprint but is suppressed during `apply` / `up`. It's only applied during `destroy`, and only long enough to do its work — typically a one-shot job that needs to run before a stateful component disappears. Examples: snapshot-and-archive a database before the operator tears it down, drain a queue, deregister a load balancer.

```yaml
kustomize:
- name: db-snapshot-on-destroy
  source: core
  path: ops/snapshot
  destroyOnly: true
  dependsOn:
    - postgres
```

`destroyOnly` kustomizations apply in normal dependency order during destroy, then the regular kustomizations get torn down.

## Substitutions

Flux's `postBuild.substitute` lets a Kustomization reference variables in its manifests via `${VAR_NAME}` and have them substituted at reconcile time. Windsor materializes two layers of substitutions automatically:

**`values-common`** — a blueprint-level ConfigMap injected into every kustomization's `postBuild.substituteFrom`. Includes:

| Variable | Source |
|----------|--------|
| `CONTEXT` | active context name |
| `CONTEXT_ID` | `id` from `values.yaml` |
| `DOMAIN` | `dns.domain` |
| `BUILD_ID` | from `windsor build-id`, when set |
| `REGISTRY_URL` | `docker.registry_url` |
| `LOCAL_VOLUME_PATH` | derived from `cluster.workers.volumes` |
| `LOADBALANCER_IP_START` / `_END` / `_RANGE` | from `network.loadbalancer_ips` (skipped on `docker-desktop`) |
| anything under `substitutions.common` in `values.yaml` | user-provided |
| anything under blueprint-level `substitutions:` | user-provided |

**`values-<name>`** — a per-kustomization ConfigMap, populated from the kustomization's `substitutions:` field:

```yaml
kustomize:
- name: my-app
  path: my-app
  substitutions:
    replicas: "3"
    image_tag: "v1.4.2"
```

A manifest under `kustomize/my-app/` can then reference `${replicas}` and `${image_tag}` directly. Substitution values are converted to strings; complex types are JSON-encoded.

`substitutions:` in the user-authored `blueprint.yaml` only accepts literal values. To produce dynamic substitutions — facet expressions, `terraform_output()` calls, anything resolved at compose time — declare them in a facet under `contexts/_template/facets/`; the composer evaluates them and merges the resulting plain-string values into the kustomization's `substitutions` map. See [Blueprint Templates](templates.md).

## Context-specific patches

Files placed in `contexts/<name>/patches/<kustomization-name>/` are automatically discovered and added to the kustomization's `patches`. All `.yaml` and `.yml` files in that directory contribute one patch each.

### Strategic-merge patches

Standard Kubernetes resource YAML. Fields are merged into matching resources in the kustomization output.

```yaml
# contexts/local/patches/my-app/increase-replicas.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 5
```

### JSON 6902 patches

A Kubernetes resource header (`apiVersion`, `kind`, `metadata`) selects the target; the `patches:` field is the [RFC 6902](https://www.rfc-editor.org/rfc/rfc6902) operation list:

```yaml
# contexts/local/patches/my-app/json-patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
patches:
  - op: replace
    path: /spec/replicas
    value: 5
```

Windsor detects the format from the document layout. Patches contributed by facets and patches contributed by the context directory are concatenated in declaration order.

## Reconciliation

After `apply`, Windsor annotates each blueprint source (`GitRepository` or `OCIRepository`) with `reconcile.fluxcd.io/requestedAt` set to the current timestamp. source-controller picks this up and re-fetches the artifact immediately rather than waiting for the next interval; kustomize-controller then reconciles dependent Kustomizations through its watch on source status. Only sources are annotated — Kustomizations follow automatically.

This is annotation-based, receiver-type-agnostic, and works against any Flux installation. It is best-effort: if the cluster is unreachable, the apply still succeeds.

For workstation contexts, `git-livereload` POSTs to a Flux webhook receiver each time it commits, which provides the same fast-reconcile behavior for in-tree changes that don't go through `windsor apply`.

## Inspecting

```sh
kubectl get gitrepository -A           # source objects
kubectl get ocirepository -A           # OCI sources
kubectl get kustomizations -A          # Flux Kustomization objects
kubectl get kustomization <name> -n system-gitops -o yaml
windsor show kustomization <name>      # the rendered Kustomization Windsor will apply
windsor explain kustomize.<name>.substitutions.<key>
```

Sources and kustomizations both live in the gitops namespace (default `system-gitops`).

## See also

- [`apply`](../reference/commands/apply.md), [`destroy`](../reference/commands/destroy.md), [`plan`](../reference/commands/plan.md), [`show`](../reference/commands/show.md)
- [reference/blueprint.md](../reference/blueprint.md#kustomization) — full Kustomization schema
- [Blueprint Templates](templates.md) — facet-driven composition of kustomization fragments
- [Flux Kustomization docs](https://fluxcd.io/flux/components/kustomize/kustomizations/)
