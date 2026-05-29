---
title: "windsor build-id"
description: "Print or generate a build ID."
---
# windsor build-id

```sh
windsor build-id [flags]
```

Print the current build ID, or generate a new one with --new. Build IDs are stored in .windsor/.build-id and are available to your tools as:

  - the BUILD_ID environment variable
  - a postBuild substitution in Flux kustomizations

The ID starts with the current date (YYMMDD) and includes a same-day counter so artifacts tagged on the same day stay sortable.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--new` | `false` | Generate and print a new build ID. |

## Examples

```sh
# Read the current build ID
windsor build-id

# Generate a new build ID and tag an image with it
BUILD_ID=$(windsor build-id --new)
docker build -t myapp:$BUILD_ID .
```

## See also

- [`env`](env.md)
- Source: [cmd/build_id.go](https://github.com/windsorcli/cli/blob/main/cmd/build_id.go)
