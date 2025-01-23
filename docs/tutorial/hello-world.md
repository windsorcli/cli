# Hello, World!
We'll begin with a simple "Hello, World!" demonstration to get you started with Windsor. In this tutorial, you will create a simple Pod spec that serves up the message "Hello, World!" running on the local cluster.

It is assumed you have already been through the [quick start](../quick-start.md). You have created a repository, and are able to access a local cluster. To verify this, run:

```
kubectl get nodes
```

You should see something like:

```
NAME             STATUS   ROLES           AGE   VERSION
controlplane-1   Ready    control-plane   1h    v1.31.4
worker-1         Ready    <none>          1h    v1.31.4
```

## Create the pod spec
Hashicorp provides the `http-echo` Docker image, which is suitable for our purposes. Copy the following and save it to `kustomize/hello-world.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hello-world
  namespace: default
spec:
  containers:
  - name: hello-world
    image: hashicorp/http-echo
    args:
    - "-text=Hello, World!"
    ports:
    - containerPort: 5678
```

You must also modify the `kustomization.yaml` file to reference the pod spec. This file should now read:

```
resources:
  - hello-world.yaml
```

## Check or modify the blueprint.yaml
Have a look at the `contexts/local/blueprint.yaml` file. It should look something like:

```
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: local
  description: This blueprint outlines resources in the local context
repository:
  url: http://git.test/git/tmp
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
- name: local
  path: ""
```

Let's focus on the `kustomize` section:

```
kustomize:
- name: local
  path: ""
```

If this is in your `blueprint.yaml`, your blueprint already includes the `hello-world.yaml` example. The `path` field is always relative to the `kustomize/` folder. So, this configuration results in the `kustomization.yaml` residing at `kustomize/kustomization.yaml` being processed and your pod spec loaded.

## Install the Hello World Pod

To install the Hello World pod, run the following command:

```
windsor install
```

## Validate your resources
The Windsor environment runs a [livereload gitops](../guides/local-workstation.md#local-gitops) server. As such, the Flux Kustomization operator should have begun sync'ing your new pod spec. You can verify this by running:

```
kubectl get pods
```

You should see something like:

```
NAME          READY   STATUS    RESTARTS   AGE
hello-world   1/1     Running   0          10s
```

You can connect to this pod by running:

```
kubectl port-forward pod/hello-world 5678:5678
```

If you visit [http://localhost:5678](http://localhost:5678) in your browser, you should see the message "Hello, World!"

If your pod does not appear, you can troubleshoot it by looking at the kustomization with:

```
kubectl get kustomization -A
```

As well as the git repository resources:

```
kubectl get gitrepository -A
```

<div>
  {{ footer('Kustomize', '../../guides/kustomize/index.html', 'Trusted Folders', '../../security/trusted-folders/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../guides/kustomize/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../security/trusted-folders/index.html'; 
  });
</script>
