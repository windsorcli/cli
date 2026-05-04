---
title: "windsor env"
description: "Print shell commands to export project env vars."
---
# windsor env

```sh
windsor env [flags]
```

Print shell commands that export the project's environment variables. Source the output, or rely on [`hook`](hook.md) to do this automatically when you `cd` into a project.

The variables include AWS, Kubernetes, Docker, Talos, and Terraform credentials and config paths derived from the current context.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--decrypt` | `false` | Decrypt secrets before exporting env vars. |
| `--hook` | `false` | Non-fatal mode: suppress warnings and exit `0` on errors so a misconfigured project never breaks the prompt. The shell hook installed by `windsor hook` invokes `windsor env --decrypt --hook` automatically. |

## Examples

```sh
# Source env vars manually
eval "$(windsor env)"

# Same, with secrets decrypted (1Password / SOPS)
eval "$(windsor env --decrypt)"

# Show what would be exported
windsor env
```

## See also

- [Environment reference](../environment.md), [Environment Injection](https://www.windsorcli.dev/docs/cli/environment-injection)
- [`hook`](hook.md), [`exec`](exec.md)
- Source: [cmd/env.go](https://github.com/windsorcli/cli/blob/main/cmd/env.go)
