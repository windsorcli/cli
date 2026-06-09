---
title: "windsor unlock"
description: "Release a stuck stack lock."
---
# windsor unlock

```sh
windsor unlock [flags]
```

Force-release a stuck stack lock for the current context.

A holder killed before it could release (CI cancellation, OOM, crash) leaves the lock behind, so later commands block until timeout and then fail. This clears it. It does not check whether the holder is still alive, so only run it when no other windsor process is using this context.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Skip the confirmation prompt (for scripted recovery). |

## Examples

```sh
# Clear a stuck lock interactively
windsor unlock
# → prompts: Type "local" to confirm:

# Scripted recovery
windsor unlock --force
```

## See also

- [`destroy`](destroy.md), [`up`](up.md)
- Source: [cmd/unlock.go](https://github.com/windsorcli/cli/blob/main/cmd/unlock.go)
