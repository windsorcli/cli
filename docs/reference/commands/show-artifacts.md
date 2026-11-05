---
title: "windsor show artifacts"
description: "List the charts and images that the context pulls."
---

```sh
windsor show artifacts [flags]
```

Print every chart, image and blueprint that the current context pulls, with a destination for each. The command reads each oci:// blueprint source. It copies nothing. A chart goes to the --charts-to registry under its chart name and version. An image goes to the docker.registries mirror of its host.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--charts-to` | `""` | Chart registry for chart destinations, e.g. registry.example.com/windsor. |
| `-o`, `--output` | `yaml` | Output format: yaml, flux-mirror or list. |

## Examples

```sh
# List the artifacts of the current context as YAML
windsor show artifacts

# Copy every chart and image with Flux Mirror
windsor show artifacts --charts-to registry.example.com/windsor -o flux-mirror | flux mirror sync -

# List each destination and upstream on one line
windsor show artifacts -o list
```

## See also

- [`bundle`](bundle.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
