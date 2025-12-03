---
title: "Sharing Blueprints"
description: "How to share and distribute blueprints using archives and OCI registries"
---
# Sharing Blueprints

Windsor supports sharing blueprints through OCI-compatible registries, enabling you to distribute blueprints across teams, environments, and organizations. Blueprints can also be packaged as local archive files (`.tar.gz`) for troubleshooting and development purposes.

## Overview

The primary method for sharing blueprints is through **OCI-compatible registries** such as Docker Hub, GitHub Container Registry, AWS ECR, and other OCI-compatible registries. This provides versioned, centralized distribution of blueprints.

**Local archives** (`.tar.gz` files) are available for troubleshooting artifacts or local development, but are not typically used for production distribution. Both formats contain the same blueprint template structure and are compatible with FluxCD's OCIRepository.

## Pushing to OCI Registries

The `windsor push` command packages and pushes your blueprint to an OCI-compatible registry. This is the recommended method for sharing blueprints.

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

### From OCI Registries

Reference OCI blueprints in your blueprint sources:

```yaml
sources:
  - name: shared-blueprint
    url: oci://ghcr.io/myorg/myblueprint:v1.0.0
```

When a blueprint is loaded from an OCI registry, Windsor downloads the artifact, extracts the template data, processes features, and validates the blueprint configuration and CLI version compatibility.

### From Local Archives

Local archive files (`.tar.gz`) are primarily useful for troubleshooting artifacts or local development.

To load a blueprint from a local archive when initializing a context:

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

## Bundling Blueprints

The `windsor bundle` command packages your blueprint into a `.tar.gz` archive. This is primarily useful for troubleshooting or local development, not typical production distribution.

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

If `contexts/_template/metadata.yaml` exists with both `name` and `version` fields, you can bundle without specifying a tag. See the [Metadata Reference](../reference/metadata.md) for complete documentation of metadata fields.

```yaml
# contexts/_template/metadata.yaml
name: my-blueprint
cliVersion: ">=0.7.1"
```

```bash
windsor bundle  # Uses name from metadata.yaml
```

## Artifact Structure

Both archive and OCI formats contain the same structure. The artifact includes local blueprint template files and any local Terraform modules or Kustomize components in your project:

```
artifact/
├── metadata.yaml       # Artifact metadata (required)
├── _template/          # Blueprint template files
│   ├── blueprint.yaml      # Base blueprint (required)
│   ├── schema.yaml         # JSON Schema for validation (optional)
│   ├── metadata.yaml       # Blueprint metadata (optional)
│   └── features/           # Feature definitions (optional)
│       ├── aws.yaml
│       └── observability.yaml
├── terraform/          # Local Terraform modules (if present in project)
│   └── ...
└── kustomize/          # Local Kustomize components (if present in project)
    └── ...
```

The artifact includes local Terraform modules and Kustomize components from your project's `terraform/` and `kustomize/` directories. External resources referenced via the blueprint's `sources` field (such as Git repositories or OCI artifacts) are not bundled into the artifact; they are resolved at runtime when the blueprint is used.

### Required Files

**`blueprint.yaml`** - The base blueprint definition that serves as the foundation for all contexts. See the [Blueprint Reference](../reference/blueprint.md) for complete documentation of blueprint structure and fields.

### Optional Files

**`schema.yaml`** - JSON Schema file that defines the expected structure and default values for configuration. See the [Schema Reference](../reference/schema.md) for details on supported schema features and usage.

**`metadata.yaml`** - Blueprint metadata including name, version, and CLI version constraints. See the [Metadata Reference](../reference/metadata.md) for complete metadata options.

**`features/`** - Directory containing Feature definitions that enable conditional blueprint composition based on configuration values. See the [Features Reference](../reference/features.md) for complete feature documentation.

### Additional Files

You can include any additional files in `_template/` that your features reference, such as Jsonnet files, certificates, or configuration files. These are loaded via the `${jsonnet()}` and `${file()}` functions in features. See the [Features Reference](../reference/features.md#file-loading-functions) for details on file loading.

### Terraform and Kustomize Resources

The artifact automatically includes all local Terraform modules from the `terraform/` directory and all local Kustomize components from the `kustomize/` directory in your project. These resources are bundled into the artifact at `terraform/` and `kustomize/` paths respectively.

Note that external resources referenced via the blueprint's `sources` field (Git repositories, OCI artifacts, etc.) are not bundled into the artifact. These external sources are resolved at runtime when the blueprint is used, allowing blueprints to reference shared modules and components from external repositories.

All files in `_template/`, `terraform/`, and `kustomize/` are packaged into the artifact and available when the blueprint is loaded.

## CLI Version Compatibility

The `cliVersion` field in `metadata.yaml` specifies the minimum required CLI version for your blueprint. This prevents users from attempting to use blueprints with incompatible CLI versions, avoiding runtime errors and ensuring that all blueprint features work correctly. See the [Metadata Reference](../reference/metadata.md) for complete documentation of the `cliVersion` field.

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

1. **For OCI artifacts**: When loading blueprints from OCI registries, the CLI version is validated immediately after downloading the artifact, before processing any blueprint content
2. **For local archives**: When loading from `.tar.gz` files, validation occurs during template data extraction
3. **For OCI sources**: When processing OCI sources referenced in blueprints, each source's CLI version is validated

If validation fails, the operation stops with a clear error message:

```
CLI version 0.6.5 does not satisfy required constraint '>=0.7.1'
```

### Example Scenarios

**Scenario 1: Using new Features syntax**
If your blueprint uses Features with expression functions introduced in v0.8.0:

```yaml
cliVersion: ">=0.8.0"
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
3. **Set `cliVersion` when using new features**: If your blueprint uses features introduced in a specific CLI version, set the constraint accordingly
4. **Test before sharing**: Verify your blueprint works locally before pushing, and test with the minimum specified CLI version
5. **Use `>=` for forward compatibility**: Using `">=X.Y.Z"` allows users with newer CLI versions to use your blueprint
6. **Document dependencies**: Ensure all referenced sources are accessible to users
7. **Use descriptive names**: Make blueprint names clear and descriptive
8. **Tag appropriately**: Use tags that indicate stability (e.g., `latest`, `v1.0.0`, `dev`)

<div>
  {{ footer('Blueprint Templates', '../templates/index.html', 'Hello, World!', '../../tutorial/hello-world/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../templates/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/hello-world/index.html'; 
  });
</script>

