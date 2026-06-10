---
title: "windsor set context"
description: "Switch the current context."
---
# windsor set context

```sh
windsor set context [context-name]
```

Switch the current context and persist the choice to the project config. The context directory must already exist (created by 'windsor init').

## Examples

```sh
windsor set context staging
windsor get context
# → staging
```

## See also

- [`init`](/reference/cli/commands/init), [`get context`](/reference/cli/commands/get-context)
- Source: [cmd/set.go](https://github.com/windsorcli/cli/blob/main/cmd/set.go)
