---
title: "windsor down"
description: "Stop the local workstation environment."
---
# windsor down

```sh
windsor down
```

Tear down the workstation VM, stop container runtimes, and clear local context artifacts (`.kube`, `.talos`, generated terraform stubs, etc.). Live infrastructure is **not** destroyed by `down` — run [`destroy`](destroy.md) first if you need to remove cloud resources. Workstation contexts only.

## Flags

None.

## Examples

```sh
# Standard teardown
windsor down

# Full teardown including infrastructure
windsor destroy --confirm=local
windsor down
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [`up`](up.md), [`destroy`](destroy.md)
- Source: [cmd/down.go](https://github.com/windsorcli/cli/blob/main/cmd/down.go)
