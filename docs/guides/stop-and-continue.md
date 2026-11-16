---
title: "Stop part-way and continue"
description: "Stop windsor up or windsor bootstrap after one step, do a manual task, then continue."
---

Use `--to` to stop `windsor up` or `windsor bootstrap` after one step. Do your manual task. Then run the same command with `--continue`.

A manual task can be a DNS change, a credential change, or a look at the cluster before the Flux install.

## Stop after a step

Name the last step to run. A step is a Terraform component ID or a Kustomization name.

```sh
windsor bootstrap --to cluster
```

Windsor applies every Terraform component up to and including `cluster`, in blueprint order. This includes everything `cluster` depends on. Then it stops with exit code 0 and prints the command to continue:

```
Stopped after terraform component "cluster". Run 'windsor bootstrap --continue' to continue.
```

A stopped run does not install the Flux blueprint, print the post-run notes, or write the version marker.

To see the valid names, pass a name that does not exist. The error lists every Terraform component and Kustomization.

## Continue

```sh
windsor bootstrap --continue
```

Windsor skips the Terraform components that the stopped run finished. It runs the rest, installs all Kustomizations, and removes the record of the stop. `bootstrap --continue` asks for no confirmation, because you confirmed the first run.

Pass `--to` with `--continue` to go one step at a time:

```sh
windsor up --to cluster
windsor up --continue --to gitops
windsor up --continue
```

## Stop at a Kustomization

When the name is a Kustomization, Windsor applies all Terraform components. Then it installs that Kustomization and the Kustomizations it depends on, and waits for those only. The other Kustomizations wait for `--continue`.

## Names that match both

When a name is both a Terraform component and a Kustomization, Windsor asks for a prefix. Use `terraform/<id>` or `kustomize/<name>`.

## The backend components

When the blueprint declares a backend, `bootstrap` applies the backend components as one unit and moves their state to the remote backend. A stop inside that unit would leave the state half-moved. `--to` accepts only the last component of the unit. Use that component to stop after the backend.

## How Windsor remembers

A stop writes `.windsor/contexts/<context>/partial-run.yaml`. The file lists the finished Terraform components. Windsor trusts that list, and does not check the state again.

`--continue` fails with a hint in these cases:

- No run stopped.
- The stopped run came from the other command.
- The Terraform components changed since the stop.

A run with no flags ignores the record, runs everything, and removes the record when it succeeds. This is always safe.

The record lives on one machine, like the stack lock. On another machine, or in CI, run the command with no flags.

A run that fails writes no record. Fix the cause and run the command again.

## See also

- [`up`](../reference/commands/up.md), [`bootstrap`](../reference/commands/bootstrap.md)
- [`apply terraform`](../reference/commands/apply-terraform.md) to apply one component
