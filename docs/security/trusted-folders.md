---
title: "Trusted Folders"
description: "Windsor only injects environment variables in folders you have explicitly trusted."
---
# Trusted Folders

A Windsor project drives terraform, secrets backends, and shell environment injection from files in your repository. If you `cd` into a malicious project, those files would otherwise be evaluated automatically — a clear environment-injection vector.

Windsor mitigates this by gating environment injection behind an explicit trust step. You must run `windsor init` in a project before [`windsor env`](../reference/commands/env.md) (and the [shell hook](../reference/commands/hook.md)) will load anything from it.

## How trust is recorded

Trusted directories are stored in `$HOME/.config/windsor/.trusted`. Each line is an absolute path to a project root that you have run `windsor init` in. Any subdirectory of a trusted folder inherits the trust.

The file is a plain newline-delimited list. You can inspect or edit it directly:

```sh
cat ~/.config/windsor/.trusted
```

## Reviewing a project before you trust it

Before running `windsor init` in a project you didn't author, review:

1. **`windsor.yaml`** — the project root config. Watch for unfamiliar `terraform.backend` settings, unexpected secrets backends, or overrides that point at untrusted endpoints.
2. **`contexts/<context>/blueprint.yaml`** — the blueprint. Check the `repository.url` and any `sources` entries; these can pull external OCI artifacts.
3. **`contexts/<context>/values.yaml`** — context values. Look for cluster endpoints, registry URLs, or DNS overrides that don't match what you expect.
4. **`contexts/_template/`** — facets, schema, metadata. Facets carry expressions that run during composition; an unfamiliar facet should be read like a script.

A useful drive-by command:

```sh
windsor show blueprint --raw     # see exactly what the project would compose
```

`--raw` keeps deferred expressions visible so you can read what they do before any terraform runs.

## Removing trust

To untrust a folder, edit `~/.config/windsor/.trusted` and remove the line. Windsor will re-prompt the next time you `windsor init` in that path.

```sh
# Remove a single project from trusted folders
sed -i.bak '\|/path/to/project|d' ~/.config/windsor/.trusted
```

## What still works without trust

Commands that do not inject environment variables — [`version`](../reference/commands/version.md), [`hook`](../reference/commands/hook.md), [`get`](../reference/commands/get.md) — work in any directory. So does the shell hook itself; it simply emits nothing for untrusted directories.

## See also

- [Securing Secrets](secrets.md)
- [Environment Injection guide](../guides/environment-injection.md)
- [`init`](../reference/commands/init.md), [`env`](../reference/commands/env.md), [`hook`](../reference/commands/hook.md)
