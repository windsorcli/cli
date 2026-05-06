---
title: "windsor set"
description: "Set a Windsor resource."
---
# windsor set

```sh
windsor set context <context-name>
```

Set a Windsor resource. Currently supports `context`.

## set context

```sh
windsor set context <context-name>
```

Switch the current context. Persists the choice to the project config. The context directory must already exist (created by [`init`](init.md)).

## Examples

```sh
windsor set context staging
windsor get context     # → staging
```

## See also

- [Contexts guide](https://www.windsorcli.dev/docs/cli/contexts), [Contexts reference](../contexts.md)
- [`init`](init.md), [`get`](get.md)
- Source: [cmd/set.go](https://github.com/windsorcli/cli/blob/main/cmd/set.go)
