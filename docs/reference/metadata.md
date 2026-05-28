---
title: "Metadata"
description: "Optional metadata.yaml file that ships alongside a blueprint at contexts/_template/metadata.yaml."
---
# Metadata

Optional metadata.yaml file that ships alongside a blueprint at
contexts/_template/metadata.yaml. Used by 'windsor bundle' and 'windsor push'
to derive the artifact name and tag, and by the CLI to validate version
compatibility before loading the blueprint.

## Fields

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Blueprint name. Required for bundling and pushing. **(required)** |
| `author` | `string` | Author or maintainer of the blueprint. |
| `cliVersion` | `string` | Semver constraint for the required CLI version. When set, the CLI validates that its current version satisfies this constraint before loading the blueprint. Examples: '>=0.7.1', '~0.7.0', '>=0.7.0 <0.8.0'. If the constraint is not satisfied, blueprint loading fails with an explanatory error. |
| `description` | `string` | One-line description of the blueprint. |
| `homepage` | `string` | URL to the blueprint's homepage or documentation. |
| `license` | `string` | License identifier (e.g., MIT, Apache-2.0). |
| `tags` | `array<string>` | Tags for categorizing or organizing the blueprint. |
| `version` | `string` | Blueprint version. Used for artifact tagging when pushing to registries; serves as the default tag when --tag is omitted on 'windsor bundle' / 'windsor push'. |

## Examples

```yaml
name: my-blueprint
version: 1.0.0
description: A sample blueprint
author: John Doe
tags:
- infrastructure
- kubernetes
homepage: https://example.com/my-blueprint
license: MIT
cliVersion: ">=0.7.1"
```

## See also

- [`windsor bundle`](commands/bundle.md), [`windsor push`](commands/push.md)
- [Sharing blueprints](https://www.windsorcli.dev/docs/blueprints/sharing)
- Source schema: [pkg/runtime/config/schemas/metadata.yaml](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schemas/metadata.yaml)
