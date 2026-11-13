---
title: "windsor show artifacts"
description: "List the charts and images that the context pulls."
---

```sh
windsor show artifacts [flags]
```

Print each chart and image that an artifact tag names in the files that the composed blueprint uses, and each oci:// blueprint source. The command reads the blueprint paths, the resources and components of each kustomization file, and local Terraform modules. It also reads all of contexts/, _template/ and kustomize/crds/, and it lists a tag in a facet only when the blueprint uses the tagged value. Use --all to read whole sources. The command copies nothing. Use -o flux-mirror or -o list to give the list to a relocation tool. Use -o cyclonedx to print a CycloneDX 1.6 BOM, in which each oci:// source holds the artifacts of its files.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Read the artifact tags of whole sources, also of the files that the blueprint does not use. |
| `--charts-to` | `""` | Chart registry for chart destinations, e.g. registry.example.com/windsor. |
| `-o`, `--output` | `yaml` | Output format: yaml, flux-mirror, list or cyclonedx. |

## Examples

```sh
# List the artifacts of the current context as YAML
windsor show artifacts

# Write a Flux Mirror config for the context
windsor show artifacts --charts-to registry.example.com/windsor -o flux-mirror | flux mirror sync -

# List each destination and upstream on one line
windsor show artifacts -o list

# List the artifact tags of whole sources
windsor show artifacts --all

# Print the CycloneDX BOM that windsor bundle writes
windsor show artifacts --all -o cyclonedx
```

## See also

- [`bundle`](bundle.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
