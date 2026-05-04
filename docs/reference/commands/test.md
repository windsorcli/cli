---
title: "windsor test"
description: "Run blueprint composition tests."
---
# windsor test

```sh
windsor test [test-name]
```

Run static tests that compare blueprint composition against expected outputs. Tests live in `contexts/_template/tests/`.

A test runs the blueprint composer in isolation (no terraform, no cluster, no live secrets) and asserts the resulting blueprint, kustomization, or values match a fixture. Use `windsor test` to validate that schema or facet changes don't accidentally regress composition.

When a test name is provided, only that test runs; otherwise every test under `contexts/_template/tests/` runs.

## Examples

```sh
# Run all tests
windsor test

# Run a single named test
windsor test cluster-defaults
```

## See also

- [Blueprint testing](https://www.windsorcli.dev/docs/blueprints/testing)
- [Testing reference](../testing.md)
- Source: [cmd/test.go](https://github.com/windsorcli/cli/blob/main/cmd/test.go)
