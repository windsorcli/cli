---
title: "windsor exec"
description: "Run a command with project env vars injected."
---
# windsor exec

```sh
windsor exec <command> [args...]
```

Run a command with the project's environment variables and decrypted secrets injected. Useful for one-off commands that need the full Windsor environment without sourcing it into your shell.

`exec` is implicitly `--decrypt`: 1Password and SOPS secrets are dereferenced before the child process starts.

## Examples

```sh
# Run kubectl with the project's KUBECONFIG and credentials
windsor exec kubectl get pods -A

# Run a script that needs cloud credentials
windsor exec ./scripts/deploy.sh
```

## See also

- [Environment Injection guide](../../guides/environment-injection.md)
- [Secrets Management guide](../../guides/secrets-management.md)
- [`env`](env.md), [`hook`](hook.md)
- Source: [cmd/exec.go](https://github.com/windsorcli/cli/blob/main/cmd/exec.go)
