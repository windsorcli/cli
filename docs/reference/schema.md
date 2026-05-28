---
title: "Schema"
description: "Blueprint input-schema file (contexts/_template/schema.yaml)."
---
# Schema

`contexts/_template/schema.yaml` is the input-schema file for a blueprint
template. It validates the values your blueprint receives — defaults
plus anything the user set via `windsor set` or `values.yaml` files —
and supplies the default values shown by `windsor values`.

The file is a JSON Schema document written in YAML.

## Dialect

The `$schema` field is required and must be
`https://json-schema.org/draft/2020-12/schema`. Any other value is
rejected at load time.

## Validation

The schema validates against the full JSON Schema 2020-12 specification.
Structural keywords (`type`, `properties`, `required`,
`additionalProperties`, `items`), composition (`allOf`, `anyOf`,
`oneOf`, `not`), references (`$ref`, `$defs`), constraints
(`minLength`, `maxLength`, `minimum`, `maximum`, `pattern`, `format`,
`const`, `enum`), and conditional logic (`if`/`then`/`else`,
`dependentSchemas`, `dependentRequired`) all work.

Validation failures abort the command that triggered the load and
report each violation with its instance path and the keyword that
failed.

## Defaults

A `default:` declared on a property surfaces in `windsor values` and
seeds the value the blueprint receives if the user did not override it.

Defaults only surface when declared directly on a property under a
nested `properties:` tree. Defaults declared inside the following
constructs are validated but do **not** seed any value:

- `array` schemas (`items.default`, `prefixItems[*].default`)
- composition branches (`allOf`/`anyOf`/`oneOf`/`not`)
- `$ref` targets and `$defs` entries
- conditional branches (`if`/`then`/`else`)
- `additionalProperties` and `patternProperties` schemas

## Example

```yaml
$schema: https://json-schema.org/draft/2020-12/schema
type: object
additionalProperties: false
properties:
  platform:
    type: string
    enum: [none, metal, docker, aws, azure, gcp]
    default: none
  observability:
    type: object
    additionalProperties: false
    properties:
      enabled:
        type: boolean
        default: false
      backend:
        type: string
        enum: [quickwit, loki, elasticsearch]
        default: quickwit
  cluster:
    type: object
    additionalProperties: false
    properties:
      enabled:
        type: boolean
        default: true
      workers:
        type: object
        additionalProperties: false
        properties:
          count:
            type: integer
            minimum: 1
            default: 1
```

## See also

- [Blueprint reference](blueprint.md) — top-level blueprint definition
- [Metadata reference](metadata.md), [Facets reference](facets.md)
- [`show values`](commands/show-values.md), [`set`](commands/set.md)
