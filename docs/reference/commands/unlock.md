---
title: "windsor unlock"
description: "Clear stale stack lock information."
---

```sh
windsor unlock [flags]
```

Clear stale stack lock information for the current context.

The lock frees itself when its holder exits, including after a crash, an OOM kill, or a CI cancellation. A crash can leave holder details behind, and this command removes them. If a running process still holds the lock, the command names it and exits with an error. Stop that process to free the lock.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Skip the confirmation prompt (for scripted recovery). |

## Examples

```sh
# Clear stale lock information interactively
windsor unlock
# → prompts: Type "local" to confirm:

# Scripted recovery
windsor unlock --force
```

## See also

- [`destroy`](destroy.md), [`up`](up.md)
- [Global flags](../global-flags.md) — `--lock-timeout` waits for a lock instead of failing immediately
- Source: [cmd/unlock.go](https://github.com/windsorcli/cli/blob/main/cmd/unlock.go)
