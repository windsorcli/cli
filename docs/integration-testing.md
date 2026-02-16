# Integration testing

## Refactor to make integration tests easier

1. **Shared project setup helper**
   - `SetupIntegrationProject(t, windsorYAML string) string`: create temp dir, write `windsor.yaml`, `os.Chdir` into it, register `t.Cleanup` to restore cwd. Returns project root.
   - Keeps integration tests DRY and ensures real `GetProjectRoot()` (from cwd) resolves to the test project.

2. **No runtime override by default**
   - Integration tests do not set `runtimeOverridesKey`. Commands call `runtime.NewRuntime()` with no opts, so Shell, ConfigHandler, Terraform, etc. are real.
   - To mock only specific pieces: build `rt := runtime.NewRuntime()` (after chdir so project root is correct), set `rt.<Field> = mock`, then `ctx := context.WithValue(..., runtimeOverridesKey, rt)` and run the command.

3. **Optional: isolate home dir for shell**
   - If commands touch the shell’s “trusted” or home-based paths, integration can override `UserHomeDir` (e.g. in `pkg/runtime/shell` shims or via a partial runtime override with a mock Shell that uses a temp home). Extend the helper if needed.

4. **Output capture**
   - Reuse existing `captureOutput(t)` and `rootCmd.SetOut` / `rootCmd.SetErr` so integration tests can assert on stdout/stderr. Restore `rootCmd`’s Out/Err in cleanup so the process doesn’t leak state.

5. **Build tag**
   - Use `//go:build integration` so default `go test ./cmd/` stays fast and `go test -tags=integration ./cmd/` runs the integration suite.

6. **Optional: external integration package**
   - If tests are moved to a top-level `integration/` package, export `RunWithContext(ctx context.Context, args []string) error` from `cmd` so the external package can run the CLI without accessing unexported `rootCmd`.

## Running integration tests

```bash
go test -tags=integration ./cmd/ -run Integration -v
```

Without the tag, integration test files are not built and these tests are not run.

## Task targets

| Task | Description |
|------|-------------|
| `task test` | Unit tests only (excludes integration-tagged tests). Writes `coverage.out` and `coverage.html`. |
| `task test:integration` | Only tests matching `TestIntegration` (e.g. `TestIntegration_BuildID`). Pass a custom filter with `task test:integration -- RunName`. |
| `task test:all` | Full suite (unit + integration) with one coverage run. Writes `coverage.out` and `coverage.html`. |

Use `task test` for fast CI; use `task test:all` when you want coverage that includes integration tests.

## Line-by-line coverage in VS Code

"Go: Toggle Test Coverage In Current Package" only runs unit tests for the current package. To see coverage that includes integration tests:

1. Generate the full coverage profile: run **Terminal → Run Task → Run Full Test Coverage** (or `task test:all` in the shell). This writes `coverage.out` in the workspace root.
2. Open Command Palette (**Cmd+Shift+P**) and run **Go: Apply Cover Profile**.
3. Enter the path to the profile. Use the full path (e.g. `/Users/you/cli/coverage.out`) if the extension does not accept `coverage.out` or `${workspaceFolder}/coverage.out`.

You get the same line-by-line gutter coverage, but from the combined unit + integration run.
