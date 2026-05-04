---
title: "windsor exec"
description: "Run a command with project env vars injected."
---
# windsor exec

```sh
windsor exec [--] <command> [args...]
```

Run a command with the project's environment variables and decrypted secrets injected. Useful for one-off commands that need the full Windsor environment without sourcing it into your shell.

`exec` is implicitly `--decrypt`: 1Password and SOPS secrets are dereferenced before the child process starts.

## Passing flags to the inner command

If the command you're running takes flags of its own — long (`--foo`) or short (`-x`) — pass `--` first so they aren't parsed as `windsor` flags. Without it, Cobra intercepts the flag and aborts with `unknown flag: --foo` or `unknown shorthand flag: 'x' in -x`.

```sh
windsor exec -- terraform plan --var-file=local.tfvars
windsor exec -- kubectl get pods -A
```

`--` is unnecessary only when the inner command takes no flags or only positional arguments — in practice almost always pass it.

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

- [Environment reference](../environment.md), [Environment Injection](https://www.windsorcli.dev/docs/cli/environment-injection)
- [Secrets Management](https://www.windsorcli.dev/docs/cli/secrets-management)
- [`env`](env.md), [`hook`](hook.md)
- Source: [cmd/exec.go](https://github.com/windsorcli/cli/blob/main/cmd/exec.go)
