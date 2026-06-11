---
title: "windsor exec"
description: "Run a command with project env vars injected."
---
# windsor exec

```sh
windsor exec [--] <command> [args...]
```

Run a command with the project's environment variables and decrypted secrets injected. Useful for one-off commands that need the full Windsor environment without sourcing it into your shell.

exec is implicitly --decrypt: 1Password and SOPS secrets are dereferenced before the child process starts.

If the command you're running takes flags of its own — long (--foo) or short (-x) — pass '--' first so they aren't parsed as 'windsor' flags. Without it, Cobra intercepts the flag and aborts with 'unknown flag'. The '--' is unnecessary only when the inner command takes no flags or only positional arguments.

## Examples

```sh
# Inner command has its own flags — separate with --
windsor exec -- terraform plan --var-file=staging.tfvars
windsor exec -- kubectl logs my-pod --tail=50
windsor exec -- helm install my-app ./chart --namespace=apps

# A wrapper script that takes no flags itself
windsor exec ./scripts/deploy.sh
```

## See also

- [`env`](env.md), [`hook`](hook.md)
- Source: [cmd/exec.go](https://github.com/windsorcli/cli/blob/main/cmd/exec.go)
