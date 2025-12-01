---
title: "Sharing Blueprints"
description: "How to share and distribute blueprints using archives and OCI registries"
---
# Sharing Blueprints

Windsor supports sharing blueprints through two methods: local archive files (`.tar.gz`) and OCI-compatible registries. This enables you to distribute blueprints across teams, environments, and organizations.

## Overview

Blueprints can be shared as:
- **Local archives** (`.tar.gz` files) - For local distribution or version control
- **OCI artifacts** - For registry-based distribution compatible with Docker Hub, GitHub Container Registry, AWS ECR, and other OCI-compatible registries

Both formats contain the same blueprint template structure and are compatible with FluxCD's OCIRepository.

## Bundling Blueprints

The `windsor bundle` command packages your blueprint into a `.tar.gz` archive for distribution.

### Basic Usage

```bash
# Bundle with automatic naming
windsor bundle -t myapp:v1.0.0

# Bundle to specific file
windsor bundle -t myapp:v1.0.0 -o myapp-v1.0.0.tar.gz

# Bundle to directory (filename auto-generated)
windsor bundle -t myapp:v1.0.0 -o ./dist/

# Bundle using metadata.yaml for name/version
windsor bundle
```

### Bundle Contents

The bundle includes all files from `contexts/_template/`:
- `_template/blueprint.yaml` - Base blueprint definition
- `_template/schema.yaml` - JSON Schema for validation (if present)
- `_template/metadata.yaml` - Blueprint metadata (if present)
- `_template/features/` - All feature definitions
- Any additional files in `_template/` (e.g., Jsonnet configs, certificates)

### Using metadata.yaml

If `contexts/_template/metadata.yaml` exists with a `name` field, you can bundle without specifying a tag:

```yaml
# contexts/_template/metadata.yaml
name: my-blueprint
cliVersion: ">=0.7.1"
```

```bash
windsor bundle  # Uses name from metadata.yaml
```

## Pushing to OCI Registries

The `windsor push` command packages and pushes your blueprint to an OCI-compatible registry.

### Prerequisites

Before pushing, authenticate with your registry:

```bash
# Docker Hub
docker login docker.io

# GitHub Container Registry
docker login ghcr.io

# AWS ECR
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin <account-id>.dkr.ecr.us-east-1.amazonaws.com
```

### Basic Usage

```bash
# Push to Docker Hub
windsor push docker.io/myuser/myblueprint:v1.0.0

# Push to GitHub Container Registry
windsor push ghcr.io/myorg/myblueprint:v1.0.0

# Push using metadata.yaml for name/version
windsor push registry.example.com/blueprints
```

### OCI URL Format

OCI URLs follow the format:
```
oci://registry/repository:tag
```

Or when used in blueprint sources:
```
oci://ghcr.io/windsorcli/core:v0.3.0
```

The `oci://` prefix is optional when pushing (the command adds it automatically), but required when referencing in blueprint sources.

## Using Shared Blueprints

### From Local Archives

Load a blueprint from a local archive when initializing a context:

```bash
windsor init production --blueprint ./my-blueprint.tar.gz
```

The archive path can be:
- Absolute: `/path/to/blueprint.tar.gz`
- Relative to the current directory: `./archives/blueprint.tar.gz`

The archive should contain a `_template` directory with blueprint files including:
- `_template/blueprint.yaml` - The base blueprint definition
- `_template/schema.yaml` - JSON Schema for configuration validation (optional)
- `_template/features/` - Feature definitions (optional)
- `_template/metadata.yaml` - Blueprint metadata including CLI version compatibility (optional)

### From OCI Registries

Reference OCI blueprints in your blueprint sources:

```yaml
sources:
  - name: shared-blueprint
    url: oci://ghcr.io/myorg/myblueprint:v1.0.0
```

When a blueprint is loaded from an OCI registry:
1. The artifact is downloaded and cached
2. Template data is extracted from `_template/` directory
3. Features are processed and merged into the base blueprint
4. Schema validation is applied if `schema.yaml` is present
5. CLI version compatibility is checked if `metadata.yaml` is present

### OCI Source Features

When using OCI sources, Features from the OCI artifact are automatically processed:
- Features are evaluated against your context configuration
- Only matching features are applied
- Feature inputs are merged into existing components (when `applyOnly` mode is used)

## Artifact Structure

Both archive and OCI formats contain the same structure:

```
_template/
├── blueprint.yaml      # Base blueprint
├── schema.yaml         # JSON Schema (optional)
├── metadata.yaml       # Metadata with CLI version (optional)
└── features/           # Feature definitions (optional)
    ├── aws.yaml
    └── observability.yaml
```

## CLI Version Compatibility

The `cliVersion` field in `metadata.yaml` specifies the minimum required CLI version for your blueprint. This prevents users from attempting to use blueprints with incompatible CLI versions, avoiding runtime errors and ensuring that all blueprint features work correctly.

### Why CLI Version Compatibility Matters

As Windsor evolves, new features are added to the CLI that blueprints may depend on. For example:
- New expression functions in Features
- Enhanced schema validation capabilities
- Additional blueprint fields or merge strategies
- Changes to artifact format or OCI handling

If a user tries to load a blueprint that requires newer CLI features with an older CLI version, they may encounter:
- Unrecognized fields or syntax
- Missing functionality
- Unexpected behavior or errors

### Setting CLI Version Requirements

Specify the required CLI version in `contexts/_template/metadata.yaml`:

```yaml
name: my-blueprint
version: 1.0.0
cliVersion: ">=0.7.1"
```

The `cliVersion` field uses semantic versioning constraints. Common patterns:

- `">=0.7.1"` - Requires CLI version 0.7.1 or higher (recommended for most cases)
- `"~0.7.0"` - Requires CLI version compatible with 0.7.x (allows patch updates)
- `">=0.7.0 <0.8.0"` - Requires CLI version between 0.7.0 (inclusive) and 0.8.0 (exclusive)
- `">=0.8.0"` - Requires a specific major.minor version or higher

### When Version Checking Occurs

CLI version validation happens **early** in the blueprint loading process:

1. **For OCI artifacts**: When `GetTemplateData()` extracts the artifact, it immediately validates the CLI version before processing any blueprint content
2. **For local archives**: When loading from `.tar.gz` files, validation occurs during template data extraction
3. **For OCI sources**: When processing OCI sources referenced in blueprints, each source's CLI version is validated

If validation fails, the operation stops with a clear error message:

```
CLI version 0.6.5 does not satisfy required constraint '>=0.7.1'
```

### Best Practices

1. **Set `cliVersion` when using new features**: If your blueprint uses features introduced in a specific CLI version, set the constraint accordingly
2. **Test with minimum version**: Verify your blueprint works with the minimum specified CLI version
3. **Update constraints carefully**: When bumping `cliVersion`, ensure you're actually using features that require it
4. **Use `>=` for forward compatibility**: Using `">=X.Y.Z"` allows users with newer CLI versions to use your blueprint
5. **Document version requirements**: Mention CLI version requirements in your blueprint's documentation

### Example Scenarios

**Scenario 1: Using new Features syntax**
If your blueprint uses Features with expression functions introduced in v0.7.1:

```yaml
cliVersion: ">=0.7.1"
```

**Scenario 2: Backward compatibility**
If your blueprint works with any 0.7.x version but requires 0.7.0 minimum:

```yaml
cliVersion: ">=0.7.0"
```

**Scenario 3: Breaking changes**
If your blueprint requires a major version due to breaking changes:

```yaml
cliVersion: ">=0.8.0"
```

### What Happens Without cliVersion

If `cliVersion` is not specified in `metadata.yaml`, the CLI will:
- Skip version validation
- Attempt to load the blueprint regardless of CLI version
- May fail with cryptic errors if incompatible features are used

**Recommendation**: Always include `cliVersion` in your `metadata.yaml` to protect users and provide clear error messages.

## Best Practices

1. **Version your blueprints**: Use semantic versioning in tags (e.g., `v1.0.0`, `v1.1.0`)
2. **Include metadata.yaml**: Always include `metadata.yaml` with `name` and `cliVersion` for better compatibility checking
3. **Test before sharing**: Verify your blueprint works locally before pushing
4. **Document dependencies**: Ensure all referenced sources are accessible to users
5. **Use descriptive names**: Make blueprint names clear and descriptive
6. **Tag appropriately**: Use tags that indicate stability (e.g., `latest`, `v1.0.0`, `dev`)

