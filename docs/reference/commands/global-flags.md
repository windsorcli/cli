---
title: "Global flags"
description: "Persistent flags accepted by every windsor command."
---
# Global flags

These are persistent flags defined on the root command (`cmd/root.go`), not on any single
subcommand — they're accepted by every `windsor` command but don't appear in an individual
command's own `--help` flag list or in its reference page here.

| Flag | Default | Description |
|------|---------|-------------|
| `-v`, `--verbose` | `false` | Enable verbose output. |
| `--no-cache` | `false` | Bypass the OCI artifact cache and force re-download of remote sources. Propagates to the `NO_CACHE` environment variable that `ArtifactBuilder.Pull` reads; an explicit `--no-cache` always wins over a pre-existing `NO_CACHE` in the environment. |
| `--lock-timeout` | `0` (fail immediately) | Duration to wait for the stack lock before failing, e.g. `30s`, `5m`. Every command that acquires the per-context stack lock (`apply`, `up`, `bootstrap`, `destroy`, …) fails fast on contention by default; pass a duration to wait instead. See [`unlock`](unlock.md) for force-releasing a lock left behind by a killed holder. |

## Examples

```sh
# Force a fresh pull of every OCI source instead of using the cache
windsor apply --no-cache

# Wait up to 5 minutes for the stack lock instead of failing immediately
windsor apply --lock-timeout=5m
```

## See also

- [Contexts directory](../contexts.md) — where the state these flags affect (locks, cached
  artifacts) lives
- [`unlock`](unlock.md)
- Source: [cmd/root.go](https://github.com/windsorcli/cli/blob/main/cmd/root.go)
