---
title: "windsor explain"
description: "Trace a blueprint value back to its sources."
---
# windsor explain

```sh
windsor explain <path>
```

Print the value at the given dotted path and the contributions that produced it. Use `explain` to debug blueprint composition: when a value isn't what you expected, it tells you which facet, source, or expression set it (and which others were overridden).

## Path forms

| Path pattern | Meaning |
|------------|---------|
| `terraform.<component>.inputs.<field>` | A terraform input value. |
| `terraform.<component>.inputs.<field>.components` | The list of contributions for a list field. |
| `kustomize.<name>.substitutions.<key>` | A Flux substitution. |
| `kustomize.<name>.patches` | Patches applied to a kustomization. |
| `configMaps.<name>.<key>` | A blueprint-level ConfigMap entry. |

Status markers in the output:

- `(deferred)` — value depends on a terraform output not yet available
- `(empty)` — resolved to an empty string
- `(not set)` — the referenced facet config was never provided
- `(cycle)` — the expression chain forms a cycle

## Examples

```sh
# Where does the cluster endpoint come from?
windsor explain terraform.cluster.inputs.cluster_endpoint

# Trace a Flux substitution
windsor explain kustomize.dns.substitutions.external_domain

# Inspect a list field's contributions
windsor explain terraform.cluster.inputs.common_config_patches.components
```

## See also

- [`show`](show.md), [`plan`](plan.md)
- [Facets reference](../facets.md), [Blueprint reference](../blueprint.md)
- [Explain guide](../../guides/explain.md)
- Source: [cmd/explain.go](https://github.com/windsorcli/cli/blob/main/cmd/explain.go)
