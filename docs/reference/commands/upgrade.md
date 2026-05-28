---
title: "windsor upgrade"
description: "Upgrade cluster components."
---
# windsor upgrade

```sh
windsor upgrade
```

Upgrade cluster components. Currently supports Talos node upgrades via the 'cluster' (parallel) and 'node' (one-at-a-time, with health verification) subcommands.

## Subcommands

- [`windsor upgrade cluster`](upgrade-cluster.md) — Upgrade cluster nodes in parallel.
- [`windsor upgrade node`](upgrade-node.md) — Upgrade a single cluster node and wait for it to rejoin.

## See also

- [`check node-health`](check-node-health.md)
- Source: [cmd/upgrade.go](https://github.com/windsorcli/cli/blob/main/cmd/upgrade.go)
