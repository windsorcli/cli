---
title: "Testing"
description: "Schema and matching semantics for windsor test files."
---
# Testing

Reference for `.test.yaml` files consumed by [`windsor test`](commands/test.md). For the conceptual overview see [Blueprint testing](https://www.windsorcli.dev/docs/blueprints/testing).

Tests live under `contexts/_template/tests/` and use the `.test.yaml` extension. Each file declares one or more cases:

```yaml
cases:
  - name: aws-provider-includes-vpc
    values:
      provider: aws
    expect:
      terraform:
        - name: vpc
          source: core
    exclude:
      terraform:
        - name: vnet
```

## TestCase

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Unique name for the test case. |
| `values` | `map[string]any` | Configuration values to inject before composition. Dotted keys allowed (`cluster.driver: talos`). |
| `terraformOutputs` | `map[string]map[string]any` | Mock outputs for `terraform_output()` expressions. See [Terraform outputs](#terraform-outputs). |
| `expect` | `BlueprintExpectation` | Components that must exist with matching properties. |
| `exclude` | `BlueprintExpectation` | Components that must not exist. |

## BlueprintExpectation

| Field | Type | Description |
|-------|------|-------------|
| `terraform` | `[]TerraformExpectation` | Expected Terraform components. |
| `kustomize` | `[]KustomizeExpectation` | Expected Kustomizations. |

## TerraformExpectation

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Component name. |
| `path` | `string` | Component path. Use as an alternative match key when name is omitted. |
| `source` | `string` | Expected source. |
| `dependsOn` | `[]string` | Expected dependencies — partial match (contains). |

## KustomizeExpectation

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Kustomization name. |
| `path` | `string` | Expected path. |
| `source` | `string` | Expected source. |
| `dependsOn` | `[]string` | Expected dependencies — partial match. |
| `components` | `[]string` | Expected components — partial match. |

## Matching semantics

- **Partial matching** — only the fields you specify are checked. Fields you omit are ignored, even when present in the generated blueprint.
- **Array contains** — `dependsOn` and `components` pass when the generated array contains every value you list, in any order. Extra entries in the generated array are allowed.
- **Match by name or path** — components can be selected by `name` or by `path`; whichever you specify is the match key.

## Terraform outputs

When facets use `terraform_output("<component>", "<key>")`, supply mock values per component:

```yaml
terraformOutputs:
  network:
    vpc_id: "vpc-123456"
    subnet_ids:
      - "subnet-abc"
      - "subnet-def"
    cidr_block: "10.0.0.0/16"
```

- The outer key is the component id used in the `terraform_output()` call.
- Missing outputs resolve to `nil`, enabling `?? <fallback>` behavior.
- Mock outputs only live for the duration of the test — they don't touch real Terraform state.

## Automatic validation

Every test case automatically validates the composed blueprint for these errors. They cause the case to fail; you don't need to assert them in `expect`/`exclude`.

| Error | Trigger |
|-------|---------|
| Duplicate Terraform component | Two components share the same id (name or path). |
| Duplicate Kustomization | Two kustomizations share the same name. |
| Duplicate Kustomize component | A kustomization's `components` array has duplicate entries. |
| Circular dependency | A `dependsOn` chain forms a cycle. |
| Invalid dependency | A `dependsOn` references a non-existent component. |

Sample messages:

```
duplicate terraform component ID "cluster" (found at indices 0 and 2)
circular dependency detected in terraform components: a -> b -> a
terraform component "network" depends on non-existent component "missing"
kustomization "app" has duplicate component "base"
```

## Running

```bash
windsor test                # run every case
windsor test <case-name>    # run one case
```

`windsor test` runs against the active context's composed blueprint and does not modify the on-disk context.

## Common errors

| Message | Likely cause |
|---------|--------------|
| `No test files found` | No `.test.yaml` under `contexts/_template/tests/`. |
| `Composition error: ...` | Blueprint failed to generate — check `values` and facet expressions. |
| `terraform component not found: <name>` | The expected component wasn't in the generated blueprint. Often a `when` expression that didn't match. |
| `terraform output key '<X>' not found for component '<Y>'` | A `terraform_output()` references an output that isn't in `terraformOutputs`. |

## See also

- [`windsor test`](commands/test.md) — command flags and exit codes.
- [Blueprint testing](https://www.windsorcli.dev/docs/blueprints/testing) — conceptual overview, when to reach for it.
- [Facets reference](facets.md), [Blueprint reference](blueprint.md).
