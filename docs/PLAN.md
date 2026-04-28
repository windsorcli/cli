# v0.9.0 Documentation Overhaul — Checklist

Working tracker for the v0.9.0 docs refresh. Mirror of the approved plan; check items off as they land.

## Phase 1 — Foundation
- [x] `docs/PLAN.md` (this file)
- [ ] `docs/nav.yaml` updated for new layout (commands subdir + lifecycle guide)
- [ ] `docs/quick-start.md` rewritten for v0.9 lifecycle
- [ ] `docs/guides/lifecycle.md` new

## Phase 2 — Cobra string audit
Verb-first imperative, no trailing period, ~60 chars `Short`, 1–2 sentence `Long`. Flags: sentence case, no trailing period, present tense.

- [ ] `cmd/apply.go` (apply, apply terraform, apply kustomize)
- [ ] `cmd/bootstrap.go`
- [ ] `cmd/bundle.go` — strip embedded examples
- [ ] `cmd/check.go` (check, check node-health)
- [ ] `cmd/configure.go` (configure, configure network)
- [ ] `cmd/context.go` (legacy hidden — leave)
- [ ] `cmd/destroy.go` (destroy, destroy terraform, destroy kustomize)
- [ ] `cmd/down.go`
- [ ] `cmd/env.go`
- [ ] `cmd/exec.go`
- [ ] `cmd/explain.go`
- [ ] `cmd/get.go` (get, get contexts, get context)
- [ ] `cmd/hook.go`
- [ ] `cmd/init.go` — fix `--vm-driver` period, `--reset` slash phrasing, `--platform` repetition
- [ ] `cmd/install.go` — fill empty Long
- [ ] `cmd/plan.go` (plan, plan terraform, plan kustomize)
- [ ] `cmd/push.go` — strip embedded examples
- [ ] `cmd/set.go` (set, set context)
- [ ] `cmd/show.go` (show, show blueprint, show kustomization, show values)
- [ ] `cmd/test.go`
- [ ] `cmd/up.go` — keep `--install` deprecation message
- [ ] `cmd/upgrade.go` (upgrade, upgrade cluster, upgrade node)
- [ ] `cmd/version.go`

## Phase 3 — Per-command reference
22 pages under `docs/reference/commands/`. Each page: frontmatter, synopsis, one paragraph, flags table, 1–2 examples, see also.

- [ ] `apply.md`
- [ ] `bootstrap.md`
- [ ] `bundle.md`
- [ ] `check.md`
- [ ] `configure.md`
- [ ] `destroy.md`
- [ ] `down.md`
- [ ] `env.md`
- [ ] `exec.md`
- [ ] `explain.md`
- [ ] `get.md`
- [ ] `hook.md`
- [ ] `init.md`
- [ ] `install.md`
- [ ] `plan.md`
- [ ] `push.md`
- [ ] `set.md`
- [ ] `show.md`
- [ ] `test.md`
- [ ] `up.md`
- [ ] `upgrade.md`
- [ ] `version.md`

## Phase 4 — Existing pages
- [ ] `docs/index.md` rewrite (drop "seamless")
- [ ] `docs/install.md` condense GPG verification
- [ ] `docs/guides/contexts.md` add multi-context example
- [ ] `docs/guides/local-workstation.md` ephemeral config, single-node default, `configure network`, colima rightsizing
- [ ] `docs/guides/environment-injection.md` add sample `windsor env` output
- [ ] `docs/guides/kustomize.md` add `destroyOnly`/`deleteOnly`
- [ ] `docs/guides/terraform.md` backend hardening, `TF_VAR_operation`, `bootstrap`
- [ ] `docs/guides/secrets-management.md` fix vault property name
- [ ] `docs/guides/templates.md` ordinal enforcement, common substitutions, AST-aware merge, `value` consolidation
- [ ] `docs/guides/sharing.md` bundled artifact manifests
- [ ] `docs/guides/testing.md` strip reference content (move to ref)
- [ ] `docs/guides/explain.md` new
- [ ] `docs/tutorial/hello-world.md` v0.9 workflow
- [ ] `docs/security/trusted-folders.md` expand to ~60 lines
- [ ] `docs/security/secrets.md` sharpen
- [ ] `docs/reference/blueprint.md` ordinals, common substitutions, repo injection, drop `cleanup`, `value` consolidation
- [ ] `docs/reference/configuration.md` schema updates, AWS, `workstation.arch`
- [ ] `docs/reference/contexts.md` add example `values.yaml`
- [ ] `docs/reference/facets.md` ordinals, common substitutions, deferred-in-substitutions, local configs, AST merge
- [ ] `docs/reference/metadata.md` add example
- [ ] `docs/reference/testing.md` new (split from guide)

## Phase 5 — Validation walk
- [ ] `task build` produces `cmd/windsor/windsor`
- [ ] `windsor --help` and per-command `--help` match rewritten cobra strings
- [ ] `windsor init local` in tmp dir
- [ ] `windsor show blueprint`, `show values`, `explain <path>`
- [ ] `windsor test` against `contexts/_template/tests/`
- [ ] `windsor plan terraform --summary`, `windsor plan kustomize --summary`
- [ ] `windsor configure network --dns-address=...`
- [ ] **Tier 2:** quick-start end-to-end (`init` → `up` → `apply --wait` → `destroy --confirm=local`)
- [ ] **Tier 2:** hello-world tutorial end-to-end

## Acceptance
- No remaining references to `windsor down --clean`, `windsor up --install`, `windsor compose`, blueprint `cleanup` field.
- No "seamless" / "powerful" / "enterprise-grade" in `docs/`.
- Every doc file appears in `docs/nav.yaml`.
- Every command in `cmd/` has a corresponding `docs/reference/commands/<cmd>.md`.
