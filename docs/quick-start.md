---
title: "Quick Start Guide"
description: "A step-by-step guide to launching the Windsor environment in your project."
---
# Quick Start Guide
This guide will walk you through launching the windsor environment in your project. At the end of this guide, you should be running a local Kubernetes cluster with a single worker and controlplane.

This guide is expected to run optimally on a machine with 8 CPU cores, 8GB of RAM, and 60GB of free storage space available.

It is assumed you have installed the Windsor CLI and configured `windsor hook` in your shell. Please see the [Installation](./install.md) page for instructions.

### Install tool dependencies
To fully leverage the Windsor environment, you will need several tools installed on your system. You may install these tools manually or using your preferred tools manager (_e.g._ Homebrew). The Windsor project recommends [aqua](https://github.com/aquaproj/aqua). For your convenience, we have provided a sample setup file for aqua. Place this file in the root of your project.

To validate your toolchain, run:

```
windsor check
```

Create an `aqua.yaml` file in your project's root directory with the following content:
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/aquaproj/aqua/main/json-schema/aqua-yaml.json
# aqua - Declarative CLI Version Manager
# https://aquaproj.github.io/
# checksum:
#   enabled: true
#   require_checksum: true
#   supported_envs:
#   - all
registries:
  - type: standard
    ref: v4.285.0
packages:
- name: hashicorp/terraform@v1.10.3
- name: siderolabs/talos@v1.9.1
- name: kubernetes/kubectl@v1.32.0
- name: docker/cli@v27.4.1
- name: docker/compose@v2.32.1
```

To install the tools specified in `aqua.yaml`, run:
```bash
aqua install
```

### Initialize the "local" context
If you have not done so, be sure to initialize a git repository in the root of your project.

```sh
git init
```

The windsor tool will create a few folders in your project. In particular, it will create a folder called `contexts/` where your context configurations will reside. Initialize windsor with the docker vm driver by running:

```sh
windsor init local
```

Verify that the default 'local' context was selected:

```sh
windsor context get
```

### Start the environment
Start the local environment and install the default local blueprint, run:

```sh
windsor up --install
```

This command will start appropriate docker containers, run kubernetes nodes and support services with docker compose, and bootstrap your cluster using Terraform. It can take up to 5 minutes to fully launch, so be patient!

### Verify the environment
Display the list of Kubernetes nodes:

```sh
kubectl get nodes
```

### Wait and watch
Consider what is now occurring on your local development workstation. A kubernetes environment, including a variety of supporting services are being installed. These include storage, networking, DNS, and a variety of foundational supporting services. As such, it can take a while to load! So, be patient. Make a cup of tea, catch up on some reading. It typically takes around 5 minutes on a modern Apple M3 to spin up the full environment.

If you'd like to watch, you can run:

```
kubectl get kustomizations -A --watch
```

This will show you live updates of each feature being deployed on your cluster. Additionally, you may be interested in the status of various helm charts. You can check the status of these by running:

```
kubectl get helmrelease -A
```

Or, check the status of all your pods:

```
kubectl get pods -A
```

It is also recommended to familiarize yourself with the `system-` namespaces. Each feature on your cluster is deployed in to a generic namespace. List these by running:

```
kubectl get namespaces -A
```

### Tear down the environment
Windsor provides a command to help you tear down your development environment. To destroy the cluster and support services by running:

```sh
windsor down --clean
```

<div>
  {{ footer('Installation', '../install/index.html', 'Contexts', '../guides/contexts/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../install/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../guides/contexts/index.html'; 
  });
</script>
