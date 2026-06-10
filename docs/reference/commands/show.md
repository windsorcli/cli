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

- [`windsor show blueprint`](/reference/cli/commands/show-blueprint) — Display the fully rendered blueprint.
- [`windsor show kustomization`](/reference/cli/commands/show-kustomization) — Display the Flux Kustomization resource for a component.
- [`windsor show values`](/reference/cli/commands/show-values) — Display the effective context values.

## See also

- [`explain`](/reference/cli/commands/explain), [`plan`](/reference/cli/commands/plan)
- [Blueprint reference](/reference/cli/blueprint), [Configuration reference](/reference/cli/configuration)
- Source: [cmd/show.go](https://github.com/windsorcli/cli/blob/main/cmd/show.go)
