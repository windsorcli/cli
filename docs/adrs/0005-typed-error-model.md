# ADR 0005 — Typed error model: `WindsorError`, breadcrumbs, central rendering

- Status: Accepted. `internal/werror` and the central `cmd/` error renderer shipped. A
  `WindsorError` carries a message and a remediation, and no error code (point 2).
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- Fills the "Typed error model" placeholder from `release-v0.10.0.md`'s Wave 1. Sequenced there
  deliberately, not deferred past this release's refactor waves — see Context for why deferring it
  would repeat the exact double-rework problem [ADR 0003](0003-test-strategy-black-box-default.md)
  was written to prevent.

## Context

Surveyed directly (`grep -rlE 'fmt\.Errorf\(.*%w'`, excluding tests): **84 files, 882 wrap sites**
across the codebase (close to the 877 the original release-planning doc estimated). Cross-referenced
against Wave 2's per-package cleanup order:

| Wave 2 directory | Files with typed wrap sites |
|---|---|
| `pkg/runtime` | 27 |
| `cmd` | 24 |
| `pkg/provisioner` | 9 |
| `pkg/workstation` | 8 |
| `pkg/composer/blueprint` | 5 |
| `pkg/composer/terraform` | 4 |
| `api` | 3 |
| `pkg/provisioner/kubernetes` | 2 |
| `pkg/composer/artifact`, `pkg/test` | 1 each |
| `pkg/tui` | 0 wrap sites, but still handles `err` 33 times |

**Every directory in Wave 2's sweep order touches error-handling code.** If the error model isn't
decided before that sweep starts, each of these 84 files gets edited twice — once for its
structural cleanup (file splits, shims extraction, black-box test conversion), again later when the
error model lands and has to migrate the same sites. This is the identical problem ADR 0003 exists
to prevent, just not yet visible when that ADR was written because the wrap-site count hadn't been
checked against the sweep order. It belongs in Wave 1, alongside logging and test strategy, exactly
where `release-v0.10.0.md` already placed it — this ADR fills that placeholder rather than moving it.

Current state, verified: no central error formatter exists. Errors propagate as
`fmt.Errorf("...: %w", err)` chains, surfaced through Cobra's default printer with ad-hoc
`cmd.SilenceErrors = true` plus manual `err.Error()` prints scattered across `cmd/` files
individually. A user sees whatever the deepest wrap produced, concatenated through however many
layers wrapped it — the "nested-colon-string problem" the release doc's keystone section names.

Checked against comparable typed-error models before finalizing this shape — Kubernetes
`apimachinery`'s `StatusError{Code, Reason, Message, Details}` with `errors.IsNotFound(err)`-style
predicates, Docker's `errdefs` typed-interface categories, AWS SDK v2's `APIError{ErrorCode(),
ErrorMessage()}`, Stripe's `{type, code, message, param, doc_url}`, and gRPC's
`google.rpc.Status{code, message, details[]}` — where `details` notably splits into an
`ErrorInfo{domain, reason, metadata}` detail *and a separate* `DebugInfo{stack_entries[]}` detail.
That split is the one this ADR's original draft was missing: this codebase's own wrap chains today
are plain `fmt.Errorf("...: %w", err)` concatenation (confirmed — `github.com/pkg/errors` and
`hashicorp/go-multierror` are present only as transitive, unused dependencies), which collapses into
one opaque string exactly where gRPC and Terraform's `tfdiags` keep the "what happened" and "here's
the trail that led here" separate. Point 1 below adopts that split.

## Decision

### 1. `WindsorError` type, plus a lightweight breadcrumb tier below it

```go
type WindsorError struct {
    Message     string // one-line, user-facing, no Go error-chain colons
    Remediation string // imperative sentence — what the user should do next
    Cause       error  // wrapped original error; nil for a genuinely new condition
}

func (e *WindsorError) Error() string { return e.Message }
func (e *WindsorError) Unwrap() error { return e.Cause }

func New(message, remediation string, cause error) *WindsorError
```

`Unwrap()` is load-bearing, not incidental — `go-style`'s existing `errors.As`/`errors.Is` rule
depends on the chain staying walkable through a `WindsorError`, so wrapping an error in one never
breaks a downstream typed check.

**A second, cheaper type sits below `WindsorError` for the common case that doesn't warrant a
remediation:**

```go
// Wrap adds one named breadcrumb frame — cheap, no remediation, safe to use at any
// layer boundary worth naming. Implements Unwrap(), so errors.As/errors.Is see straight through it.
func Wrap(err error, format string, args ...any) error

// Breadcrumbs walks the full Unwrap() chain (through Wrap frames and a terminal WindsorError alike)
// and returns each frame's own message, outermost first — for verbose rendering and JSON traces.
// Never parses err.Error() strings; each frame's message is captured structurally at Wrap()/New()
// time, which is the whole reason Wrap exists instead of relying on fmt.Errorf concatenation.
func Breadcrumbs(err error) []string
```

Three tiers now exist, cheapest first: plain `fmt.Errorf("...: %w", err)` for genuinely low-value
internal propagation (unchanged); `werror.Wrap(err, "applying component %s", id)` for a layer
boundary worth naming in a trail; `werror.New(...)` for the actual user-facing boundary with a
message and a remediation. Point 4 revises migration guidance around this third tier.

Both types live in `internal/werror` (sibling to `internal/shims`/`internal/util` from
[ADR 0002](0002-target-architecture-and-package-topology.md)), for the identical reason: both
`cmd/` and every `pkg/` subpackage need to construct one, and `pkg/internal/` would be invisible to
`cmd/`. Named `werror`, not `errors`, to avoid the unqualified-import collision with the stdlib
package every file already imports.

### 2. No error codes until a consumer needs one

A `WindsorError` carries no code, category, or docs URL. A code is useful only when something reads
it: a script that branches on the failure type, or a docs page that explains it. Windsor has
neither. The message and the remediation are what a user acts on, and neither needs a code.

Codes also cost upkeep. Each code needs a registry entry, a uniqueness check, and a promise never
to change it, because a shipped code is part of the CLI's interface. Domain prefixes add a second
cost: an error that moves between packages keeps a prefix that names the wrong place. git, cargo,
and terraform print no codes in normal operation, and their users rely on the message.

Add a code field when a real consumer exists, most likely a `--format json` script that must branch
on the failure type. Adding a field then is a compatible change. Removing a shipped code is not.
When codes are added, they SHOULD follow these rules:

- One flat namespace with an opaque number, as rustc's `E0308` does. The prefix identifies Windsor,
  not a package.
- A failure kind, such as `config` or `auth`, is a separate field and is not part of the code.
- Two errors share a code when the user fixes them with the same action (Google AIP-193).
- A retired code is never reused.

### 3. Central renderer at the `cmd/` boundary, incremental during migration

One formatter, one call site — replacing the per-command `SilenceErrors` + manual `err.Error()`
print pattern scattered across `cmd/` today. The text format follows git:

- If the returned error `errors.As`-unwraps to a `*WindsorError`, render its message and, on the
  last line, its remediation:
  ```
  error: <message>
  hint: <remediation>
  ```
- If it doesn't (an untyped error — expected throughout the migration, and always possible for a
  genuinely unanticipated failure), render `error: <err.Error()>` — the existing wrap-chain string,
  centralized to one formatting call instead of 84 scattered ones, but not fabricating a
  remediation the code doesn't have. This is what makes the migration incremental and safe: the CLI
  never regresses to a worse experience mid-migration, it just doesn't get the richer treatment
  until a given site converts.
- **`--verbose` adds the breadcrumb trail**, from `werror.Breadcrumbs(err)`, between the error line
  and the hint. Each `werror.Wrap` frame and the terminal cause is on its own line, in the order
  they were added. The hint stays last, because the last line is the one a user reads first:
  ```
  error: Apply failed.
  trace:
    applying component cluster
    running terraform apply
    exit status 1
  hint: Run windsor plan to see the change.
  ```
  This is why `Wrap` captures each frame's message structurally instead of leaning on
  `fmt.Errorf` string concatenation (Context) — the renderer never parses `err.Error()` to recover
  the frames, it walks the chain.
- `--format json` (the global output flag from ADR 0007): a `WindsorError` serializes to
  `{"message", "remediation"}`, and an untyped error to `{"message"}`. JSON output carries no trace
  yet. A trace under `--verbose` needs the verbose setting passed to the JSON presenter.
- This renderer is deliberately minimal — console text and JSON only, no TUI awareness. The
  presenter (ADR 0007) unifies it with logging and progress rendering behind one contract.

### 4. Migration: three tiers, matched to what each wrap site is actually worth

Not every wrap site becomes a `WindsorError`. Forcing all 882 into rich errors with remediations would be the
same mistake as mocking every interface method regardless of whether anything calls it
([ADR 0003](0003-test-strategy-black-box-default.md)'s finding): effort spent on surface nobody
distinctly needs. But leaving most of them as bare `fmt.Errorf` — this ADR's original position —
under-uses the breadcrumb tier point 1 now adds, since a bare wrap contributes nothing to
`Breadcrumbs()`. Revised guidance, per site:

- **Stays bare `fmt.Errorf("...: %w", err)`** when the wrap is genuinely uninformative on its own —
  re-raising the same error one call frame up with no new fact worth naming. Rare in practice once
  the other two tiers exist; most wraps do name *something* ("resolving local state path for %s").
- **Becomes `werror.Wrap(err, "...")`** for the common case: a layer boundary worth naming in a
  trail, but with no independent remediation to write. This is
  now the **default choice** for a site being touched anyway (not a mechanical sweep — only sites a
  package's own Wave 2 pass already reaches), since it costs one function call more than
  `fmt.Errorf` and makes `Breadcrumbs()` actually useful.
- **Becomes `werror.New(...)`** only at the point in the call chain that has both (a) enough context
  to write a specific, actionable remediation, and (b) a real chance of being the error a user
  actually sees uncaught — a malformed context value, a missing terraform binary, an expired
  credential. Not every intermediate layer, even ones already converted to `Wrap`.

This still rides the same per-package cadence as ADR 0002/0003: a package's cleanup pass converts
*its own* sites as part of that pass, not as a separate 882-site sweep. Sites untouched by any pass
yet stay bare `fmt.Errorf` and still render correctly (point 3's fallback) — a fine steady state for
a partially-converted codebase, not a broken one; they just don't contribute to the breadcrumb trail
until their package's pass reaches them.

## Consequences

- **Positive:** closes the "nested-colon-string problem" and the ad-hoc `SilenceErrors` pattern the
  release doc's keystone section names, with remediations that help a user as soon as each site
  converts, without waiting for the full 882-site migration to complete.
- **No code taxonomy to maintain.** A contributor who adds an error writes a message and a
  remediation, and does not pick a domain or a number.
- **Scripts cannot branch on a failure type yet.** A `--format json` consumer sees a message and a
  remediation only. Point 2 says when and how to add a code.
- **Migration debt is visible, not hidden** — an unconverted site still renders acceptably (point 3),
  so there's no pressure to rush all 882 sites at once, but also no forcing function beyond each
  package's own Wave 2 pass to actually convert the ones worth converting.
- **Three error-construction choices now exist at every call site** (`fmt.Errorf`, `werror.Wrap`,
  `werror.New`) instead of one — a real increase in decision surface for whoever's writing the code,
  offset by point 4's concrete criteria for each ("worth naming in a trail" vs. "worth a
  remediation") being checkable rather than a matter of taste.
- **`Breadcrumbs()` is only as complete as the `Wrap` calls that built the chain** — a site left as
  bare `fmt.Errorf` contributes nothing to the trail, so early in the migration the breadcrumb list
  will have gaps corresponding to not-yet-touched packages. Same accepted steady state as the code
  migration itself.

## Alternatives considered

- **Domain-prefixed codes (`DOMAIN-NNN`, such as `BLUEPRINT-001`) with a derived docs URL** (this
  ADR's earlier decision). Rejected before any release shipped a code: no script or docs page reads
  the codes, and the domains followed the package layout, so a code would name the wrong place after
  a refactor. Point 2 records the rules for codes if a consumer appears.
- **Reserved numeric ranges per subsystem** (e.g. `1000-1999` for config, `2000-2999` for
  composer). Rejected for the same reason, and because ranges reserve capacity upfront.
- **Mechanical mass-conversion of all 882 sites to `WindsorError` in one pass.** Rejected: most wrap
  sites have no independent remediation to write, and the effort mirrors ADR 0003's
  mock-rationalization finding — construct what's actually needed, not every technically possible
  instance. This is why the breadcrumb tier exists as a *cheaper* middle option rather than the
  choice being binary (full `WindsorError` or nothing).
- **Rely on `fmt.Errorf` string concatenation for the breadcrumb trail instead of a structured
  `Wrap`/`Breadcrumbs` type**, parsing `err.Error()` to recover frames for `--verbose`/JSON
  rendering. Rejected: this is the same `strings.Contains`-on-error-text smell `go-style` already
  forbids for classification, just applied to rendering instead of dispatch — fragile against
  format changes and impossible to serialize cleanly as a JSON array.
- **A single `WindsorError` type with no separate lightweight tier** (this ADR's original draft).
  Rejected on review: it forced a false choice between "full message+remediation" and "opaque string,"
  when gRPC's `DebugInfo`/`ErrorInfo` split and Terraform's `tfdiags.Detail` both show a named,
  structured-but-code-free middle tier is standard practice for exactly this gap.
