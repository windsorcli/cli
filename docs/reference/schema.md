---
title: "Input Schema Validation"
description: "Reference for JSON Schema file structure and supported features"
---
# Input Schema Validation

Blueprints can include a JSON Schema file (`_template/schema.yaml`) that defines the expected structure and default values for configuration. The schema is used to:

- Validate user configuration values
- Provide default values for missing configuration keys
- Ensure configuration consistency across contexts

## Schema Format

The schema file must be valid JSON Schema. Supported schema versions:

- `https://json-schema.org/draft/2020-12/schema` - Standard JSON Schema Draft 2020-12

**Note:** Windsor implements a subset of JSON Schema Draft 2020-12. The following features are supported:

- **Types**: `object`, `string`, `array`, `integer`, `boolean`, `null`
- **Validation keywords**: `properties`, `required`, `enum`, `pattern` (for strings), `additionalProperties` (boolean or schema object), `items` (for arrays)
- **Default values**: `default` keyword for providing default configuration values
- **Nested objects**: Recursive validation of nested object structures

Unsupported features include: `format`, `minimum`/`maximum`/`multipleOf`, `minLength`/`maxLength`, `minItems`/`maxItems`, `uniqueItems`, `allOf`/`anyOf`/`oneOf`/`not`, `$ref`/`$defs`, `const`, and others.

## Example Schema

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
  cluster:
    type: object
    properties:
      enabled:
        type: boolean
        default: true
      workers:
        type: object
        properties:
          count:
            type: integer
            default: 1
        additionalProperties: false
    additionalProperties: false
additionalProperties: false
```

## Default Values

Default values specified in the schema are automatically merged with user configuration. When a configuration key is missing, the schema default is used. This ensures that blueprint processing always has complete configuration values.

## Schema Loading

The schema is automatically loaded from:
- `_template/schema.yaml` in blueprint archives or local template directories
- `schema` key in template data (for OCI artifacts)

If no schema is provided, configuration validation is skipped and defaults are not applied.

## Usage in Features

Schema defaults are available when evaluating feature conditions and inputs. This means you can rely on default values being present even if users don't explicitly configure them:

```yaml
# Feature condition can rely on schema defaults
when: observability.enabled == true && observability.backend == 'quickwit'
```

The schema ensures that `observability.enabled` defaults to `false` and `observability.backend` defaults to `"quickwit"` if not specified in the user's configuration.

