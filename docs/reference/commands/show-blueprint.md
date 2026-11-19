---
title: "windsor show blueprint"
description: "Display the fully rendered blueprint."
---

```sh
windsor show blueprint [flags]
```

Print the fully composed blueprint to stdout, including all fields from underlying sources and computed values, except post-run messages (which bootstrap and up print separately, when applicable). Defaults to YAML; use --json for JSON. Unresolved deferred values render as '<deferred>' by default; use --raw to keep the original expression text instead. YAML output replaces a string longer than 20 lines with a placeholder; use --full to print every value. JSON output always prints every value.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--full` | `false` | Print strings longer than 20 lines in full instead of a placeholder. |
| `--json` | `false` | Output as JSON instead of YAML. |
| `--raw` | `false` | Keep deferred expressions as text instead of <deferred>. |

## Examples

```sh
# Print the composed blueprint as YAML
windsor show blueprint

# Print long multi-line strings in full
windsor show blueprint --full

# Same, but keep deferred expressions visible
windsor show blueprint --raw

# JSON output for tooling
windsor show blueprint --json
```

## See also

- [`explain`](explain.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
