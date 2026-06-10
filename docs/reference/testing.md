---
title: "Testing"
description: "Schema for windsor test files (*.test.yaml) consumed by 'windsor test'."
---
# Testing

Schema for windsor test files (*.test.yaml) consumed by 'windsor test'. Test
files live under contexts/_template/tests/ and define cases that apply input
values to configuration, compose the blueprint in isolation, and assert that
specific terraform components and kustomizations are present (or absent).

## Fields

| Field | Type | Description |
|------|------|-------------|
| `cases` | `array<object>` | Test cases to execute. **(required)** |

## cases[]

| Field | Type | Description |
|------|------|-------------|
| `name` | `string` | Unique identifier for the test case. **(required)** |
| `exclude` | `object` | Components and kustomizations that must NOT be present in the composed blueprint. Same partial-match semantics as 'expect'. |
| `expect` | `object` | Components and kustomizations that must be present in the composed blueprint. Partial matching: only fields you specify are checked. |
| `expectError` | `boolean` | When true, the test passes only if blueprint composition fails. Use for testing invalid configurations that the framework should reject. Defaults to false. |
| `terraformOutputs` | `map<object>` | Mock outputs for terraform_output() expressions. Keys are component IDs; values are maps of output key to value. Example: terraformOutputs.network.vpc_id = "vpc-123". |
| `values` | `object` | Configuration values to apply before composing the blueprint. These override any existing configuration for the test. Dotted keys are allowed (e.g. 'cluster.driver: talos'). |

### cases[].exclude

| Field | Type | Description |
|------|------|-------------|
| `kustomize` | `array<object>` | Kustomizations to assert on. |
| `terraform` | `array<object>` | Terraform components to assert on. |

#### cases[].exclude.kustomize[]

| Field | Type | Description |
|------|------|-------------|
| `components` | `array<string>` | Expected kustomize components. Match is 'contains'. |
| `dependsOn` | `array<string>` | Expected dependency names. Match is 'contains'. |
| `name` | `string` | Kustomization name. Used as the match key. |
| `path` | `string` | Expected path. Asserted only when set. |
| `source` | `string` | Expected source name. Asserted only when set. |
| `substitutions` | `map<string>` | Expected PostBuild substitutions. Strict equality per key — every specified key must be present with the exact expected value. |

#### cases[].exclude.terraform[]

| Field | Type | Description |
|------|------|-------------|
| `dependsOn` | `array<string>` | Expected dependency IDs. Match is 'contains' — the actual list may have extras. |
| `inputs` | `object` | Expected input values. Strict equality per key — every specified key must be present with the exact expected value. |
| `name` | `string` | Component name (or path-derived ID when name is omitted). Used as the match key. |
| `path` | `string` | Module path. Used as the match key when 'name' is omitted. |
| `source` | `string` | Expected source name. Asserted only when set. |

### cases[].expect

| Field | Type | Description |
|------|------|-------------|
| `kustomize` | `array<object>` | Kustomizations to assert on. |
| `terraform` | `array<object>` | Terraform components to assert on. |

#### cases[].expect.kustomize[]

| Field | Type | Description |
|------|------|-------------|
| `components` | `array<string>` | Expected kustomize components. Match is 'contains'. |
| `dependsOn` | `array<string>` | Expected dependency names. Match is 'contains'. |
| `name` | `string` | Kustomization name. Used as the match key. |
| `path` | `string` | Expected path. Asserted only when set. |
| `source` | `string` | Expected source name. Asserted only when set. |
| `substitutions` | `map<string>` | Expected PostBuild substitutions. Strict equality per key — every specified key must be present with the exact expected value. |

#### cases[].expect.terraform[]

| Field | Type | Description |
|------|------|-------------|
| `dependsOn` | `array<string>` | Expected dependency IDs. Match is 'contains' — the actual list may have extras. |
| `inputs` | `object` | Expected input values. Strict equality per key — every specified key must be present with the exact expected value. |
| `name` | `string` | Component name (or path-derived ID when name is omitted). Used as the match key. |
| `path` | `string` | Module path. Used as the match key when 'name' is omitted. |
| `source` | `string` | Expected source name. Asserted only when set. |

## Examples

```yaml
cases:
  - exclude:
      terraform:
        - name: vnet
    expect:
      terraform:
        - dependsOn:
            - network
          name: vpc
          source: core
    name: aws-platform-includes-vpc
    values:
      platform: aws
  - expectError: true
    name: missing-required-fails
    values: {}
  - expect:
      kustomize:
        - name: dns
          substitutions:
            external_dns_zone_id: Z123456
    name: dns-substitution-uses-terraform-output
    terraformOutputs:
      network:
        external_dns_zone_id: Z123456
```

## See also

- [`windsor test`](/reference/cli/commands/test)
- [Blueprint reference](/reference/cli/blueprint), [Facets reference](/reference/cli/facets)
- [Blueprint testing](/blueprints/testing)
- Source schema: [pkg/runtime/config/schemas/artifacts/testing.yaml](https://github.com/windsorcli/cli/blob/main/pkg/runtime/config/schemas/artifacts/testing.yaml)
