---
title: "windsor show kustomization"
description: "Display the Flux Kustomization resource for a component."
---
# windsor show kustomization

```sh
windsor show kustomization <component-name> [flags]
```

Print the Flux Kustomization resource for the named component, including blueprint-level ConfigMaps in postBuild.substituteFrom. The output matches what 'windsor apply' would write to the cluster. Defaults to YAML; use --json for JSON. Unresolved deferred values render as '<deferred>' by default; use --raw to keep the original expression text instead.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON instead of YAML. |
| `--raw` | `false` | Keep deferred expressions as text instead of <deferred>. |

## Examples

```sh
# Inspect the Flux Kustomization for one component
windsor show kustomization dns

# JSON for tooling
windsor show kustomization dns --json
```

## See also

- [`apply`](/reference/cli/commands/apply), [`plan`](/reference/cli/commands/plan)
- [Blueprint reference](/reference/cli/blueprint)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
