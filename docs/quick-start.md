---
title: "Quick Start"
description: "Bring up a local Kubernetes cluster with Windsor in under 10 minutes."
---
# Quick Start

This walk-through brings up a single-node Kubernetes cluster on your machine using the default `local` blueprint. Plan for ~5–10 minutes on a recent Mac, longer on slower hardware.

**Recommended:** 8 CPU cores, 8 GB RAM, 60 GB free disk. Smaller machines work but the install will be slow.

This guide assumes Windsor is installed and `windsor hook` is configured in your shell. See [Installation](./install.md).

## 1. Install tool dependencies

Windsor invokes external tools (terraform, kubectl, talos, docker, compose). The recommended way to manage them is [aqua](https://aquaproj.github.io/).

Create `aqua.yaml` in your project root:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/aquaproj/aqua/main/json-schema/aqua-yaml.json
registries:
  - type: standard
    ref: v4.499.0
packages:
  - name: hashicorp/terraform@v1.10.3
  - name: siderolabs/talos@v1.13.0
  - name: kubernetes/kubectl@v1.32.0
  - name: docker/cli@v27.4.1
  - name: docker/compose@v2.32.1
```

```sh
aqua install
```

## 2. Initialize a project

If you don't have a git repo already, initialize one — Windsor uses git as the project anchor.

```sh
git init
windsor init local
```

`windsor init local` creates the `contexts/local/` directory, writes `windsor.yaml`, and adds the directory to your trusted-folders list. The default `local` context uses Docker as the VM driver and the bundled local blueprint.

Verify the toolchain is wired up:

```sh
windsor check
windsor get context     # → local
```

## 3. Bring up the cluster

```sh
windsor up --wait
```

`up` starts the VM, runs Terraform to provision the workstation infrastructure, then installs the Flux blueprint into the cluster. `--wait` blocks until every kustomization reports ready. Expect ~5 minutes on a fast Mac.

While it runs, you can watch progress in another shell:

```sh
kubectl get kustomizations -A --watch
kubectl get helmrelease -A
kubectl get pods -A
```

When `up` returns, you have a working cluster.

## 4. Inspect the environment

```sh
kubectl get nodes
kubectl get namespaces       # note the system-* namespaces
windsor show blueprint       # the fully composed blueprint
windsor show values          # the effective context values
```

`windsor explain <path>` traces a single value back to where it came from in the composition:

```sh
windsor explain terraform.cluster.inputs.cluster_endpoint
```

## 5. Tear it down

To destroy the cluster and stop the VM:

```sh
windsor destroy --confirm=local
windsor down
```

`destroy` removes the live infrastructure (Terraform state and Flux kustomizations). `down` stops the VM and clears local context artifacts. `--confirm=local` is the non-interactive equivalent of typing `local` at the destroy prompt.

## See also

- [Lifecycle](guides/lifecycle.md) — how `init`, `up`, `apply`, `destroy`, `down` fit together
- [Local Workstation](guides/local-workstation.md) — VM driver options, networking, registries
- [Hello, World!](tutorial/hello-world.md) — deploy a real app onto the cluster
