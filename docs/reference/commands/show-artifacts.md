---
title: "windsor show artifacts"
description: "List the blueprint artifacts that the context pulls."
---

```sh
windsor show artifacts [flags]
```

Print each oci:// blueprint source that the current context pulls. The command copies nothing. Use -o flux-mirror or -o list to give the list to a relocation tool.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--charts-to` | `""` | Chart registry for chart destinations, e.g. registry.example.com/windsor. |
| `-o`, `--output` | `yaml` | Output format: yaml, flux-mirror or list. |

## Examples

```sh
# List the artifacts of the current context as YAML
windsor show artifacts

# Write a Flux Mirror config for the context
windsor show artifacts --charts-to registry.example.com/windsor -o flux-mirror | flux mirror sync -

# List each destination and upstream on one line
windsor show artifacts -o list
```

## See also

- [`bundle`](bundle.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
