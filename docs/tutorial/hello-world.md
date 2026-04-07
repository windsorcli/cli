---
title: "Hello, World!"
description: "Building a 'Hello, World!' page on a local cloud with the Windsor CLI"
---
# Hello, World!

This tutorial demonstrates building and deploying a simple "Hello, World!" web application to your local Windsor cluster. You'll learn how to build a container image, tag it using Windsor's build ID feature, push it to the local registry, and deploy it using a local Kustomize component.

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

## Build a containerized web service

Create a simple Express.js web service. Add the following files to your project:

Create `Dockerfile`:

```dockerfile
FROM node:22-alpine

WORKDIR /usr/src/app
COPY package.json package-lock.json server.js ./
RUN npm install
EXPOSE 8080
CMD ["node", "server.js"]
```

Create `server.js`:

```js
const express = require('express');
const app = express();

app.get('/', (req, res) => {
  res.send(`
    <!DOCTYPE html>
    <html>
    <head><title>Hello, World!</title></head>
    <body>
      <h1>Hello, World!</h1>
      <p>Welcome to Windsor!</p>
    </body>
    </html>
  `);
});

const PORT = 8080;
app.listen(PORT, () => {
  console.log(`Server is running on http://localhost:${PORT}`);
});
```

Create `package.json`:

```json
{
  "name": "hello-world",
  "version": "1.0.0",
  "description": "Hello World web service",
  "main": "server.js",
  "scripts": {
    "start": "node server.js"
  },
  "dependencies": {
    "express": "^4.21.1"
  },
  "license": "ISC"
}
```

## Tag and push to local registry

Windsor provides special features for local development: the `REGISTRY_URL` environment variable points to your local registry, and `BUILD_ID` provides unique build identifiers for artifact tagging.

### Generate a build ID

Windsor's build ID feature generates unique identifiers in the format `YYMMDD.RANDOM.#`. The `BUILD_ID` environment variable is automatically available through Windsor's environment injection. Generate a new build ID:

```bash
windsor build-id --new
```

The build ID is now available as the `BUILD_ID` environment variable and will be used in your Docker commands.

### Build and tag the image

Build your Docker image and tag it using both the build ID and the local registry URL:

```bash
# Build the image
docker build -t hello-world:$BUILD_ID .

# Tag for local registry with build ID
docker tag hello-world:$BUILD_ID ${REGISTRY_URL}/hello-world:$BUILD_ID
```

### Push to local registry

Push the image to your local registry:

```bash
docker push ${REGISTRY_URL}/hello-world:$BUILD_ID
docker push ${REGISTRY_URL}/hello-world:latest
```

The `REGISTRY_URL` environment variable is automatically set by Windsor and points to your local registry (typically `registry.test:5000`). This registry is accessible from your Kubernetes cluster, allowing you to use locally built images in your deployments.

## Create a local Kustomize component

Create a local Kustomize component for your hello-world application. This component will be stored in your project's `kustomize/` directory.

Create the directory structure:

```bash
mkdir -p kustomize/hello-world
```

Create `kustomize/hello-world/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-world
  namespace: hello-world
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hello-world
  template:
    metadata:
      labels:
        app: hello-world
    spec:
      containers:
      - name: hello-world
        image: ${REGISTRY_URL}/hello-world:${BUILD_ID}
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "200m"
```

Create `kustomize/hello-world/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: hello-world
  namespace: hello-world
spec:
  selector:
    app: hello-world
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

Create `kustomize/hello-world/namespace.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: hello-world
```

Create `kustomize/hello-world/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - namespace.yaml
  - deployment.yaml
  - service.yaml
```

Create the ingress component directory:

```bash
mkdir -p kustomize/hello-world/ingress
```

Create `kustomize/hello-world/ingress/ingress.yaml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: hello-world-ingress
  namespace: hello-world
spec:
  rules:
  - host: hello-world.${DOMAIN:-test}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: hello-world
            port:
              number: 80
```

Create `kustomize/hello-world/ingress/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1alpha1
kind: Component
resources:
  - ingress.yaml
```

The ingress uses the `${DOMAIN:-test}` substitution, which is automatically provided by Windsor as a postBuild substitution variable. This allows the ingress to use your configured domain (or default to `test` if not set).

## Reference the component in your blueprint

Add the hello-world kustomization to your `contexts/local/blueprint.yaml`. Add it to the `kustomize` section, typically after the `ingress` kustomization since hello-world depends on it:

```yaml
kustomize:
  # ... existing kustomizations from core ...
  - name: ingress
    path: ingress
    source: core
    # ... ingress configuration ...
  - name: hello-world
    path: hello-world
    dependsOn:
      - ingress
    components:
      - ingress
  # ... other kustomizations ...
```

The `path: hello-world` references the `kustomize/hello-world/` directory in your project. Since no `source` is specified, Windsor uses the local kustomize directory. The `dependsOn: [ingress]` ensures the ingress controller is deployed before hello-world, and `components: [ingress]` includes the ingress component we created.

## Deploy to your cluster

Deploy the hello-world application to your local cluster:

```bash
windsor install
```

This will apply the Kustomization resource to your cluster. Flux will process the kustomization and deploy your application.

## Validate your resources

Check the status of your hello-world deployment:

```bash
kubectl get all -n hello-world
```

You should see something like:

```
NAME                              READY   STATUS    RESTARTS   AGE
pod/hello-world-7d4f8b9c6-abc123  1/1     Running   0          30s

NAME                 TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE
service/hello-world  ClusterIP   10.43.123.45    <none>        80/TCP    30s
```

## Access your application

The hello-world service is deployed with an Ingress resource that makes it accessible via your local domain. Once deployed, you can access it at:

```
http://hello-world.test:8080
```

You can also use port forwarding for local access:

```bash
kubectl port-forward -n hello-world service/hello-world 8081:80
```

Then visit http://localhost:8081

## Inspect the deployment

Check the deployment details:

```bash
kubectl describe deployment hello-world -n hello-world
```

Pay attention to the `image` field in the pod template. It should reference `${REGISTRY_URL}/hello-world:${BUILD_ID}`, which Flux will substitute with the actual registry URL and build ID values when applying the kustomization.

The `REGISTRY_URL` and `BUILD_ID` variables are automatically provided by Windsor as post-build substitution variables, making it easy to reference locally built images in your Kubernetes manifests.

<div>
  {{ footer('Sharing Blueprints', '../../guides/sharing/index.html', 'Trusted Folders', '../../security/trusted-folders/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../../guides/sharing/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../security/trusted-folders/index.html'; 
  });
</script>
