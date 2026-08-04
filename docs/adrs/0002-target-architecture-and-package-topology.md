# ADR 0002 — Target architecture and package topology

- Status: Proposed
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- This is the release doc's Wave 0 "north star" ADR — the layer-ownership map and nested folder
  structure every later Wave 2 per-package cleanup pass conforms to. It resolves `release-v0.10.0.md`
  open question #1 by deriving the topology from the current tree rather than a fresh vision,
  because the current tree turns out to already be substantially right (see Decision).

## Context

`.claude/skills/architecture/SKILL.md` already documents a layer-ownership table — CLI / Runtime /
Evaluator / Secrets / Runtime-Terraform / Composer / Provisioner / Workstation — and it holds today,
verified directly rather than assumed: `pkg/runtime/evaluator` and `pkg/composer/*` import neither
`pkg/provisioner/*` nor each other's layer inappropriately, and `pkg/provisioner/*` correctly depends
downward on `pkg/composer` (it consumes composed blueprints), never the reverse. **There is no
boundary-violation problem to fix.** What's missing is that this map has only ever lived in a skill
file, informally, not as a decision record new packages and refactors can be checked against; and two
concrete pain points the skill doesn't address at all:

**1. Monolith files, surveyed directly (`find pkg api cmd -name '*.go' | xargs wc -l`):**

| File | Lines | Package |
|---|---|---|
| `pkg/composer/blueprint/processor.go` | 2949 | composer |
| `pkg/provisioner/kubernetes/kubernetes_manager.go` | 2397 | provisioner |
| `api/v1alpha1/blueprint_types.go` | 2177 | api |
| `pkg/provisioner/provisioner.go` | 2113 | provisioner |
| `pkg/provisioner/terraform/stack.go` | 1889 | provisioner |
| `pkg/runtime/evaluator/evaluator.go` | 1621 | runtime |
| `pkg/composer/artifact/artifact.go` | 1563 | composer |
| `pkg/composer/blueprint/handler.go` | 1420 | composer |
| `pkg/test/runner.go` | 1292 | test |
| `pkg/runtime/terraform/provider.go` | 1290 | runtime |
| `pkg/composer/blueprint/composer.go` | 1196 | composer |
| `pkg/runtime/runtime.go` | 1034 | runtime |
| `pkg/workstation/virt/incus_virt.go` | 1018 | workstation |

Thirteen files over 1000 lines, four of them (`processor.go`, `kubernetes_manager.go`,
`blueprint_types.go`, `provisioner.go`) over 2000. None of these are unowned or misplaced — every one
sits in exactly the package the architecture skill's table says it should. The problem is single-file
size within a correctly-scoped package, not package boundaries.

**2. Duplicated system-call shims, surveyed directly (`grep` across all 16 `shims.go` files, plus
`cmd/shims.go`):** the same struct fields, independently declared with the same signature, recur
across 3–4+ files each — `ReadFile`, `WriteFile`, `Stat`, `MkdirAll`, `RemoveAll`, `Rename`, `Glob`,
`YamlMarshal`/`YamlUnmarshal`, `JsonMarshal`/`JsonUnmarshal`, `Getenv`/`Setenv`/`LookupEnv`,
`UserHomeDir`. 16 packages under `pkg/` plus `cmd/` itself each hand-roll their own copy of the
`Shims` struct pattern (the "proven pattern" the architecture skill names) with no shared
declaration of the primitives every one of them needs.

**3. Wide interfaces**, confirmed: `ConfigHandler` (33 methods) and `Shell` (25 methods, `pkg/runtime/
shell/shell.go`) — both single interfaces covering what are, on inspection, several genuinely
separable concerns (config: load/get/set/schema/values-render/sensitive-paths; shell: exec/verbosity/
secret-registration/session-reset).

**4. `cmd/` needs the shared primitives too** — `cmd/shims.go` already exists, duplicating the same
pattern found in `pkg/`. A shared package must be reachable from both `cmd/` and every `pkg/`
subpackage, which rules out nesting it under `pkg/` (Go's `internal/` visibility extends only to the
tree rooted at the directory *containing* the `internal/` folder — `pkg/internal/x` is invisible to
`cmd/`, a sibling of `pkg/`, not a descendant of it). The repo already has a working precedent for
this exact placement: `internal/gendocs`, a top-level `internal/` sibling to both `cmd/` and `pkg/`.

## Decision

### 1. Ratify the existing layer map as this ADR's canonical table, not a fresh one

The `architecture` skill's table — CLI (`cmd/*`) / Runtime (`pkg/runtime/runtime.go`) / Evaluator
(`pkg/runtime/evaluator/*`) / Secrets (`pkg/runtime/secrets/*`) / Runtime-Terraform (`pkg/runtime/
terraform/*`) / Composer (`pkg/composer/*`) / Provisioner (`pkg/provisioner/*`) / Workstation
(`pkg/workstation/*`) — is correct and verified against the current import graph. This ADR makes it
the recorded decision the skill file *implements*, rather than the skill being the only place it's
written down. No package moves between layers. Every Wave 2 per-package cleanup pass checks itself
against this table unchanged.

### 2. No wholesale `internal/` migration

Considered and rejected: moving all of `pkg/` under a top-level `internal/` to make the whole
application non-importable by other Go modules. This module (`github.com/windsorcli/cli`) does have
one genuinely external consumer surface — `api/v1alpha1`/`api/v1alpha2`, imported by tooling outside
this repo — but `pkg/`, `cmd/`, and `internal/` are already correctly non-`api` and nothing indicates
anything outside this repo imports them today. Moving ~150 files' import paths for an enforcement
benefit with no evidence of an actual external-import problem is exactly the kind of one-time,
high-blast-radius churn Wave 2's "touch each package once" principle warns against. Not adopted.

### 3. Shared system-call primitives move to `internal/shims`, reachable from `cmd/` and `pkg/` alike

New top-level `internal/shims` (sibling to the existing `internal/gendocs`) declares the ~12
duplicated primitives found in the survey — file I/O (`ReadFile`, `WriteFile`, `Stat`, `MkdirAll`,
`RemoveAll`, `Rename`, `Glob`, `UserHomeDir`), marshal (`YamlMarshal`/`YamlUnmarshal`,
`JsonMarshal`/`JsonUnmarshal`), and env (`Getenv`/`Setenv`/`LookupEnv`) — once, as mockable function
fields, matching the existing `Shims` struct pattern exactly (same field-of-func-type shape, not a
new convention). Every package's own `shims.go` keeps its package-specific wrappers (terraform exec,
Kubernetes client construction, `hcloud`/`aws` SDK calls, etc.) but **composes** `internal/shims` for
the common primitives instead of re-declaring them — each package's own file shrinks to only what's
genuinely specific to it. `cmd/shims.go` does the same. This is additive and mechanical per package
(swap a locally-declared field for the shared one, update the constructor), not a redesign — it can
land package-by-package in Wave 2 rather than as one cross-cutting commit.

`internal/util` (same sibling location) holds pure, non-mockable helpers with no system-call surface
— string/path manipulation, small generic collection helpers — kept separate from `shims` because
mockability is the entire reason `shims` exists and a pure helper has no business behind a function
field.

### 4. Monolith files split in place by cohesive concern, not into new sub-packages

The file-size cap is **1000 lines**, tests exempt where a single black-box test file genuinely
covers one cohesive public surface (Wave 2's own test-strategy ADR governs that exemption). Every
file on the table above splits into multiple files **within its existing package** — the
"companion file" pattern the architecture skill already names as proven (e.g. `provider_sensitive_
inputs.go` next to `provider.go`) — not into new nested sub-packages. A monolith file is a size
problem, not a boundary problem; introducing a new package boundary to fix a size problem would
undo the very ratification in point 1. Concretely, each split follows the seams the file already has:

- `processor.go` (2949) → the file's own section-header comments already separate config-block
  resolution, facet inclusion, tier compilation, and deferred-substitution handling — four
  companion files along those lines.
- `blueprint_types.go` (2177, `api/v1alpha1`) → types stay in the base file; `Merge`/`DeepCopy`,
  `ToFluxKustomization`-style compilation, and tier-derivation logic (`compileFluxSystemTiers` and
  neighbors) move to companion files in the same package — `api/` purity (types free of consumer
  policy, [[feedback_api_layer_purity]]) is unaffected since nothing crosses into `pkg/`.
- `kubernetes_manager.go` (2397) / `provisioner.go` (2113) / `terraform/stack.go` (1889) → each
  already separates by operation family (apply/prune/secrets/inventory for the manager; apply/
  destroy/lock for the provisioner and stack) — split along those families.
- `evaluator.go` (1621) → separate the expression-parsing/scope core from the builtin-function
  registrations (`env()`, `secret()`, `file()`, …), which are additive and independently testable.
- The remaining files on the table (`artifact.go`, `handler.go`, `test/runner.go`, `runtime/
  terraform/provider.go`, `composer.go`, `runtime.go`, `incus_virt.go`) get the same treatment —
  the exact split lines are a per-package Wave 2 decision, not fixed here; this ADR fixes the
  *rule* (in-place companion files, 1000-line cap, no new sub-package), not every line number.

### 5. Wide interfaces split by concern where a per-package cleanup pass touches them

`ConfigHandler` (33 methods) and `Shell` (25 methods) are flagged for the Wave 2 test-strategy /
mock-rationalization pass to split along their already-visible seams (config:
load/get-set/schema-validate/render/sensitive-paths; shell: exec/verbosity/secret-registration/
session-lifecycle) rather than staying one interface each. Not designed in full here — this ADR
names the two interfaces as in-scope for that pass, which is where the actual split shape gets
decided against real call-site usage.

### 6. `cmd/` stays pure Cobra glue

No change from current practice — already the house rule
([[feedback_cmd_is_cobra_glue]]) and already how the tree is organized: domain logic, ID generation,
and orchestration wrappers live in `pkg/`, `cmd/` only parses flags, wires a runtime, and renders
errors. Restated here because it's part of the topology decision, not because it's changing.

## Consequences

- **No package moves.** This ADR's cost is entirely file-splitting and shims-consolidation work,
  not import-path churn — lower risk than a topology reshuffle, and directly enables Wave 2 to
  proceed per-package without a separate "move things into place" phase first.
- **`internal/shims`/`internal/util` become a new shared dependency of every package that adopts
  them** — a bug in a shared primitive now has wider blast radius than a bug in one package's local
  copy. Mitigated by the primitives being thin, well-tested wrappers with no business logic.
- **The monolith split list is a floor, not a ceiling** — Wave 2 passes may find additional
  companion-file splits worth making as they go; this ADR's role is only to fix the rule (in-place,
  1000-line cap) so those decisions don't need their own ADR each time.
- **This unblocks Wave 1 and Wave 2 immediately** — the open question this ADR resolves
  ("derive from current tree") is answered, so ADR 0001 (apiv2 header) and the not-yet-written Wave 1
  foundations ADRs can proceed without re-litigating package placement.

## Alternatives considered

- **Design a materially different nested topology** (e.g. re-grouping by feature rather than layer,
  or introducing a new top-level `domain/` package). Rejected: the current layer map already holds
  under verification, and the actual pain points (monolith files, shim duplication, wide interfaces)
  are orthogonal to package grouping — a new topology would not fix any of them and would add
  import-path churn for no measured benefit.
- **Move shared shims under `pkg/internal/`.** Rejected per point 4 in Context — `cmd/` cannot import
  it there; Go's `internal/` visibility is scoped to the directory tree rooted at the folder
  containing `internal/`, and `cmd/` is a sibling of `pkg/`, not a descendant.
- **New sub-packages for each monolith split** (e.g. `pkg/composer/blueprint/facets/`,
  `pkg/composer/blueprint/tiers/`). Rejected: introduces new package boundaries and import cycles to
  manage for a problem that's purely about file size within an already-correctly-scoped package.
