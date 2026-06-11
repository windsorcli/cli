---
title: "windsor show blueprint"
description: "Display the fully rendered blueprint."
---
# windsor show blueprint

```sh
windsor show blueprint [flags]
```

Print the fully composed blueprint to stdout, including all fields from underlying sources and computed values. Defaults to YAML; use --json for JSON. Unresolved deferred values render as '<deferred>' by default; use --raw to keep the original expression text instead.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON instead of YAML. |
| `--raw` | `false` | Keep deferred expressions as text instead of <deferred>. |

## Examples

```sh
# Print the composed blueprint as YAML
windsor show blueprint

# Same, but keep deferred expressions visible
windsor show blueprint --raw

# JSON output for tooling
windsor show blueprint --json
```

## See also

- [`explain`](explain.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
