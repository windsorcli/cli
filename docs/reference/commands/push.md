---
title: "windsor push"
description: "Push the blueprint to an OCI registry."
---
# windsor push

```sh
windsor push <registry/repo[:tag]>
```

Bundle the current blueprint and push it to an OCI-compatible registry (Docker Hub, GHCR, ECR, etc.). The pushed artifact is consumable by FluxCD's `OCIRepository`.

The registry argument is required. When the tag is omitted, `metadata.yaml` (`name`, `version`) is used to derive it.

Authentication uses your existing Docker credential helper. If push fails with an auth error, the CLI suggests `docker login <registry>`.

## Examples

```sh
# Docker Hub
windsor push docker.io/myorg/myblueprint:v1.0.0

# GitHub Container Registry
windsor push ghcr.io/myorg/myblueprint:v1.0.0

# Tag from metadata.yaml
windsor push registry.example.com/myorg/myblueprint
```

## See also

- [Sharing guide](../../guides/sharing.md)
- [Metadata reference](../metadata.md)
- [`bundle`](bundle.md)
- Source: [cmd/push.go](https://github.com/windsorcli/cli/blob/main/cmd/push.go)
