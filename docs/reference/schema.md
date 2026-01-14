---
title: "Schema"
description: "Reference for JSON Schema file structure and supported features"
---
# Input Schema Validation

Blueprints can include a JSON Schema file (`_template/schema.yaml`) that defines the expected structure and default values for configuration.

## Schema File Structure

The schema file must be valid JSON Schema Draft 2020-12. The schema is located at `_template/schema.yaml` in blueprint templates.

```yaml
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  # ... property definitions
```

## Supported Types

Windsor supports the following JSON Schema types:

| Type | Description |
|------|-------------|
| `object` | Key-value pairs |
| `string` | Text values |
| `array` | Ordered lists |
| `integer` | Whole numbers |
| `boolean` | True/false values |
| `null` | Null values |

## Supported Validation Keywords

### Type Keywords

| Keyword | Type | Description |
|---------|------|-------------|
| `type` | `string` | Data type of the value |

### Object Keywords

| Keyword | Type | Description |
|---------|------|-------------|
| `properties` | `object` | Object property definitions |
| `required` | `array` | Required property names |
| `additionalProperties` | `boolean` or `object` | Control additional properties. `false` disallows, object validates |

### String Keywords

| Keyword | Type | Description |
|---------|------|-------------|
| `enum` | `array` | Allowed values |
| `pattern` | `string` | Regex pattern for validation |

### Array Keywords

| Keyword | Type | Description |
|---------|------|-------------|
| `items` | `object` | Schema for array items |

### Default Values

| Keyword | Type | Description |
|---------|------|-------------|
| `default` | `any` | Default value when property is missing |

## Nested Objects

Nested object structures are supported recursively. Each nested object can have its own `properties`, `required`, `additionalProperties`, and `default` values.

## Unsupported Features

The following JSON Schema Draft 2020-12 features are **not** supported:

- **Numeric constraints**: `minimum`, `maximum`, `multipleOf`
- **String length constraints**: `minLength`, `maxLength`
- **Array constraints**: `minItems`, `maxItems`, `uniqueItems`
- **Composition keywords**: `allOf`, `anyOf`, `oneOf`, `not`
- **References**: `$ref`, `$defs`
- **Format validation**: `format` keyword
- **Constants**: `const` keyword
- **Conditional validation**: `if`, `then`, `else`
- **Dependent schemas**: `dependentSchemas`, `dependentRequired`

## Schema File Location

The schema file must be located at `contexts/_template/schema.yaml` in your blueprint template directory.

## Example

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

<div>
  {{ footer('Facets', '../facets/index.html', 'Metadata', '../metadata/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../facets/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../metadata/index.html'; 
  });
</script>
