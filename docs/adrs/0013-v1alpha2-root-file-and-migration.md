# ADR 0013 — v1alpha2 root file, project paths, and the migration command

- Status: Proposed.
- Date: 2026-11-14
- Deciders: Ryan VanGundy
- Base: [ADR 0001](0001-apiv2-common-header-schema.md). It makes `contexts/<name>/values.yaml` the
  file for per-context values and converts v1alpha1 on load. This ADR finishes that move for the
  root `windsor.yaml`.
- Surfaced by cli#3706. Milestone M10 of the cli v0.10.0 charter.

## Context

ADR 0001 point 1 says the root `windsor.yaml` holds `version` and project-wide settings, and
that the CLI reads the root `contexts:` map only to convert a v1alpha1 project. The code stops
short of that. It has no project-wide settings to hold, it accepts any key, and it offers no way
to migrate. Verified from source:

- **`LoadRoot` reads one key.** It decodes the root file into a struct with a `contexts` field
  and nothing else ([`typed_source.go`](../../pkg/runtime/config/typed_source.go)). Every other
  key, including `version`, is dropped without notice. Nothing in `pkg/` or `cmd/` branches on
  `version` today, so ADR 0001's remark that the root `version` selects the schema set no longer
  holds. A root file with a typo in a key loads without error.
- **One other reader exists.** `getTerraformDriver` in
  [`tools_manager.go`](../../pkg/runtime/tools/tools_manager.go) opens `windsor.yaml` itself and
  reads `terraform.driver`. This root key is not in any schema.
- **Four sources load, in this order.** Root `contexts.<name>`, then
  `contexts/<name>/windsor.yaml` (or `.yml`), then `.windsor/contexts/<name>/workstation.yaml`,
  then `values.yaml`. Later sources win. The first two are v1alpha1 forms. `convertV1alpha1` maps `provider` to `platform` on the
  merged map. It runs for every context, whatever the file version. No notice appears.
- **Two sites write the root file, both as `version: v1alpha1`.** `EnsureRoot`
  (`typed_source.go`) and `EnsureProjectAnchor` ([`anchor.go`](../../pkg/runtime/shell/anchor.go)).
- **Root `environment` does not exist.** `windsor env` reads `environment` from the merged
  context map ([`windsor_env.go`](../../pkg/runtime/env/windsor_env.go)). A root-level value
  reaches it only through the v1alpha1 `contexts:` map, one context at a time. cli#3706 asks for
  variables that apply to every context.
- **Directory names are hard-coded.** `filepath.Join` with a `"terraform"`, `"kustomize"`, or
  `"contexts"` literal appears 41 times in 19 non-test files under `pkg/` and `cmd/`, for example
  in `composer/terraform/module_resolver.go`, `provisioner/terraform/stack.go`,
  `runtime/terraform/backend.go`, and `composer/blueprint/handler_repository.go`. The artifact
  builder and the tag scanner hold the same names as string constants
  ([`artifact.go`](../../pkg/composer/artifact/artifact.go),
  [`tags.go`](../../pkg/composer/artifact/tags.go)). Some of the 41 joins build paths under
  `.windsor/`. The sweep MUST classify each one.
- **The bundle writes a fixed layout.** `Bundle` copies `contexts/_template` to `_template/`, and
  `kustomize` and `terraform` to the same names ([`artifact.go`](../../pkg/composer/artifact/artifact.go)).
  The archive does not carry `contexts/<name>/`.
- **Command names.** `windsor upgrade` upgrades a blueprint. No command named `migrate` exists.
- **The root loader reads `windsor.yaml` only.** Project-root detection also accepts
  `windsor.yml` ([`shell.go`](../../pkg/runtime/shell/shell.go)). This ADR does not change that.

## Decision

### 1. The v1alpha2 root file has three keys

```yaml
version: v1alpha2
environment:
  AWS_DEFAULT_REGION: us-east-2
paths:
  terraform: infra/terraform
  kustomize: infra/kustomize
  contexts: infra/contexts
```

`version` is required. `environment` and `paths` are optional. The root object is closed: the CLI
MUST reject any other key. The root file MUST NOT hold per-context values.

The CLI validates the root file against a fixed schema in `api/config/v1alpha2`. The schema is
separate from the per-context schema that ADR 0001 defines.

The CLI MUST fail the load for each of these, with a message that names the file, the key, and
the command `windsor migrate`:

- A `contexts:` key.
- Any key other than `version`, `environment`, and `paths`.
- A `version` value other than `v1alpha1` and `v1alpha2`.

`EnsureRoot` and `EnsureProjectAnchor` MUST write `version: v1alpha2` from the release that ships
this ADR.

A project with `contexts/<name>/windsor.yaml` or `windsor.yml` and a v1alpha2 root MUST fail the
load. The message names the file and the command. The CLI MUST NOT ignore the file, because the
settings in it would stop applying without notice.

### 2. Root `environment`

`environment` is a map of variable name to string. It follows the rules of the context
`environment` map in `values.yaml`.

Precedence, lowest first:

1. Schema defaults.
2. Root `environment`.
3. Context `environment` from `values.yaml`.

The merge is per key. A context value replaces a root value with the same name, and an empty
string counts as a value. A context cannot unset a root variable. Other root variables remain.

The loader adds the root map to the config data before the context sources, so the existing deep
merge gives the precedence. `GetStringMap("environment")` then returns the merged map, and
`windsor env` needs no new code path. `windsor show values` shows the merged map.

A root value MAY hold the same `${...}` expressions and secret references as a context value. The
CLI evaluates them in the scope of the active context, at the time `windsor env` runs. It
registers each resolved secret with the output scrubber, as it does for context values.
The root file is read before any context is known. Only the `environment` map reaches the
context data. The root file does not validate against a blueprint schema, because it exists
before blueprints load.

### 3. Root `paths`

`paths` has four keys: `base`, `terraform`, `kustomize`, and `contexts`. Each is a path relative
to the project root, except that `terraform`, `kustomize`, and `contexts` are relative to `base`.
The default of `base` is the project root. The defaults of the other three are `terraform`,
`kustomize`, and `contexts`. The setting is project-wide and never per context.

This example moves all three source directories under one directory:

```yaml
version: v1alpha2
paths:
  base: infra
```

It gives `infra/terraform`, `infra/kustomize`, and `infra/contexts`.

Validation. The CLI MUST reject a value that:

- is empty, absolute, or contains a `..` element;
- resolves, after cleaning, to a path inside `.windsor/`;
- for `terraform`, `kustomize`, or `contexts`, resolves to `base` itself;
- for `terraform`, `kustomize`, or `contexts`, equals or contains another one of these three.

`base` MAY be unset, which means the project root. A set `base` MUST NOT resolve to the project
root, because the default already does. The overlap rule does not apply to `base`, which contains
the other three by design. The CLI resolves symbolic links when it checks that every path stays
inside the project root.

What stays fixed:

- `windsor.yaml` and `.windsor/` stay at the project root. `base` moves the three source
  directories only.
- `.windsor/` is the CLI's state directory. It MUST NOT move. Users do not edit it, and tools
  find the active context through it.
- `_template` is the name of the template directory. Its location is `<paths.contexts>/_template`,
  so it moves with `contexts` and has no key of its own.
- The name `values.yaml` and the `contexts/<name>` layout under the configured directory.

One resolver owns every project directory. The runtime builds it from the root file once per
command, and exposes `Base()`, `Terraform()`, `Kustomize()`, `Contexts()`, `ContextDir(name)`, and
`TemplateDir()`, each returning an absolute path. `Terraform()`, `Kustomize()`, and `Contexts()`
return `base` joined with their key. No other package builds these paths. A test
MUST fail when a `filepath.Join` call outside the resolver uses one of the three literals. The
same test covers the string constants in `composer/artifact`.

`windsor bundle` reads the sources through the resolver and writes the standard layout. The
archive layout does not depend on the project paths, so every consumer of an artifact reads one
layout:

- `kustomize/` at the archive root.
- `terraform/` at the archive root.
- `_template/` at the archive root, as the builder writes it today.

A source that the project consumes is read at its standard layout, not through the project's
configured paths.

The `paths` work touches about 20 files that other changes also edit. It MUST land alone, after
the schema, `environment`, and migration changes, and with no other change in flight on those
packages.

### 4. Migration

The CLI adds `windsor migrate`. The name follows the verb-first rule, and the command is
separate from `windsor upgrade`, which upgrades blueprints.

```text
windsor migrate [--dry-run]
```

The command is project-wide. It takes no context argument. It converts every v1alpha1 source
into the v1alpha2 form:

1. For each context, merge the root `contexts.<name>` map and then
   `contexts/<name>/windsor.yaml`, in that order. These are the same sources and precedence that
   the loader uses.
2. Map `provider` to `platform`. When both are present, `platform` wins, as in `convertV1alpha1`.
   The command applies the same rule to a `provider` key in an existing `values.yaml`.
3. Compare the result with `contexts/<name>/values.yaml`, key path by key path.
   - A key that only the sources hold is added to `values.yaml`.
   - A key with an equal value in both needs no change.
   - A key with a different value in `values.yaml` is a conflict.
4. When any context has a conflict, the command MUST write nothing and MUST exit non-zero. It
   prints every conflict with the key path, both values, and both file names. The user resolves
   the conflict and runs the command again.
5. Otherwise the command writes each `values.yaml` in the header key order of ADR 0001 point 7,
   and deletes each `contexts/<name>/windsor.yaml`. It rewrites the root file to `version:
   v1alpha2`, with no `contexts:` key.
6. A root key outside `contexts` and `version` moves out of the root file. Today the only such
   key is `terraform.driver`. The command copies it into the `values.yaml` of each context that
   exists, and keeps a conflicting value as a conflict.

`--dry-run` runs steps 1 to 4, prints the planned moves and the conflicts, and writes nothing.
A project that has nothing to migrate exits with code 0 and prints that fact, so the command is
safe to run again. The command does not touch `.windsor/`.

The command creates no backup. Git holds the history, and the output of `--dry-run` lists every
file the command deletes.

`getTerraformDriver` reads `terraform.driver` from the merged context config after this change,
not from the root file.

### 5. Notice and deprecation

When the CLI loads a root file at `version: v1alpha1` (or with no `version`), or loads any
`contexts/<name>/windsor.yaml`, it prints one notice to stderr per command. The notice names each
file and says to run `windsor migrate`. A command that the shell hook runs on every prompt prints
the notice at most once per shell session.

The CLI reads v1alpha1 files, and converts them, in v0.10.x and v0.11.x. v0.12.0 removes the
read path, the `provider` mapping in the loader, and the `api/v1alpha1` types the conversion
needs. After that, a v1alpha1 root file fails the load with the message from point 1.
`windsor migrate` stays.

## Consequences

- **A root file with an unknown key stops loading.** Today such a key is ignored. A v1alpha2 root
  file now fails, with a message that names `windsor migrate` when the key is `contexts` or a
  v1alpha1 leftover.
- **The `provider` alias removal in ADR 0001 is an accepted breaking change.** A v1alpha2 file
  that sets only `provider` has no platform. `windsor migrate` converts the key. This ADR adds
  nothing to that decision. The release notes and the docs list it under breaking changes.
- **A root `terraform.driver` moves into `values.yaml`.** The driver stays per context. The tools
  check reads the merged config.
- **Custom directory names change every tool that reads the project tree.** Editors, CI scripts,
  and `.gitignore` entries that name `terraform/` or `contexts/` need the new names. The docs
  guide for `paths` says so.
- **Kustomization paths in a blueprint need a check.** A blueprint that names `kustomize/...` as
  a path for a git-hosted project may point at the wrong place when `paths.kustomize` is set. The
  `paths` change MUST test this case and either map the prefix or reject the combination with a
  clear error.
- **`windsor init` and the anchor write v1alpha2 roots.** Tests that compare the root file
  change on purpose.
- **Core and the docs move once.** `windsorcli/core` runs `windsor migrate` on its own
  `windsor.yaml` files. The docs gain a guide for the v1alpha2 root file and one for the
  migration.

## Implementation order

1. Root schema and loader: the closed schema, version handling, rejection messages, and the
   v1alpha2 writes in `EnsureRoot` and `EnsureProjectAnchor`.
2. Root `environment`, and the move of `terraform.driver` to the merged config.
3. `windsor migrate` and the load notice. This step is independent of step 2.
4. Root `paths` and the resolver, alone. The `Paths` type in `api/config/v1alpha2/root.go` gains
   `Base string` with `yaml:"base,omitempty"` in this step, with the rules. Step 1 does not add it.

Each step ships with unit tests and, for `windsor migrate` and the `paths` keys, integration
tests.

## Alternatives considered

- **Keep `contexts:` in the root file and warn.** Rejected: two files hold per-context values
  forever, and ADR 0001 point 1 already rejects that.
- **Allow unknown root keys.** Rejected: a typo such as `enviroment:` loads without effect.
- **Per-context `paths`.** Rejected: the project tree has one layout, and a per-context
  directory would break the shared `terraform/` and `kustomize/` sources.
- **Let `paths` move `.windsor/`.** Rejected: the state directory needs one known location, and
  nothing a user edits lives there.
- **Only the three keys, no `base`.** Rejected: a project that keeps all three under one
  directory, as the example in point 1 does, repeats that directory three times and can leave one
  out by mistake. Other tools (Hugo, Jekyll, Ansible, Nx) use per-kind directory keys, and
  Terraform and Terragrunt use a separate setting to move the whole working tree. A base plus
  per-kind keys is less common, but the combination is additive and breaks nothing.
- **A key for the template directory.** Rejected: the template belongs with the contexts, and a
  second key allows a split layout with no use.
- **Name the command `windsor upgrade --config`.** Rejected: `upgrade` moves a blueprint between
  versions, and a config conversion has different inputs and failure modes.
- **Last write wins on a conflict.** Rejected: the migration could silently discard a value that
  the user set on purpose.
- **Keep the v1alpha1 read path with no end date.** Rejected: the conversion code and the
  `provider` alias stay in every load path.

## References

- `pkg/runtime/config`: `typed_source.go`, `handler_persistence.go`.
- `pkg/runtime/tools/tools_manager.go`: `getTerraformDriver`.
- `pkg/runtime/shell/anchor.go`: `EnsureProjectAnchor`.
- `pkg/runtime/env/windsor_env.go`: `environment` evaluation.
- `pkg/composer/artifact`: `artifact.go`, `tags.go`.
- cli#3706: root-level configuration.
