---
title: "windsor configure network"
description: "Configure workstation host/guest networking and DNS."
---
# windsor configure network

```sh
windsor configure network [flags]
```

Run after 'windsor up' has provisioned the workstation. Installs the host route and in-VM forwarding required for cluster reachability on VM-backed runtimes, and writes the per-domain DNS resolver entry so '*.<dns.domain>' resolves to the cluster's DNS service.

Prompts for sudo on macOS/Linux; must be run from an Administrator PowerShell on Windows. Use --dry-run to preview without modifying host state, or --revert to remove the host configuration this command previously installed.

If the current context has no workstation enabled, the command is a no-op and prints 'workstation disabled'.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dns-address` | `""` | DNS service address (e.g. from Terraform workstation output) |
| `--dry-run` | `false` | Describe what 'configure network' would do without invoking sudo or modifying host state |
| `--revert` | `false` | Remove the host route, in-VM forwarding, and DNS resolver entry previously installed by 'configure network' |

## Examples

```sh
# Wire network using the DNS address from Terraform workstation output
windsor configure network

# Preview the changes without invoking sudo
windsor configure network --dry-run

# Wire network and explicitly set the DNS service address
windsor configure network --dns-address=10.5.0.2

# Remove the host configuration installed by this command
windsor configure network --revert
```

## See also

- [`up`](/reference/cli/commands/up)
- Source: [cmd/configure.go](https://github.com/windsorcli/cli/blob/main/cmd/configure.go)
