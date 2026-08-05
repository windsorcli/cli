# Release v0.9.1 — Docs Catch-Up Plan

- Status: In progress
- Date: 2026-08-04
- Purpose: Scratch working plan, not an ADR. Tracks the docs punch list found while auditing
  `docs/reference/**` against everything shipped between v0.8.1 and v0.9.0, and the order it's
  getting done in. Deleted or folded into a proper changelog once v0.9.1 ships.

## Why this exists

v0.10.0 planning (see `release-v0.10.0.md`) allows breaking changes and touches architecture —
the opposite of what we want live while we might need to cut an emergency patch. v0.9.1 is the
safe place to land doc fixes: no architecture boundaries crossed, low risk, quick to release.

## Sequence

### 1. Verify Tier-1 schema-validation gaps — done

Confirmed live via `SchemaValidator.Validate` against the real embedded artifacts: all three
fields were real on their Go structs and rejected by the schema.

- [x] `contexts{}.azure.region` (`AzureConfig.Region`) — was rejected
      (`Additional property 'region' does not match the schema`). Fixed: added `region` to
      `configuration.yaml`'s azure block.
- [x] `flux[].globalDependency` (`FluxSystem.GlobalDependency`) — was rejected. Fixed: added to
      `blueprint.yaml`'s flux-system object.
- [x] `flux[].secrets` (`FluxSystem.Secrets`) — was rejected. Fixed: added `secrets` (with
      `namespaces`/`data` sub-properties) to `blueprint.yaml`. `kustomize[].secrets` needed no fix
      — that field is `yaml:"-"` (composer-internal only, never user-authored).

Regression coverage added: `pkg/runtime/config/schema_artifacts_test.go` — loads the real
artifacts and asserts each field validates, plus asserts unknown fields are still rejected (no
regression toward over-permissiveness). `go build ./...`, `go vet ./pkg/runtime/config/...`, and
`go test ./pkg/runtime/config/... ./pkg/composer/...` all pass.

Staged, not committed — awaiting review before commit.

Swept every other top-level config block (`vsphere`, `aws`, `docker`, `git`, `cluster`, `network`,
`dns`) struct-field-by-struct-field against its schema artifact as a sanity check — no further
drift found. `cluster.controlplanes`/`.workers` are `additionalProperties: true` (permissive), so
their under-documented field list is a pre-existing docs gap (already tracked in step 3), not a
validation bug like azure/flux were.

### 2. Docs PR — global flags + secrets-file convention — done

- [x] New `docs/reference/global-flags.md` documenting `--lock-timeout`, `--no-cache`,
      `-v`/`--verbose`; cross-linked from `unlock.md`, `bootstrap.md`, `contexts.md`. (Originally
      placed under `docs/reference/commands/` — corrected in the step below once that turned out
      to be a generated, wiped-and-regenerated directory.)
- [x] `docs/reference/contexts.md` — new `## Secrets files` section covering `secrets.yaml`/`.yml`
      (plaintext, gitignored) vs. `secrets.enc.yaml`/`.yml` (SOPS-encrypted), the content-not-
      filename detection, and the plaintext-refusal error message. Verified flag defaults/behavior
      and the dot-path key flattening directly against `cmd/root.go` and
      `pkg/runtime/secrets/sops_provider.go` before writing.

### Discovery: most of `docs/reference/` is generated, not hand-authored

Found via `.github/workflows/ci.yaml`'s `docs-check` job: `go run ./internal/gendocs commands`
and `go run ./internal/gendocs schema` regenerate `docs/reference/commands/**` (wiped and rebuilt
from the cobra command tree — `Long`, `Example`, flags, and a `docs.seealso`/`docs.source`
annotation) and every `docs/reference/<name>.md` whose basename matches a
`pkg/runtime/config/schemas/artifacts/*.yaml` file (`blueprint.md`, `configuration.md`,
`facets.md`, `metadata.md`, `testing.md` — driven by each schema's `description:` fields plus an
optional `<name>.seealso.md` sidecar for the See-also section). CI fails on any diff between
committed and regenerated output.

This meant every hand-edit made directly to those files in steps 2–4 as originally written would
have been silently reverted by CI's regen, and the new `global-flags.md` — living inside the
wiped `commands/` directory — would have been deleted outright. `docs/reference/contexts.md` and
`docs/installation.md` are confirmed NOT covered (`internal/gendocs/main.go` lists `contexts` as a
still-future generator), so hand-edits there are safe.

**Fix applied, on a fresh branch (`docs/v0.9.1-reference-completeness`, off this one) rather than
patched into the two now-superseded branches:**

- Moved `global-flags.md` to `docs/reference/global-flags.md` (sibling of `contexts.md`, outside
  both generated trees).
- Moved every piece of prose that needs to survive regeneration into its actual source: schema
  `description:` fields (`pkg/runtime/config/schemas/artifacts/blueprint.yaml`,
  `configuration.yaml`), a `.seealso.md` sidecar fix (`blueprint.seealso.md`'s dead `schema.md`
  link → `contexts.md`), and cobra `Long`/`Annotations["docs.seealso"]` edits in `cmd/unlock.go`,
  `cmd/bootstrap.go`, `cmd/destroy.go`, `cmd/show.go`.
- Verified by actually running both generators twice in a row (second run produced zero further
  diff — a stable fixed point, which is what `docs-check` requires) plus `go build`, `go vet`,
  `go test ./...`, and `task scan` (gosec, 0 issues) on the result.

### 3. `blueprint.md` field-table completeness — done (superseded, see discovery above)

- [x] `flux[].secrets{}` subsection + `secrets` row (namespaces, data, dockerconfigjson synthesis,
      owner-tracked pruning, auto-roll-on-change) — now sourced from `blueprint.yaml`'s schema
      description, not hand-written in `blueprint.md`.
- [x] `globalDependency` row — same.
- [x] `contexts{}.terraform.lock.timeout` — added as a real nested schema property
      (`configuration.yaml`'s `lock.timeout`) with full description; `configuration.md`'s
      subsection is now generated from it.
- [x] `sensitive: true` schema-authoring keyword — documented in `contexts.md` (safe, hand-authored
      file), cross-linked from `cmd/show.go`'s `show values` `Long` text and from `blueprint.yaml`'s
      `substitutions` field description.
- [x] Substitutions-rejection note — folded into `blueprint.yaml`'s `substitutions` field
      description (the actual rule: references a `sensitive: true` property, not specifically
      `secret()` calls — corrected from the original punch-list wording).

### 4. Small consistency fixes — done (superseded, see discovery above)

- [x] `vm.driver`/hyperv — confirmed false positive (see prior note below), no change needed.
- [x] Hetzner kubernetes-backend-by-default — folded into `configuration.yaml`'s `platform` field
      description.
- [x] Destroy-pivots-to-local-state-for-kubernetes-backend — added to `cmd/destroy.go`'s `Long`
      text.
- [x] `azure.region` — already present in the schema artifact from step 1; confirmed it also
      renders correctly in generated `configuration.md` output.

### 5. Downstream

- [ ] Once v0.9.1 tags, bump `windsorcli.github.io`'s `docs/versions.yaml` CLI pin so the public
      site picks up steps 2–4.

## Reference

Full punch list and repo-relationship notes captured in conversation on 2026-08-04; not
duplicated here — this file is the sequencing checklist, not the research artifact.
