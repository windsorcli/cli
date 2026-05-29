---
title: "windsor hook"
description: "Print shell init code for the given shell."
---
# windsor hook

```sh
windsor hook <shell>
```

Print init code that wires 'windsor env' into your shell. The hook re-runs 'windsor env --hook' whenever your prompt fires, exporting Windsor's per-context environment variables automatically when you cd into a project.

Supported shells: zsh, bash, fish, tcsh, powershell.

Add the output to your shell's rc file (or evaluate it directly during shell startup) so the hook installs on every new session.

## Examples

```sh
# zsh / bash
eval "$(windsor hook zsh)"
eval "$(windsor hook bash)"

# fish
windsor hook fish | source

# powershell
windsor hook powershell | Out-String | Invoke-Expression
```

## See also

- [`env`](env.md), [`exec`](exec.md)
- Source: [cmd/hook.go](https://github.com/windsorcli/cli/blob/main/cmd/hook.go)
