# Windsor CLI — Claude Code Context

Skills are defined in `.agent/skills/` and are discovered automatically by Claude Code, Cursor, and other adopting tools.

## Non-negotiable rules

1. **Function comment placement** — behavior documentation belongs in function header comments, never inside function bodies.

2. **Go testing** — use standard `go test` only. No `testify`. Follow the mandatory one-`t.Run` workflow from `test-engineer`.

3. **Style and organization** — follow STYLE.md and `go-style` for section headers, file layout, and naming.

4. **Architecture boundaries** — follow `architecture` for runtime/evaluator/secrets/terraform ownership. Do not cross layer boundaries.

5. **Large cross-cutting work** — follow `large-pr` for phased implementation and change-map reporting.

6. **Security and pre-commit review** — run `/review-pr` before every commit. It runs `task scan`, `go vet`, and five parallel bug passes automatically.

7. **Integration tests** — every new CLI command or flag requires an integration test. Follow `integration-tests`.

## Key commands

```
task test:all          # full unit + integration suite
task test:integration  # integration tests only
task scan              # gosec security scan
task build             # build for current platform
```
