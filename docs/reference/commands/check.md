---
title: "windsor check"
description: "Verify required tools are installed."
---
# windsor check

```sh
windsor check
```

Runs the standard preflight in two passes: tool version checks for local CLIs (terraform, kubectl, talosctl, etc.), then a credential check for the platform configured on the current context (e.g. 'aws sts get-caller-identity' for platform aws).

Fails fast if a required tool is missing or at the wrong version, or if credentials don't resolve.

## Subcommands

- [`windsor check node-health`](check-node-health.md) — Check the health of cluster nodes.

## Examples

```sh
# Verify the toolchain and cloud credentials
windsor check
```

## See also

- [`upgrade`](upgrade.md), [`up`](up.md)
- Source: [cmd/check.go](https://github.com/windsorcli/cli/blob/main/cmd/check.go)
