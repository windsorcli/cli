---
name: windsor-large-pr-control
description: Structure large cross-cutting changes into coherent phases with explicit dependency mapping and test checkpoints. Use when a feature touches multiple runtime subsystems.
---

# Windsor Large PR Control

## Use when
- A feature affects multiple subsystems (runtime, evaluator, secrets, terraform, cmd, docs).
- A clean multi-PR split would require temporary churn or reversions.

## Required planning output
- Provide a change map:
  - subsystem
  - reason for change
  - risk
  - verification

## Implementation phases (single PR, multiple logical commits)
1. Contract/tests baseline for behavior intent.
2. Core semantics changes.
3. Lifecycle/orchestration updates.
4. Integration wiring and persistence/security policy.
5. Boundary cleanup and style normalization.
6. Documentation updates.

## Control rules
- Avoid speculative refactors not required for feature correctness.
- If refactor is required, tie it to a boundary/risk in the change map.
- Keep each phase testable.
- Report what changed and why after each phase.

## Final review checks
- No boundary regressions introduced.
- No hidden global side effects.
- Public/private visibility is intentional.
- Files follow STYLE.md structure and comment rules.
