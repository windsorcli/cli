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

- [x] New `docs/reference/commands/global-flags.md` documenting `--lock-timeout`, `--no-cache`,
      `-v`/`--verbose`; cross-linked from `unlock.md`, `bootstrap.md`, `contexts.md`.
- [x] `docs/reference/contexts.md` — new `## Secrets files` section covering `secrets.yaml`/`.yml`
      (plaintext, gitignored) vs. `secrets.enc.yaml`/`.yml` (SOPS-encrypted), the content-not-
      filename detection, and the plaintext-refusal error message. Verified flag defaults/behavior
      and the dot-path key flattening directly against `cmd/root.go` and
      `pkg/runtime/secrets/sops_provider.go` before writing.

### 3. Docs PR — `blueprint.md` field-table completeness (depends on step 1's outcome)

- [ ] Add `secrets` row + subsection to `flux[]`/`kustomize[]` tables (namespaces sub-key,
      dockerconfigjson synthesis, owner-tracked pruning, auto-roll-on-change behavior).
- [ ] Add `globalDependency` row.
- [ ] Flesh out `contexts{}.terraform.lock.timeout` — currently only a source-code pointer, no
      field/type/example.
- [ ] Document the `sensitive: true` schema-authoring keyword (not used by any built-in schema
      yet, but real and undiscoverable today).
- [ ] Note near the `substitutions` field: `secret()` values are rejected there (must use
      `secrets:` instead).

### 4. Docs PR — small consistency fixes

- [ ] `configuration.md` `vm.driver` enum — add a Hyper-V value or explain the gap.
- [ ] Hetzner: kubernetes-backend-by-default behavior, and destroy-uses-local-state-for-
      kubernetes-backend nuance.

### 5. Downstream

- [ ] Once v0.9.1 tags, bump `windsorcli.github.io`'s `docs/versions.yaml` CLI pin so the public
      site picks up steps 2–4.

## Reference

Full punch list and repo-relationship notes captured in conversation on 2026-08-04; not
duplicated here — this file is the sequencing checklist, not the research artifact.
