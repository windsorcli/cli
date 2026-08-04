# ADR 0001 — apiv2 common header schema and v1alpha1 retirement

- Status: Proposed
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- Carried forward from the v0.9.0 planning round (previously numbered 0010) with no scope change —
  none of this was started. It leads the v0.10.0 sequence because the codebase-sweep wave
  (`release-v0.10.0.md`) touches every package that reads config, and each of those passes should
  land against the finished header, not the two-schema straddle described below.

## Context

The config API has a half-finished v1alpha1→v1alpha2 migration. This ADR records the target model
so the remaining moves land coherently rather than field-by-field.

Verified from source:

- **Two config type layouts coexist and disagree on structure.** v1alpha1 is a root `Config` holding
  a `Contexts map[string]*Context`, and each `Context` carries thirteen typed sub-configs — `AWS`,
  `Azure`, `GCP`, `VSphere`, `Docker`, `Git`, `Terraform`, `VM`, `Cluster`, `Network`, `DNS`,
  `Secrets`, `Environment` ([`api/v1alpha1/config_types.go:32`](../../api/v1alpha1/config_types.go#L32)).
  v1alpha2 is a flat per-context header — `Version`, `Workstation`, `Environment`, `Secrets`,
  `Providers`, `Terraform`, with no `contexts` map and no top-level platform sub-configs
  ([`api/v1alpha2/config/config.go:13`](../../api/v1alpha2/config/config.go#L13)). The
  provider-specific blocks that were siblings of `Context` in v1alpha1 collapse into one `Providers`
  block in v1alpha2.
- **v1alpha1 is still the only parse path; v1alpha2 types are dead weight at read time.** For any
  `version` other than `v1alpha1`, `LoadRoot` loads the v1alpha2 *schemas* for validation
  ([`typed_source.go:68`](../../pkg/runtime/config/typed_source.go#L68)) but then still unmarshals
  the file into a `v1alpha1.Config` ([`typed_source.go:76`](../../pkg/runtime/config/typed_source.go#L76)).
  The v1alpha2 `Config`/`ProvidersConfig`/`TerraformConfig`/`WorkstationConfig` structs have full
  `Merge`/`DeepCopy` + tests but nothing calls them to parse a file. Everything below the version key
  flows as a dynamic `map[string]any` validated against the merged schema — the typed structs are
  not the source of truth for reads.
- **The schema itself lives in two overlapping places.** The CLI embeds its own baseline schema at
  [`pkg/runtime/config/schemas/common.yaml`](../../pkg/runtime/config/schemas/common.yaml) (loaded
  via `//go:embed schemas/*.yaml`, [`handler.go:14`](../../pkg/runtime/config/handler.go#L14)), and
  v1alpha2 embeds a second set per subsystem — `providers`, `secrets`, `terraform`, `workstation` —
  merged in by `LoadSchemas` walking `schemasFS`
  ([`api/v1alpha2/config/schemas.go:18`](../../api/v1alpha2/config/schemas.go#L18)). Both define
  `workstation`, `dns`, `vm`, and friends. They are hand-maintained in parallel with the Go structs,
  so a field lives in up to three places.
- **The schema is composed at runtime, not owned by any single file.** Every fragment — the CLI's
  own embedded schemas and the `schema.yaml` a blueprint (i.e. `windsorcli/core`) ships — is fed to
  the same `SchemaValidator.LoadSchemaFromBytes`, which deep-merges each over the accumulated
  `sv.Schema` with overlay properties winning
  ([`schema_validator.go:87`](../../pkg/runtime/config/schema_validator.go#L87),
  [`schema_merge.go`](../../pkg/runtime/config/schema_merge.go)). Core's fragment enters via the
  blueprint loader ([`loader.go:501`](../../pkg/composer/blueprint/loader.go#L501)), and the CLI's
  enters via the embedded loaders — first fragment is the base, later fragments overlay. So the
  effective schema for a context **is** the merge of the CLI header and whatever the blueprint
  contributes; there is no single authoritative file, and no need for one fragment to restate
  another's fields.
- **The schema carries settings the CLI never consumes.** `workstation/schema.yaml` still defines
  full cluster node-group detail — `cluster.controlplanes` / `cluster.workers` with per-node
  `nodes`, `hostports`, `volumes`, `image`
  ([`api/v1alpha2/config/workstation/schema.yaml:145`](../../api/v1alpha2/config/workstation/schema.yaml#L145)).
  These describe control-plane and compute-layer provisioning owned by `windsorcli/core`, not
  settings the CLI reads to do its own work.
- **Rendering is alphabetical, not header-ordered.** `RenderValuesWithDescriptions` walks
  `sortedUnionKeys`, which `sort.Strings` the union of schema and value keys
  ([`values_renderer.go:211`](../../pkg/runtime/config/values_renderer.go#L211)). JSON-Schema
  property order is lost the moment a fragment is unmarshalled into `map[string]any`, so there is
  today no carrier for a defined order. The vendored `goccy/go-yaml` (v1.19.2) does provide
  `yaml.MapSlice`/`MapItem` for an order-preserving decode, unused so far in `pkg/`.

## Decision

Finish the migration to a single **common header schema** (apiv2 / v1alpha2) that (a) is the CLI's
one header fragment, defined once and merged with the blueprint's schema rather than mirroring it,
(b) statically parses its typed header while leaving the remainder a schema-validated dynamic map,
(c) contains only settings the CLI itself reads, and (d) renders first, in declaration order, when
values are written out.

### 1. The CLI header is a schema fragment that merges with core's — not a mirror of it

The effective schema is composed at load time: the CLI contributes a header fragment, the blueprint
(`windsorcli/core`) contributes its own `schema.yaml`, and `LoadSchemaFromBytes` deep-merges both
into one `sv.Schema` ([`schema_validator.go:87`](../../pkg/runtime/config/schema_validator.go#L87)).
This is the mechanism behind the observations above: the CLI header and the core schema become one
"common" schema for any blueprint precisely because they merge, so neither has to restate the
other. The CLI owns and defines only the header fragment — the fields it consumes — and lets core's
fragment fill in everything a blueprint adds on top. Merge precedence is load order (base first,
overlays win on conflict): the CLI header loads first as the base, the blueprint fragment overlays
it, so core can extend or refine header fields without the CLI importing or code-generating from
core. This keeps api-layer purity (types in `api/`, no consumer policy) intact.

Concretely, `pkg/runtime/config/schemas/common.yaml` folds into the v1alpha2 embedded fragments
([`api/v1alpha2/config/*/schema.yaml`](../../api/v1alpha2/config)) so the CLI defines each header
field in exactly one place before the merge. Core defining the same fields is good practice and
expected — the merge tolerates it (overlay wins) — but is not load-bearing: the CLI never depends
on core to supply a header field it reads. Compatibility is a property of the merge, not a
separately maintained mirror; an optional drift test can assert the CLI header stays assignable
from a core-authored values file, but it is a safety net, not the contract.

### 2. Typed header + dynamic remainder — the static-load seam

The typed header is exactly the v1alpha2 `Config` struct: `Version`, `Workstation`, `Environment`,
`Secrets`, `Providers`, `Terraform`. `LoadRoot`/`LoadContext` parse this header into the struct
directly (retiring the v1alpha1-unmarshal fallback at
[`typed_source.go:76`](../../pkg/runtime/config/typed_source.go#L76)), and everything the header
does not claim stays a `map[string]any` validated against the merged schema — the path
facet-authored `additionalProperties` regions already travel. The seam is the struct's field set —
a field earns a typed home only when CLI code reads it as more than an opaque value.

### 3. Scope the header to CLI-consumed settings only

A setting stays in the header (and its schema) only if `pkg/`/`cmd/` reads it to make a decision.
Provisioning detail consumed solely by core — cluster `controlplanes`/`workers` node groups and
compute-layer node specs
([`workstation/schema.yaml:145`](../../api/v1alpha2/config/workstation/schema.yaml#L145)) — is
removed from the CLI schema and, where a v1alpha1 typed struct exists for it, that struct is
retired. Such values do not vanish from a user's file: they still validate and pass through as
dynamic `additionalProperties` and reach core unchanged; the CLI simply stops maintaining a type
and schema for settings it never reads. Removal follows the established rule — silently drop the
retired typed field, no `UnmarshalYAML` deprecation hook.

### 4. Header renders first, in declaration order

When effective values serialize to a real `values.yaml`, the header keys emit first, in the order
the v1alpha2 `Config` declares them (`version`, `workstation`, `environment`, `secrets`,
`providers`, `terraform`), followed by the dynamic remainder. This replaces the alphabetical
`sortedUnionKeys` ([`values_renderer.go:199`](../../pkg/runtime/config/values_renderer.go#L199))
with an ordered key source: an explicit declaration-order list for the header, then the remaining
keys. Preserving nested schema order below the top level requires decoding fragments through
`yaml.MapSlice` (or an explicit per-object order list) instead of `map[string]any`, since map
iteration and re-marshal both discard order.

### 5. Fold in sensitive-value enforcement while `GetContextValues` is already being reworked

A separate, smaller gap in the same function: `GetContextValues`
([`pkg/runtime/config/resolve.go`](../../pkg/runtime/config/resolve.go)) is the chokepoint an
earlier ADR (the prior round's ADR 0009, "sensitive schema properties") specified for removing
sensitive-marked values before they can reach a plaintext `values-<name>` ConfigMap or `windsor
show` output. What shipped instead is two independent enforcement sites — `RedactSensitiveValues`
([`values_renderer.go`](../../pkg/runtime/config/values_renderer.go)), used by `windsor show`, and
`sensitivePathsInValue` ([`processor.go`](../../pkg/composer/blueprint/processor.go)), a fail-closed
guard on `substitute:` — with `GetContextValues` itself never scrubbing. Both current consumers
happen to be covered, but the guarantee depends on every future consumer independently remembering
to call one of the two guards rather than on the source no longer carrying the value.

Since this ADR already rewrites `GetContextValues`'s read path end to end, land the fix here rather
than as a separate change touching the same function twice: `GetContextValues` removes
sensitive-marked paths from the map it returns; `RedactSensitiveValues` and `sensitivePathsInValue`
become simple consequences of consuming an already-scrubbed map instead of each re-implementing the
check. `IsSensitivePath`/`GetSensitivePaths` stay as the enumeration primitive. `windsor show`'s
display-placeholder behavior (showing a key *was* sensitive without exposing the value) still needs
the sensitive-path set exposed alongside the scrubbed map, not the values themselves. The separate
resolution path that legitimately reads a sensitive value to back a facet `secrets:` entry
(`ResolveSecrets`/`PlaceSecrets`) is unaffected — it reads the raw config value directly, not
through `GetContextValues`, exactly as it does today.

### 6. v1alpha1 retires behind this release

v1alpha1 stays only as long as a real config file names `version: v1alpha1`. The typed `Context`
with its thirteen sub-configs is the legacy layout; new work targets the header. Consumers still
importing `api/v1alpha1` (blueprint composer, provisioner, flux, terraform, and roughly three
dozen other files across `cmd/`, `pkg/composer/blueprint`, `pkg/provisioner/*`,
`pkg/runtime/config`, `pkg/runtime/terraform`, `pkg/workstation`) migrate to the header struct or
to reading the dynamic map, one package at a time, so no single commit rewrites the whole read
path.

## Consequences

- **The header struct becomes load-bearing.** Its `Merge`/`DeepCopy`/mock stay in lockstep with the
  schema on every field change, now for reads too, not just validation.
- **Two schema files become one**, ending the common.yaml↔v1alpha2 double maintenance; the merge
  step in [`schema_merge.go`](../../pkg/runtime/config/schema_merge.go) still composes facet
  fragments over the header, so facet extensibility is unchanged.
- **Core-only settings degrade to opaque pass-through** — they validate loosely
  (`additionalProperties`) and reach core intact, but the CLI no longer type-checks or documents
  them. Accepted: they were never CLI policy.
- **Ordered rendering costs a decode change** — the dynamic map path must carry order (`MapSlice`)
  or the renderer must consult an explicit order list; alphabetical output is not order-preserving
  and cannot be patched in place.
- **No user-file breakage at the cut** — v1alpha1 files keep parsing on the legacy path; the header
  path activates on `version` ≠ `v1alpha1`, exactly as `LoadRoot` already gates schema loading
  today.
- **This is the largest single item carried into v0.10.0's Wave 0/1.** Every per-package cleanup
  pass in Wave 2 that touches config reads should land after this, not before, or it re-touches the
  same call sites twice.
- **Sensitive-value enforcement moves from two ad-hoc call sites to the chokepoint**, closing the
  "a future `GetContextValues` consumer could forget to scrub" gap as a side effect of the header
  rewrite, at no extra cost beyond what point 5 above already requires.

## Deferred / rejected alternatives

- **Generate the CLI schema from core** — a build-time dependency on the core repo's schema.
  Rejected: the runtime merge already composes the two, so the CLI never needs a copy of core's
  fields; coupling the build to core would restate a relationship the merge already expresses.
- **Promote every v1alpha1 sub-config into a typed v1alpha2 block** — keeps `cluster`/`network`/
  platform detail as first-class CLI types. Rejected by point 3: the CLI would maintain types for
  settings it never reads. Those stay dynamic pass-through.
- **Keep the dynamic-map-only model, no typed header** — simplest, but forfeits static typing for
  the fields the CLI reads on every command and leaves point 4's ordering with no declaration to
  anchor to. Rejected.
- **`UnmarshalYAML` deprecation warnings on removed fields** — rejected per house rule; retired
  fields are dropped silently and pass through as `additionalProperties`.

## References

- Source seams cited inline: `config_types.go`, `config.go`, `schemas.go`, `typed_source.go`,
  `common.yaml`, `workstation/schema.yaml`, `values_renderer.go`, `schema_merge.go`, `handler.go`,
  `resolve.go`, `processor.go`.
