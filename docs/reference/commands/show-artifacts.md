---
title: "windsor show artifacts"
description: "List the charts and images that the context pulls."
---

```sh
windsor show artifacts [flags]
```

Print every chart, image and blueprint that the current context pulls, with a destination for each. The command renders each Flux Kustomization offline with the flux CLI and copies nothing. A chart goes to the --charts-to registry under its chart name and version. An image goes to the docker.registries mirror of its host. A --set value applies to this command only and is not saved.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | List the blueprint-wide set from each source's artifact-manifest.yaml. |
| `--charts-to` | `""` | Chart registry for chart destinations, e.g. registry.example.com/windsor. |
| `-o`, `--output` | `yaml` | Output format: yaml, flux-mirror or list. |
| `--set` | `[]` | Override a config value for this command, e.g. --set charts.registry=. May be repeated. |

## Examples

```sh
# List the artifacts of the current context as YAML
windsor show artifacts

# Copy every chart and image with Flux Mirror
windsor show artifacts --charts-to registry.example.com/windsor -o flux-mirror | flux mirror sync -

# List the vendor upstreams of a core context that already uses a chart registry
windsor show artifacts --set charts.registry= --charts-to registry.example.com/windsor

# List the blueprint-wide set from each source's artifact-manifest.yaml
windsor show artifacts --all -o list
```

## See also

- [`bundle`](bundle.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
