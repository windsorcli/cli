---
title: "windsor show"
description: "Display rendered resources."
---
# windsor show

```sh
windsor show
```

Print rendered Windsor resources to stdout. Reads the project state and runs blueprint composition without applying anything.

Unresolved deferred values render as '<deferred>' by default in 'blueprint' and 'kustomization' output; pass --raw to keep the original expression text instead.

## Subcommands

- [`windsor show blueprint`](show-blueprint.md) — Display the fully rendered blueprint.
- [`windsor show kustomization`](show-kustomization.md) — Display the Flux Kustomization resource for a component.
- [`windsor show values`](show-values.md) — Display the effective context values.

## See also

- [`explain`](explain.md), [`plan`](plan.md)
- [Blueprint reference](../blueprint.md), [Configuration reference](../configuration.md)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
