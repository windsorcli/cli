---
title: "Local Workstation"
description: "How Windsor runs a local Kubernetes cluster, DNS, registries, and a git mirror on your machine."
---
# Local workstation

A workstation context runs a VM-backed Kubernetes cluster on your machine, with DNS, container registries, and a local git mirror configured to mimic production. Workstation contexts are the only place `windsor up` and `windsor down` apply — every other context (`staging`, `prod`, etc.) drives directly with `apply` / `destroy`.

`windsor init local` scaffolds a workstation context and `windsor up` brings it up:

```sh
git init
windsor init local
windsor up --wait
```

## Workstation configuration

After `init`, two files describe the context:

```yaml
# contexts/local/values.yaml — user-authored
dev: true
dns:
  domain: test
id: w2g5rk7d
```

```yaml
# .windsor/contexts/local/workstation.yaml — system-managed, ephemeral
platform: docker
workstation:
  arch: arm64
  dns:
    address: 127.0.0.1
  runtime: docker-desktop
```

`workstation.yaml` is system-managed — Windsor writes it during `init` / `up` based on `--vm-driver`, `--platform`, and the host architecture. Treat it as ephemeral; do not check it in. The file is regenerated whenever flags or platform change.

Top-level keys:

| Key | Meaning |
|-----|---------|
| `platform` | Workstation platform: `docker`, `metal`, `incus`, `aws`, `azure`, `gcp`, `none`. Drives backend defaults and which terraform components are picked. |
| `workstation.runtime` | VM/container runtime: `docker-desktop`, `colima`, `docker` (host docker). |
| `workstation.arch` | VM architecture (`arm64` or `amd64`); inferred from host `GOARCH` when unset. |
| `workstation.address` | VM IP, set by Windsor after VM boot. |
| `workstation.dns.address` | DNS service IP that the host resolver should be pointed at. |

## Anatomy of a workstation

```mermaid
flowchart TB
    subgraph Host["Host machine"]
        Shell["Shell + windsor hook<br/>(env injected each prompt)"]
        CLIs["windsor · kubectl · docker · terraform"]
        FS[("Project filesystem<br/>contexts/local/ · .windsor/")]
        Resolver["Host DNS resolver<br/>*.test → dns.test"]
    end

    subgraph Docker["Docker daemon — windsor-local bridge · 10.5.0.0/16"]
        direction TB
        subgraph Support["Docker support services (containers)"]
            direction LR
            DNS["dns.test · 10.5.0.3<br/>CoreDNS for *.test"]
            Reg["Registry mirrors<br/>registry.test · gcr.test · ghcr.test<br/>quay.test · registry-1.docker.test · registry.k8s.test"]
            Git["git.test · 10.5.0.6<br/>git-livereload (mirrors project)"]
        end
        subgraph Talos["controlplane-1.test · 10.5.0.2<br/>Talos container (privileged)"]
            subgraph K8s["Kubernetes (running on Talos)"]
                direction LR
                Flux["Flux"]
                Apps["Workloads installed by Flux<br/>kyverno · openebs · ingress<br/>cert-manager · in-cluster CoreDNS<br/>BookInfo demo"]
            end
        end
    end

    Shell --> CLIs
    CLIs -.->|DOCKER_HOST| Docker
    CLIs -.->|KUBECONFIG → :6443| Talos
    Resolver -.->|UDP/53| DNS
    Flux -.->|polls| Git
    Git -.->|webhook on commit| Flux
    FS -.->|bind-mount| Git
```

The Docker daemon is on the host with `docker-desktop` / `docker`, and inside the Lima VM with `colima` / `colima-incus`. Either way, every container — support services and Talos alike — joins the `windsor-local` bridge, so they address each other on `10.5.0.x`.

Key flows:

- The shell hook re-evaluates `windsor env` each prompt, exporting `KUBECONFIG`, `DOCKER_HOST`, `TALOSCONFIG`, `REGISTRY_URL`, etc., so plain `kubectl` and `docker` commands target the workstation.
- `dns.test` (CoreDNS, on the docker network) resolves `*.test` names. The host resolver is pointed at it by `windsor configure network`. Pods inside Kubernetes have a separate cluster-scoped CoreDNS for service DNS.
- `git-livereload` runs as a docker container, not in the cluster. It mirrors your working tree onto an HTTP git server at `git.test` and pings the Flux webhook on each change. Flux reconciles from `git.test`.
- `${project_root}/.volumes` bind-mounts onto the Talos container, so PVCs surface as host directories under your project.

## VM drivers

| Driver | Platform | When to use |
|--------|----------|-------------|
| `docker-desktop` | macOS / Linux / Windows | Lightest setup; no separate VM. Some networking features fall back to host port-forwarding. |
| `docker` | Linux | Use the host's docker daemon directly. |
| `colima` | macOS / Linux | Full VM via Lima. Required for Layer-2 load balancing, full IP-range networking, block-device emulation. |
| `colima-incus` | macOS / Linux | Same as `colima`, but provisions Incus inside the VM for cluster-of-VMs workloads. Requires `limactl` ≥ 2.0.3. |

Pick at init time:

```sh
windsor init local --vm-driver=colima
windsor init local --vm-driver=docker-desktop
windsor init local --vm-driver=colima-incus
```

`--vm-driver` writes `workstation.runtime` (with `colima-incus` aliased to `colima` plus `platform: incus`). When `--platform` isn't provided, Windsor infers it from the driver: `colima` and `docker-desktop` → `docker`; `colima-incus` → `incus`.

### Feature comparison

| Feature | Full virtualization (`colima`, `colima-incus`) | Light virtualization (`docker-desktop`, `docker`) |
|---------|------------------------------------------------|---------------------------------------------------|
| DNS | resolves to in-cluster service IPs | resolves to `127.0.0.1` with port-forwarding |
| Kubernetes cluster | container or VM nodes | container nodes only |
| Network emulation | full IP range, Layer-2 load balancing | `localhost` with port-forwarding and NodePort |
| Device emulation | filesystem and block devices | filesystem only |

## Cluster topology

Windsor runs Kubernetes via [Talos](https://github.com/siderolabs/talos). Configure the cluster in `contexts/<name>/values.yaml`:

```yaml
cluster:
  driver: talos
  controlplanes:
    count: 1
    cpu: 4
    memory: 4
  workers:
    count: 1
    cpu: 4
    memory: 8
    hostports:
    - 80:30080/tcp
    - 443:30443/tcp
    volumes:
    - ${project_root}/.volumes:/var/mnt/local
```

When fields are unset, Windsor derives sensible defaults. The default topology is **single-node**: one schedulable controlplane and zero workers, where the controlplane runs both control-plane and workload pods.

| Topology | Default `cpu` | Default `memory` (GB) |
|----------|---------------|------------------------|
| Single schedulable controlplane (workers count 0) | 8 | 12 |
| Dedicated controlplane (workers count > 0) | 4 | 4 |
| Worker | 4 | 8 |

`controlplanes.schedulable` is automatically `true` when `workers.count == 0` and `controlplanes.count == 1`.

`hostports` are container-to-host port mappings (only applied for `docker-desktop` and `docker`). `volumes` are bind-mounts on the worker filesystem — typically used to expose `${project_root}/.volumes/` to the cluster as PVC storage.

### VM rightsizing

For Colima, the VM size is derived from cluster topology rather than fixed:

```
cpu    = (controlplanes.count × controlplanes.cpu) + (workers.count × workers.cpu) + 1
memory = (controlplanes.count × controlplanes.memory) + (workers.count × workers.memory) + 3 GB
```

The `+1 / +3` overhead covers the Ubuntu base and Docker services running inside the VM. Floors of 2 vCPU and 4 GB always apply. If the calculated size exceeds the host's physical resources minus a 4 GB reserve, Windsor warns and you can tune individual node values in `values.yaml` or pass `--vm-cpu` / `--vm-memory` to `init`.

When `cluster.driver` is unset, Windsor falls back to half of the host's CPU and memory.

## DNS

The local DNS resolver routes the configured domain (default: `test`) to a CoreDNS instance running in the cluster. Services like `http://aws.test`, `http://git.test`, `http://registry.test`, and any in-cluster service DNS resolve through it.

```yaml
# contexts/local/values.yaml
dns:
  domain: test       # IANA-reserved for testing
```

`.test` is reserved by IANA for testing and is the recommended choice. Override `dns.domain` if you must.

### configure network

`windsor configure network` wires the host's resolver and routes for the active workstation. It runs automatically after the workstation Terraform component applies during `up`, but you can re-run it manually if DNS or routing drifts:

```sh
windsor configure network --dns-address=10.5.0.10
```

`--dns-address` is the DNS service IP — typically taken from the workstation Terraform component's outputs. Run from the project root; the trusted-folder gate applies.

### Verifying DNS

```sh
# Full virtualization → in-cluster IP
dig @dns.test registry.test
# ;; ANSWER SECTION:
# registry.test.   3600   IN  A  10.5.0.3

# Docker Desktop → loopback
dig @dns.test registry.test
# ;; ANSWER SECTION:
# registry.test.   3600   IN  A  127.0.0.1
```

## Container registries

The workstation runs pull-through mirrors for major public registries plus a local generic registry:

| Registry | Endpoint | Purpose |
|----------|----------|---------|
| `gcr.io` mirror | `http://gcr.test:5000` | Google Container Registry |
| `ghcr.io` mirror | `http://ghcr.test:5000` | GitHub Container Registry |
| `quay.io` mirror | `http://quay.test:5000` | Red Hat Quay |
| Docker Hub mirror | `http://registry-1.docker.test:5000` | docker.io |
| `registry.k8s.io` mirror | `http://registry.k8s.test:5000` | Kubernetes upstream |
| Local | `http://registry.test:5000` | Generic local registry |

Add private mirrors via `docker.registries`:

```yaml
docker:
  registries:
    1234567890.dkr.ecr.us-east-1.amazonaws.com:
      remote: https://1234567890.dkr.ecr.us-east-1.amazonaws.com
```

`REGISTRY_URL` in your shell points at the active local generic registry — use it directly:

```sh
docker tag my-image:latest ${REGISTRY_URL}/my-image:latest
docker push ${REGISTRY_URL}/my-image:latest
```

Cache lives in `.windsor/.docker-cache` and is persisted across `up`/`down`. On Docker Desktop, only `http://registry.test:5002` is exposed (single mirror, port-forwarded).

## AWS Localstack

Localstack provides a local AWS API. Enable it inside the `aws:` block:

```yaml
aws:
  region: us-east-2
  localstack:
    enabled: true
    services:
      - s3
      - dynamodb
      - sqs
```

When `up` is next run, Localstack starts and the AWS endpoint becomes available at `http://aws.test:4566`. `aws.endpoint_url` is automatically pointed at it for in-shell `aws` invocations (see [Environment reference](../reference/environment.md)).

## Local git mirror

When `dev: true`, Windsor runs [git-livereload](https://github.com/windsorcli/git-livereload). Saves to your local files surface as commits to `http://git.test/git/<project>`, which is what Flux subscribes to in the local context — so pushing to a remote isn't required for the local GitOps loop.

A Flux webhook is also wired up so changes reconcile faster than the configured `interval`:

```sh
curl -X POST http://worker-1.test:30292/hook/<token>
```

git-livereload triggers it automatically after each filesystem change.

## Build IDs

Windsor maintains a build identifier for tagging artifacts during local development. Stored in `.windsor/.build-id`, exposed as `BUILD_ID` and as a Flux substitution variable.

```sh
windsor build-id            # current
windsor build-id --new      # rotate
```

Format: `YYMMDD.RRR.N` — date, three random digits for collision avoidance, and a same-day sequence counter.

## Verifying

```sh
kubectl get nodes               # nodes Ready
kubectl get kustomizations -A   # all Ready, no Pending
windsor explain workstation.address    # VM address
windsor show values             # effective context values
```

The default blueprint installs Istio's [BookInfo](https://istio.io/latest/docs/examples/bookinfo/) demo. Visit `http://bookinfo.test:8080` (or `:80` if hostports are mapped to the standard web ports).

## See also

- [Lifecycle](lifecycle.md) — `up` / `down` phase boundaries
- [Environment reference](../reference/environment.md) — `DOCKER_HOST`, `KUBECONFIG`, `TALOSCONFIG`, `REGISTRY_URL`
- [`configure`](../reference/commands/configure.md), [`up`](../reference/commands/up.md), [`down`](../reference/commands/down.md)
- [Reference: Configuration](../reference/configuration.md) — full schema for `workstation`, `cluster`, `dns`, `docker`
