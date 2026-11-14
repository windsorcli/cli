---
title: "windsor migrate"
description: "Convert a v1alpha1 project to v1alpha2."
---

```sh
windsor migrate [flags]
```

Move the per-context settings from the root windsor.yaml and from contexts/<name>/windsor.yaml into contexts/<name>/values.yaml, and rewrite the root windsor.yaml to version v1alpha2. The command maps provider to platform and copies a root terraform.driver into each context.

If a key has different values in a source and in values.yaml, the command writes nothing, lists each conflict, and exits with an error. Resolve the conflicts and run the command again. A project with nothing to migrate exits with code 0.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Print the planned changes and the conflicts. Write nothing. |

## Examples

```sh
# List the files the command would write and delete
windsor migrate --dry-run

# Convert the project
windsor migrate
```

## See also

- [`init`](init.md), [`upgrade`](upgrade.md)
- Source: [cmd/migrate.go](https://github.com/windsorcli/cli/blob/main/cmd/migrate.go)
