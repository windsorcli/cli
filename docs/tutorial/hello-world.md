---
title: "Hello, World!"
description: "Building a 'Hello, World!' page on a local cloud with the Windsor CLI"
---
# Hello, World!
We'll begin with a simple "Hello, World!" demonstration to get you started with Windsor. In this tutorial, you will use the core blueprint's static website demo to serve a simple static HTML page. You should have some flavor of `npm` or `yarn` installed.

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
Let's first begin by building the application service. This service will involve an express.js web service with livereload configured. Add the following files to your project ([source](https://github.com/windsorcli/core/tree/main/kustomize/demo/static/assets)).

Create `Dockerfile`:

```dockerfile
FROM node:22-alpine

WORKDIR /usr/src/server
COPY package.json package-lock.json server.js ./
RUN npm install
EXPOSE 8080
CMD ["node", "server.js"]
```

Create `server.js`:

```js
const express = require('express');
const livereload = require('livereload');
const connectLivereload = require('connect-livereload');
const path = require('path');

// Create a livereload server
const liveReloadServer = livereload.createServer();
liveReloadServer.watch('/usr/src/app');

// Create an express app
const app = express();

// Use connect-livereload middleware
app.use(connectLivereload());

// Serve static files from the '/usr/src/app' directory
app.use(express.static('/usr/src/app'));

// Start the server
const PORT = 8080;
app.listen(PORT, () => {
  console.log(`Server is running on http://localhost:${PORT}`);
});
```

Create `package.json`:

```json
{
  "name": "demo-static",
  "version": "1.0.0",
  "description": "Demo project to run server.js using express and livereload",
  "main": "server.js",
  "scripts": {
    "start": "node server.js"
  },
  "dependencies": {
    "connect-livereload": "^0.6.1",
    "express": "^4.21.1",
    "livereload": "^0.9.3"
  },
  "author": "",
  "license": "ISC"
}
```

Create `docker-compose.yaml`:

```
services:
  demo:
    build:
      context: .
    image: ${REGISTRY_URL}/demo:latest
```

Build and push the image:

```
docker-compose build demo && \
docker-compose push demo
```

## Configure the static website demo component
To enable the static website demo, add the following to `contexts/local/blueprint.yaml`:

```
kustomize:
...
- name: demo
  path: demo
  dependsOn:
  - ingress-base
  force: true
  components:
  - bookinfo
  - bookinfo/ingress
  - static
  - static/ingress
...
```

You should have added the lines `static` and `static/ingress` to the `demo` block. This will enable the static demo as well as configure its ingress. To deploy these to your local cluster, run:

```sh
windsor install
```

## Validate your resources
Check the status of all resources in the `demo-static` namespace by running:

```
kubectl get all -n demo-static
```

You should see something like:

```
NAME                          READY   STATUS    RESTARTS   AGE
pod/website-5b8b697588-g8zrf  1/1     Running   0          10s
```

As well as a service and ingress:

```
kubectl get svc -n demo-static
```

```
kubectl get ingress -n demo-static
```

## Create static content
Check your environment variables by running `windsor env`. You should see one with the name `PV_DEMO_STATIC_CONTENT`. This path points to a folder under `.volumes`, such as `pvc-420ddd3a-e7fd-455a-a2a4-a3388638777c`. This is where you should place your static content, or direct the output of your build command. Place this file at `${PV_DEMO_STATIC_CONTENT}/index.html`:

```
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Hello World</title>
</head>
<body>
    <h1>Hello, World!</h1>
    <p>Welcome to the static website demo.</p>
</body>
</html>
```

Assuming everything is working as expected, you should be able to visit http://static.test:8080 and see your content appear.

## Inspect the environment
Let's have a look at what's been created. Run the following:

```
kubectl describe deployment website -n demo-static
```

Pay attention to the `image` tag. It should read `registry.test/demo:latest`. This image was pulled from the local registry that you pushed your image to in a previous step.

Inspect the persistent volume by running:

```
kubectl get pvc -n demo-static
```

You should see a persistent volume claim with a name matching the folder under `.volumes/`. This volume is bind mounted to your host, such that it shares content with pods under development.

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
