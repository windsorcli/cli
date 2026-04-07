---
name: integration-tests
description: Write integration tests that verify CLI command and flag behavior end-to-end. Use when adding new commands, subcommands, or flags, or when expanding coverage in the integration/ directory.
---

# Windsor Integration Tests

## Apply when
- Adding a new CLI command or subcommand.
- Adding or changing a flag that affects command behavior.
- Expanding coverage in the `integration/` directory.
- Verifying realistic file handling flows against real config artifacts.

## Integration test intent

Integration tests live in `integration/` (package `integration`) and validate
CLI behavior end-to-end by running the built binary against real fixture
directories. They do **not** import `cmd` or any internal package. They do **not**
use mocks or test internal call sequences.

Every integration test must include:
- **At least one success path** — a test that expects `err == nil`.
- At least one error or edge case (untrusted directory, missing argument, etc.).

Skipping the success path is not acceptable. A command that only has failure tests
has no verified happy path.

## File location and build tag

```
integration/plan_test.go      ← one file per command group
```

```go
//go:build integration
// +build integration

package integration
```

## Helpers

All tests use the helpers in `integration/helpers/`:

| Helper | Purpose |
|---|---|
| `helpers.PrepareFixture(t, "name")` | Copy fixture to temp dir and run `windsor init` (directory is trusted). Returns `(dir, env)`. |
| `helpers.CopyFixtureOnly(t, "name")` | Copy fixture without running `init` (directory is **not** trusted). Returns `(dir, env)`. |
| `helpers.RunCLI(dir, args, env)` | Run the built binary. Returns `(stdout, stderr, err)`. |

Always pass the returned `env` to `RunCLI`. Append `WINDSOR_CONTEXT=<name>` when
the command needs a specific context.

## Fixture conventions

Fixtures live in `integration/fixtures/<name>/`. A minimal fixture needs:

```
fixtures/<name>/
  windsor.yaml                              # declares contexts
  contexts/_template/blueprint.yaml         # loaded as the template blueprint
```

`contexts/_template/` is Windsor's template root — its `blueprint.yaml` is merged
into every context's generated blueprint. The user blueprint at
`contexts/<ctx>/blueprint.yaml` is also merged if present.

**YAML pitfall:** `null` is a YAML keyword. Quote it when used as a string value:

```yaml
terraform:
  - path: "null"   # correct — string "null"
  - path: null     # wrong — parsed as nil, path becomes ""
```

## Example: command with external tooling (terraform)

For commands that invoke `terraform`, create a fixture with a minimal `.tf` file so
the success test runs without cloud credentials:

```
fixtures/plan/
  windsor.yaml
  contexts/_template/blueprint.yaml        # declares terraform component
  terraform/null/main.tf                   # minimal local config
```

```yaml
# windsor.yaml
version: v1alpha1
contexts:
  local: {}
```

```yaml
# contexts/_template/blueprint.yaml
kind: Blueprint
apiVersion: blueprints.windsorcli.dev/v1alpha1
metadata:
  name: plan
terraform:
  - path: "null"
```

```hcl
# terraform/null/main.tf
terraform {}
```

## Example test file

```go
//go:build integration
// +build integration

package integration

import (
    "strings"
    "testing"

    "github.com/windsorcli/cli/integration/helpers"
)

// Success path — every command must have one.
func TestPlanTerraform_SucceedsWithMinimalLocalConfig(t *testing.T) {
    t.Parallel()
    dir, env := helpers.CopyFixtureOnly(t, "plan")
    _, stderr, err := helpers.RunCLI(dir, []string{"init", "local"}, env)
    if err != nil {
        t.Fatalf("init local: %v\nstderr: %s", err, stderr)
    }
    env = append(env, "WINDSOR_CONTEXT=local")
    _, stderr, err = helpers.RunCLI(dir, []string{"plan", "terraform", "null"}, env)
    if err != nil {
        t.Fatalf("plan terraform null: %v\nstderr: %s", err, stderr)
    }
}

// Error path — untrusted directory.
func TestPlanTerraform_FailsWhenNotInTrustedDirectory(t *testing.T) {
    t.Parallel()
    dir, env := helpers.CopyFixtureOnly(t, "plan")
    _, stderr, err := helpers.RunCLI(dir, []string{"plan", "terraform", "null"}, env)
    if err == nil {
        t.Fatal("expected failure but command succeeded")
    }
    if !strings.Contains(string(stderr), "trusted") {
        t.Errorf("expected stderr to mention 'trusted', got: %s", stderr)
    }
}

// Error path — missing required argument.
func TestPlanTerraform_FailsWithNoArgument(t *testing.T) {
    t.Parallel()
    dir, env := helpers.PrepareFixture(t, "plan")
    _, _, err := helpers.RunCLI(dir, []string{"plan", "terraform"}, env)
    if err == nil {
        t.Fatal("expected failure but command succeeded")
    }
}
```

## Coverage requirement

Every new CLI command or flag must have:
- At least one success-path integration test.
- At least one error or edge case.

## Run integration tests

```
task test:integration
task test:all
```
