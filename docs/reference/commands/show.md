---
title: "windsor show"
description: "Display rendered resources."
---
# windsor show

```sh
windsor show blueprint [flags]
windsor show kustomization <component-name> [flags]
windsor show values [flags]
```

Print rendered Windsor resources to stdout. Reads the project state and runs blueprint composition without applying anything.

Unresolved deferred values render as `<deferred>` by default in `blueprint` and `kustomization` output; pass `--raw` to keep the original expression text instead.

## show blueprint

Print the fully composed blueprint.

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON instead of YAML. |
| `--raw` | `false` | Keep deferred expressions as text instead of `<deferred>`. |

## show kustomization

```sh
windsor show kustomization <component-name>
```

Print the Flux Kustomization resource for the named component, including ConfigMap substitutions in `postBuild.substituteFrom`. The output matches what `apply` would write to the cluster.

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON instead of YAML. |
| `--raw` | `false` | Keep deferred expressions as text instead of `<deferred>`. |

## show values

Print the effective context values, merging schema defaults with `values.yaml` overrides. YAML output includes schema descriptions as comments.

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON instead of YAML. |

## Examples

```sh
# What will my blueprint actually look like after composition?
windsor show blueprint

# Same, but keep deferred expressions visible
windsor show blueprint --raw

# Inspect the Flux Kustomization for one component
windsor show kustomization dns

# Effective values for the current context
windsor show values
```

## See also

- [Lifecycle guide](../../guides/lifecycle.md)
- [`explain`](explain.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md), [Configuration reference](../configuration.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
