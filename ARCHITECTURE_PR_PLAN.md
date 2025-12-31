# Architecture Refactoring: Unified Expression Evaluation with Terraform Output Provider System

## Overview

This document outlines a 3-4 PR plan to refactor the expression evaluation system to support:
- Unified expression evaluation through Runtime
- Terraform outputs fetched on-demand via provider system
- Config handler extensibility with dynamic value providers
- JIT evaluation for all expressions (terraform inputs, kustomize substitutions)
- Session-based caching for terraform outputs

## Architecture Goals

1. **Blueprint stores expressions, not values** - In-memory blueprint contains expressions that are evaluated JIT
2. **Config handler as plugin system** - Extensible with providers that handle dynamic values (terraform.*, cluster.*, etc.)
3. **Unified evaluator** - Single expression evaluator in Runtime used by all components
4. **On-demand terraform outputs** - Terraform outputs fetched when `terraform.*` references are encountered
5. **Session caching** - Cache terraform outputs per evaluation session to avoid repeated `terraform output` calls

---

## PR 1: Terraform Output Provider Foundation

### Goal
Create the foundation for on-demand terraform output fetching with session-based caching.

### Changes

#### New Package: `pkg/runtime/terraform`
- Create new package for terraform-specific runtime components
- Implement `TerraformOutputProvider` interface/struct:
  ```go
  type TerraformOutputProvider struct {
      // Session cache for outputs
      cache map[string]map[string]any
      
      // Dependencies
      terraformEnv *env.TerraformEnvPrinter
      blueprintHandler blueprint.BlueprintHandler
  }
  
  // GetOutput fetches terraform output for a component, using cache if available
  func (p *TerraformOutputProvider) GetOutput(componentID, outputKey string) (any, error)
  
  // GetOutputs fetches all outputs for a component
  func (p *TerraformOutputProvider) GetOutputs(componentID string) (map[string]any, error)
  
  // ClearCache clears the session cache
  func (p *TerraformOutputProvider) ClearCache()
  ```

#### Integration Points
- Reuse `TerraformEnvPrinter.CaptureTerraformOutputs()` for actual output capture
- Provider wraps this with caching layer
- Provider needs access to blueprint to resolve component paths

#### Testing
- Unit tests for caching behavior
- Tests for output fetching
- Tests for cache invalidation

### Files to Create
- `pkg/runtime/terraform/provider.go` - TerraformOutputProvider implementation
- `pkg/runtime/terraform/provider_test.go` - Tests

### Files to Modify
- None (foundation only)

### Success Criteria
- TerraformOutputProvider can fetch outputs with caching
- Cache works per session
- Can clear cache
- Tests pass

---

## PR 2: Config Handler Provider System

### Goal
Extend config handler to support dynamic value providers that handle `terraform.*` references.

### Changes

#### Config Handler Extensions
- Add provider registration mechanism:
  ```go
  type ConfigHandler interface {
      // ... existing methods ...
      
      RegisterProvider(prefix string, provider ValueProvider)
      GetWithProvider(key string) (any, error)
  }
  
  type ValueProvider interface {
      GetValue(key string) (any, error)
  }
  ```

- When `Get()` or `GetWithProvider()` encounters `terraform.*` pattern:
  - Detect pattern (e.g., `terraform.compute.outputs.controlplanes`)
  - Parse component ID and output key
  - Delegate to terraform provider
  - Provider fetches and caches output
  - Return value

#### Terraform Provider Integration
- Register terraform provider with config handler in Runtime initialization
- Provider implements `ValueProvider` interface
- Handles keys like `terraform.<componentID>.outputs.<outputKey>`

#### Testing
- Tests for provider registration
- Tests for `terraform.*` key resolution
- Tests for provider delegation
- Integration tests with terraform provider

### Files to Create
- `pkg/runtime/config/provider.go` - Provider interface and registration
- `pkg/runtime/config/provider_test.go` - Tests

### Files to Modify
- `pkg/runtime/config/config_handler.go` - Add provider system
- `pkg/runtime/runtime.go` - Register terraform provider during initialization
- `pkg/runtime/terraform/provider.go` - Implement ValueProvider interface

### Success Criteria
- Config handler can resolve `terraform.*` keys via provider
- Provider system is extensible (can add more providers later)
- Terraform outputs fetched on-demand when accessed
- Tests pass

---

## PR 3: Unified Expression Evaluator in Runtime

### Goal
Create unified expression evaluator in Runtime that uses config handler (with providers) for all expression evaluation.

### Changes

#### New Unified Evaluator in Runtime
- Create `pkg/runtime/evaluator.go`:
  ```go
  type ExpressionEvaluator struct {
      runtime *Runtime
  }
  
  // Evaluate evaluates a single expression
  func (e *ExpressionEvaluator) Evaluate(expression string, config map[string]any) (any, error)
  
  // EvaluateDefaults recursively evaluates default values
  func (e *ExpressionEvaluator) EvaluateDefaults(defaults map[string]any, config map[string]any) (map[string]any, error)
  
  // InterpolateString replaces ${} expressions in strings
  func (e *ExpressionEvaluator) InterpolateString(s string, config map[string]any) (string, error)
  ```

- Evaluator uses Runtime's config handler (which uses providers)
- When evaluating expressions, `terraform.*` references automatically go through provider system
- Supports all existing expression features (jsonnet, file, etc.)

#### FeatureEvaluator Migration
- Update `FeatureEvaluator` to use unified evaluator from Runtime
- Remove duplicate expr evaluation logic
- Keep FeatureEvaluator for blueprint-specific features (jsonnet, file loading)
- Delegate expression evaluation to Runtime evaluator

#### TerraformEnvPrinter Simplification
- Remove expression evaluation logic from TerraformEnvPrinter
- Remove `evaluateTerraformInputReferences`, `evaluateInputs`, etc.
- TerraformEnvPrinter only handles:
  - Environment variable generation
  - Terraform CLI args generation
  - Output capture (used by provider)

#### Testing
- Tests for unified evaluator
- Tests for terraform.* reference evaluation
- Integration tests with config provider system
- Ensure FeatureEvaluator still works correctly

### Files to Create
- `pkg/runtime/evaluator.go` - Unified expression evaluator
- `pkg/runtime/evaluator_test.go` - Tests

### Files to Modify
- `pkg/composer/blueprint/feature_evaluator.go` - Use Runtime evaluator
- `pkg/runtime/env/terraform_env.go` - Remove expression evaluation logic
- `pkg/provisioner/terraform/stack.go` - Use Runtime evaluator for input evaluation
- `pkg/runtime/runtime.go` - Add evaluator to Runtime struct

### Success Criteria
- All expression evaluation goes through unified evaluator
- Terraform.* references work via provider system
- FeatureEvaluator uses unified evaluator
- TerraformEnvPrinter simplified (no expr logic)
- Tests pass

---

## PR 4: Migration & Cleanup

### Goal
Complete migration to new system, ensure JIT evaluation works everywhere, remove all old evaluation logic.

### Changes

#### JIT Evaluation Points
1. **Terraform Input Evaluation** (during stack execution):
   - Before each terraform apply, evaluate component inputs
   - Terraform.* references fetch outputs on-demand
   - Inject as TF_VAR_* environment variables

2. **Kustomize Substitution Evaluation** (during kustomize apply):
   - Before creating ConfigMaps, evaluate substitutions
   - Terraform.* references fetch outputs on-demand
   - Create ConfigMaps with evaluated values

3. **Manual Terraform Execution** (via env vars):
   - When `terraform env` is loaded, evaluate inputs for current component
   - Terraform.* references fetch outputs on-demand
   - Inject as TF_VAR_* environment variables

#### Blueprint Expression Handling
- Ensure blueprint inputs/substitutions remain as expressions (not pre-evaluated)
- Expressions only evaluated when needed (JIT)
- Remove any code that pre-evaluates and stores values

#### Cleanup
- Remove all duplicate expression evaluation code
- Remove old `evaluateTerraformInputReferences` from stack
- Ensure no pre-evaluation happens during blueprint composition
- Update all call sites to use new system

#### Testing
- End-to-end tests for terraform input evaluation
- End-to-end tests for kustomize substitution evaluation
- Tests for manual terraform execution
- Tests for JIT evaluation behavior
- Ensure no regressions

### Files to Modify
- `pkg/provisioner/terraform/stack.go` - Use Runtime evaluator, remove old logic
- `pkg/provisioner/kubernetes/kubernetes_manager.go` - Use Runtime evaluator for substitutions
- `pkg/composer/blueprint/blueprint_handler.go` - Ensure expressions not pre-evaluated
- `pkg/runtime/env/terraform_env.go` - Final cleanup
- All test files - Update to use new system

### Success Criteria
- All JIT evaluation points work correctly
- Terraform inputs evaluate with terraform.* references
- Kustomize substitutions evaluate with terraform.* references
- Manual terraform execution works
- No duplicate evaluation logic remains
- All tests pass
- No regressions

---

## Migration Strategy

### Backward Compatibility
- Existing blueprints continue to work
- Expressions in blueprints are backward compatible
- No breaking changes to blueprint format

### Rollout
- Each PR is independently testable
- Can merge incrementally
- No need to complete all PRs before using

### Testing Strategy
- Unit tests for each component
- Integration tests for provider system
- End-to-end tests for full flow
- Manual testing for edge cases

---

## Future Enhancements (Post-Migration)

1. **Additional Providers**: Add providers for cluster.*, dns.*, etc.
2. **Provider Caching**: More sophisticated caching strategies
3. **Expression Optimization**: Cache compiled expressions
4. **Provider Plugins**: Allow external providers to be registered

---

## Notes

- Session cache is cleared after each operation (CLI tool, no stale cache concerns)
- Terraform outputs are never stored in config, always fetched on-demand
- Blueprint expressions are never serialized (unless user explicitly writes them)
- All evaluation is JIT - no pre-evaluation and storage


