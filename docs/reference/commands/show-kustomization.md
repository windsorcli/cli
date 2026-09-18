---
title: "windsor show kustomization"
description: "Display the Flux Kustomization resource for a component."
---

```sh
windsor show kustomization [component-name] [flags]
```

Print the Flux Kustomization resource for the named component, including blueprint-level ConfigMaps in postBuild.substituteFrom. The output matches what 'windsor apply' would write to the cluster. Omit the name to list every compiled component. Defaults to YAML; use --json for JSON.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON instead of YAML. |

## Examples

```sh
# List every compiled component
windsor show kustomization

# Inspect the Flux Kustomization for one component
windsor show kustomization dns

# JSON for tooling
windsor show kustomization dns --json
```

## See also

- [`apply`](apply.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
