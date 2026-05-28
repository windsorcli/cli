---
title: "windsor get contexts"
description: "List all available contexts."
---
# windsor get contexts

```sh
windsor get contexts
```

List contexts in the project. Output is a tab-aligned table with columns NAME, PROVIDER, BACKEND, CURRENT. The current context is marked with '*'. The PROVIDER column shows the configured platform (column header retained for backwards compatibility); BACKEND shows the configured terraform.backend.type or '<none>' when unset.

## Examples

```sh
windsor get contexts

# Sample output:
#   NAME    PROVIDER  BACKEND  CURRENT
#   local   docker    <none>   *
#   prod    aws       s3
```

## See also

- [Contexts guide](https://www.windsorcli.dev/docs/cli/contexts), [Contexts reference](../contexts.md)
- [`set`](set.md)
- Source: [cmd/get.go](https://github.com/windsorcli/cli/blob/main/cmd/get.go)
