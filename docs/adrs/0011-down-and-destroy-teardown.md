# ADR 0011 — windsor down and windsor destroy: one teardown order that the names explain

- Status: Proposed. The deciders chose Option 4. The PR that adds this ADR also implements it,
  so the merge accepts it.
- Date: 2026-11-03
- Deciders: Ryan VanGundy
- Surfaced by cli#3468, which reports that the teardown order reads backwards.

## Context

A full teardown of a workstation context takes two commands, in this order:

```sh
windsor destroy --confirm=local
windsor down
```

`windsor destroy` deletes every Flux Kustomization, then every Terraform component, in reverse
dependency order. It shows a plan and asks for confirmation first. `windsor down` stops the
container runtime, deletes the workstation VM, and calls `PerformCleanup`. That call removes the
`.terraform` and `.tfstate` caches and the `.kube` and `.talos` directories. `down` runs only in a
workstation context. In any other context it prints a hint to use `destroy` and exits 0.

The order matters. `destroy` reads the Terraform state and the kubeconfig. `down` deletes both. If
`down` runs first, `destroy` has nothing to work from, and any resource outside the VM is
orphaned. The names suggest the opposite order. `down` is the inverse of `up`, so it reads as the
full teardown. `destroy` sounds like the last step. The help text of `down` has to say that it does
not destroy live infrastructure.

The docs teach a third pattern. `content/getting-started/first-project.md` and
`content/provisioning/destroy.md` in `windsorcli/docs` say that `windsor down` alone is the normal
local teardown, because the cluster goes away with the VM. They present `destroy` then `down` as
the slow, orderly path. `acceptance` uses that path for docker contexts
(`internal/docker/docker.go`, `Cleanup`), so it also tests `destroy` against a local cluster.

### How ADR 0009 applies

[ADR 0009](0009-destroy-ground-truth-and-failure-scope.md) states one principle for `destroy`: the
cluster MUST NOT disappear while an object in it still has an obligation to release state outside
the cluster. An object has that obligation while it has a finalizer or a deletionTimestamp. Windsor
does not classify which finalizers are internal and which are external, because the Kubernetes API
does not say.

`windsor down` deletes the cluster with the VM and waits on no finalizer. That is safe only when no
controller in the workstation cluster owns external state. A local context that runs Crossplane
against a real cloud account breaks that assumption. Windsor cannot tell the two cases apart without
the classifier that ADR 0009 rejects. So today `down` alone can orphan cloud resources, and nothing
reports it.

ADR 0009 also limits overrides. Windsor MUST NOT proceed past an obligation on its own judgment. An
operator MAY choose to skip a step, because that choice is not Windsor's guess.

## Options

### Option 1: rename `down` to `stop`

`windsor stop` does what `down` does today. `destroy` stays the only command that tears down live
infrastructure.

- Deprecation: v0.10.0 adds `stop`. `down` stays as a hidden command with cobra's `Deprecated`
  field set. It prints `Command "down" is deprecated, use "windsor stop" instead` and then runs
  `stop`. v0.11.0 removes `down`. The docs site keeps `reference/cli/commands/down` as a redirect
  to `stop`.
- ADR 0009: no change. `stop` still deletes the cluster without a finalizer wait. The help text
  names the risk instead of the command. The orphan gap stays open.
- Cost: `stop` implies that a later `up` resumes the same environment. It does not. `stop` deletes
  the VM and its data, so the new name is wrong in a different way. `up` and `stop` are also not a
  pair that operators expect.

Downstream changes:

- `acceptance`: change `"down"` to `"stop"` in `internal/docker/docker.go`. Update the
  `windsor down` lines in `CLAUDE.md`, `README.md`, and the review prompt in
  `.github/workflows/claude-code-review.yaml`. Update the comments in `internal/{aws,azure,gcp,hetzner}`
  that say there is no `windsor down` step. The destroy workflows (`destroy.yaml`,
  `destroy-retry.yaml`) call `acc teardown`, so they need no change.
- `action`: no change. `destroy/action.yaml`, `README.md`, and `.github/workflows/ci.yaml` wrap
  only `windsor destroy`.
- `docs`: change `windsor down` to `windsor stop` in `Taskfile.yaml` (`task sweep`), in the
  `docs-validation` skill, and in the `up` / `down` row of `content/components/terraform.md`.
  Change it on every workstation page, `first-project.md`, `provisioning/destroy.md`,
  `provisioning/overview.md`, and `contexts/overview.md`. The cloud pages use only
  `windsor destroy` and need no change.

### Option 2: `down` runs `destroy` first

`windsor down` runs the full `destroy` path, then deletes the VM and the local artifacts. A new
flag, `--skip-destroy`, skips the destroy and gives the current fast behavior. The issue suggests
`--keep-infra`. This ADR proposes `--skip-destroy`, because `down` still deletes the cluster in the
VM. The flag names the step it skips, not a result it does not give.

- Behavior: `down` accepts `--confirm` and `--continue` and passes them to the destroy step. The
  destroy step MUST NOT prompt when its plan has nothing to destroy, for example after a completed
  `windsor destroy`. If the destroy step fails or stops on an obligation, `down` MUST stop before
  it deletes the VM or the state. The operator needs both to run the destroy again.
- Deprecation: none for the name. The behavior change fails loudly. A script that calls bare
  `windsor down` without a TTY gets the confirmation error, with a hint to pass `--confirm=<context>`
  or `--skip-destroy`. The v0.10.0 release notes name the change.
- ADR 0009: closes the gap by default. `down` waits on every finalizer and stops and reports on a
  stalled one, as `destroy` does. `--skip-destroy` is the operator's choice, so it fits ADR 0009's
  limit on overrides.
- Cost: the default local teardown becomes slower. It also needs the same credentials as
  `destroy`, because the destroy plan checks them.

Downstream changes:

- `acceptance`: change `"down", "--verbose"` to `"down", "--skip-destroy", "--verbose"` in
  `internal/docker/docker.go`. `Cleanup` already runs `destroy` and `DiagnoseStuckDestroy` first,
  and it must delete the VM when that destroy fails. Update `CLAUDE.md` and `README.md` to say that
  `windsor down` is the full teardown and that the harness calls `destroy` on its own for the
  diagnosis step. The destroy workflows and the review prompt need no change.
- `action`: no change. `windsor destroy` keeps its behavior.
- `docs`: change `task sweep` in `Taskfile.yaml` to `windsor down --skip-destroy`, because the sweep
  is best effort and must not prompt. Change the `docs-validation` skill to run
  `windsor down --confirm=local` and to describe the sweep change. Update the `down` cell in
  `content/components/terraform.md`. Rewrite the teardown text in `first-project.md`,
  `provisioning/destroy.md`, and the workstation pages, which say that `down` is fast and skips the
  orderly teardown. The cloud pages need no change.

### Option 3: `down` fails when live infrastructure remains

`windsor down` keeps its name and its job. Before it deletes anything, it checks for live
infrastructure. If it finds some, it fails with a hint to run `windsor destroy` first.

- Behavior: under ADR 0009 the check MUST use ground truth, not a classifier. "Live" means a
  Terraform component with a non-empty state, or a Flux Kustomization on the cluster. Every
  workstation context has both after `windsor up`. So `down` fails after every `up` until `destroy`
  runs. The fast local teardown needs an override, for example `--force`.
- Deprecation: none for the name. The new refusal fails loudly, with a hint.
- ADR 0009: closes the gap, but only by making `destroy` then `down` mandatory. The check cannot
  limit itself to external resources without the classifier that ADR 0009 rejects.
- Cost: the operator still types two commands in an order the names do not explain. The refusal
  explains the order after the fact. In practice the check behaves like Option 2 with a worse
  default, plus a `--force` flag.

Downstream changes:

- `acceptance`: `internal/docker/docker.go` already runs `destroy` before `down`. When that destroy
  fails, `down` then refuses, and the VM leaks. `Cleanup` must pass `--force` to `down`. Update
  `CLAUDE.md` and `README.md` to say that `down` refuses while infrastructure remains.
- `action`: no change.
- `docs`: change `task sweep` in `Taskfile.yaml` to `windsor down --force`. Change the
  `docs-validation` skill flow, which runs `windsor up` and then `windsor down`, to run
  `windsor destroy --confirm=local` first. Update the `down` cell in
  `content/components/terraform.md`. Rewrite `first-project.md`, `provisioning/destroy.md`, and
  the workstation pages, which say that `down` alone is the teardown. The cloud pages need no
  change.

### Option 4: `down` keeps its default, and `--destroy` opts in

`windsor down` keeps its current behavior. A new flag, `windsor down --destroy`, runs the full
`destroy` path first, then deletes the VM and the local artifacts. The issue does not list this
option. It came up in review of Option 2.

- Behavior: `--destroy` runs the same steps as `windsor destroy` with no component argument: the
  backend check, the credential check, the plan, and the confirmation. `down` also accepts
  `--confirm` and `--continue`, and passes them to the destroy step. `down` MUST refuse
  `--confirm` and `--continue` without `--destroy`, because nothing else reads them. If the
  destroy step fails, or `--continue` reports a failed component, `down` MUST stop before it
  deletes the VM or the state. The operator needs both to run the destroy again. In a context
  without a workstation, `down --destroy` fails with a hint to run `windsor destroy`.
- Deprecation: none. The name and the default do not change. The two-command order keeps working.
- ADR 0009: `--destroy` puts the workstation cluster under ADR 0009. Plain `down` stays outside it,
  as it is today. That is the operator's choice when the operator leaves out the flag, and the
  help text names the risk.
- Cost: the orphan gap stays open for an operator who does not know to pass `--destroy`.

Downstream changes:

- `acceptance`: none required. `internal/docker/docker.go` keeps `destroy` then `down`, because
  `Cleanup` runs `DiagnoseStuckDestroy` between them. `CLAUDE.md` and `README.md` MAY name
  `windsor down --destroy` as the one-command teardown. The destroy workflows and the review prompt
  need no change.
- `action`: no change.
- `docs`: `task sweep` in `Taskfile.yaml` and the `docs-validation` skill need no change. Update
  the `up` / `down` row in `content/components/terraform.md` to name `--destroy`. In
  `provisioning/destroy.md`, and optionally `first-project.md`, replace "run `destroy`, then
  `down`" with `windsor down --destroy --confirm=<context>`. The cloud pages need no change.

## Decision

Option 4. The short command does what most users want from it. On a workstation, that is the fast
teardown that the docs already teach. The orderly teardown becomes one command with one flag, and
its name says what it adds.

A first draft of this ADR recommended Option 2. Review rejected it. `windsor down` is the shortest
teardown command, and Option 2 makes it slow, interactive, and dependent on cloud credentials by
default. Option 1 trades one wrong name for another and breaks every downstream repo. Option 3
fires on every workstation after `up`, because ADR 0009 permits only a ground-truth check.

## Consequences

- `cmd/down.go` adds `--destroy`, `--confirm`, and `--continue`. With `--destroy`, `down` requests
  every tool, because the blueprint decides which layers the destroy runs.
- `cmd/destroy.go` moves the full destroy path into `destroyAll`, which `windsor destroy` and
  `windsor down --destroy` both call. `windsor destroy` does not change.
- The help text of `destroy` names `windsor down --destroy` for workstation contexts.
- `docs/reference/commands/down.md` and `destroy.md` are regenerated from the help text.

## References

- cli#3468: the issue that this ADR answers.
- [ADR 0009](0009-destroy-ground-truth-and-failure-scope.md): the destroy principle and its limit
  on overrides.
- `cmd/down.go`, `cmd/destroy.go`, `pkg/project/project.go` (`PerformCleanup`),
  `pkg/workstation/workstation.go` (`Down`).
