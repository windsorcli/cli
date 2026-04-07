---
name: large-pr
description: Structure large cross-cutting changes into coherent phases with explicit dependency mapping and test checkpoints. Use when a feature touches multiple runtime subsystems or would otherwise produce an unreviewable single commit.
---

# Windsor Large PR

## Apply when
- A feature affects multiple subsystems (runtime, evaluator, secrets, terraform, cmd, docs).
- A clean multi-PR split would require temporary churn or reversions.

## Required planning output

Produce a change map before any implementation:

| Subsystem | Reason for change | Risk | Verification |
|---|---|---|---|
| ... | ... | ... | ... |

## Implementation phases (single PR, multiple logical commits)

1. **Contract/test baseline** — define behavior intent with stubs or failing tests.
2. **Core semantics** — implement the primary logic change.
3. **Lifecycle/orchestration updates** — wire new behavior into runtime flow.
4. **Integration wiring and security policy** — persistence, secrets, permission checks.
5. **Boundary cleanup and style normalization** — apply STYLE.md, remove speculative code.
6. **Documentation updates** — update relevant docs and comments.

## Control rules

- Avoid speculative refactors not required for feature correctness.
- If a refactor is required, tie it to a boundary or risk in the change map.
- Keep each phase independently testable.
- Report what changed and why after each phase.

## Final review checks

- No boundary regressions introduced.
- No hidden global side effects.
- Public/private visibility is intentional.
- Files follow STYLE.md structure and comment rules.
- `task scan` passes with no unresolved High findings.
- `task test:all` passes.
