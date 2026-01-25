---
title: "Blueprint Testing"
description: "Static testing for blueprint composition"
---
# Blueprint Testing

Windsor provides a static testing framework for validating blueprint composition without provisioning infrastructure. The `windsor test` command allows you to verify that given specific configuration values, your blueprints and facets produce the expected components.

## Overview

Blueprint tests validate:

- **Facet conditional logic** - Ensure `when` expressions evaluate correctly
- **Component inclusion** - Verify expected components are present
- **Component exclusion** - Confirm unwanted components are absent
- **Property assertions** - Check component properties match expectations
- **Blueprint integrity** - Automatic validation for duplicates, circular dependencies, and invalid references

Tests are defined as YAML files that specify input values and expected outputs.

## Automatic Validation

Every test automatically validates the generated blueprint for common issues:

### Duplicate Components

- **Duplicate Terraform Components**: Detects when multiple terraform components share the same ID (name or path)
- **Duplicate Kustomizations**: Detects when multiple kustomizations share the same name
- **Duplicate Kustomization Components**: Detects duplicate component names within a kustomization's `components` array

### Dependency Validation

- **Circular Dependencies**: Detects circular references in `dependsOn` chains for both terraform and kustomize components
- **Invalid Dependencies**: Detects `dependsOn` references that point to non-existent components

These validations run automatically for every test case and will cause the test to fail if any issues are detected. You don't need to explicitly test for these - they're always checked.

Example validation errors:

```
duplicate terraform component ID "cluster" (found at indices 0 and 2)
circular dependency detected in terraform components: a -> b -> a
terraform component "network" depends on non-existent component "missing"
kustomization "app" has duplicate component "base"
```

## Quick Start

1. Create a test file in `contexts/_template/tests/`:

```yaml
# contexts/_template/tests/provider.test.yaml
cases:
  - name: aws-provider-includes-vpc
    values:
      provider: aws
    expect:
      terraform:
        - name: vpc
          source: core
```

2. Run tests:

```bash
windsor test
```

3. View results:

```
✓ aws-provider-includes-vpc

1 passed, 0 failed
```

## Test File Structure

Test files use the `.test.yaml` extension and contain one or more test cases:

```yaml
cases:
  - name: descriptive-test-name
    values:
      # Configuration values to set before composition
      key: value
      nested.key: nested-value
    expect:
      # Components that MUST exist with matching properties
      terraform:
        - name: component-name
          source: expected-source
          path: expected/path
          dependsOn:
            - dependency-name
      kustomize:
        - name: kustomization-name
          path: expected/path
          components:
            - expected-component
    exclude:
      # Components that must NOT exist
      terraform:
        - name: excluded-component
      kustomize:
        - name: excluded-kustomization
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `cases` | `[]TestCase` | Array of test cases to run |
| `cases[].name` | `string` | Unique name for the test case |
| `cases[].values` | `map[string]any` | Configuration values to inject |
| `cases[].terraformOutputs` | `map[string]map[string]any` | Mock terraform outputs for `terraform_output()` expressions |
| `cases[].expect` | `BlueprintExpectation` | Components that must exist |
| `cases[].exclude` | `BlueprintExpectation` | Components that must not exist |

### BlueprintExpectation

| Field | Type | Description |
|-------|------|-------------|
| `terraform` | `[]TerraformExpectation` | Expected Terraform components |
| `kustomize` | `[]KustomizeExpectation` | Expected Kustomizations |

### TerraformExpectation

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Component name to match |
| `path` | `string` | Component path to match (alternative to name) |
| `source` | `string` | Expected source value |
| `dependsOn` | `[]string` | Expected dependencies (partial match) |

### KustomizeExpectation

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Kustomization name to match |
| `path` | `string` | Expected path value |
| `source` | `string` | Expected source value |
| `dependsOn` | `[]string` | Expected dependencies (partial match) |
| `components` | `[]string` | Expected components (partial match) |

## Matching Behavior

### Partial Matching

Expectations use **partial matching** - only specified fields are checked:

```yaml
expect:
  terraform:
    - name: cluster
      # Only checks that 'cluster' exists
      # Ignores source, path, inputs, etc.
```

```yaml
expect:
  terraform:
    - name: cluster
      source: core
      # Checks 'cluster' exists AND has source 'core'
      # Still ignores path, inputs, etc.
```

### Array Contains

For array fields (`dependsOn`, `components`), matching uses **contains semantics**:

```yaml
expect:
  terraform:
    - name: cluster
      dependsOn:
        - network
        # Passes if dependsOn includes 'network'
        # Does not require exact match
```

### Matching by Path

Components can be matched by `path` instead of `name`:

```yaml
expect:
  terraform:
    - path: vm/colima
      # Matches component with path 'vm/colima'
```

## Mocking Terraform Outputs

When your blueprints use `terraform_output()` expressions to reference outputs from other terraform components, you need to provide mock values for these outputs in your tests. Use the `terraformOutputs` field to specify mock outputs per component.

### Structure

```yaml
terraformOutputs:
  component-id:
    output-key: output-value
    another-key: another-value
```

The outer key is the terraform component ID (name or path), and the inner map contains the output key-value pairs.

### Example

```yaml
cases:
  - name: network-outputs-used-by-cluster
    values:
      cluster.enabled: true
    terraformOutputs:
      network:
        vpc_id: "vpc-123456"
        subnet_ids:
          - "subnet-abc"
          - "subnet-def"
        cidr_block: "10.0.0.0/16"
    expect:
      terraform:
        - name: cluster
          # Cluster component uses terraform_output("network", "vpc_id")
          # which will resolve to "vpc-123456" during test execution
```

### When to Use

Use `terraformOutputs` when:
- Your facets or component configurations use `terraform_output()` expressions
- You need to test behavior that depends on terraform outputs
- You want to verify that components correctly reference outputs from other components

### Notes

- If a `terraform_output()` expression references a component/output that isn't in `terraformOutputs`, it will return `nil` (enabling `??` fallback behavior)
- Mock outputs are only used during test execution - they don't affect actual terraform state
- The component ID in `terraformOutputs` must match the component ID used in the `terraform_output()` call

## Example Test Cases

### Testing Facet Conditions

```yaml
# Test that AWS provider includes AWS-specific components
cases:
  - name: aws-provider-components
    values:
      provider: aws
    expect:
      terraform:
        - name: vpc
        - name: eks-cluster
    exclude:
      terraform:
        - name: gke-cluster
        - name: aks-cluster

  - name: gcp-provider-components
    values:
      provider: gcp
    expect:
      terraform:
        - name: gke-cluster
    exclude:
      terraform:
        - name: eks-cluster
        - name: vpc
```

### Testing Driver Selection

```yaml
# Test cluster driver selection
cases:
  - name: talos-driver-selected
    values:
      cluster.driver: talos
    expect:
      terraform:
        - name: talos-cluster
          source: core
    exclude:
      terraform:
        - name: k3s-cluster

  - name: k3s-driver-selected
    values:
      cluster.driver: k3s
    expect:
      terraform:
        - name: k3s-cluster
    exclude:
      terraform:
        - name: talos-cluster
```

### Testing Dependencies

```yaml
# Test that flux depends on cluster
cases:
  - name: flux-depends-on-cluster
    values:
      kustomize.enabled: true
    expect:
      kustomize:
        - name: flux-system
          dependsOn:
            - cluster
          components:
            - base
            - sync
```

### Testing Feature Flags

```yaml
# Test optional feature enablement
cases:
  - name: observability-enabled
    values:
      observability.enabled: true
    expect:
      kustomize:
        - name: prometheus
        - name: grafana

  - name: observability-disabled
    values:
      observability.enabled: false
    exclude:
      kustomize:
        - name: prometheus
        - name: grafana
```

### Testing Terraform Output Dependencies

```yaml
# Test that components correctly use terraform outputs
cases:
  - name: cluster-uses-network-outputs
    values:
      cluster.enabled: true
      network.enabled: true
    terraformOutputs:
      network:
        vpc_id: "vpc-123456"
        subnet_ids:
          - "subnet-abc"
          - "subnet-def"
    expect:
      terraform:
        - name: cluster
          # Cluster component should be created with network outputs
          # The blueprint may use: terraform_output("network", "vpc_id")
```

### Testing Conditional Logic Based on Outputs

```yaml
# Test facets that depend on terraform outputs
cases:
  - name: high-availability-when-multiple-subnets
    values:
      cluster.enabled: true
    terraformOutputs:
      network:
        subnet_ids:
          - "subnet-1"
          - "subnet-2"
          - "subnet-3"
    expect:
      terraform:
        - name: cluster
          # Facet condition checks: len(terraform_output("network", "subnet_ids")) > 2
          # Should include HA configuration
```

## Running Tests

### Run All Tests

```bash
windsor test
```

### Run Specific Test

```bash
windsor test test-name
```

### Example Output

**All tests passing:**
```
✓ aws-provider-components
✓ gcp-provider-components
✓ talos-driver-selected
✓ flux-depends-on-cluster

4 passed, 0 failed
```

**Tests with failures:**
```
✓ aws-provider-components
✗ gcp-provider-components
  terraform component not found: gke-cluster
✓ talos-driver-selected
✗ flux-depends-on-cluster
  kustomize[flux-system].dependsOn: missing "cluster"

2 passed, 2 failed
```

## Best Practices

### Naming Conventions

Use descriptive names following the pattern `{condition}-{expectation}`:

```yaml
cases:
  - name: aws-includes-vpc           # Good
  - name: local-excludes-cloud       # Good
  - name: test-1                     # Bad - not descriptive
```

### Test Organization

Organize tests by feature area:

```
contexts/_template/tests/
├── provider.test.yaml      # Provider-specific facets
├── cluster.test.yaml       # Cluster driver selection
├── networking.test.yaml    # Network configuration
└── observability.test.yaml # Observability stack
```

### Focus on Boundaries

Test facet condition boundaries - the cases where behavior changes:

```yaml
cases:
  # Test the boundary where observability toggles
  - name: observability-on
    values:
      observability.enabled: true
    expect:
      kustomize:
        - name: prometheus

  - name: observability-off
    values:
      observability.enabled: false
    exclude:
      kustomize:
        - name: prometheus
```

### Minimal Values

Only set values that affect the test outcome:

```yaml
# Good - minimal values
cases:
  - name: aws-vpc
    values:
      provider: aws
    expect:
      terraform:
        - name: vpc

# Avoid - unnecessary values
cases:
  - name: aws-vpc
    values:
      provider: aws
      cluster.enabled: true
      cluster.driver: talos
      cluster.workers.count: 3
      # ... many unrelated values
    expect:
      terraform:
        - name: vpc
```

### Test Both Inclusion and Exclusion

When testing mutual exclusion, verify both sides:

```yaml
cases:
  - name: aws-selected
    values:
      provider: aws
    expect:
      terraform:
        - name: vpc
    exclude:
      terraform:
        - name: vnet      # Azure
        - name: gcp-net   # GCP
```

## Troubleshooting

### "No test files found"

Ensure test files are in `contexts/_template/tests/` and use the `.test.yaml` extension.

### "Composition error"

The blueprint failed to generate. Check:
- Required values are set in the test case
- Facet expressions are valid
- Blueprint structure is correct

### "Component not found"

The expected component wasn't in the generated blueprint. Check:
- Facet `when` condition evaluates correctly for the given values
- Component name/path matches exactly
- No typos in component names

### "terraform output key 'X' not found for component 'Y'"

A `terraform_output()` expression is trying to access an output that doesn't exist in your `terraformOutputs`. Check:
- The component ID in `terraformOutputs` matches the component ID in the `terraform_output()` call
- The output key exists in the mock outputs for that component
- The output key name matches exactly (case-sensitive)

<div>
  {{ footer('Sharing Blueprints', '../sharing/index.html', 'Hello, World!', '../../tutorial/hello-world/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../sharing/index.html'; 
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/hello-world/index.html'; 
  });
</script>
