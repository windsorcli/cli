---
title: "Explain"
description: "Trace blueprint values back to their sources to debug composition."
---
# Explain

Windsor blueprints are composed from many sources: facets, context values, deferred terraform outputs, and inline expressions. When a value isn't what you expected, [`windsor explain <path>`](../reference/commands/explain.md) tells you where it actually came from.

## When to reach for it

Use `explain` whenever you find yourself asking:

- "Why is this terraform input set to that value?"
- "Which facet wins when both define the same substitution?"
- "Is this expression deferred, or is it actually empty?"
- "Why isn't my override taking effect?"

## Path syntax

Paths are dotted, addressing fields in the composed blueprint:

| Path pattern | Meaning |
|------------|---------|
| `terraform.<component>.inputs.<field>` | A terraform input value. |
| `terraform.<component>.inputs.<field>.components` | The list of contributions for a list field. |
| `kustomize.<name>.substitutions.<key>` | A Flux substitution value. |
| `kustomize.<name>.patches` | Patches applied to the kustomization. |
| `configMaps.<name>.<key>` | A blueprint-level ConfigMap entry. |

## Reading the output

Each line under the path is a contribution. Indentation shows nesting; status markers tell you why a contribution looks the way it does:

| Marker | Meaning |
|--------|---------|
| `(deferred)` | Depends on a terraform output not yet available. Run `windsor apply terraform <component>` first. |
| `(empty)` | Resolved to an empty string. |
| `(not set)` | The referenced facet config was never provided. |
| `(cycle)` | The expression chain refers back to itself. |

## Examples

```sh
# Where does the cluster endpoint come from?
windsor explain terraform.cluster.inputs.cluster_endpoint

# Trace a Flux substitution that looks wrong
windsor explain kustomize.dns.substitutions.external_domain

# Inspect every contribution to a list field
windsor explain terraform.cluster.inputs.common_config_patches.components
```

## See also

- [`explain` reference](../reference/commands/explain.md)
- [`show` reference](../reference/commands/show.md) — print the rendered blueprint
- [Facets reference](../reference/facets.md) — where most contributions originate
