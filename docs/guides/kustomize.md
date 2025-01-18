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
  - name: system-csi
    path: system-csi
    source: core
    components:
      - longhorn
```

This would result in importing the `system-csi` resource from `core`, and specifically using the `longhorn` driver. By including the `source` field referencing `core`, this reference will be used when the Kustomization is generated on your cluster.

<div>
  {{ footer('Terraform', '../../guides/terraform/index.html', 'Hello World', '../../tutorial/hello-world/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../guides/terraform/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/hello-world/index.html'; 
  });
</script>
