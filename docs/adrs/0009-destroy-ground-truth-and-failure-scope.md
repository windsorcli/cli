# ADR 0009 — windsor destroy: hold the barrier as long as the work declares

- Status: Proposed
- Date: 2026-09-20
- Deciders: Ryan VanGundy
- Surfaced investigating cli#3417 (three independent destroy stalls in one acceptance run)
  alongside cli#3395/#3396 (abandoned-inventory grace window) and cli#3279/#3277 (finalizer-race
  false positives). Earlier drafts proposed a blueprint classification field, a Helm label
  sweep, and a delete retry. None survived scrutiny; all three are recorded under Alternatives.

## Context

`DeleteBlueprint` (`pkg/provisioner/kubernetes/kubernetes_manager.go`) walks a blueprint's
Kustomizations in reverse-dependency order. Each goes through `deleteKustomization`, which
requires individually-confirmed disappearance before moving on, and any single failure aborts the
whole remaining walk through `abortDestroy`. cli#3417 frames the cost precisely: across
roughly 15 links, run-level failure probability compounds as `1 - (per-link reliability)^15`, and
each failed link burns 20 to 38 minutes before it surfaces.

Two point fixes already landed. cli#3396 scaled the abandoned-inventory grace window by inventory
size; cli#3277 wired the delete-wait floor to a Kustomization's `spec.timeout`. Both narrow
per-link risk. Neither addresses the gaps below.

**The wait is a barrier, and it gives up for the wrong reasons.** kustomize-controller is a
reconciler. It holds a finalizer, prunes the inventory, and retries its own reconcile
indefinitely. Windsor does not make a delete happen and cannot speed one up. It blocks only so
Terraform does not destroy the cluster while Flux is still deleting things inside it. Every
defect below is the barrier reporting failure while Flux was still converging.

A single transient read ends the whole run. The wait loop returns on any non-NotFound error from
one `GetResource`, which fails that Kustomization, which aborts the remaining walk.
One throttled or timed-out API call during a poll is enough. `DefaultKustomizationWaitMaxFailures`
has sat in `pkg/constants/constants.go:174` for exactly this, unreferenced, until Decision 1
wired it up.

The budget that decides when to give up is an install budget. cli#3277 wired `specTimeout`
to the Kustomization's own `spec.timeout`, and `core`'s facets do declare one on nearly every tier:
26 at 5m, 19 at 10m, 8 at 15m, 7 at 20m, 6 at 30m. That number dominates the `inventorySize`
scaling in `kustomizationDeletionTimeout`, so the barrier is not as short as a one-entry
inventory would suggest. It is simply the wrong number: `spec.timeout` is what the author budgeted
for the chart to install, and `DeleteTimeout` exists precisely because delete latency differs. Every
one of `core`'s 27 HelmReleases declares `spec.timeout`; none declares `spec.uninstall.timeout`.

**Verification stops at the HelmRelease wrapper.** `firstLiveInventoryEntry` resolves
each inventory entry through `resolveScopedGVR` and issues a live `GetResource`. For an
entry that is the resource, `NotFound` is complete proof. For a `helm.toolkit.fluxcd.io/HelmRelease`
entry it proves only that helm-controller cleared its own finalizer, which it can do after
abandoning a stuck uninstall. This is not a corner case in `windsorcli/core`: **27 of 31 install
tiers are HelmRelease-wrapped**, including `provisioning/install/crossplane`,
`pki/install/cert-manager`, `policy/install/kyverno`, and every CSI driver. Resources tiers run the
other way, 12 of 14 raw, which is why `demo-resources` and `database-resources` are directly
verifiable today.

**The HelmRelease API already exposes what windsor needs, and nothing reads it.** helm-controller's
`v2.HelmRelease` (api v1.6.4, against the Flux 2.9.5 that `core` deploys) carries
`status.inventory` as a `ResourceInventory`, whose `ResourceRef.ID` uses the identical
`<namespace>_<name>_<group>_<kind>` encoding windsor already parses in `decodeInventoryID`.
`spec.uninstall` carries `timeout`, `keepHistory`, `disableWait`, and a `deletionPropagation` enum
of `background` (default), `foreground`, or `orphan`. `status.history` records each release
snapshot's Helm `Status`, and `status.storageNamespace` names where the release Secret lives.
Windsor reads none of it. `GetHelmReleasesForKustomization` already decodes HelmRelease
entries out of a Kustomization's inventory, but only feeds `describeStuckHelmReleases`,
which appends condition text to a timeout message.

One more finding from auditing `core`: exactly one HelmRelease of the 27 sets uninstall semantics at
all. `lb/install/aws-lb-controller/helm-release.yaml` sets `deletionPropagation: foreground` and
`disableWait: false`. Someone hit the failure that setting prevents, fixed it in place, and the
other 26 charts still run on defaults.

## Decision

### 1. Tolerate a transient read during the delete wait

Ship this first and alone. It is the smallest change here and it removes a single point of failure
for the whole destroy run.

The wait loop MUST NOT abort on one failed status read. It SHOULD tolerate up to
`DefaultKustomizationWaitMaxFailures` consecutive read errors, sleeping a poll interval between
them, and MUST reset that count on any successful read so only a sustained outage surfaces. A
NotFound keeps its own meaning and its existing branch. A persistent read failure still fails the
delete, with the error it fails on today.

Tolerance opens one window that MUST be closed with it. `lastObj` is assigned only on a successful
read, and it is the snapshot the NotFound branch verifies its inventory against. If every read
fails and the Kustomization then disappears, `lastObj` is nil, `describeAbandonedInventory` finds
no entries, and the delete reports clean without windsor having read the inventory once. So a
disappearance with a nil `lastObj` and at least one failed read MUST fail, naming the read error.
A first read that returns NotFound with no failures stays a clean delete, which is the existing
fast-path and is unchanged.

This is not a retry of the delete. Nothing is re-issued and no budget is extended. The loop simply
declines to treat one unreadable poll as proof that Flux stopped working.

### 2. Size the wait from the HelmRelease's uninstall timeout

The barrier should wait as long as the work declares it needs, which is the direct fix for
everything a retry was going to approximate.

For a HelmRelease entry, `deleteKustomization` SHOULD read `spec.uninstall.timeout` and extend the
wait with it, the way `extendWaitFor` already folds in the other sources. Resolve it exactly as
`Uninstall.GetTimeout` does, falling back to the HelmRelease's `spec.timeout`. Where a Kustomization
wraps several HelmReleases, take the longest, since the barrier clears only when the slowest does.
A HelmRelease that declares neither timeout, or that cannot be read, MUST contribute nothing rather
than a default, so this never shortens a wait. Cap it at `kustomizationSpecTimeoutCeiling`, the same
bound the Kustomization's own `spec.timeout` already answers to.

An explicit `DeleteTimeout` MUST still bound the result. `DeleteTimeout` is documented as bounding a
delete, so the chart-declared budget belongs with the other spec-derived floors it replaces, not
stacked on top of it — otherwise a blueprint that sets a deliberately short delete window silently
inherits a chart's much longer one, and the operator's knob does nothing.

This decision only pays off once `core` declares the field, and that ordering is the point rather
than a caveat. With `spec.uninstall.timeout` unset, `GetTimeout` returns `spec.timeout`, which
tracks the Kustomization timeout windsor already reads — cert-manager declares 20m against the
facet's 20m, kyverno 30m against 30m — so windsor would read a second number that matches the
first. Declaring it is what creates the signal.

Declaring it earns its keep in `core` independently of this decision. helm-controller bounds its own
uninstall operations by that value, so a chart whose teardown genuinely outlasts its install gets
longer before helm-controller abandons the uninstall and clears its finalizer — which is the
upstream half of the failure Decision 3 exists to detect.

### 3. Follow the HelmRelease's own inventory, and snapshot it during the wait

For an inventory entry whose GVK is `helm.toolkit.fluxcd.io/HelmRelease`, `firstLiveInventoryEntry`
SHOULD read that HelmRelease's `status.inventory`, decode its entries with the existing
`decodeInventoryID`, and verify each through the same `resolveScopedGVR` plus `GetResource` path it
already uses. The ID encoding is identical, so this reuses the decoder, the scope resolution, and
the liveness check without new machinery. Bound the walk at one level: a nested HelmRelease is
checked for liveness like any other resource, but not descended into. Skipping it outright would
count a live one as gone.

`deleteKustomization`'s wait loop MUST capture each HelmRelease's inventory as it polls. Reading it
only after the Kustomization disappears is too late, since the HelmRelease is usually gone by then.
Windsor already refetches the Kustomization every interval and holds `lastObj`; this extends that
habit one level down.

Accumulate a union across polls rather than keeping the last reading. Implementation found the
reason: helm-controller shrinks `status.inventory` as an uninstall proceeds, so the final reading
can be empty while the resources it listed moments earlier are still live. Keeping only the last
one would verify nothing precisely when there is most to verify.

This resolves the `helm.sh/resource-policy: keep` case as a side effect, and that is worth stating
because two earlier drafts could not. A kept resource was applied by the release, so it appears in
the release's inventory. Verifying that entry finds the object still live, and windsor correctly
reports the Kustomization as not drained.

Degrade explicitly. `status.inventory` is an optional field, so when it is absent the entry stays
unverifiable and Decision 4 MUST NOT act on it.

A HelmRelease windsor never observed live MUST be unverifiable, including one already absent on the
first poll. An earlier draft made that case clean, reasoning that flux must have finished the
uninstall before windsor looked. Review found it reopens the exact leak this decision closes, on
the run where it matters most: after helm-controller abandons an uninstall and the operator reruns,
the first poll of the rerun finds the HelmRelease already gone. The snapshot lives in memory for
one call, so nothing from the failed run survives, and the abandoned resources appear in no
inventory at all. Every entry would verify as gone and terraform would destroy the cluster over
them.

Three states, not two. A HelmRelease never observed is unverifiable. One whose inventory only
partly decodes is unverifiable, since a short list must not be verified as though it were the whole
chart. One observed live that never published an inventory records nothing and falls back to the
older, shallower answer, because that flux cannot report what a chart owns and failing every
destroy on that basis would be worse than the gap it closes.

#### Verified on a live cluster, 2026-09-20

A `local` context running Flux 2.9.5 with helm-controller v1.6.4 settles the open questions.

- `status.inventory` is populated on every one of its 13 HelmReleases. The optional field is not a
  theoretical dependency at this version.
- It is where the resources actually are. The Kustomization layer sees 189 entries; the HelmRelease
  layer beneath holds 331. One Kustomization inventory entry stands in for 67 objects under
  `kube-prometheus-stack`, 52 under `kyverno`, 51 under `cilium`, 41 under `cert-manager`.
- The ID encoding really is identical, confirmed against `cli-utils@v1.3.0` `ParseObjMetadata`,
  which `kustomize-controller@v1.7.3/internal/inventory` uses for the Kustomization side. One
  decoder serves both layers.

The decoder needed fixing first, and that shipped separately. Flux encodes a colon in an RBAC name
as a double underscore, so 36 of those 331 entries carry more than four underscore-separated fields.
The previous `SplitN(id, "_", 4)` truncated the name and swept group and kind into `Kind`. Those
entries then failed to resolve and were counted as gone.

#### The failure bias this decision closes, and what it leaves open

That last failure is the pattern. Three paths turn an entry windsor cannot read into an entry
windsor calls absent: `decodeInventoryEntries` drops what it cannot decode, `resolveScopedGVR`
reports a `NoMatchError` as not-found, and the liveness walk continues past both. Every one fails
toward "gone", which on a destroy path means "safe to proceed".

Two consequences of that bias are fixed here, since verification correctness is the point of this
decision:

- The drained check read `inventoryFound` from the raw slice while `entries` held the decoded list.
  An inventory that failed to decode would have been reported "fully drained". It now requires
  every entry to decode.
- `describeAbandonedInventory` swallowed the liveness error and returned nil, so an API failure
  became a clean delete. It now returns the error, and the caller fails rather than reports a delete
  it could not confirm.

One path stays open, deliberately. `resolveScopedGVR` still reports a `NoMatchError` as not-found,
so an inventory entry whose API type no longer exists counts as gone. That is usually right: the
CRD layers tear down last, and removing a CRD cascades its custom resources. It is wrong only where
a chart installs CRDs without the dependency edge that would order them, and closing it risks
failing destroys that are legitimately complete. Worth revisiting with evidence from a real run
rather than on principle.

### 4. Auto-heal a stuck finalizer only where the whole chain verified

Where every inventory entry, including every HelmRelease child entry from Decision 3, is confirmed
gone, `deleteKustomization` SHOULD issue the `PatchResource` call its error currently asks the
operator to run by hand. Nothing else can hold the object: every applied resource is
confirmed absent, and Kubernetes garbage-collects their children. This closes cli#3279's ask.

Where any entry is unverifiable, windsor MUST keep failing with a message and MUST NOT clear the
finalizer. Trading a loud failure for a silent orphan is the wrong direction.

Two `spec.uninstall` settings make a HelmRelease unverifiable no matter what its inventory says, and
windsor MUST treat them as such: `deletionPropagation: orphan`, which deliberately leaves dependents
behind, and `disableWait: true`, which returns before resources are gone. Neither appears in `core`
today. Detecting them is a cheap read on an object windsor already fetches.

### 5. Scope the abort to the failed Kustomization's dependency closure

Deferred behind the others. It carries the most risk of the five, because it changes a safety
property rather than adding one.

Destroy runs reverse-topologically, so K's dependents are already deleted when K is attempted, and
what stays pending is K's own dependency closure plus unrelated nodes. The hazard is specific:
delete `provisioning-install`, the Crossplane controller, while `demo-resources`'s
`DatabaseInstance` is mid-delete, and no finalizer ever reconciles that cloud database again.

So `abortDestroy` MUST skip every pending Kustomization inside K's transitive dependency closure,
and SHOULD continue the walk outside it, using the `DependsOn` edges
`reverseTopologicalKustomizations` already walks. The payoff is uneven:
`applyCrdLayerBarrier` (`pkg/composer/blueprint/composer.go:960`) puts the CRD layer inside almost
every closure, so the gain is for a stall on a leaf whose siblings never named it.

## Follow-on for `core`, not decided here

`core` sets uninstall semantics on 1 of 27 HelmReleases. The `aws-lb-controller` case is real
precedent — a controller that must release cloud load balancers before it disappears — and it is
undocumented.

Two things follow. `core` SHOULD declare `spec.uninstall.timeout` per chart, chosen for how long
that chart's teardown actually takes rather than copied from `spec.timeout`, since a value equal to
the install budget carries no information for Decision 2 to read. And `core` SHOULD decide its
`deletionPropagation` default deliberately and record the reasoning, now that Decision 4 makes
`orphan` and `disableWait: true` load-bearing for whether windsor trusts a delete at all. This ADR
does not mandate a propagation value: `foreground` is slower and can hang visibly, so applying it
blanket would trade one stall class for another.

## Consequences

- Decision 1 lands standalone and removes a single failed API read as a cause of run-wide destroy
  failure. It adds no schema, no API surface, and no new constant.
- Decision 2 replaces an install budget with a teardown budget, which is the honest form of "wait
  longer" that a retry would only have approximated. It is inert until `core` declares
  `spec.uninstall.timeout`, so the two changes are worth sequencing together.
- Decisions 3 and 4 together extend auto-heal from the roughly half of the walk that is raw-manifest
  to the HelmRelease-wrapped majority, which is what made the narrower version of this ADR barely
  worth shipping.
- Verification depth is bounded by what helm-controller populates. A live cluster at Flux 2.9.5
  populates `status.inventory` on every HelmRelease; where it is absent, windsor falls back to
  today's behavior: fail with a message, change nothing.
- Decision 2 reads each HelmRelease once per delete, on the first poll whose `status.inventory` is
  readable, since a spec does not change mid-teardown. It retries on later polls rather than
  latching on a first attempt that found nothing, because a Kustomization applied moments earlier
  may not have reconciled that far. Decision 3 needs a read per poll instead, because it tracks
  `status.inventory` as it shrinks.
- cli#3405 (terraform destroy's fixed 30-minute bound and its skip-retry-on-timeout) stays out of
  scope: same shape, different subsystem, own constants, own open question about the bound.
- cli#3371 (progress visibility) stays deferred to the TUI overhaul.
- `--continue` does not weaken any of this. A kustomize-stage failure stops terraform either way:
  `Provisioner.DestroyAll` returns the error without the flag, and defers the terraform stage with
  it. So a Kustomization windsor cannot confirm is drained never becomes a cluster terraform
  destroys underneath it. What changes is the operator's experience: a run that used to report
  clean over a HelmRelease-wrapped tier can now stop. A rerun re-walks the whole blueprint, since
  windsor keeps no resume state; Kustomizations already gone return immediately.
- An inventory entry windsor cannot decode now fails the destroy rather than being dropped from the
  set. After the decoder fix this is rare, and the alternative is verifying a list that is shorter
  than the inventory it came from.

## Alternatives considered

- **Retrying a failed Kustomization delete**, bounded and gated on inventory progress, mirroring
  `execTerraformDestroyWithRetry` (`pkg/provisioner/terraform/stack.go:1195`). This was Decision 1
  in an earlier draft and was implemented before being withdrawn. Rejected on two grounds. First,
  it does not do what its evidence claims: cli#3395's converging rerun and cli#3417's CI retries
  show that more elapsed time worked, not that re-entering the delete worked. Since
  kustomize-controller never stopped reconciling, a retry only re-issues a `DeleteResource` that is
  a no-op against a terminating object and then polls again, which is a longer wait spelled as a
  loop. Decision 2 buys the same time by declaring it. Second, it is unsafe at the boundary: a
  Kustomization that vanishes between attempts sends the next attempt into
  `deleteKustomization`'s `NotFound`-on-entry branch, which returns nil, and a retry starts with no
  `lastObj`, so the abandoned-inventory check it would have tripped cannot run. The retry converts
  a live orphan into a reported success — the precise failure the surrounding machinery exists to
  catch. A pre-commit review found this on the implemented version.
- **A facet-author-declared `externalResources` field**, marking a Kustomization as owning nothing
  outside the cluster so its wait could shorten. Rejected: a static claim about dynamic reality. A
  StatefulSet's PVC on a dynamically-provisioned storage class makes a CSI driver delete a real
  cloud disk on reclaim, invisible in the facet YAML and dependent on whichever storage class an end
  user's config resolves to. A stale classification would not error. It would under-wait, report
  success, and abandon infrastructure.
- **Sweeping the namespace for objects labeled with the Helm release.** Rejected:
  `meta.helm.sh/release-name` is an annotation, so no label selector expresses release membership;
  `ListResourcesByLabel` takes one GVR and the client has no discovery call; and a namespace sweep
  misses cluster-scoped chart resources, including the `ProviderConfig` in cli#3395. Decision 2
  obtains the same answer from the release's own inventory instead.
- **Gating the auto-heal on Helm's release Secret.** Rejected as a gate: `resource-policy: keep`
  resources outlive an uninstall Helm reports clean, and the Secret is deleted on success unless
  `keepHistory` is set. Decision 2 supersedes the need for it.
- **Reading `HelmRelease.status.history` after the fact.** Rejected: readable only while the object
  exists. Decision 2's snapshot-during-the-wait is the same idea applied at a point where the data
  is actually there.
- **Suspending and resuming a terminating Kustomization** to force a fresh reconcile when a delete
  stalls. Rejected on risk, and recorded because `DeleteBlueprint` already suspends and resumes
  around each delete: Flux skips suspended objects, finalizer processing included, so suspending
  one mid-delete can wedge the deletion the nudge was meant to unstick.
- **Stopping the dependents of a failed Kustomization** rather than its dependencies. An earlier
  draft of Decision 5, and it inverts the hazard: under reverse-topological order the dependents are
  already gone, so it would halt an empty set and keep deleting what a terminating Kustomization
  still needs.
- **Leaving abort-everything in place** and relying on the merged point fixes. Rejected: cli#3396 and
  cli#3277 narrow per-link risk without touching the run-level compounding.

## References

- cli#3417, cli#3405, cli#3395/#3396, cli#3279/#3277, cli#3371
- `pkg/provisioner/kubernetes/kubernetes_manager.go`: `deleteKustomization`, `specTimeout`
  (314), `GetHelmReleasesForKustomization`, `decodeInventoryID`, `DeleteBlueprint`
  (1540), `abortDestroy`, `describeStuckHelmReleases`, `allInventoryEntriesGone`
  (2389), `resolveScopedGVR`, `firstLiveInventoryEntry`,
  `reverseTopologicalKustomizations`
- `github.com/fluxcd/helm-controller/api v1.6.4`, package `v2`: `ResourceInventory` /
  `ResourceRef.ID` (`inventory_types.go`), `Uninstall` with `Timeout` / `KeepHistory` /
  `DisableWait` / `DeletionPropagation` and `GetTimeout` (`helmrelease_types.go:1215-1262`),
  `HelmReleaseStatus.Inventory` / `History` / `StorageNamespace`, `Snapshot.Status`
  (`snapshot_types.go`)
- `pkg/provisioner/kubernetes/client/client.go`: `PatchResource`, `ListResourcesByLabel` (48), and
  the absence of any discovery method
- `pkg/provisioner/terraform/stack.go`: `execTerraformDestroyWithRetry` (1195), the retry
  pattern Alternatives rejects for this path
- `pkg/composer/blueprint/composer.go`: `applyCrdLayerBarrier` (960)
- `core`: 27 of 31 install tiers HelmRelease-wrapped, 12 of 14 resources tiers raw;
  `kustomize/lb/install/aws-lb-controller/helm-release.yaml:20-22` the sole uninstall config;
  `terraform/gitops/flux/variables.tf` (Flux 2.9.5)
