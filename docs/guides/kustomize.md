---
title: "Kustomize"
description: "The Windsor CLI facilitates building Kubernetes systems using Flux and Kustomize."
---
# Kustomize

The Windsor CLI facilitates building Kubernetes systems by introducing a rational approach to composing Kubernetes configuration using [Flux](https://github.com/fluxcd/flux2) and [Kustomize](https://github.com/kubernetes-sigs/kustomize).

## Folder Structure
The following files and folders are relevant when working with Kubernetes in a Windsor project.

```plaintext
contexts/
└── local/
    ├── .kube/
    │   └── config
    └── blueprint.yaml
kustomize/
└── my-app/
    ├── prometheus/
    │   ├── kustomization.yaml
    │   └── service-monitor.yaml
    ├── kustomization.yaml
    ├── deployment.yaml
    └── service.yaml
```

In this example structure, the app `my-app` consists of a simple deployment and service. It also includes a [Kustomize component](https://kubectl.docs.kubernetes.io/guides/config_management/components/). The component pattern is used heavily by the Windsor project. In this example, the component adds Prometheus support.

## Blueprint
The following example `blueprint.yaml` now references `my-app`:

```
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: example-project
  description: This blueprint outlines example resources
repository:
  url: http://github.com/my-org/example-project
  ref:
    branch: main
  secretName: flux-system
sources:
- name: core
  url: github.com/windsorcli/core
  ref:
    branch: main
terraform:
- source: core
  path: cluster/talos
- source: core
  path: gitops/flux
kustomize:
- name: my-app
  path: my-app
  components:
    - prometheus
```

Each entry under `kustomize` follows the [Flux Kustomization spec](https://fluxcd.io/flux/components/kustomize/kustomizations/). As such, you may include patches and any other necessary settings for modifying the behavior of `my-app`.

When running `windsor up --install` or `windsor install`, all Kustomization resources are applied to your cluster. This involves creating [GitRepository](https://fluxcd.io/flux/components/source/gitrepositories/) resources from the corresponding `repository` and `sources`, as well as [Kustomizations](https://fluxcd.io/flux/components/kustomize/kustomizations/).

You can observe these resources on your cluster by running the following commands,

To see the GitRepository resources run:

```
kubectl get gitrepository -A
```

To see Kustomizations, run:

```
kubectl get kustomizations -A
```

You will find these all placed in the `system-gitops` namespace.

## Importing Resources
You can import Kustomize resources from remote sources. To import a component from `core`, add the following under the blueprint's `kustomize` section:

```yaml
...
kustomize:
  - name: csi
    path: csi
    source: core
    components:
      - longhorn
```

This would result in importing the `csi` resource from `core`, and specifically using the `longhorn` driver. By including the `source` field referencing `core`, this reference will be used when the Kustomization is generated on your cluster.

## Context-Specific Patches

Windsor automatically discovers and includes patches from your context directory. Patches placed in `contexts/<context>/patches/<kustomization-name>/` are automatically discovered and applied to the corresponding kustomization.

### Directory Structure

Place patch files in a directory named after the kustomization:

```plaintext
contexts/
└── local/
    ├── blueprint.yaml
    └── patches/
        └── my-app/
            ├── increase-replicas.yaml
            └── add-annotations.yaml
```

### Patch Discovery

All `.yaml` and `.yml` files in `contexts/<context>/patches/<kustomization-name>/` are automatically discovered and added to the kustomization's patches. Windsor automatically detects the patch format:

- **Strategic merge patches**: Standard Kubernetes resource YAML that will be merged into the base resources. See the [Kubernetes documentation on strategic merge patches](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/#create-apply-and-delete) for details.
- **JSON 6902 patches**: Patches that use a Kubernetes resource structure with `apiVersion`, `kind`, and `metadata` fields, but include a `patches` field (instead of `spec`) containing an array of JSON 6902 operations. See [RFC 6902](https://www.rfc-editor.org/rfc/rfc6902) for the JSON Patch specification.

### Examples

For a kustomization named `my-app`, create patches in `contexts/local/patches/my-app/`:

**Strategic Merge Patch:**

```yaml
# contexts/local/patches/my-app/increase-replicas.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 5
```

**Strategic Merge Patch with Annotations:**

```yaml
# contexts/local/patches/my-app/add-annotations.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    environment: local
    managed-by: windsor
```

**JSON 6902 Patch:**

Windsor also supports JSON 6902 patches using a Kubernetes resource structure with a `patches` field:

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
  - op: add
    path: /spec/template/metadata/annotations/environment
    value: local
```

In this format, the `apiVersion`, `kind`, and `metadata` fields identify the target resource, while the `patches` field contains an array of JSON 6902 operations (`op`, `path`, `value`). Windsor automatically extracts the target selector from the resource metadata.

These patches are automatically discovered and applied when the blueprint is processed. Patches defined in features (via the blueprint's `features/` directory) are merged with context-specific patches, with all patches being applied in order.

For more information on patch formats, see:
- [Kubernetes Kustomize documentation](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/)
- [RFC 6902 - JSON Patch](https://www.rfc-editor.org/rfc/rfc6902)

<div>
  {{ footer('Environment Injection', '../environment-injection/index.html', 'Local Workstation', '../local-workstation/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../environment-injection/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../local-workstation/index.html'; 
  });
</script>
