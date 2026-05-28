---
title: "Schema"
description: "Blueprint input-schema file (contexts/_template/schema.yaml)."
---
# Schema

`contexts/_template/schema.yaml` is the input-schema file for a blueprint
template. It validates the merged values that `windsor` passes into
rendering and supplies the default values surfaced by `windsor values`.

The file is a JSON Schema document written in YAML. Validation runs
through [`kaptinlin/jsonschema`](https://github.com/kaptinlin/jsonschema),
a full JSON Schema 2020-12 compiler — every keyword in the 2020-12 spec
is honoured for validation.

## Dialect

The `$schema` field is required. The validator accepts two URIs:

| URI | Status |
|-----|--------|
| `https://json-schema.org/draft/2020-12/schema` | Recommended for new schemas. |
| `https://windsorcli.dev/schema/2026-02/schema` | Accepted for backwards compatibility; rewritten to the canonical 2020-12 URI at compile time. |

Any other value is rejected at load time.

## Validation

The compiled schema validates the merged values object — composer
defaults overlaid with user-supplied values from `windsor set` and from
`values.yaml` files. Validation failures surface as
`<instance-path>: <keyword>: <message>` per leaf and abort the command
that triggered the load.

All JSON Schema 2020-12 keywords work: structural (`type`,
`properties`, `required`, `additionalProperties`, `items`),
composition (`allOf`, `anyOf`, `oneOf`, `not`), references (`$ref`,
`$defs`), constraints (`minLength`, `maxLength`, `minimum`, `maximum`,
`pattern`, `format`, `const`, `enum`), and conditional
(`if`/`then`/`else`, `dependentSchemas`, `dependentRequired`).

## Defaults

A separate pass walks `properties` recursively and collects every
`default:` it encounters. The collected map is what `GetSchemaDefaults`
returns and what `windsor values` displays under the "defaults" layer.

The walk only descends through nested objects declared as
`type: object` with their own `properties`. Defaults declared inside
the following constructs are validated but are **not** extracted into
the defaults layer:

- `array` schemas (`items.default`, `prefixItems[*].default`)
- composition branches (`allOf`/`anyOf`/`oneOf`/`not`)
- `$ref` targets and `$defs` entries
- conditional branches (`if`/`then`/`else`)
- `additionalProperties` and `patternProperties` schemas

Place defaults directly under each leaf property of a nested
`properties` tree to make sure they surface.

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
- Source loader: [pkg/runtime/config/schema_validator.go](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schema_validator.go)
