---
title: "windsor configure"
description: "Configure workstation resources."
---
# windsor configure

```sh
windsor configure network [flags]
```

Configure workstation host/guest resources. Currently supports networking and DNS via the `network` subcommand.

## configure network

Run from the project root after the workstation Terraform component is applied. Wires VM-to-host networking and (optionally) configures the host to resolve a DNS service address that lives inside the VM.

| Flag | Default | Description |
|------|---------|-------------|
| `--dns-address` | `""` | DNS service address (typically a Terraform workstation output). When unset, DNS is not configured. |

If the current context has no workstation enabled, the command is a no-op and prints `workstation disabled`.

## Examples

```sh
# Wire network without DNS
windsor configure network

# Wire network and point host DNS at the VM service IP
windsor configure network --dns-address=10.5.0.2
```

## See also

- [Local Workstation guide](../../guides/local-workstation.md)
- [`up`](up.md)
- Source: [cmd/configure.go](https://github.com/windsorcli/cli/blob/main/cmd/configure.go)
