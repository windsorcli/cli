---
title: "windsor show values"
description: "Display the effective context values."
---
# windsor show values

```sh
windsor show values [flags]
```

Print the effective context values, merging schema defaults with values.yaml overrides. YAML output includes schema descriptions as comments. Use --json for plain JSON.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON instead of YAML. |

## Examples

```sh
# Effective values for the current context, with schema descriptions
windsor show values

# Plain JSON for tooling
windsor show values --json
```

## See also

- [`explain`](explain.md)
- [Configuration reference](../configuration.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
