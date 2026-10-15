# ADR 0001 — v1alpha2 config header and v1alpha1 retirement

- Status: Accepted and implemented. Revised 2026-10-12 after a source audit showed that several
  premises of the first draft were wrong. Revised 2026-10-14 to withdraw the scrubbing in point 8,
  after a caller audit showed that the evaluator needs raw values.
- Date: 2026-08-04, revised 2026-10-12 and 2026-10-14
- Deciders: Ryan VanGundy
- The black-box tests in `pkg/runtime/config/*_public_test.go` pin current behavior. This ADR's
  implementation MUST keep them passing, except the tests named under Consequences, which change
  on purpose.

## Context

The CLI and the blueprint (`windsorcli/core`) each contribute a schema fragment, and the CLI merges
them at runtime into one schema per context. Core may define the same fields as the CLI. The CLI
needs three guarantees from that merge:

1. Every blueprint's merged schema contains the settings the CLI needs.
2. Those settings match functions the CLI has, such as its supported platforms and cloud
   integrations.
3. `windsor init` writes `values.yaml` keys in an order the CLI defines, `id` first.

The current code gives none of these. Verified from source:

- **Four files hold context config, with overlapping roles.** In order of precedence, lowest
  first:
  1. The root `windsor.yaml` `contexts.<name>` map. `LoadRoot` decodes it through
     `v1alpha1.Config`, so it keeps only typed fields and drops every other key without notice.
  2. `contexts/<name>/windsor.yaml` (or `.yml`), read as an untyped map.
  3. `.windsor/contexts/<name>/workstation.yaml`, system state that the CLI writes.
  4. `contexts/<name>/values.yaml`, the file `windsor init` and `SaveConfig` write.

  The root file and `values.yaml` can hold the same settings. The root file's `version` also
  selects the schema set ([`typed_source.go:68`](../../pkg/runtime/config/typed_source.go#L68)).
- **The v1alpha2 Go types do not match the file layout, and nothing uses them.**
  `api/v1alpha2/config.Config` nests settings under `providers:` and `workstation.cluster`, but
  `values.yaml` and every schema fragment are flat: `id`, `platform`, and `aws` are top-level keys.
  No code decodes into the v1alpha2 types. The directory `api/v1alpha2/config` declares
  `package v1alpha2`, which does not match its path.
- **The CLI's v1alpha2 schema fragments never load in practice.** `LoadRoot` loads them only when
  the root `version` is not `v1alpha1`. `EnsureRoot` writes `v1alpha1`, core's own `windsor.yaml`
  says `v1alpha1`, and no code writes another version. The only CLI fragment that loads is
  [`schemas/common.yaml`](../../pkg/runtime/config/schemas/common.yaml), which defines `platform`
  and `workstation`.
- **The CLI fragment already wins the merge.** Fragments load in this order:
  1. `contexts/_template/schema.yaml`, during `LoadConfig`, when the project has one.
  2. The v1alpha2 fragments, during `LoadConfig`, for a non-v1alpha1 root (never, as above).
  3. Each blueprint source's `schema.yaml`, during `LoadBlueprint`
     ([`loader.go:501`](../../pkg/composer/blueprint/loader.go#L501)).

  `common.yaml` merges again after every one of them
  ([`handler.go`](../../pkg/runtime/config/handler.go), `applySystemSchemaPlugins`), so it is
  always the last overlay.
- **A type conflict replaces the field without notice.** When two fragments give one property
  different types, the later fragment replaces it whole
  ([`schema_merge.go`](../../pkg/runtime/config/schema_merge.go), `typesConflict`). Compatible
  fragments merge conservatively: required lists union, enums intersect, and bounds tighten.
- **Core closes its root object.** Core's schema sets `additionalProperties: false` at the root
  and defines about 35 top-level properties. A key that no fragment defines fails validation.
- **The CLI reads a small, known set of paths.** Outside `pkg/runtime/config`, the CLI reads
  these by key or through `GetConfig()`:

  | Key | Paths the CLI reads |
  |---|---|
  | `id`, `platform`, `dev`, `environment` | the value itself |
  | `provider` | legacy alias for `platform` |
  | `aws` | `profile`, `region`, `endpoint_url`, `mwaa_endpoint`, `s3_hostname` |
  | `azure` | `environment`, `kubelogin_mode`, `region`, `subscription_id`, `tenant_id` |
  | `gcp` | `credentials_path`, `project_id`, `quota_project` |
  | `vsphere` | `server`, `user`, `insecure` |
  | `terraform` | `enabled`, `backend.*`, `lock.timeout` |
  | `secrets` | `sops.enabled`, `onepassword.vaults` |
  | `workstation` | `runtime`, `address`, `dns.address`, `arch` |
  | `dns` | `domain`, `enabled` |
  | `network` | `cidr_block`, `loadbalancer_ips.start`, `loadbalancer_ips.end` |
  | `cluster` | `driver`; `controlplanes` and `workers` `count`/`cpu`/`memory`, for colima sizing only; `workers.volumes` |
  | `docker` | `enabled`, `registry_url`, `registries.*.remote`, `registries.*.hostport` |
  | `gitops` | `mode`, `namespace` |

- **Writes are alphabetical.** `SaveConfig` marshals a map, and `RenderValuesWithDescriptions`
  sorts keys with `sortedUnionKeys`
  ([`values_renderer.go`](../../pkg/runtime/config/values_renderer.go)). Nothing carries a
  defined order.
- **Sensitive values have two enforcement sites and no chokepoint.** `RedactSensitiveValues`
  covers `windsor show`, and `sensitivePathsInValue`
  ([`processor.go`](../../pkg/composer/blueprint/processor.go)) guards `substitute:`.
  `GetContextValues` returns sensitive values unscrubbed, so each new consumer must remember a
  guard.

## Decision

### 1. One file per role

- `windsor.yaml` at the project root is the project file. It holds `version` and project-wide
  settings. It does not hold per-context values.
- `contexts/<name>/values.yaml` is the only file for per-context values that an operator writes.
- `.windsor/contexts/<name>/workstation.yaml` stays as system state that the CLI owns.

The root `contexts:` map and `contexts/<name>/windsor.yaml` are v1alpha1 forms. The CLI MUST read
them only to convert a v1alpha1 project (point 9). It MUST NOT write them.

### 2. A flat header type that mirrors the file

The header is one Go type, `Context`, in a new package `api/config/v1alpha2`. Its fields are flat
and match the top-level keys of `values.yaml`:

```go
type Context struct {
    ID          string            `yaml:"id"`
    Platform    string            `yaml:"platform"`
    Dev         *bool             `yaml:"dev,omitempty"`
    Environment map[string]string `yaml:"environment,omitempty"`
    Secrets     *Secrets          `yaml:"secrets,omitempty"`
    AWS         *AWS              `yaml:"aws,omitempty"`
    Azure       *Azure            `yaml:"azure,omitempty"`
    GCP         *GCP              `yaml:"gcp,omitempty"`
    VSphere     *VSphere          `yaml:"vsphere,omitempty"`
    Terraform   *Terraform        `yaml:"terraform,omitempty"`
    Workstation *Workstation      `yaml:"workstation,omitempty"`
    DNS         *DNS              `yaml:"dns,omitempty"`
    Network     *Network          `yaml:"network,omitempty"`
    Cluster     *Cluster          `yaml:"cluster,omitempty"`
    Docker      *Docker           `yaml:"docker,omitempty"`
    GitOps      *GitOps           `yaml:"gitops,omitempty"`
}
```

The config handler decodes the merged map into this type. Keys the header does not define stay in
the map, validated against the merged schema, and reach core unchanged.

### 3. The header holds only paths the CLI reads

A path is in the header only when code in `pkg/` or `cmd/` reads it to make a decision. The table
under Context is the starting list. A shared object such as `dns`, `network`, or `cluster` carries
only the CLI's subfields. Core defines the rest, and the merge unions them.

`cluster` node counts and sizes are read only for colima VM sizing. They are not in the header.
Colima reads them as untyped values that the blueprint defines, and the header keeps
`cluster.driver` alone.

Settings that only the blueprint reads stay out of the CLI fragment, even when the CLI fragment
defined them before. `workstation.services` and `workstation.dns.forward` are examples: core
defines both, with the same defaults.

### 4. One CLI schema fragment, always loaded last

The CLI ships one schema fragment that matches the header type field for field. It replaces
`common.yaml` and the four v1alpha2 fragments. The CLI merges it after every schema fragment it
loads, whatever the root `version`, as `common.yaml` merged before. Every merged schema therefore
contains every CLI-owned path. A context that loads no fragment has no schema, and validation does
not run for it.

The fragment adds properties and types only. It MUST NOT close an object, so a blueprint can
extend every object that the CLI and the blueprint share.

A test MUST fail when the header type and the fragment disagree on a path or a type.

### 5. A conflicting blueprint fails the load

When a blueprint fragment gives a CLI-owned path a type that conflicts with the CLI fragment, the
load MUST fail with an error that names the path and the blueprint source. A blueprint MAY define
a CLI-owned path with the same type, MAY add constraints, and MAY narrow an enum. Paths that the
CLI does not own keep the current merge behavior.

### 6. CLI functions define the platform and cloud fields

The `platform` enum MUST equal the list of platforms the CLI supports, either generated from that
list or checked against it by a test. A cloud block (`aws`, `azure`, `gcp`, `vsphere`) is in the
header only when the CLI has an integration behind it.

`provider` is not in the v1alpha2 header. Conversion from v1alpha1 maps `provider` to `platform`
(point 9).

### 7. The header defines the write order

When the CLI writes a new `values.yaml`, header keys come first, in the field order of `Context`,
then the remaining keys in alphabetical order. `RenderValuesWithDescriptions` uses the same order.
A patch write keeps the existing file's order.

The order comes from the struct by reflection over its `yaml` tags, so the type is the only place
that defines it.

### 8. `GetContextValues` returns raw values

`GetContextValues` does not remove sensitive values. The expression evaluator reads it as the
scope for every `${...}` expression. Core uses sensitive values through that scope, in terraform
inputs, facet `when:` and `requires:` checks, and Flux `secrets:` entries. A scrubbed map would
break those uses and protect only callers that read non-sensitive keys.

Sensitive values are guarded at each sink:

- `windsor show values` replaces them with a placeholder (`RedactSensitiveValues`).
- The substitution guard (`sensitivePathsInValue`) rejects a bare `${path}` that names one.
- Terraform marks a variable `sensitive` to keep it out of plan output.

Issue #3570 tracks two further guards: registering sensitive values with the output scrubber,
and a substitution check that parses the expression.

### 9. v1alpha1 converts into v1alpha2

v1alpha2 is the only version the rest of the code uses. A project at `version: v1alpha1` loads
through a conversion function into the v1alpha2 header. The conversion reads the root `contexts:`
map and `contexts/<name>/windsor.yaml`, and it maps `provider` to `platform`. The `api/v1alpha1`
config types then hold only what the conversion needs. Each of the importing packages moves to the
v1alpha2 header in its own change. The conversion reads the files as untyped maps, so the
`api/v1alpha1` config types are not needed once every package reads the header.

`api/config/v1alpha2` holds types, tags, doc comments, and the embedded fragment. It has no
`Merge` methods, because the config handler merges maps before it decodes the header.

## Consequences

- **The CLI schema guarantee becomes real.** Every context's schema contains the CLI-owned paths,
  and an incompatible blueprint fails with a clear error instead of a silent replacement.
- **Values that a closed blueprint object rejected now validate.** Core closes `vsphere` and
  `terraform` without some fields the CLI reads, such as `vsphere.server` and
  `terraform.lock.timeout`. The CLI fragment adds those fields to the merged schema.
- **`vm` is not in v1alpha2.** `workstation` replaces it. The CLI still reads `vm.runtime` and the
  `vm` sizing fields as untyped values until those reads move to `workstation`.
- **`workstation` is no longer closed by the CLI.** A blueprint that closes it still can.
- **One schema file replaces five.** `common.yaml` and the four v1alpha2 fragments merge into one
  fragment that a test keeps in line with the Go type.
- **Root `contexts:` values stop being written, and untyped keys in them stop being dropped.**
  Conversion reads every key. The test `RootFileKeepsOnlyTypedContextFields` changes on purpose.
- **`windsor init` output changes order.** `id` and `platform` come first. Tests that compare
  whole files change on purpose.
- **`provider` stops working as an alias in v1alpha2 files.** A v1alpha2 file that sets only
  `provider` has no platform. This is an accepted breaking change for v0.10.0.
- **Core keeps its full definitions.** Core does not need to remove the fields it shares with the
  CLI. Its definitions must only stay compatible with the CLI's types.
- **`GetConfig` becomes `GetHeader`.** The config handler decodes its data into the v1alpha2
  header. Terraform backend blocks pass to `terraform init` whole, read as untyped maps, because
  the header types only the backend fields that the CLI checks.
- **The `provider` fallbacks go away.** Load converts `provider` to `platform`, so code reads
  `platform` alone.
- **Every config-reading package moves once.** The Wave 2 passes for those packages land after
  this ADR, so they do not touch the same reads twice.

## Implementation order

1. Build `api/config/v1alpha2` with the header type, the fragment, and the drift test.
2. Load the fragment last for every context, with conflict detection (points 4 and 5).
3. Add the v1alpha1 conversion (point 9).
4. Add ordered writes (point 7).
5. Decode the header from the merged map (point 2), and move the packages that import
   `api/v1alpha1` config types to it. All of them were in `pkg/runtime`, so one change moved them.
6. Delete the old v1alpha2 package and the `api/v1alpha1` config types.

Every step has shipped. Point 8 needs no implementation step.

## Alternatives considered

- **Keep the nested v1alpha2 types.** Rejected: they do not match any real file, so every read
  would need a translation layer.
- **Load the CLI fragment first as a base.** Rejected: a later blueprint fragment could then
  replace a CLI-owned type without notice, which breaks guarantee 1.
- **Generate the CLI schema from core.** Rejected: the runtime merge already composes the two, and
  a build-time dependency on core would couple the release of both repos.
- **Type every setting core defines.** Rejected by point 3: the CLI would maintain types for
  settings it never reads.
- **Keep the dynamic map only, with no typed header.** Rejected: the CLI reads these paths on
  every command and needs a declaration to anchor the write order.
- **Warn on a conflicting blueprint instead of failing.** Rejected: a warning lets a context run
  with a CLI setting the CLI cannot interpret.

## References

- `pkg/runtime/config`: `typed_source.go`, `handler.go`, `schema_merge.go`, `values_source.go`,
  `values_renderer.go`, `resolve.go`, `cli_schema.go`, `key_order.go`.
- `api/config/v1alpha2`: `context.go` and `schema.yaml`.
- `pkg/composer/blueprint`: `loader.go`, `processor.go`.
- `windsorcli/core`: `contexts/_template/schema.yaml`.
