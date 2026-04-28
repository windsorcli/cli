---
title: "windsor build-id"
description: "Print or generate a build ID."
---
# windsor build-id

```sh
windsor build-id [--new]
```

Print the current build ID, or generate a new one with `--new`. Build IDs are stored in `.windsor/.build-id` and are available to your tools as:

- the `BUILD_ID` environment variable
- a postBuild substitution available in Flux kustomizations

The format is `YYMMDD.RANDOM.#` where `RANDOM` is a 3-digit collision-prevention suffix and `#` is a same-day counter.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--new` | `false` | Generate and print a new build ID. |

## Examples

```sh
# Read the current build ID
windsor build-id          # → 250428.137.3

# Generate a new build ID and tag an image with it
BUILD_ID=$(windsor build-id --new)
docker build -t myapp:$BUILD_ID .
```

## See also

- [Local Workstation guide](../../guides/local-workstation.md)
- [Hello, World! tutorial](../../tutorial/hello-world.md)
- Source: [cmd/build_id.go](https://github.com/windsorcli/cli/blob/main/cmd/build_id.go)
