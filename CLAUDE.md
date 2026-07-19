# Windsor CLI — Claude Code Context

Skills are defined in `.claude/skills/` and are discovered automatically by Claude Code, Cursor, and other adopting tools.

## Non-negotiable rules

1. **Function comment placement** — behavior documentation belongs in function header comments, never inside function bodies.

2. **Go testing** — use standard `go test` only. No `testify`. Follow `test-engineer` for test design and structure.

3. **Style and organization** — follow STYLE.md and `go-style` for section headers, file layout, and naming.

4. **Architecture boundaries** — follow `architecture` for cmd/runtime/evaluator/secrets/provisioner/composer ownership. Do not cross layer boundaries.

5. **Large cross-cutting work** — follow `large-pr` for phased implementation and change-map reporting.

6. **Security and pre-commit review** — run `/review-pr` before every commit; it runs `task scan`, `go vet`, and six parallel bug passes automatically. For a PR-level or cross-repo review instead, use the generic `code-review` skill.

7. **Integration tests** — every new CLI command or flag requires an integration test. Follow `integration-tests`.

8. **PRs** — use `create-pr` to push and open/update a PR, and `address-pr-feedback` to work through review comments and CI failures one finding at a time.

## Key commands

```
task test:all          # full unit + integration suite
task test:integration  # integration tests only
task scan              # gosec security scan
task build             # build for current platform
```
