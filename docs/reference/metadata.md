---
title: "Blueprint Metadata"
description: "Reference for metadata.yaml file structure and fields"
---
# Blueprint Metadata

Blueprints can include a `_template/metadata.yaml` file that provides additional metadata about the blueprint, including CLI version compatibility requirements.

## Metadata File Structure

```yaml
name: my-blueprint
version: 1.0.0
description: A sample blueprint
author: "John Doe"
tags:
  - infrastructure
  - kubernetes
homepage: https://example.com/my-blueprint
license: MIT
cliVersion: ">=0.7.1"
```

| Field         | Type     | Description                                      |
|---------------|----------|--------------------------------------------------|
| `name`        | `string` | The blueprint name. Required for bundling and pushing. |
| `version`     | `string` | The blueprint version. Used for artifact tagging when pushing to registries. |
| `description` | `string` | A description of the blueprint.                  |
| `author`      | `string` | The author or maintainer of the blueprint.       |
| `tags`        | `[]string` | Tags for categorizing or organizing the blueprint. |
| `homepage`    | `string` | URL to the blueprint's homepage or documentation. |
| `license`     | `string` | License identifier (e.g., "MIT", "Apache-2.0"). |
| `cliVersion`  | `string` | Semver constraint for required CLI version (e.g., ">=0.7.1", "~0.7.0"). If specified, the CLI validates that the current version satisfies the constraint before loading the blueprint. |

## CLI Version Compatibility

The `cliVersion` field uses semantic versioning constraints. Examples:
- `">=0.7.1"` - Requires CLI version 0.7.1 or higher
- `"~0.7.0"` - Requires CLI version compatible with 0.7.x
- `">=0.7.0 <0.8.0"` - Requires CLI version between 0.7.0 (inclusive) and 0.8.0 (exclusive)

If the CLI version doesn't satisfy the constraint, blueprint loading will fail with an error.

## Using Metadata in Bundling

When using `windsor bundle` or `windsor push`, the metadata file is used to:

1. **Automatic naming**: If `name` is specified, you can bundle without providing a tag:
   ```bash
   windsor bundle  # Uses name from metadata.yaml
   ```

2. **Version tagging**: The `version` field is used for artifact tagging when pushing to registries.

3. **Compatibility checking**: The `cliVersion` field ensures users have a compatible CLI version before attempting to use the blueprint.

## Metadata Location

The metadata file should be placed at:
- `contexts/_template/metadata.yaml` - For local template directories
- `_template/metadata.yaml` - In blueprint archives
- Included in OCI artifacts as part of the template data

## Best Practices

1. **Always include `name`**: Makes bundling and sharing easier
2. **Use semantic versioning**: Follow semver for the `version` field
3. **Set `cliVersion` constraints**: Protect users from incompatible CLI versions
4. **Keep descriptions clear**: Help users understand what the blueprint does

