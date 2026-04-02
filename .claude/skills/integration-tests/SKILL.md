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

Integration tests live in `integration/` and validate:

- Command and subcommand execution paths.
- Flag and parameter behavior (presence, defaults, overrides).
- Real file handling flows (config artifacts, facets, context files).
- Realistic output and exit codes rather than internal call patterns.

They do **not** validate internal helper sequencing or mock return values.

## Structure

```go
func TestCmd_Subcommand(t *testing.T) {
    t.Run("FlagOverridesDefault", func(t *testing.T) {
        // Given a real context directory with minimal config
        dir := t.TempDir()
        // write fixture files...

        // When the command runs with the flag
        output, err := runCLI(t, dir, "subcommand", "--flag=value")

        // Then the output reflects the override
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if !strings.Contains(output, "expected-string") {
            t.Errorf("output missing expected-string: %q", output)
        }
    })

    t.Run("MissingRequiredFlag", func(t *testing.T) {
        // Given no flag provided
        dir := t.TempDir()

        // When the command runs without the required flag
        _, err := runCLI(t, dir, "subcommand")

        // Then an error is returned
        if err == nil {
            t.Fatal("expected error, got nil")
        }
    })
}
```

## Coverage requirement

Every new CLI command or flag must have:
- At least one integration test covering its primary execution path.
- At least one error or edge case.

## Run integration tests

```
task test:integration
task test:all
```
