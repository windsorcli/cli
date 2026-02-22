---
title: "Blueprint Templates"
description: "Understanding how blueprint templates work in Windsor"
---
# Blueprint Templates

The `contexts/_template/` directory contains blueprint template files that are shared across all contexts. This directory structure allows you to define reusable blueprint components, facets, and schemas that can be customized per-context.

## Overview

Blueprint templates provide a way to:
- Define reusable blueprint components shared across contexts
- Use Facets for conditional blueprint composition
- Validate configuration with JSON Schema
- Specify CLI version compatibility requirements

When a context is initialized, Windsor loads the base blueprint from `_template/blueprint.yaml` and processes Facets from `_template/facets/` to build the final blueprint for that context.

## Directory Structure

```
contexts/
└── _template/
    ├── blueprint.yaml      # Base blueprint definition
    ├── schema.yaml         # JSON Schema for configuration validation (optional)
    ├── metadata.yaml       # Blueprint metadata including CLI version compatibility (optional)
    └── facets/            # Facet definitions (optional)
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
    url: oci://ghcr.io/windsorcli/core:v0.5.0
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

The schema file must use the Windsor schema dialect. Supported `$schema` values:
- `https://windsorcli.dev/schema/2026-02/schema` - Windsor schema dialect (recommended)
- `https://json-schema.org/draft/2020-12/schema` - JSON Schema Draft 2020-12 (compatibility)

**Note:** Windsor implements a subset of JSON Schema Draft 2020-12. See [Schema Reference](../reference/schema.md) for supported features.

Example:

```yaml
$schema: https://windsorcli.dev/schema/2026-02/schema
type: object
properties:
  provider:
    type: string
    default: "none"
    enum: ["none", "metal", "docker", "aws", "azure", "gcp"]
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

### facets/

Directory containing Facet definitions. Facets enable conditional blueprint composition based on configuration values.

Facets are automatically loaded from:
- `_template/facets/*.yaml` - Individual facet files
- `_template/facets/**/*.yaml` - Nested facet directories

Facets are processed by ordinal (ascending), then by name. When a facet does not set `ordinal` in YAML, it is derived from the facet filename: `config-*` 100, `provider-base`/`platform-base` 199, `provider-*`/`platform-*` 200, `options-*` 300, `addon-*`/`addons-*` 400. Higher ordinal means higher precedence when merging (addons override provider-base for same-name kustomize entries).

Example facet:

```yaml
kind: Facet
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: aws-facet
  description: AWS-specific infrastructure components
when: provider == 'aws'
terraform:
  - path: network/vpc
    source: core
    inputs:
      cidr: ${network.cidr_block ?? "10.0.0.0/16"}
    strategy: merge
```

For detailed information about Facets, see the [Facets Reference](../reference/facets.md).

## How Templates Work

1. **Template Loading**: When a blueprint is loaded for a context, Windsor first loads files from `contexts/_template/`
2. **Schema Validation**: The schema from `_template/schema.yaml` (if present) validates and provides defaults for configuration values from `windsor.yaml` and `values.yaml`
3. **Facet Processing**: Facets from `_template/facets/` are evaluated against the context's configuration and merged into the base blueprint
4. **Context Overrides**: Context-specific `blueprint.yaml` files can override or extend the base blueprint

## Blueprint Composition Order

When Windsor composes the final blueprint, it merges layers in this specific order:

1. **Deployed Sources** - OCI sources with `deploy: true` (or no `deploy` flag, which defaults to `true`) have their components merged directly into the result. Sources with `deploy: false` are not merged but remain available for component reference.
2. **Base Template** - The `_template/blueprint.yaml` is always fully merged. This serves as the foundation layer and is never filtered or conditionally included.
3. **User Blueprint** - Context-specific `blueprint.yaml` files override and extend the result. The user blueprint acts as an override layer without filtering - all components from previous layers are retained unless explicitly disabled.

**Important Notes:**
- The `_template/` directory is always included in full - there is no `deploy` flag for the base template
- Only OCI sources (those with `oci://` URLs) can have their components merged. Non-OCI sources (Git URLs, etc.) remain in the Sources array for component reference but are not loaded as blueprints
- The `deploy` flag on sources only applies to OCI sources. It defaults to `true` if not specified
- Sources with `deploy: false` are "index-only" - their components aren't merged, but components can still reference them via `source: <name>`

## File Resolution

Files referenced in facets (via `jsonnet()` or `file()` functions) are resolved relative to the facet file location within `_template/`:

- Facet at `_template/facets/aws.yaml` can reference `_template/facets/config.jsonnet`
- Use `../configs/config.jsonnet` for files in parent directories
- Paths work with both local filesystem and in-memory template data (from OCI artifacts)

<div>
  {{ footer('Secrets Management', '../secrets-management/index.html', 'Sharing Blueprints', '../sharing/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../secrets-management/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../sharing/index.html'; 
  });
</script>

