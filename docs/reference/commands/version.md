---
title: "windsor version"
description: "Print the CLI version, commit, build date, Go toolchain, and platform."
---
# windsor version

```sh
windsor version
```

Print five lines: the semver Version, the build's Commit SHA, the Build Date, the Go toolchain that built the binary, and the target Platform (GOOS/GOARCH).

Snapshot builds emitted by goreleaser have ' (nightly build)' appended to the Version line so operators can tell at a glance that the binary is an unreleased main-branch build rather than a tagged release. Tagged releases use clean semver and are returned unchanged.

## Examples

```sh
$ windsor version
Version: 0.9.0
Commit SHA: 4e0d9104
Build Date: 2026-05-27T18:30:00Z
Go: go1.26.3
Platform: darwin/arm64
```

## See also

- Source: [cmd/version.go](https://github.com/windsorcli/cli/blob/main/cmd/version.go)
