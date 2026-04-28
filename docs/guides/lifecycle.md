---
title: "Lifecycle"
description: "How init, up, apply, plan, destroy, and down fit together."
---
# Lifecycle

Windsor groups its commands into five phases. Knowing which phase a command belongs to is the fastest way to learn the CLI.

| Phase | Command | What it does |
|-------|---------|--------------|
| Scaffold | [`init`](../reference/commands/init.md) | Creates the context, writes `windsor.yaml`, marks the directory trusted. |
| Workstation | [`up`](../reference/commands/up.md) / [`down`](../reference/commands/down.md) | Starts/stops the local VM and container runtime. Workstation contexts only. |
| Install | [`apply`](../reference/commands/apply.md) | Runs Terraform components, then installs the Flux blueprint. |
| Inspect | [`plan`](../reference/commands/plan.md) / [`show`](../reference/commands/show.md) / [`explain`](../reference/commands/explain.md) | Previews changes, prints rendered resources, traces values. |
| Tear down | [`destroy`](../reference/commands/destroy.md) | Destroys live infrastructure (Terraform + Flux). |

## Workstation contexts

A workstation context (typically `local`) runs a VM-backed Kubernetes cluster on your machine. It uses `up` and `down`.

```sh
windsor init local              # scaffold
windsor up                      # start VM, terraform, install blueprint
# ... work ...
windsor down                    # stop VM, clean up local artifacts
```

`windsor up` is workstation-only. It starts the configured VM driver (colima, docker-desktop, or docker), runs Terraform for the workstation infrastructure, and installs the blueprint via Flux. Use `--wait` to block until kustomizations report ready.

`windsor down` stops the VM and cleans local context artifacts. It does not destroy live cloud resources — for that, run `destroy` first.

## Non-workstation contexts

Staging, production, and any other deployment context does not run a VM. It uses `apply` and `destroy`.

```sh
windsor init staging
windsor set context staging
windsor apply --wait
# ... work ...
windsor destroy --confirm=staging
```

`windsor apply` runs Terraform components and installs the blueprint, the same way `up` does — but without managing a workstation. It also supports targeted runs:

```sh
windsor apply terraform cluster      # apply just one terraform component
windsor apply kustomize observability # apply just one Flux kustomization
```

## Inspecting before you apply

`windsor plan` previews changes across both layers without applying them. With no argument, it prints a summary table; with a component name, it streams the full plan for that component.

```sh
windsor plan                         # summary across all components
windsor plan terraform cluster       # full streaming terraform plan
windsor plan kustomize --summary     # compact kustomize summary
```

`windsor show` prints rendered resources to stdout.

```sh
windsor show blueprint               # the fully composed blueprint
windsor show values                  # effective context values
windsor show kustomization dns       # the Flux Kustomization for one component
```

`windsor explain <path>` traces where a single value in the composed blueprint came from.

```sh
windsor explain terraform.cluster.inputs.cluster_endpoint
windsor explain kustomize.dns.substitutions.external_domain
```

## Tearing down

Live infrastructure is destroyed with `destroy`. The command requires confirmation — either interactively, or via `--confirm=<context>` for CI.

```sh
windsor destroy --confirm=local              # destroy everything in this context
windsor destroy terraform cluster --confirm=cluster  # one component only
windsor destroy kustomize observability --confirm=observability
```

For workstation contexts, the typical full teardown is `destroy` followed by `down`:

```sh
windsor destroy --confirm=local
windsor down
```

For non-workstation contexts, `destroy` is the whole story.

## See also

- [`init`](../reference/commands/init.md), [`up`](../reference/commands/up.md), [`apply`](../reference/commands/apply.md), [`destroy`](../reference/commands/destroy.md), [`down`](../reference/commands/down.md)
- [Local Workstation](local-workstation.md) — virtualization options and workstation-specific config
- [Contexts](contexts.md) — multi-context workflows
