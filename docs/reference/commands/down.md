---
title: "windsor down"
description: "Stop the local workstation environment."
---
# windsor down

```sh
windsor down
```

Tear down the workstation VM, stop container runtimes, and clear local context artifacts (.kube, .talos, generated terraform stubs, etc.). Live infrastructure is NOT destroyed by down — run 'windsor destroy' first if you need to remove cloud resources. Workstation contexts only.

If any host-side network or DNS configuration was previously installed by 'windsor configure network', down prints a follow-up command at the end so the operator can clean up leftover host state.

## Examples

```sh
# Standard teardown
windsor down

# Full teardown including cloud infrastructure
windsor destroy --confirm=local
windsor down
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [`up`](up.md), [`destroy`](destroy.md), [`configure network`](configure-network.md)
- Source: [cmd/down.go](https://github.com/windsorcli/cli/blob/main/cmd/down.go)
