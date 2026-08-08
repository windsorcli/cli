# Release v0.10.0 — Planning & ADR Sequence

- Status: Drafting
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- Purpose: Seed and sequence the ADRs that lead up to the v0.10.0 release. This is a living
  planning document, not an ADR. Each numbered ADR below is authored separately in this directory.

## Note on ADR numbering and pruning (2026-08-04)

The prior release cycle's ADRs (0001–0012) were audited against the shipped codebase, twice:

**First pass** — everything fully implemented, or explicitly rejected with nothing left to do, was
deleted: per-context `.env` injection, tier-aware `windsor test` validation, and the rejected
flat-entries-under-flux proposal. Work that's done from this repo's side with only cross-repo or
merge-pending work remaining moved to the Stragglers list below instead of staying an ADR. Six ADRs
were provisionally carried forward, rewritten to scope just their outstanding decision.

**Second pass** — those six were re-checked against the release's own stated charter (smooth the
UX via the presenter/logging/error/TUI keystone; smooth the codebase via architecture/DRY/tests),
not just "is there a gap." Five turned out to be speculative future hardening with no dependency on
this release, or pure documentation opinions that don't need an ADR in any release, or work that
would just be superseded once a not-yet-written Wave 1 ADR lands. They were pruned to one-line
Backlog notes rather than carried forward as full documents — see below. Only one ADR survived:
apiv2 header schema retirement, which is a genuine, 0%-started blocker for the Wave 2 sweep.

**Going forward, `docs/adrs/` is tracked in git, not gitignored — ADRs are deleted and the sequence
restarts after every release, rather than accumulating indefinitely. A carried-forward ADR should
justify itself against the release's actual charter, not just against "this isn't finished yet."

## Why this release exists

v0.9.0 was a rapid, scope-creep-heavy cycle — effectively a pre-GA reshaping. v0.10.0 is the
deliberate counter-cycle. It has two goals:

1. **Smooth the UX.** Consistent, leveled, optionally-JSON log output; a standard, genuinely
   helpful error model; and a real TUI (BubbleTea) for interactive selection and live, async
   progress rendering across Terraform and Kubernetes resources.
2. **Smooth the codebase** for developer and agent ergonomics. Clarify the target architecture,
   apply better Go composition, nest the package tree, centralize utilities, DRY, cap file size,
   rework tests toward black-box coverage, rationalize mocks, and standardize comments/docs.

## Decisions locked in scoping

| Decision | Choice | Consequence |
|---|---|---|
| Sequencing | Cleanup first, then UX | Foundations (logging/error/output contracts) are pulled *into* the cleanup wave so packages are touched once, not twice. |
| Scope stance | Broad but phased | Everything discussed is in scope, gated by wave with hard checkpoints. |
| Breaking changes | Allowed | Last big pre-GA reshaping. Log format, error rendering, flags, and config fields may change. GA discipline starts at v1.0. |
| Error model | Typed `WindsorError` | Structured type: code/category + user message + remediation + wrapped cause; rendered by one central formatter. |
| UX ambition | Framework + async everywhere | BubbleTea + concurrent multi-project Terraform via graph + live Kubernetes sub-resource updates + interactive menus + chat-style prompting. |
| Cleanup breadth | Every package gets a pass | Systematic sweep of all `pkg/` packages. Highest schedule risk; first cut line. |
| Logging | `slog` API + swappable handlers | Call sites target `log/slog`. Backends: `slog.JSONHandler` (JSON), `charmbracelet/log` (console), a TUI-routing handler (while BubbleTea owns the screen). Logger injected via context, never global. |

### Schedule risk

"Every package gets a pass" and "async everywhere" are the two most expansive choices on the
board, chosen against a release whose stated purpose is to escape scope creep. They are phased
and gated so a slip in either does not hold the release hostage. Intermediate tags
(`0.10.0-beta.N`) ship at wave boundaries. If the cycle runs long, per-package cleanup narrows to
the monolith hit-list and async narrows to a single flagship flow.

## Architectural keystone: emit events, don't print

Today, business layers reach the terminal directly — `fmt.Fprintf(os.Stderr, ...)` and the global
`tui.Active` singleton live deep inside [`pkg/runtime/shell`](../../pkg/runtime/shell/shell.go),
[`pkg/provisioner`](../../pkg/provisioner/provisioner.go), and
[`pkg/workstation`](../../pkg/workstation/). That coupling is the root cause behind all three UX
complaints (inconsistent logs, nested errors, no live progress).

The unifying fix is one seam. Business layers **emit domain events** — "module X applying",
"resource Y ready", "needs input Z", "failed with WindsorError W" — to an injected **presenter**.
The presenter renders those events three ways behind one contract:

- **Console** — leveled, colored, human-readable (default).
- **JSON** — one structured record per event (`--output json` / CI).
- **TUI** — a live BubbleTea view: multi-line resource list, per-line spinner → green check,
  async sub-resource trees.

Logging, error rendering, progress display, and interactive prompting all become *renderings of the
same event stream*, not four independent subsystems. This is the spine the cleanup wave builds and
the UX wave consumes.

## Current-state facts (verified from source)

- **No logger, no logger interface, no structured logging.** All output is raw `fmt.Print*` /
  `Fprint*` to `os.Stdout` / `os.Stderr`, plus a hardcoded-ANSI spinner layer
  ([`pkg/tui/tui.go`](../../pkg/tui/tui.go)). `logrus`/`zap`/`slog` appear only as transitive deps.
- **877 `fmt.Errorf("...: %w")` wrap sites**, surfaced via Cobra's default printer with ad-hoc
  `SilenceErrors` + manual `err.Error()` prints in `cmd/`. No central formatter. This is the
  nested-colon-string problem.
- **Output reached via package globals** (`tui.Active`) and hardcoded OS streams inside the runtime
  and provisioner layers — the coupling the keystone removes.
- **The one existing abstraction** is the spinner: `Spinner` interface + global `Active` +
  `WithProgress` nesting ([`pkg/tui/tui.go:39`](../../pkg/tui/tui.go#L39)). BubbleTea grows out of
  or replaces this.
- **Monoliths over the 1000-line cap:** [`blueprint/processor.go`](../../pkg/composer/blueprint/processor.go)
  (2685), [`kubernetes_manager.go`](../../pkg/provisioner/kubernetes/kubernetes_manager.go) (2312),
  [`provisioner.go`](../../pkg/provisioner/provisioner.go) (2110),
  [`blueprint_types.go`](../../api/v1alpha1/blueprint_types.go) (2106),
  [`terraform/stack.go`](../../pkg/provisioner/terraform/stack.go) (1889),
  [`evaluator.go`](../../pkg/runtime/evaluator/evaluator.go) (1605),
  [`artifact.go`](../../pkg/composer/artifact/artifact.go) (1553),
  [`test/runner.go`](../../pkg/test/runner.go) (1247). Largest test: `processor_test.go` (7322).
- **`shims.go` hand-rolled in ~14 packages; no shared `util`/`internal` package.** Clearest DRY win.
- **Tests are 100% white-box** (`package xxx` everywhere; `*_public_test.go` is a naming convention,
  not black-box testing). 113 unit / 28 cmd / 17 integration. Mocks hand-written (no mockgen), 19 of
  them; widest interfaces are `config/handler` (31 methods) and `shell` (25 methods).

## Carried forward from the prior round

Only one ADR survived both audit passes:

| ADR | Wave fit | Why |
|---|---|---|
| [0001 — apiv2 common header schema & v1alpha1 retirement](0001-apiv2-common-header-schema.md) | Wave 0/1 | 0% started; every config-touching cleanup pass in Wave 2 should land after this, not before. Now also folds in the sensitive-value-enforcement chokepoint fix (below), since both rework the same `GetContextValues` read path. |

## Backlog (deferred past v0.10.0 — not carried forward as ADRs)

Real gaps the prior round's audit found, kept as one-line pointers rather than full ADRs — none
has a dependency on this release's charter (presenter/logging/errors/TUI; architecture/DRY/tests),
so none blocks it. Revisit if a future release's scope actually touches this surface:

- **Blueprint-lifecycle provenance and safety.** `windsorcli.dev/blueprint-version` and
  `windsorcli.dev/origin` labels are missing from managed objects; no guard inspects
  `prune: disabled`/`Retain`/`resource-policy: keep` before a destructive prune; `plan` doesn't
  classify a transition as created/updated/migrated/reclaimed; no `--dry-run` alias exists.
- **Kustomization tier namespace/label conventions.** Ratifying functional-layer namespaces and an
  `app.kubernetes.io/name` vendor label is a documentation decision for `windsorcli/core`'s
  `docs/reference/facets.md` — doesn't need a CLI ADR in any release.
- **CLI self-update nudge.** No background check against `windsorcli/cli`'s latest release exists;
  `upgrade` also doesn't say when it skipped a newer, incompatible core tag. Ambient UX, not tied
  to the presenter keystone driving this release's actual UX wave.
- **Hetzner ADR/code drift.** The deleted prior-round ADR's text (backend-type default, credential
  delivery mechanism) no longer matches shipped code (`kubernetes` default, `.env`-only via
  `env()`). Purely a stale-doc-text problem — already resolved by deleting the stale doc.
- **`explain`/`trace` attribution rendering.** `ExplainContribution.SourceName`/`Ordinal` are
  already tracked end-to-end but `cmd/explain.go`'s `printContribution` only ever prints them as a
  fallback when no facet path exists — never alongside one, which is the common case. This is
  presenter/rendering work; fold it into the Wave 1 "Presenter / event stream" ADR when that's
  written rather than deciding it in isolation now.
- **Omni support: two distinct problems, not one layer.** Sidero's Terraform provider for Omni is
  an early skeleton (`omni_user` only, no etcd-backup/cluster-template coverage) — not a viable
  parity path either way. Both problems below compose the same way `flux:` already does — CLI
  resolves/renders the Omni resource manifests through kustomize, same pipeline `kustomize:`/
  `flux:` entries go through today, not a bespoke templating path. (1) *Configuring Omni's own
  objects* (etcd backups, machine classes) is a reconciled-resource-YAML problem, the Flux shape
  not the terraform plan/apply/state shape; canonical answer if/when it's needed is a generic
  COSI-envelope CRD (`OmniResource`: `type`/`metadata`/`spec` passthrough, no per-kind schema to
  track) reconciled by a thin controller-runtime operator, with the CRs themselves delivered via
  Flux like everything else — check with Sidero first, they may already be building this. (2) *Omni
  as a cluster-formation backend* — an alternative or complement to terraform for standing up the
  Talos cluster itself — is the one with no getting around it; its manifests (machine classes,
  cluster templates, schematics) go through the same kustomize composition as any other blueprint
  content before being applied via the Omni SDK. See the cross-domain dependency item below, which
  this depends on. Omni's own cloud bring-up story (infra providers) only reaches bare-metal/
  Proxmox/vSphere/libvirt/KubeVirt today, no AWS/GCP/Azure — so formation is composable with
  terraform, not a replacement for it, on cloud platforms specifically. No dependency on this
  release's charter (presenter/logging/errors/TUI; architecture/DRY/tests) — revisit when a
  release actually scopes Omni work.
- **Cross-domain dependency graph (`dependsOn`/`GlobalDependency` generalized across terraform,
  omni, and flux).** Decided direction, not yet an ADR. Cluster provisioning has real structural
  ordering that today has no vocabulary: substrate (terraform) → formation (terraform or Omni,
  composable) → bootstrap (CNI/cilium, must precede Flux itself) → Flux-managed resources. Forcing
  this through today's whole-phase-then-whole-phase provisioner execution ("all terraform, then
  all flux") produces ad hoc alternating terraform/omni/terraform layers. Rejected: a `postInstall`
  boolean (doesn't scale past two phases, breaks the moment a third domain — Omni — exists);
  a nested `cluster:` container owning its own private `terraform:`/`omni:` (terraform is
  legitimately dual-purpose — arbitrary infra and cluster-forming both — a container duplicates
  the primitive). Decided instead: reuse the mechanism `flux:` systems already prove out, not their
  container type — `install`/`resources` compile down into ordinary `Kustomization` objects in the
  same flat list the plain `kustomize:` entries use; the new part is just two barrier mechanisms
  (implicit tier edges, `GlobalDependency`'s "declare the precondition once, it wires into every
  downstream consumer automatically"). Generalize both across domains: `dependsOn` gains
  kind-qualified cross-domain references (`terraform:vpc`, `omni:cluster`, `flux:cilium`);
  `GlobalDependency` widens from "within this flux system" to "across everything the provisioner
  runs." `terraform:`/`omni:`/`flux:` stay flat, peer, general-purpose lists — no new blueprint
  container. Provisioner execution moves from two fixed phases to resolving one cross-domain DAG
  and walking it, though each domain still owns its own actual execution semantics (terraform
  apply, Omni SDK, Flux controller) — the DAG only decides entry order. Explicitly out of scope for
  this mechanism: live-endpoint/health-gated "day 2" service configuration (e.g. configuring
  openbao once it's up and authenticated) — that's dynamic/runtime discovery, not static plan-time
  topology, and stays a phase-gated concern (run after the graph reaches Ready), never a graph
  edge. No dependency on this release's charter — revisit alongside the Omni work above, since
  Omni-as-formation-backend is the concrete case that needs it.

## Stragglers (tracked here, not as standalone ADRs)

Work items surfaced by the same audit that don't need a CLI-repo ADR — either because the remaining
work lives entirely in `windsorcli/core`, or because the CLI-side decision is already made and only
execution (a merge) remains:

- **Facet `install`/`resources` migration in `core`.** The CLI-side `flux:` mechanism (prior ADR
  0004) is fully shipped; migrating `core`'s facets off flat `kustomize:` entries onto it is
  `windsorcli/core` content, tracked there.
- **Flux-native SOPS decryption wiring in `core`.** The CLI-side generic `decryption` field (prior
  ADR 0010) is fully shipped; the `sops-age` secret entry and `decryption:` gating in `core`'s
  gitops facet is `windsorcli/core` content, tracked there.
- **Merge PR #3132** (`fix/talos-upgrade-empty-response`) — Talos node-reboot detection by boot ID
  is fully coded and tested (prior ADR 0012) but sits on an unmerged branch. This needs a merge
  decision, not a new ADR.

## ADR sequence

Numbers below continue from the carried-forward series above (next available is 0008). Waves are
ordered; ADRs within a wave may proceed in parallel unless a dependency is noted.

### Wave 0 — North star (must land first)

- [x] **[0002 — Target architecture & package topology](0002-target-architecture-and-package-topology.md).**
  Derived from the current tree, not a fresh vision (resolves open question #1 below): the existing
  layer map already holds under verification, so this ADR ratifies it rather than redesigning it,
  and fixes the two real pain points found instead — 13 files over the 1000-line cap (split in
  place, no new sub-packages) and 16+1 duplicated `shims.go` files (consolidated into a new
  top-level `internal/shims`/`internal/util`, reachable from both `cmd/` and `pkg/`). No package
  moves. *Everything downstream references this.*

### Wave 1 — Foundations (the cleanup wave's contracts) — ✅ complete, all decisions made

- [x] **[0003 — Test strategy: black-box default and characterize-then-refactor](0003-test-strategy-black-box-default.md).**
  Moved up from Wave 2 — it's a contract every later pass depends on, not an independent cleanup
  item, and ADR 0001 needs it immediately (see below). Surveyed directly: 0 of 158 existing test
  files are black-box today (100% `package xxx`, confirmed not assumed). Decides: black-box
  (`package xxx_test`) is the default; a private-method test is permitted only for complex
  unexported logic where public coverage is impractical (parsers, edge-case math — matching
  `test-engineer`'s existing exemption); >80% coverage floor per package; mock rationalization by
  real call sites, not customization frequency — confirmed one genuinely dead interface method
  (`ConfigHandler.RegisterProvider`, zero production call sites) and confirmed the naive
  never-customized-in-tests signal produces false positives (three real, actively-called methods
  looked identical to it by that signal alone). Dead methods drop from the interface, its
  implementation, and its mock together; live-but-uncustomized methods stay and get test coverage
  added instead. `ConfigHandler` (33 methods) and `Shell` (25) then split along their existing
  seams once the dead weight is gone. The **sequencing rule**, binding on ADR 0001 and every Wave 2
  per-package pass:
  for a package about to be structurally touched, black-box-characterize its current public
  behavior *first*, refactor under that net, confirm the black-box tests pass unchanged (the proof
  the refactor was behavior-preserving), and only then add narrow white-box tests for whatever
  complex unexported logic survives and still needs them. Never refactor first and patch tests
  after — that pays the test-rewrite cost twice for the same code.
- [x] **[0006 — Structured logging contract](0006-structured-logging-contract.md).** Surveyed
  directly: 0 `slog` usage, 0 `charmbracelet`/`bubbletea` deps, 46 raw print sites, and 9 files
  reaching `tui.Active` directly from business layers (`pkg/workstation`, `pkg/composer/artifact`,
  `pkg/runtime/shell`, `pkg/provisioner*`) — confirming the keystone section's coupling claim.
  `log/slog` as the call-site API, retrieved from `context.Context` via a typed key in
  `internal/logging` (closing an existing string-keyed `context.WithValue("verbose", ...)` smell in
  `cmd/root.go`); console (`charmbracelet/log`, new dep)/JSON (stdlib)/TUI-routing (reserved seam,
  Wave 3 fills it in) handlers; `--verbose` unifies the two currently-disconnected verbosity
  mechanisms into one; `WindsorError` logs as structured fields via `LogValue()`, not string
  interpolation. Does **not** migrate the nine `tui.Active` call sites — that's explicitly the
  Presenter ADR's job below, which needs the domain-event vocabulary this ADR doesn't define.
- [x] **[0005 — Typed error model](0005-typed-error-model.md).** Surveyed directly: 84 files, 882
  wrap sites, spanning every Wave 2 directory — decided here rather than deferred, for the identical
  reason ADR 0003 was promoted. Checked against prior art (Kubernetes `apimachinery`, Docker
  `errdefs`, AWS SDK v2, Stripe, gRPC `google.rpc.Status`, Terraform `tfdiags`) before finalizing.
  Three tiers in `internal/werror`: plain `fmt.Errorf` for uninformative wraps (rare); `werror.Wrap`
  — a cheap, code-free breadcrumb frame, the default for a touched site, structurally capturing each
  frame's message (never parsed from `err.Error()` strings) so `--verbose`/JSON can render a real
  trace list; `werror.New` — a full `WindsorError` (doc-linkable domain-prefixed code, e.g.
  `COMPOSER-014`, mechanically-derived `DocsURL`, remediation) reserved for boundaries with a real
  actionable remediation. Domains are purpose-built for support triage, deliberately not a 1:1
  mirror of ADR 0002's architecture layer table. Minimal central renderer at the `cmd/` boundary
  (console + JSON, no TUI awareness yet — the Presenter ADR below subsumes it). Migration rides the
  same per-package Wave 2 cadence, not a mechanical 882-site sweep.
- [x] **[0007 — Presenter / event stream](0007-presenter-event-stream.md).** The keystone seam —
  and Wave 1's last gap. Survey found the progress-reporting fragmentation was worse than the
  original framing: **three** ad-hoc mechanisms today (`tui.WithProgress` closures, asymmetric
  `tui.Start`/`Fail` calls in `kubernetes_manager.go`, bespoke `outputFunc func(string)`
  parameters), not one. One `Event{Kind, Subject, ParentID, Message, Err, Record}` envelope,
  verified against real prior art (not recalled from memory) across Terraform's `-json` output,
  `sigs.k8s.io/cli-utils` (the `kubectl apply` event pipeline — validates typed fields directly on
  the struct over a generic payload), Pulumi's `engine.Event`, and BuildKit's `progressui.Display`;
  a one-method `Presenter{Emit}` port, injected via constructor (not context — matches how `ConfigHandler`/
  `Shell` are already injected) into the five types that reach `tui.Active` today. Resolves the
  release doc's open question #2: `--output json` becomes a single global flag covering logs,
  errors, and progress uniformly, not per-concern flags. Reconciles cleanly with ADR 0006 (the
  slog handler routes through `Emit`, call sites unchanged) and ADR 0005 (the error renderer builds
  a `KindFailed` event instead of formatting directly, format unchanged). Migration is per-package,
  riding Wave 2's existing order — not done here.
- [x] **Shared foundations package(s).** Already fully decided by
  [ADR 0002](0002-target-architecture-and-package-topology.md) point 3 (`internal/shims`/
  `internal/util`) — no separate ADR needed; this line is kept only so Wave 1's list reads complete.

### Immediate implication for ADR 0001 (apiv2 header)

ADR 0001 rewrites `pkg/runtime/config`'s read path (v1alpha1-unmarshal-everything → typed v1alpha2
header + dynamic map) — precisely the kind of internal-representation change the characterize-then-
refactor rule exists for. Its implementation should not wait for the Wave 1 test-strategy ADR to be
formally written and land everywhere; it pulls forward a **scoped** slice of that policy just for
`pkg/runtime/config` and `api/v1alpha2/config`: black-box the current behavior (assert through
`LoadConfig`/`LoadContext`/`GetContextValues`, not through `typedSource` internals) as 0001's actual
first implementation step, then do the header rewrite under that net.

### Wave 2 — Codebase sweep (every package)

- [x] **[0004 — Comment and documentation standard](0004-comment-and-doc-standard.md).** Ratifies
  `.claude/skills/go-style/SKILL.md` as-is (already correct: no in-body comments, 6-line header
  ceiling, section-divider convention) and adds the two gaps this round's refactor waves exposed,
  both now written directly into that skill: a function-length guideline (<100 lines the norm,
  100–150 look-twice, 150+ splits — evidence-based against the codebase's own current distribution,
  only 5 of ~1158 functions currently exceed it) and a hard rule against ADR/ticket/WIP/phase-label
  references leaking into comments. Test-comment discipline (BDD `Given`/`When`/`Then`, black-box
  package declaration, mock rationalization) is sharpened in `test-engineer` alongside ADR 0003
  rather than duplicated here.
- **Per-package cleanup passes** (each conforms to Wave 0–1, including the characterize-then-refactor
  rule above; not each its own ADR unless a boundary moves). Suggested order by density/blast-radius:
  1. `pkg/composer/blueprint/` (densest — processor/handler/composer/trace/loader)
  2. `pkg/provisioner/` (root monolith) + `pkg/provisioner/kubernetes/`
  3. `pkg/runtime/` (shell, evaluator, config, env, secrets, terraform, tools)
  4. `pkg/composer/artifact/` + `pkg/composer/terraform/`
  5. `pkg/workstation/` (network, virt)
  6. `pkg/test/`, `pkg/tui/`, `cmd/`, `api/`

### Wave 3 — UX (builds on the presenter)

- **ADR — TUI framework adoption.** BubbleTea + charmbracelet; program lifecycle; how the logger
  and presenter route through the running program; non-TTY / CI fallback.
- **ADR — Async execution & progress model.** Concurrent multi-project Terraform via dependency
  graph; live kustomize/Kubernetes sub-resource watching; hierarchical progress events;
  cancellation and context propagation.
- **ADR — Interactive input.** Selection menus and chat-style prompting for missing values and
  secrets, sourced through the same presenter/event contract.

## Implementation sequence

The seven ADRs each decide *what* to build; this is the order to actually build it in, derived from
the dependency graph between their own `internal/` packages — not restated per-ADR, since none of
them fully spells out how it depends on the others' output.

### Phase A — Foundational `internal/` packages (build once, no migration yet)

Five small, low-risk packages, in this order because each depends on the previous:

1. **`internal/shims` + `internal/util`** (ADR 0002) — no dependency on anything else in this list;
   purely mechanical extraction from the 16+1 duplicated `shims.go` files. Do this first: it's the
   lowest-risk, and every later step benefits from it existing.
2. **`internal/werror`** (ADR 0005) — depends on nothing but stdlib.
3. **`internal/logging`** (ADR 0006) — depends on `werror` for the `LogValue()` bridge
   (`*WindsorError` → structured `code`/`category`/`message` log fields); otherwise independent.
4. **`internal/presenter`** (ADR 0007) — depends on **both** `werror` (`Event.Err`) and `logging`
   (the slog-handler-to-`Emit` bridge is defined in terms of the handler type `logging` provides).
   Must come after both.
5. **`Runtime`/`cmd/root.go` wiring** — ties the four together: `NewRuntime` constructs the logger
   and presenter; `cmd/root.go` injects the logger via context (replacing the dual
   `SetVerbosity`+string-keyed-`context.WithValue` mechanism ADR 0006 found) and wires the presenter
   into the central error renderer (replacing the per-command `SilenceErrors`+manual-print pattern
   ADR 0005 found). This step is what makes Phase A "done" — a command run today would already show
   coded errors and leveled logs, before any business-layer migration happens.

**Phase A gates both Phase B and Phase C below** — not just a nice-to-have ordering. `pkg/runtime/
config` (Phase B) will want `CONFIG-*` `WindsorError`s for its own new validation-failure paths;
building the header rewrite before `internal/werror` exists means coming back to add error codes
later, which is the exact "touch the file twice" problem this whole planning exercise exists to
avoid — just one level up, across ADRs instead of within one.

### Phase B — ADR 0001 (apiv2 header schema)

Runs after Phase A (or, if parallelizing, after Phase A steps 1–3 specifically — `pkg/runtime/
config` needs `werror` and probably wants `logging`, not `presenter`). Internally sequenced per
ADR 0001/0003's own text: black-box characterize `pkg/runtime/config`'s current behavior first,
then the v1alpha1→v1alpha2 header rewrite, then the sensitive-value chokepoint fold-in (ADR 0001
§5), then the package-by-package `api/v1alpha1` import retirement.

### Phase C — Wave 2 per-package sweep, one pass per package, six contracts bundled into each

This is the synthesis point: Wave 2 is not six separate sweeps (one per ADR). For each package, in
ADR 0002's already-decided density/blast-radius order —

1. `pkg/composer/blueprint/` 2. `pkg/provisioner/` + `pkg/provisioner/kubernetes/` 3. `pkg/runtime/`
4. `pkg/composer/artifact/` + `pkg/composer/terraform/` 5. `pkg/workstation/`
6. `pkg/test/`, `pkg/tui/`, `cmd/`, `api/`

— a single pass applies, in order, within that package:

1. **Black-box characterize** current public behavior (ADR 0003), if it doesn't already exist.
2. **Structural refactor**: split files over the 1000/150-line caps into companion files, adopt
   `internal/shims`/`internal/util` in place of the local `shims.go` (ADR 0002).
3. **Selective error conversion** (ADR 0005): construct `werror.New`/`werror.Wrap` only where the
   per-ADR criteria are met, not mechanically.
4. **Selective logging conversion** (ADR 0006): raw `fmt.Print*` sites convert to `slog` calls
   touched by this pass.
5. **Presenter injection** (ADR 0007) — **only for the five types that need it**:
   `TerraformStack` (`pkg/provisioner/terraform`), `KubernetesManager`
   (`pkg/provisioner/kubernetes`), `Provisioner` (`pkg/provisioner`), the workstation virt/network
   types (`pkg/workstation`), and `ArtifactBuilder` (`pkg/composer/artifact`). Every other package's
   pass skips this step.
6. **Comment/doc standard** (ADR 0004) applied to whatever this pass already touched.
7. Confirm black-box tests pass unchanged — the proof steps 2–6 preserved behavior.

Steps 3–6 don't all apply to every package (step 5 especially, per the list above) — this is a
checklist to run per pass, not a mandate that every package changes in every dimension.

### Phase D — Wave 3

Starts once Phase C has reached the packages Wave 3 actually touches —
`pkg/provisioner/terraform` and `pkg/provisioner/kubernetes` specifically, both early in Phase C's
order (step 2) — not necessarily after all of Phase C. Each Wave 3 item (TUI framework, async
execution, interactive input) still gets its own ADR when reached, per the existing wave list above;
this phase just confirms none of them are blocked on Phase C finishing in full.

## Open questions to resolve next

1. ~~**Target-architecture vision**~~ — resolved: [ADR 0002](0002-target-architecture-and-package-topology.md)
   derives the topology from the current tree.
2. **JSON output surface** — is JSON a global `--output json` that covers logs *and* errors *and*
   progress events uniformly, or per-concern flags?
3. **`WindsorError` code taxonomy** — flat string codes, numeric ranges per subsystem, or
   category enums? Do codes appear in user-facing output for support/docs linking?
4. **Async blast radius** — is concurrent Terraform gated behind a flag initially (opt-in), or the
   default once the graph executor lands?
5. **Per-package cleanup as PRs** — one PR per package (many small), or grouped by wave? Interacts
   with the `large-pr` phased-change convention.
6. **Beta cadence** — tag `0.10.0-beta.N` at each wave boundary, or continuous pre-releases?
7. **Feature requests to slot in** — you mentioned prioritizing a few. List them so they can be
   mapped onto the waves above. First one recorded: Omni layer direction, see Backlog above —
   doesn't map onto a v0.10.0 wave, tracked for a future release's scope.

## ADR lifecycle policy

`docs/adrs/` is tracked in git (no longer gitignored). ADRs number sequentially within a release
cycle, starting at 0001. At the close of a release, every ADR is either deleted (fully implemented,
or rejected with nothing left to do) or reduced to just its outstanding decision and carried
forward into the next release's sequence, renumbered from 0001 — as happened at the top of this
document. The directory never accumulates indefinitely; it reflects the current release's open and
recently-closed decisions only. Long-term architectural history, once genuinely settled, belongs in
reference docs (`docs/reference/`), not in an ever-growing ADR backlog.
