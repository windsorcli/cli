# Windsor CLI

[![CI](https://img.shields.io/github/actions/workflow/status/windsorcli/cli/ci.yaml)](https://github.com/windsorcli/cli/actions)

Windsor is a tool for composing, versioning, and distributing infrastructure blueprints.

A blueprint is a Terraform stack plus Kubernetes manifests, parameterized by conditional fragments called *facets*. The same blueprint retargets across substrates — a laptop, bare metal, EKS, or AKS — by varying which facets match. Blueprints publish as versioned OCI artifacts.

Composition compiles to plain Terraform and Kustomize. Nothing proprietary runs in the deployed infrastructure — Windsor is only present at build time. Open source under [MPL 2.0](LICENSE). Runs on macOS, Linux, and Windows.

See [windsorcli.dev](https://windsorcli.dev) for documentation.

## Install

| Method | Command |
| --- | --- |
| Homebrew (macOS, Linux) | `brew tap windsorcli/cli && brew install windsor` |
| Chocolatey (Windows) | `choco install windsor` |

Manual binary downloads: [windsorcli.dev/docs/cli/installation](https://windsorcli.dev/docs/cli/installation).

```bash
windsor version
```

### Shell hook

Windsor manages per-context environment (`KUBECONFIG`, AWS and Azure credentials, registry endpoints, `TALOSCONFIG`). Add the hook so `kubectl`, `terraform`, and the rest of your toolchain see the right cluster:

```bash
eval "$(windsor hook bash)"     # ~/.bashrc
eval "$(windsor hook zsh)"      # ~/.zshrc
```

PowerShell:

```powershell
Invoke-Expression (& windsor hook powershell)
```

Without the hook, plain `kubectl` will not connect to the cluster Windsor brought up.

## Quick start

The default blueprint is [Core](https://github.com/windsorcli/core), a Kubernetes platform. You need [Terraform](https://developer.hashicorp.com/terraform/install) and Docker Desktop or [Colima](https://github.com/abiosoft/colima) on macOS.

```bash
git init
windsor init local
windsor up
```

`windsor up` provisions the substrate, boots a Talos cluster in Docker, and hands off to Flux to reconcile Core. A few minutes on a modern laptop.

```bash
windsor exec -- kubectl get pods -A
```

Stop and clean up:

```bash
windsor down
```

## Concepts

### Contexts

A context is a named target — a laptop VM, a bare-metal cluster, an AWS account, or an Azure subscription. Each context has its own configuration under `contexts/<name>/`, its own credentials, and its own kubeconfig if a cluster is involved.

```bash
windsor set context local
windsor set context aws-prod
```

Secrets resolve through [SOPS](https://github.com/getsops/sops)-encrypted files or [1Password](https://developer.1password.com/docs/cli/) vaults. The shell hook re-points `kubectl`, `terraform`, and other tools when the context changes.

### Blueprints

A blueprint is a Terraform stack plus optional Kubernetes manifests, packaged as a versioned OCI artifact. Windsor does not ship a blueprint of its own; the current context references one, and Windsor composes it.

The reference blueprint is [Core](https://github.com/windsorcli/core) — a Kubernetes platform that runs the same way on a laptop, on bare metal, on EKS, and on AKS.

Anyone can publish a blueprint. The ecosystem is decentralized by design — extend Core, replace it, or build something unrelated and reference it from a context.

### Intent

A context's `values.yaml` describes intent: a compact, schema-validated description of what the operator wants. The schema lives with the blueprint, so each blueprint defines its own input vocabulary. Facets translate that intent into Terraform inputs and Kustomize overlays for the chosen substrate. The same intent retargets across substrates because the translation, not the values, varies.

### Facets

A facet is a conditional fragment of a blueprint. It declares a `when` expression and the Terraform stacks and Kustomize entries it contributes when the condition holds:

```yaml
kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws

when: platform == 'aws'

terraform:
  - name: network
    path: network/aws-vpc
    dependsOn:
      - backend
    inputs:
      cidr_block: ${network_effective.cidr_block}
      domain_name: ${dns.private_domain ?? ''}
```

Facets compose in a defined order so platform-, option-, and addon-level choices stack predictably. Later facets can extend or override earlier ones. Expressions are evaluated by [expr](https://github.com/expr-lang/expr).

## Auditable composition

Every blueprint Windsor composes can be inspected without applying it.

```bash
windsor show blueprint                                       # full rendered Terraform and Kustomize
windsor explain terraform.network.inputs.cidr_block          # trace where a value came from
windsor test                                                 # run the blueprint's composition tests
```

`windsor explain` walks from a rendered value back to the facet that produced it, the expression that evaluated, and the inputs that fed it. There is no opaque template stage.

## Lifecycle

| Command | Purpose |
| --- | --- |
| `windsor init [context]` | Create or switch a context. |
| `windsor check` | Verify required tools and versions. |
| `windsor up` | Provision infrastructure, boot the substrate, install the blueprint. |
| `windsor bootstrap [context]` | `up` for staging and production. Manages two-phase Terraform backend init when the backend itself is being created. |
| `windsor exec <cmd>` | Run a command with the current context's environment. |
| `windsor show blueprint` | Print the rendered blueprint. |
| `windsor explain <path>` | Trace a rendered value back to its source. |
| `windsor test` | Run the blueprint's static composition tests. |
| `windsor down [--clean]` | Stop the local environment. |
| `windsor destroy <component>` | Destroy a specific Terraform or Kustomize component. |

`windsor up` is the entry point for local; `windsor bootstrap` for staging and production. Upgrades happen the same way: change the blueprint or its inputs, then re-apply.

## Distribution

Bundle the current project and push it to any OCI-compatible registry:

```bash
windsor bundle
windsor push ghcr.io/your-org/your-blueprint:v1.0.0
```

Other contexts (and other repos, and other teams) reference the published artifact in their config and pick it up on the next `windsor up`. This is how Core ships.

## License

[Mozilla Public License 2.0](LICENSE).

## Contributing

Build, test, and scan with `task build`, `task test:all`, and `task scan`. Tooling versions are pinned in [aqua.yaml](aqua.yaml). Open an issue or pull request on [GitHub](https://github.com/windsorcli/cli).
