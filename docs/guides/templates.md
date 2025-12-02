---
title: "Blueprint Templates"
description: "Understanding how blueprint templates work in Windsor"
---
# Blueprint Templates

The `contexts/_template/` directory contains blueprint template files that are shared across all contexts. This directory structure allows you to define reusable blueprint components, features, and schemas that can be customized per-context.

## Overview

Blueprint templates provide a way to:
- Define reusable blueprint components shared across contexts
- Use Features for conditional blueprint composition
- Validate configuration with JSON Schema
- Specify CLI version compatibility requirements

When a context is initialized, Windsor loads the base blueprint from `_template/blueprint.yaml` and processes Features from `_template/features/` to build the final blueprint for that context.

## Directory Structure

```
contexts/
└── _template/
    ├── blueprint.yaml      # Base blueprint definition
    ├── schema.yaml         # JSON Schema for configuration validation (optional)
    ├── metadata.yaml       # Blueprint metadata including CLI version compatibility (optional)
    └── features/           # Feature definitions (optional)
        ├── aws.yaml
        ├── observability.yaml
        └── ...
```

## Template Files

### blueprint.yaml

The base blueprint definition that serves as the foundation for all contexts. This file defines:

- Repository configuration
- Source definitions
- Base Terraform components
- Base Kustomizations

When a context is initialized, this base blueprint is loaded and can be extended or overridden by context-specific configurations.

Example:

```yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: base
  description: Base blueprint for all contexts
repository:
  url: github.com/org/blueprints
  ref:
    branch: main
sources:
  - name: core
    url: github.com/windsorcli/core
    ref:
      tag: v0.5.0
terraform:
  - source: core
    path: cluster/talos
kustomize:
  - name: ingress
    path: ingress/base
    source: core
```

### schema.yaml

JSON Schema file that defines the expected structure and default values for configuration. The schema is used to:

- Validate user configuration values from `windsor.yaml` and `values.yaml`
- Provide default values for missing configuration keys
- Ensure configuration consistency across contexts

The schema file must be valid JSON Schema. Supported schema versions:
- `https://json-schema.org/draft/2020-12/schema` - Standard JSON Schema Draft 2020-12

**Note:** Windsor implements a subset of JSON Schema Draft 2020-12. See [Schema Reference](../reference/schema.md) for supported features.

Example:

```yaml
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  provider:
    type: string
    default: "none"
    enum: ["none", "aws", "azure", "generic"]
  observability:
    type: object
    properties:
      enabled:
        type: boolean
        default: false
      backend:
        type: string
        default: "quickwit"
        enum: ["quickwit", "loki", "elasticsearch"]
    additionalProperties: false
additionalProperties: false
```

### metadata.yaml

Blueprint metadata including CLI version compatibility constraints:

```yaml
cliVersion: ">=0.7.1"
```

This ensures that blueprints are only used with compatible CLI versions. Version constraints support:

- `>=0.7.1` - Requires CLI version 0.7.1 or higher
- `~0.7.0` - Requires CLI version compatible with 0.7.x
- `>=0.7.0 <0.8.0` - Requires CLI version between 0.7.0 (inclusive) and 0.8.0 (exclusive)

If the CLI version doesn't satisfy the constraint, blueprint loading will fail with an error.

### features/

Directory containing Feature definitions. Features enable conditional blueprint composition based on configuration values.

Features are automatically loaded from:
- `_template/features/*.yaml` - Individual feature files
- `_template/features/**/*.yaml` - Nested feature directories

Features are processed in alphabetical order by name, then merged into the base blueprint.

Example feature:

```yaml
kind: Feature
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-feature
  description: AWS-specific infrastructure components
when: provider == 'aws'
terraform:
  - path: network/vpc
    source: core
    inputs:
      cidr: ${network.cidr_block ?? "10.0.0.0/16"}
    strategy: merge
```

For detailed information about Features, see the [Features Reference](../reference/features.md).

## How Templates Work

1. **Template Loading**: When a blueprint is loaded for a context, Windsor first loads files from `contexts/_template/`
2. **Schema Validation**: The schema from `_template/schema.yaml` (if present) validates and provides defaults for configuration values from `windsor.yaml` and `values.yaml`
3. **Feature Processing**: Features from `_template/features/` are evaluated against the context's configuration and merged into the base blueprint
4. **Context Overrides**: Context-specific `blueprint.yaml` files can override or extend the base blueprint

## File Resolution

Files referenced in features (via `jsonnet()` or `file()` functions) are resolved relative to the feature file location within `_template/`:

- Feature at `_template/features/aws.yaml` can reference `_template/features/config.jsonnet`
- Use `../configs/config.jsonnet` for files in parent directories
- Paths work with both local filesystem and in-memory template data (from OCI artifacts)

