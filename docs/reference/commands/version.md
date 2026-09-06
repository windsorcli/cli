---
title: "windsor version"
description: "Print the CLI version, commit, build date, Go toolchain, and platform."
---

```sh
windsor version
```

Print five lines: the semver Version, the build's Commit SHA, the Build Date, the Go toolchain that built the binary, and the target Platform (GOOS/GOARCH).

Goreleaser appends ' (nightly build)' to the Version line for snapshot builds. This marks the binary as an unreleased main-branch build, not a tagged release. Tagged releases show clean semver, unchanged.

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
