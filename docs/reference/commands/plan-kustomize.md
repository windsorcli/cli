---
title: "windsor plan kustomize"
description: "Plan Flux kustomization changes."
---
# windsor plan kustomize

```sh
windsor plan kustomize [component]
```

Stream 'flux diff' for a specific kustomization, or all kustomizations when no argument is given. Inherits --summary, --json, and --no-color from the parent 'plan' command.

## Examples

```sh
# Stream the diff for one kustomization
windsor plan kustomize dns

# Compact summary across all kustomizations
windsor plan kustomize --summary
```

## See also

- [Lifecycle guide](https://www.windsorcli.dev/docs/cli/lifecycle)
- [`plan`](plan.md), [`apply kustomize`](apply-kustomize.md), [`destroy kustomize`](destroy-kustomize.md)
- Source: [cmd/plan.go](https://github.com/windsorcli/cli/blob/main/cmd/plan.go)
