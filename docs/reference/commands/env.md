---
title: "windsor env"
description: "Print the project's environment variables."
---

```sh
windsor env [flags]
```

Print the project's environment variables as KEY=value lines.
Secrets show as ******** unless you pass --decrypt.

The output is for display. To set the variables in your shell, run 'windsor hook'
or use 'eval' with --hook. To run one command with them, use 'windsor exec'.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--decrypt` | `false` | Decrypt secrets before printing env vars. |
| `--hook` | `false` | Print shell commands that export the variables. Suppress warnings and exit 0 on errors so a misconfigured project never breaks the prompt. The shell hook installed by 'windsor hook' invokes 'windsor env --decrypt --hook' automatically. |
| `--shell` | `""` | Print setenv commands for the given shell instead of POSIX export commands. Only tcsh is supported. |

## Examples

```sh
# Show the variables
windsor env

# Show the variables with secrets decrypted (1Password / SOPS)
windsor env --decrypt

# Set the variables in the current bash or zsh shell
eval "$(windsor env --decrypt --hook)"

# tcsh
eval "`windsor env --shell tcsh`"
```

## See also

- [`hook`](hook.md), [`exec`](exec.md)
- Source: [cmd/env.go](https://github.com/windsorcli/cli/blob/main/cmd/env.go)
