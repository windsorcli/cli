---
title: "windsor list"
description: "List all available contexts."
---

```sh
windsor list
```

List contexts in the project. This is a shortcut for 'windsor get contexts' and prints the same table.

## Examples

```sh
windsor list

# Sample output:
#   NAME    PROVIDER  BACKEND  CURRENT
#   local   docker    <none>   *
#   prod    aws       s3
```

## See also

- [`get contexts`](get-contexts.md), [`set context`](set-context.md)
- Source: [cmd/list.go](https://github.com/windsorcli/cli/blob/main/cmd/list.go)
