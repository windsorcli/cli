# ADR 0012 — Context-scoped backend keys: keep contexts apart on a shared remote backend

- Status: Proposed. The deciders chose the `auto_prefix` option. The PR that adds this ADR also
  implements it, so the merge accepts it.
- Date: 2026-11-13
- Deciders: Ryan VanGundy
- Surfaced by cli#3179.

## Context

A remote backend stores the state of each component under a key. For S3 and AzureRM the key is
`<prefix><component path>/terraform.tfstate`. For GCS it is the prefix `<prefix><component path>`.
For Kubernetes it is the secret suffix `<prefix>-<component path>`. The optional
`terraform.backend.prefix` is the only part that can differ between contexts. It is empty by
default.

The key has no context in it. Two contexts that use one bucket, storage account, or namespace
produce the same key for the same component. Terraform then reads and writes one state for both
contexts, and the second `windsor up` can destroy the resources of the first. Nothing warns the
user. The local backend does not have this problem, because its path starts with
`.windsor/contexts/<context>/`.

Customers share a bucket or table across many contexts. This is a common setup for on-prem
industrial teams and for large enterprises. They need a fix and a stable upgrade path
([core ADR 0003](https://github.com/windsorcli/core/blob/main/docs/adr/0003-versioning-and-upgrade-contract.md)).

The key SHOULD have included the context from the start. A change of the default key now would
orphan the state of every existing deployment. Terraform would find no state at the new key and
plan to create every resource again. The default key of an existing context MUST NOT change.

### What the CLI already does

- Every context has an `id`, a random string that `windsor init` generates and writes to the
  context config (`GenerateContextID`).
- `terraform init` runs on every `windsor up` and `windsor apply`. It carries the backend key in
  `-backend-config` arguments, without `-reconfigure` or `-migrate-state`. A changed key makes
  Terraform refuse the init, which is safe but gives no guided fix.
- The stack already moves state when the Kubernetes backend namespace changes. It compares the
  backend pointer in `TF_DATA_DIR` with the configured value and adds
  `-migrate-state -force-copy`. This ADR reuses that path.

## Decision

### 1. `terraform.backend.auto_prefix` adds the context to the key

`terraform.backend.auto_prefix` is a boolean. A missing value means `false`. When it is `true`,
Windsor puts `<context>-<id>/` between `terraform.backend.prefix` and the component path:

```
<prefix><context>-<id>/<component path>/terraform.tfstate
```

For example, `team-a/staging-w8k2x9q/clusters/aws/terraform.tfstate`.

- The manual `prefix` stays outermost. Bucket policies, IAM rules, and lifecycle rules usually
  match a leading path such as `team-a/*`. The automatic segment sits next to the component, so
  those rules keep working.
- A slash in the context name becomes a hyphen.
- The segment applies to S3, AzureRM, GCS, and Kubernetes. For Kubernetes it joins the secret
  suffix with a hyphen, and Windsor refuses a suffix longer than 63 characters instead of cutting
  it. A cut would end the component path and could give two components one suffix.
- The local backend and the `none` backend do not change.
- With `auto_prefix: true` and no context ID, Windsor stops with the remediation "Run windsor
  init to generate the context ID". It MUST NOT compute a key without the ID, because that would
  point Terraform at an empty state.

The name says that Windsor writes this part of the prefix and the user writes the rest. The end
state is a key that always has the context in it. A later release can make `true` the default.
`false` then becomes the way to keep a legacy key.

### 2. An existing context keeps its key

A context that does not set `auto_prefix` keeps the key it has today. An upgrade changes no key.

### 3. `windsor init` turns it on for a new context

`windsor init` writes `terraform.backend.auto_prefix: true` to `values.yaml` when no config
source holds the context. The sources are the context entry in the root `windsor.yaml`, a context
`windsor.yaml` or `windsor.yml`, and `contexts/<context>/values.yaml`. `--set` can override the
value.

The check MUST NOT use a missing `id` as the signal. A context from before IDs existed has no
`id`. It would get one on its next `windsor init`, take the new key, and lose its state.

The CLI schema (`api/config/v1alpha2/schema.yaml`) declares the field. The CLI merges it over the
schema of the blueprint, so a blueprint whose schema closes `terraform.backend` still accepts it.

### 4. Turning it on moves the state without a manual step

On `windsor up` and `windsor apply`, the stack runs the init with `-migrate-state -force-copy`
when all of these are true:

- `auto_prefix` is `true`.
- The backend pointer in `TF_DATA_DIR` has the same backend type as the config.
- The key in the pointer equals the legacy key, which is the key with `auto_prefix` off.
- The configured key differs from the legacy key.

Any other difference between the pointer and the config keeps the current behavior. Terraform
refuses the init and the user decides. This limits the automatic move to the one change that this
ADR defines. A typo in `prefix` does not trigger a copy.

`windsor plan` reads the state at the legacy key with `-reconfigure` and writes nothing, as it
does for a namespace change.

The move copies the state. It does not delete the object at the legacy key. Terraform has no
command for that. The user can delete it after a successful apply.

### 4.1 What the user must still do

The move depends on the backend pointer, a file in `.windsor/contexts/<context>/.terraform/<component>/`
that exists only on a machine that already ran `windsor up`. Three cases follow:

- On a machine with no pointer, such as a new CI runner or a fresh checkout, nothing moves. A
  `windsor up` there reads the new, empty key and plans to create every resource. To avoid this,
  the user MUST run the first `windsor up` after the change from a machine that has a pointer.
- Every teammate has a pointer at the legacy key until they run `windsor up`. A second teammate
  who runs it after the first copies the legacy state over the state at the new key. If the
  state changed between the two runs, the copy loses that change. Teams MUST turn the option on
  in one commit, run one `windsor up`, and tell the others to pull before they run `windsor up`.
- CI jobs that run `windsor up` MUST wait until that first run finishes.

### 5. A warning for the dangerous case

On `windsor up` and `windsor apply`, when `auto_prefix` is off, Windsor reads the other contexts
under `contexts/` and warns once for each context that has the same backend type, the same
target, the same `prefix`, and `auto_prefix` off. The target is the bucket for S3 and GCS, the
storage account and container for AzureRM, and the namespace for Kubernetes. The warning names
both contexts and the setting that fixes it.

The check reads local files and makes no network call, so it works offline. It has limits:

- It sees only the contexts of this project. Another repository can use the same bucket.
- It does not see a target that comes from `backend.tfvars` or a facet, such as a bucket that a
  managed backend component creates.
- A warning does not stop the run.

## Options considered

### Flip the default for everyone

This is the right end state and the wrong first step. It orphans the state of every existing
deployment. It is the later goal of Decision 1, after a release that has moved the state of the
users who opt in.

### Put the context in `prefix` with an expression

`windsor init` would write `prefix: ${context}-${id}/`. This has one setting and no new name. The
CLI cannot tell a deliberate prefix change from a mistake, so Decision 4 would lose its exact
trigger. Migration would become a manual step. It also makes the `prefix` field responsible for
both the manual and the automatic part.

### Terraform workspaces

The `default` workspace keeps today's key, and a named workspace holds each context. All four
backends support workspaces. Moving an existing context needs `workspace new` and a state copy,
which `-migrate-state` does not do. Terraform also advises against workspaces for separate
environments. They change `terraform.workspace` inside modules.

### A key version number

`key_version: 2` would fit the move to a new default. It tells the user nothing without the docs.
`auto_prefix` says who writes the prefix.

## Consequences

- No existing deployment changes on upgrade.
- A new context has its own key space from its first `windsor init`.
- A user who turns the option on gets the state moved on the next `windsor up`, with the limits
  in Decision 4.1.
- `core` follow-up: the backend facets and docs SHOULD show `auto_prefix`. The CLI schema
  already accepts it, so no `core` change is required for the CLI to work.
- `windsor init` adds a `terraform.backend` block to the `values.yaml` of every new context,
  including local ones, where the value has no effect.
