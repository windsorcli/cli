# ADR 0004 — Comment and documentation standard

- Status: Proposed
- Date: 2026-08-04
- Deciders: Ryan VanGundy
- Fills the "Comment & doc standard" placeholder from `release-v0.10.0.md`'s Wave 2. Ratifies
  `.claude/skills/go-style/SKILL.md` as the canonical, already-largely-correct standard (it already
  enforces no in-body comments and a 6-line header ceiling) and records the two gaps this round's
  refactor waves exposed, both now added to that skill directly rather than left as a separate
  unenforced document. Test-comment discipline (BDD `Given`/`When`/`Then`) is governed by
  `.claude/skills/test-engineer/SKILL.md`, sharpened alongside [ADR 0003](0003-test-strategy-black-box-default.md)
  rather than duplicated here.

## Context

`go-style` already does most of what a comment-standard ADR would otherwise need to establish from
scratch: the 4-line file header, mandatory function headers, a hard "no explanatory comments inside
function bodies" rule with named anti-patterns, a 6-line ceiling on header comments, and generic
(not method-named) section headers. This held up under review — nothing in it needed reversing.

Two gaps surfaced specifically because Wave 0–1's refactor work (ADR 0001's header migration, ADR
0002's monolith splits, ADR 0003's characterize-then-refactor passes) is about to touch a large
fraction of the codebase's comments at once, and neither gap was previously written down anywhere
enforceable:

1. **No function-length guideline existed.** Surveyed directly (`awk`-based function-length count
   across every non-test, non-mock `.go` file): the codebase already mostly self-regulates — 1087 of
   ~1158 functions are under 60 lines, 51 are 60–100, 15 are 100–150, and only **5** exceed 150. The
   informal practice is sound; it was never codified, so a refactor pass (especially one splitting
   monolith files per ADR 0002) had nothing written to check a newly-extracted function against.
2. **No rule against planning artifacts leaking into comments.** Existing memory
   ([[feedback_no_wip_labels_in_code]], [[feedback_no_phase_labels]]) already establishes "no WIP
   markers, no phase labels, no fix-history narrative" as house practice, but this lived only in
   Claude's session memory, not in the checked-in skill file every contributor (human or agent) reads.
   Separately, nothing addressed **ADR/ticket references specifically** — a real risk during this
   exact release, where dozens of comments could plausibly get written as `// per ADR 0002` while the
   monolith splits are fresh in mind.

## Decision

### 1. Ratify `go-style` as-is for everything it already covers

No changes to: the 4-file package structure, section-divider format and ordering, the 4-line file
header, the "no in-body comments" rule and its anti-pattern list, the 6-line header ceiling, the
1-line struct-field rule, or the `errors.As`/`errors.Is` classification rule. These are correct and
stay exactly as written.

### 2. Function-length guideline, added to `go-style` directly

Under 100 lines is the norm; 100–150 is a look-twice zone (confirm the body is genuinely one
operation); 150+ should split into private helper methods in the same file's Private Methods
section, with mechanical field-by-field methods (`DeepCopy`, marshal/unmarshal glue) as the one
standing exception, since splitting those fragments one operation without reducing its complexity.
This is descriptive of current practice, not a new constraint invented for this ADR — the survey
above shows the codebase already lands here in the overwhelming majority of cases; codifying it
gives ADR 0002's monolith splits (and any future large function) a concrete bar to check new,
extracted functions against.

### 3. No planning or process artifacts in comments, added to `go-style` directly

A comment documents current behavior only — never a WIP/historical marker, never an ADR/ticket/issue
reference, never a phase or wave label. This closes both the previously-memory-only house rule and
the new, release-specific risk of comments accumulating ADR citations while this cycle's refactors
are underway. Rationale and decision history belong in the ADR or the commit that implemented it,
never in the source the decision produced — a reader six months out needs the current contract, not
the planning trail that produced it.

### 4. Test-comment discipline stays owned by `test-engineer`, not duplicated here

BDD `Given`/`When`/`Then` comment discipline (exactly three labeled lines per `t.Run` case, no
narrative padding), the black-box-default package declaration, the white-box exemption bar, and the
mock-rationalization rule ("only mock what's actually called," matching ADR 0003's confirmed
`RegisterProvider` finding) all live in `test-engineer`, sharpened alongside ADR 0003 in the same
change that produced this ADR. Splitting test-comment standards into a second document would create
exactly the two-places-to-check problem this ADR's own point 1 avoids for production code.

## Consequences

- **Both new `go-style` rules are checkable by a reader without deep context** — line-count and
  "does this cite an ADR" are mechanical checks, not judgment calls, which is what makes them
  suitable for the editing checklist (`go-style` updated to include both).
- **No existing code is grandfathered specially** — the function-length guideline applies as each
  file is touched by ADR 0002's splits or any later edit, not as a separate retroactive sweep; the
  five current 150+-line outliers get evaluated when their file's cleanup pass reaches them.
- **This ADR adds no new document** beyond itself — both skill files it ratifies/extends were edited
  directly rather than having their content restated here, so there is exactly one place (`go-style`,
  `test-engineer`) a contributor checks for the live rule, and this ADR is the decision record
  explaining why those two files say what they say.

## Alternatives considered

- **Write the full comment standard fresh in this ADR**, treating `go-style` as something to be
  superseded. Rejected: `go-style` was reviewed against this ADR's own concerns and found correct on
  every point except the two gaps — rewriting it wholesale would discard working, specific,
  example-backed guidance for no reason.
- **A separate test-comment-standard ADR**, splitting BDD discipline out from ADR 0003. Rejected:
  ADR 0003 already owns the test-strategy decision space end to end (black-box default, white-box
  exemption, characterize-then-refactor, mock rationalization); BDD comment discipline is one more
  facet of the same decision, not an independent one.
- **A numeric hard cap on function length instead of a three-tier guideline** (e.g., a lint-enforced
  120-line maximum). Rejected: the survey shows a small number of legitimate exceptions (mechanical
  DeepCopy methods); a hard cap would force those into an awkward split for no readability gain,
  where a look-twice zone plus a named exception handles the real distribution better.
