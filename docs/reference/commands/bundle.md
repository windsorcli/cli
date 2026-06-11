---
title: "windsor bundle"
description: "Bundle the blueprint into a .tar.gz archive."
---
# windsor bundle

```sh
windsor bundle [flags]
```

Bundle the current blueprint into a .tar.gz archive for sharing or offline deployment.

Uses metadata.yaml ('name', 'version') to derive the output filename when --tag is not given. If neither is set, --tag is required.

When --output is a directory, the filename is derived from the tag.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-o`, `--output` | `.` | Output path for the archive. May be a file or a directory. |
| `-t`, `--tag` | `""` | Tag in 'name:version' form. Required when metadata.yaml lacks name or version. |

## Examples

```sh
# Bundle using metadata.yaml for name/version, into the current directory
windsor bundle

# Explicit tag, auto-generated filename
windsor bundle -t myapp:v1.0.0

# Explicit tag, explicit output path
windsor bundle -t myapp:v1.0.0 -o ./dist/myapp-v1.0.0.tar.gz

# Tag set, output is a directory (filename auto-generated)
windsor bundle -t myapp:v1.0.0 -o ./dist/
```

## See also

- [Metadata reference](../metadata.md)
- [`push`](push.md)
- Source: [cmd/bundle.go](https://github.com/windsorcli/cli/blob/main/cmd/bundle.go)
