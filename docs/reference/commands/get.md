---
title: "windsor get"
description: "Display Windsor resources."
---
# windsor get

```sh
windsor get contexts
windsor get context
```

Display Windsor resources. Currently supports listing contexts and printing the current context.

## get contexts

List contexts in the project. Output is a tab-aligned table with columns `NAME`, `PROVIDER`, `BACKEND`, `CURRENT`. The current context is marked with `*`. The `PROVIDER` column shows the v0.9 `platform` value (column header retained for compatibility); `BACKEND` shows the configured `terraform.backend.type` or `<none>` when unset.

```
NAME    PROVIDER  BACKEND  CURRENT
local   docker    <none>   *
prod    aws       s3
```

## get context

Print the name of the current context to stdout.

## Examples

```sh
windsor get contexts
windsor get context     # → local
```

## See also

- [Contexts guide](https://www.windsorcli.dev/docs/cli/contexts), [Contexts reference](../contexts.md)
- [`set`](set.md)
- Source: [cmd/get.go](https://github.com/windsorcli/cli/blob/main/cmd/get.go)
