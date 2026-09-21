# ADR 0009 — windsor destroy: respect every finalizer, decide nothing on its behalf

- Status: Proposed. Decisions 1 through 3, 5, and 6 have shipped. 4 is revised below a second time: a
  live GCP run first showed the per-tier abort was wrong, and the classification-based gate written
  to replace it was itself withdrawn before implementation for the reason recorded under
  Alternatives.
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

### What a live GCP teardown showed, 2026-09-20

A `gcp-test` bootstrap and destroy ran the decisions below against real cloud infrastructure. The
result splits cleanly in two, and the second half is why Decisions 4 and 5 are revised.

The external half worked. A Crossplane `DatabaseInstance` held
`finalizer.managedresource.crossplane.io` until its Cloud SQL instance was really deleted, windsor
blocked on it, and the database came down. A later `gcloud sql instances list` returned `Listed 0
items.`, with a successful GKE listing as the control for credentials and project. Finalizer-free
residue passed through as warnings, 21 CRDs from `gateway-install` and 22 from `telemetry-install`.
That is the principle above behaving as designed on infrastructure that costs money.

The internal half hung, and it hung permanently. `ProviderConfig
demo-database/provider-sql-demo-db`, a Crossplane composed resource, held `in-use.crossplane.io`
with its deletionTimestamp set. Nothing removed it for 77 minutes. Its owning composite was already
gone, zero `ProviderConfigUsage` objects existed, and the provider pods were healthy, so the
finalizer's own precondition for removal was satisfied and it stayed anyway. Why provider-sql never
removed it is unresolved and is not diagnosed here.

The cascade from that one object is the finding:

1. A namespaced object with a finalizer cannot be removed, so `Namespace/demo-database` stayed
   Terminating.
2. That namespace was in `demo-resources`' inventory. Flux waited, gave up, and cleared its own
   `finalizers.fluxcd.io`, so the Kustomization disappeared while its inventory was still live.
3. Windsor detected exactly that, aborted, and skipped the 7 remaining Kustomizations rather than
   orphan them.
4. `abortDestroy` deferred terraform, so the GKE cluster kept running and billing.

Every step behaved as specified. The outcome was still wrong. Windsor blocked a cluster deletion on
an obligation that only ever concerned in-cluster bookkeeping, at a moment when nothing external was
at risk: the Cloud SQL instance was already deleted and no managed resources remained. Clearing that
one finalizer by hand let the namespace terminate in about two minutes, after which the entire
remaining walk completed without incident — which also rules out a systemic ordering defect.

The resumed run then completed end to end: 22 destroyed, 0 failed, with the project verified empty
afterwards — no cluster, no SQL instance, no reserved address, no state bucket.

A second finding came from that verification. The project held six service accounts from three
clusters that no longer existed, with their IAM bindings intact, while the run that completed
removed its own cluster's service accounts without help. So the residue traces to destroys that
stopped partway, not to terraform. Each abort of the kind described above leaves the project
slightly dirtier, and nothing reports it.

## The principle

Windsor blocks before `terraform destroy` for one reason: so the cluster does not disappear while
something inside it still has to release state that lives outside it. Everything below follows from
asking that one question precisely.

Kubernetes already answers it. A finalizer is the only native mechanism for "deleting this requires
work beyond removing the record". A controller that owns external state has no other way to clean
up, so it must set one. Crossplane does on its managed resources, the AWS load balancer controller
does on Services, CSI does on PersistentVolumes. Flux sets `finalizers.fluxcd.io` on everything it
handles, which its own API documents.

So the rule is not a heuristic about kinds. An object still carries obligation while it exists and
something is still due to act on it: it carries a finalizer, or it carries a deletionTimestamp.
Anything else is removed by the same etcd teardown that destroy is about to perform.

A finalizer is a proxy for the question that actually matters, and it covers two different
obligations:

- **External work** — delete a cloud database, release an address, deregister from a system outside
  the cluster.
- **Internal work** — decrement a usage count, drop a reference, tidy dependent records in etcd.

Internal work is moot once the cluster is deleted. The etcd it operates on is about to stop
existing, so waiting on it is waiting for a correction to a ledger that is being discarded. External
work is never moot: the database outlives the cluster, and skipping it leaves infrastructure that
costs money and that nothing tracks.

That distinction explains why finalizers exist. It is not a distinction windsor can compute. Nothing
in the Kubernetes API says which obligation a given finalizer represents — that knowledge lives in
the controller that set it, or in whoever wrote the CRD, and nowhere windsor can read it generically.
Two earlier drafts of this decision tried anyway: one by clearing a finalizer once its chain looked
fully verified, one by checking live objects against a curated set of kinds known to own external
state. Both are a classifier standing in for information windsor does not have, and both fail the
same way — silently, on whichever case the classifier had not seen yet. The withdrawal of each is
recorded under Alternatives, because the reasoning is part of the decision.

So windsor does not classify. It treats every finalizer as real, every time, with no override it
grants on its own judgment:

> An object still carries obligation while it exists and something is still due to act on it: it
> carries a finalizer, or it carries a deletionTimestamp. Windsor blocks on that obligation
> unconditionally. It never clears a finalizer, and it never proceeds past one because a check
> concluded the work behind it was probably internal.

Three jobs follow, and they stay separate:

- **Discovery** enumerates what was managed. Flux's `WaitForTermination` stops at its own inventory
  and cannot speak for objects Helm owns, so discovery has to cross that boundary. Decisions 1
  through 3 are all discovery, and their failure modes all reduce to "the enumeration is not
  complete".
- **Waiting** applies the rule above to what discovery found, so each controller gets its chance to
  act, in dependency order, for as long as its own declared budget says it needs. Exceeding the
  budget converts silent waiting into a loud, precise stop — never into a decision to proceed.
- **Report** names what survived without obligation, and what is still obligated when the wait
  ends. A chart that leaves resources behind is a real defect worth surfacing. An object still
  blocking past its budget is a decision point for the operator, handed over with everything windsor
  knows and nothing windsor has guessed.

An earlier draft carved out `CustomResourceDefinition`, because Helm never deletes CRDs and they
were failing every destroy. The principle subsumes that carve-out and does better: an abandoned CRD
holds no finalizer and does not block, while a CRD whose custom resources still exist carries
`customresourcecleanup.apiextensions.k8s.io` and does — which is the case that actually matters,
since those resources may be cloud-backed.

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
unverifiable and MUST be reported as such rather than counted gone.

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

### 4. No override exists. A stalled wait stops and reports; it never proceeds

Windsor MUST NOT remove another controller's finalizer, and MUST NOT proceed to terraform past one
that has not cleared, under any condition windsor evaluates on its own. There is no gate, no
classifier, and no list standing between "still obligated" and "safe to destroy." An object with a
live finalizer or deletionTimestamp blocks the run for as long as it exists. This is the correction
this revision makes, and it removes machinery rather than adding it: an earlier draft of this
decision proposed a check, run once before terraform, that classified live objects against a curated
set of kinds known to own external state and let the run proceed if none matched. That check does
not ship. Reasoning for its withdrawal is recorded under Alternatives.

Two things remain true regardless of how long the block lasts, and the argument for both is the
same one that ruled out clearing a finalizer. Removing one tells the API server that the
controller's work is done, which is a claim windsor cannot verify; if wrong, the object is deleted,
the cleanup never runs, and the record of what was skipped goes with it. Declining to wait asserts
nothing: the object stays, honestly unfinished, until cluster deletion takes it, and windsor can
still name it. A time-based fallback that eventually proceeds anyway is the identical mistake with
elapsed time standing in for the list — the stuck `ProviderConfig` from the GCP run had already
satisfied its own removal precondition and would not have cleared given an hour more or a week more.
Waiting longer does not make a permanently stuck object safe to pass; it only delays finding out that
it was never going to move.

What the budget from Decisions 1 through 3 governs is not whether windsor may proceed. It governs
how long windsor waits silently before it stops and says so. Once a Kustomization's wait is
exhausted, `deleteKustomization` MUST fail, and that failure MUST defer terraform: the cluster is
never destroyed while any Kustomization has not confirmed drained. What happens to the rest of the
kustomize walk when one Kustomization stalls is Decision 5's question, not this one. `deleteKustomization`
MUST report the specific objects still holding obligation, using only facts it can compute without
classification:

- the object's identity and its finalizer strings, verbatim, with no interpretation of what they
  mean
- how long it has been pending against the budget just computed
- its `ownerReferences`, and whether each named owner still exists — this is what surfaces a
  dangling composed resource like the GCP `ProviderConfig`, where the owning composite was already
  gone, and it is answerable for any object of any kind without a lookup table, because
  `ownerReferences` is generic API structure, not domain knowledge

This is strictly more useful than today's timeout message, and it costs nothing the walk was not
already computing. It is also, deliberately, not a verdict. On the GCP run, the operator decided the
dangling `ProviderConfig` was safe to clear by hand after reading exactly this evidence: owning
composite gone, no referencing objects. Windsor now assembles that evidence and hands it over rather
than reaching the same conclusion on its own.

#### The terminal check reads the cluster directly, not Flux's account of it

The report above is scoped to what Flux tracked, because that is the mechanism doing the deleting.
It says nothing about a Crossplane managed resource an operator applied by hand, a `LoadBalancer`
Service outside any GitOps tree, or a PersistentVolume a StatefulSet provisioned dynamically — none
of those appear in any Kustomization's inventory, so the walk above cannot see them. Neither could
the withdrawn gate, which scoped its check the same way.

So the check that runs immediately before terraform MUST NOT be scoped to any inventory. It MUST
enumerate every resource type the cluster's API server serves, using discovery
(`k8s.io/client-go/discovery`, the same mechanism `kubectl api-resources` uses), and list every
object of every type — metadata only, via `k8s.io/client-go/metadata`, since the check needs only
`finalizers` and `deletionTimestamp`. Every object carrying either MUST block, with no exception for
kind.

This is not the withdrawn gate returning under a new name. That gate matched live objects against a
curated set of kinds believed to own external state and let the run proceed past anything outside
it. This asks one uniform question of every object the cluster actually contains — does it still
carry obligation — and answers it exactly the way Decision 4 already does everywhere else. The list
is gone. The completeness an earlier draft achieved only for a curated subset is now total: nothing
about this check depends on knowing what a resource is, only on reading two fields every object in
Kubernetes carries.

A resource type whose List call fails, or a discovery call that cannot complete, MUST fail the check
rather than being skipped. This is the same rule Decision 3 applies to an inventory entry that will
not decode: unverifiable is not clean, and silently skipping a type windsor could not query is
indistinguishable, from the operator's side, from deciding it held nothing.

The sweep runs once, immediately before an already-expensive terraform destroy. A metadata-only list
against a few hundred resource types costs seconds, not minutes, and the overwhelming majority —
Events, Leases, EndpointSlices — carry no finalizer and contribute nothing to the report regardless
of how many exist.

### 5. Never tear down a controller a stalled object still needs

Decision 4 makes every stall a stop, so this decision is about what "stop" is allowed to mean for
the rest of the run, not about deciding which stalls matter. It applies to every stall alike, with
no classification step of its own.

Reverse-topological order means K's dependents are already gone when K is attempted, so what stays
pending when K stalls is K's own dependency closure. Deleting `provisioning-install`, the Crossplane
controller, while `demo-resources`' `DatabaseInstance` is mid-delete removes the only thing that
could ever finish reconciling that cloud database. Decision 4 would still stop the run before
terraform, since the `DatabaseInstance`'s finalizer never cleared — but by then the controller that
could have resolved it is gone, and the operator is left with a harder recovery than the stall
itself warranted.

So `DeleteBlueprint` MUST NOT proceed into a stalled Kustomization's transitive dependency closure,
using the `DependsOn` edges `reverseTopologicalKustomizations` already walks, regardless of what
the stalled object is or what its finalizer names. It SHOULD continue deleting Kustomizations
outside that closure, so one stall does not stop progress on unrelated tiers. This is unconditional
on purpose: the whole point of Decision 4 is that windsor no longer decides which stalls are the
dangerous kind, so this decision cannot make that distinction either.

Shipped: `dependencyClosure` computes the closure; `blockDependencyClosure` adds it to a `blocked`
set and warns which names were skipped; `DeleteBlueprint`'s destroy loop checks `blocked` before
attempting each Kustomization, records the failure, and continues rather than returning. The
up-front suspend loop and `remediateLoadBalancerOwners` still abort the whole run on failure —
nothing has been deleted yet at that point, so there is no partial progress worth preserving, and
no closure to compute.

The payoff is uneven, and that is unchanged from the earlier draft:
`applyCrdLayerBarrier` (`pkg/composer/blueprint/composer.go:960`) puts the CRD layer inside almost
every closure, so the gain concentrates on a stall in a leaf whose siblings never named it.

### 6. Trigger a reconcile throughout the wait, since asking again is always safe

This is not a classifier, and it is not an exception to Decision 4. It is a step windsor MAY take
while it waits, grounded in a property every reconcile-based controller already guarantees: a
reconcile MUST be idempotent and safe to trigger again at any time, because every such controller
already receives spurious extra reconciles it did not ask for, from cache resyncs, leader-election
handoffs, and watch-stream reconnects. Asking a controller to look again is not a guess about what
it will find. It is the same event the controller is already built to tolerate, requested on purpose
instead of waiting for it to happen on its own schedule.

Reading `crossplane-runtime`'s own `ProviderConfig` reconciler
(`pkg/reconciler/providerconfig/reconciler.go`) shows why this matters concretely, and corrects a
hypothesis in an earlier draft of this ADR. Finalizer removal there is deterministic: on every
reconcile it lists live `ProviderConfigUsage` objects by label, actively releases any whose owner is
gone or has finished tearing down using an uncached read chosen specifically to avoid stale-informer
false negatives, and removes `in-use.crossplane.io` the moment the count reaches zero and the
`ProviderConfig` itself carries a deletionTimestamp. Nothing in that logic depends on how the owning
composite was deleted. Given windsor had already confirmed, on the GCP run, zero live
`ProviderConfigUsage` objects and a set deletionTimestamp, both preconditions for removal were
already met — the only remaining explanation is that this object's `Reconcile` simply never ran
again after the last usage disappeared. `provider-sql`'s own controller wiring
(`pkg/controller/namespaced/postgresql/config/config.go`) confirms a plain metadata update would
have triggered exactly that: `ctrl.NewControllerManagedBy(mgr).For(&v1alpha1.ProviderConfig{})` with
no predicate filtering, so any touch to the object requeues it unconditionally. Verified live below.

The earlier hypothesis — a background-propagation delete racing the composite's disappearance
against its composed resources' finalization — does not match this mechanism and is withdrawn. It
predicted the wrong variable. Foreground propagation on the composite would not have made the
`ProviderConfig` reconciler run again; only an event on the `ProviderConfig` or a matching
`ProviderConfigUsage` does that, and per the code above, the object was never watching for a signal
tied to composite propagation in the first place. Why the reconcile was never re-triggered in the
first place is still unknown; the mechanism below is deliberately robust to not knowing.

Two cases, not one, since the guaranteed form and the best-effort form rest on different evidence:

- **A Flux-managed object** — a Kustomization, a HelmRelease — has a documented trigger already:
  `reconcile.fluxcd.io/requestedAt`. Flux's own controllers carry a predicate built to compare that
  key across old and new object versions, so setting it to a fresh value is guaranteed, by Flux's own
  contract, to enqueue a reconcile. Windsor SHOULD use this annotation on Flux objects rather than an
  arbitrary one.
- **Everything else** carries no such guarantee. A windsor-owned annotation only forces a reconcile
  where the target's own controller applies no change-filtering predicate — true for `provider-sql`'s
  `ProviderConfig` controller, confirmed by reading its wiring, and not something windsor can assume
  generally. A controller built with `predicate.GenerationChangedPredicate` or similar would never
  notice an annotation-only patch, since annotations do not bump `metadata.generation`. This case is
  best-effort by construction, and the ADR states that plainly rather than implying a guarantee it
  cannot back.

Either way the technique is the same shape: patch a value that changes on every attempt, so the write
is never a no-op mutation a client might coalesce away.

So while a Kustomization's wait is running, `deleteKustomization` patches the appropriate trigger
annotation onto each object still holding obligation, on every poll. Periodic beats one-shot: the
live run below cleared within seconds of a single patch, well inside any poll interval, so
triggering every poll catches a missed-reconcile stall almost as soon as it happens rather than only
at the edge of the tier's full budget. Nothing else in Decision 4's report changes: if the object
still holds obligation when the budget runs out regardless, the report fires exactly as specified.
This MUST NOT touch `metadata.finalizers` under any circumstance — only the trigger annotation. A
controller that never notices the trigger is no worse off than one windsor never touched.

Shipped in `triggerReconcile`, wired into the grace-window retry loop that already re-checks a
still-live entry after a Kustomization disappears (`deleteKustomization`, the branch guarded by
`abandonedInventoryGraceWindow`) — the exact loop, and the exact scenario, the live verification
below exercised. `fluxReconcileAnnotationGroups` selects `reconcile.fluxcd.io/requestedAt` for
`kustomize.toolkit.fluxcd.io`/`helm.toolkit.fluxcd.io` entries and `windsorcli.dev/reconcile-
requested-at` for everything else, matching the two cases above exactly. A patch failure is not
surfaced: the caller's existing wait and timeout handling already covers an entry that never clears,
for any reason including the trigger doing nothing.

Also shipped: the other place `firstLiveInventoryEntry` runs, when the Kustomization itself never
disappears and the main wait loop times out on its own budget. That path had no retry loop to hang a
trigger off of, unlike the grace-window one, so `deleteKustomization`'s NotFound handling was
extracted into `handleKustomizationDisappeared` — reused by both the main loop and this path, rather
than duplicated. Once the main loop times out, if `firstLiveInventoryEntry` finds a blocking entry,
`deleteKustomization` triggers it once and re-polls the Kustomization itself for
`abandonedInventoryGraceChecks` more intervals. If the Kustomization disappears during that window,
the run falls into the same `handleKustomizationDisappeared` path a normal disappearance would have,
so a resolved stall here can still report a clean delete rather than the timeout the main loop alone
would have produced.

#### Verified on a live GCP cluster, 2026-09-20

The exact deadlock from earlier in this ADR was reproduced on a second `gcp-test` destroy:
`ProviderConfig demo-database/provider-sql-demo-db` held `in-use.crossplane.io` with a
deletionTimestamp set and zero live `ProviderConfigUsage` objects, identical to the first
occurrence. A single manual `kubectl annotate` against the stuck object, using an ad hoc key ahead
of `windsorReconcileAnnotation` being named, was followed, on the very next check, by `NotFound` —
the object was gone.
The owning namespace finished terminating shortly after, on its next controller sweep. `windsor
destroy`'s own walk crossed `demo-resources` and `database-resources` without ever reaching Decision
4's report: the trigger resolved the object faster than the tier's own wait budget would have
noticed a stall at all. This is not a controlled trial — the real cause of the original missed
reconcile is still unconfirmed — but it is the mechanism working end to end against the actual
production failure, not a synthetic reproduction of it.

## Follow-on for `core`, not decided here

`core` sets uninstall semantics on 1 of 27 HelmReleases. The `aws-lb-controller` case is real
precedent — a controller that must release cloud load balancers before it disappears — and it is
undocumented.

Two things follow. `core` SHOULD declare `spec.uninstall.timeout` per chart, chosen for how long
that chart's teardown actually takes rather than copied from `spec.timeout`, since a value equal to
the install budget carries no information for Decision 2 to read. And `core` SHOULD decide its
`deletionPropagation` default deliberately and record the reasoning, since `orphan` and
`disableWait: true` determine whether a chart's residue is expected. This ADR does not mandate a
propagation value: `foreground` is slower and can hang visibly, so applying it blanket would trade
one stall class for another.

The GCP run raises one more item, unresolved and not diagnosed here. The composed-resource deadlock
itself is diagnosed above, under Decision 6, against `crossplane-runtime`'s and `provider-sql`'s
actual source rather than the observed end state alone — the propagation-policy hypothesis an
earlier draft proposed here is withdrawn. What remains open is why the `ProviderConfig`'s reconcile
did not fire again on its own before windsor gave up waiting. The source rules out several
explanations (stale-informer false negatives, GC-ordering races) but does not identify the one that
actually occurred. Decision 6's reconcile trigger is deliberately robust to not knowing: it does not
require diagnosing why the reconcile was missed, only that asking again is always safe, and the live
verification under Decision 6 confirms it resolves this exact failure in seconds. Finding the actual
root cause remains open and is worth an upstream report, but is no longer load-bearing for windsor's
own reliability.

The IAM residue found in the same project is a consequence of this ADR's subject rather than a
separate defect. Six service accounts from three clusters, bindings intact, had outlived the
clusters they belonged to. The completed run then removed its own cluster's service accounts
without help, which places the leak in destroys that stopped partway rather than in terraform's
teardown. Every abort on an in-cluster finalizer leaves a project slightly dirtier, and nothing
reports it.

## Consequences

- Decision 1 lands standalone and removes a single failed API read as a cause of run-wide destroy
  failure. It adds no schema, no API surface, and no new constant.
- Decision 2 replaces an install budget with a teardown budget, which is the honest form of "wait
  longer" that a retry would only have approximated. It is inert until `core` declares
  `spec.uninstall.timeout`, so the two changes are worth sequencing together.
- Decision 4 keeps a standalone check immediately before terraform, but removes what that check used
  to do. It no longer classifies and it grants no permission past anything it finds; it enumerates
  the cluster directly, reports what still carries obligation, and blocks. Windsor never writes to
  another controller's finalizer, in either direction.
- The cluster-wide sweep is strictly more complete than the per-Kustomization report that precedes
  it, because it does not depend on Flux having tracked a resource in the first place. A
  hand-applied managed resource, an out-of-band `LoadBalancer` Service, or a dynamically-provisioned
  PV now blocks terraform exactly as a stalled Kustomization does, where today none of them would be
  seen at all.
- A destroy that genuinely cannot finish now stops sooner and more precisely than before — at the
  Kustomization whose budget actually ran out, with the specific objects and their owner-reference
  state, rather than as a generic timeout an operator has to go investigate cold. It does not stop
  less often. A stall that Decisions 1 through 3 would have resolved as a false positive still
  resolves; a stall that is real still halts the run, every time, because there is no path left that
  proceeds past one.
- This is a genuine cost, not a hidden one: a real stall that would previously have been guessed past
  (by a classifier, had one shipped) now always requires an operator to look at it. The counter-case
  from the GCP run is the reason that cost was accepted — a classifier confident enough to proceed
  automatically would have had to be wrong about `in-use.crossplane.io` in the opposite direction to
  ever earn its keep, and getting it wrong there means an orphaned resource with no record of what
  was skipped.
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
- `--continue`'s existing behavior is unchanged: a kustomize-stage failure defers terraform, full
  stop, exactly as it does today. What changes is how fast and how precisely that failure is
  reported, per Decisions 1 through 3 and the object-level detail Decision 4 adds. A rerun still
  re-walks the whole blueprint, since windsor keeps no resume state, and Kustomizations already gone
  return immediately.
- The cluster-wide sweep adds a new place terraform can be deferred from: a kustomize stage that
  reports every Kustomization drained can still fail here, on an object Flux never tracked. That is
  new information surfacing, not a regression — the object was always there and always at risk; only
  the check that finds it is new.
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
- **Clearing a stuck finalizer once the whole chain verified gone.** Implemented on
  `feat/auto-clear-drained-finalizer` before being withdrawn. Rejected on the asymmetry Decision 4
  states: removing a finalizer writes a claim windsor cannot verify into shared state, and destroys
  the evidence of what was skipped, while declining to wait asserts nothing and lets cluster deletion
  do the same job. Its failure mode also fires independently of whether the destroy completes, where
  the alternative's is bounded by an act the operator already authorized.
- **Gating terraform on a curated set of kinds known to own external state** — Crossplane's `managed`
  CRD category, `LoadBalancer` Services, `Delete`-reclaim PersistentVolumes — and letting the run
  proceed past any finalizer outside that set. Drafted as Decision 4 and withdrawn before
  implementation. The set is real and each entry is correct, but the set is inherently incomplete: a
  third-party controller's own finalizer, on a CRD no one anticipated, passes silently, which is the
  exact failure clearing a finalizer produces, moved one layer up and made harder to see because
  nothing was touched — the run simply proceeded. Rejected once it was clear the honest fix was
  removing the proceed path, not improving the list guarding it.
- **Classifying finalizers by domain-qualified name** (`in-use.crossplane.io` as in-cluster-only,
  `finalizer.managedresource.crossplane.io` as external) instead of by kind, then proceeding past the
  in-cluster class. Rejected for the same reason as the kind-based set: reading a name a controller
  chose to publish is a stronger signal than guessing from CRD category, but it is still a table that
  does not know about a name it has not seen, and the failure on a miss is identical — silent, not
  loud.
- **A long timeout that eventually proceeds anyway**, as a bounded fallback against an indefinite
  block. Rejected: elapsed time is not evidence that a stalled object's work is done. The GCP
  `ProviderConfig` had already satisfied its own removal precondition — zero referencing objects,
  owning composite gone — and stayed blocked regardless; a longer wait would have delayed discovering
  that, not resolved it. The budget from Decisions 1 through 3 already bounds how long windsor waits
  before it stops and reports; extending that bound into a bound that proceeds is the classifier
  Decision 4 rejects, keyed on a clock instead of a list.
- **A destroy-time Kustomization in `core`** that sequences composite deletion before the namespace
  that holds its composed resources. Rejected as a teardown-time hook that encodes ordering a
  second time, outside the dependency graph that already expresses it, and only for the cases
  someone remembered to annotate.
- **A facet-author-declared `externalResources` field**, marking a Kustomization as owning nothing
  outside the cluster so its wait could shorten. Rejected: a static claim about dynamic reality. A
  StatefulSet's PVC on a dynamically-provisioned storage class makes a CSI driver delete a real
  cloud disk on reclaim, invisible in the facet YAML and dependent on whichever storage class an end
  user's config resolves to. A stale classification would not error. It would under-wait, report
  success, and abandon infrastructure.
- **Sweeping the namespace for objects labeled with the Helm release.** Rejected:
  `meta.helm.sh/release-name` is an annotation, so no label selector expresses release membership;
  `ListResourcesByLabel` takes one GVR and the client has no discovery call; and a namespace sweep
  misses cluster-scoped chart resources, including the `ProviderConfig` in cli#3395. Decision 3
  obtains the same answer from the release's own inventory instead.
- **Treating Helm's release Secret as proof a chart is gone.** Rejected: `resource-policy: keep`
  resources outlive an uninstall Helm reports clean, and the Secret is deleted on success unless
  `keepHistory` is set. Decision 3 supersedes the need for it.
- **Reading `HelmRelease.status.history` after the fact.** Rejected: readable only while the object
  exists. Decision 3's snapshot-during-the-wait is the same idea applied at a point where the data
  is actually there.
- **Suspending and resuming a terminating Kustomization** to force a fresh reconcile when a delete
  stalls. Rejected on risk, and recorded because `DeleteBlueprint` already suspends and resumes
  around each delete: Flux skips suspended objects, finalizer processing included, so suspending
  one mid-delete can wedge the deletion the trigger was meant to unstick. This is a different
  mechanism from Decision 6's reconcile trigger, not a repeat of it: this one is Flux-level, acting
  on the Kustomization and routed through Flux's own suspend semantics; Decision 6 patches the
  stalled object itself, watched directly by whatever controller set its finalizer, with no Flux
  involvement and no suspend semantics to interact with.
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
  `reverseTopologicalKustomizations`, `triggerReconcile`,
  `fluxReconcileAnnotationGroups`, `dependencyClosure`, `blockDependencyClosure`,
  `handleKustomizationDisappeared`
- `github.com/fluxcd/helm-controller/api v1.6.4`, package `v2`: `ResourceInventory` /
  `ResourceRef.ID` (`inventory_types.go`), `Uninstall` with `Timeout` / `KeepHistory` /
  `DisableWait` / `DeletionPropagation` and `GetTimeout` (`helmrelease_types.go:1215-1262`),
  `HelmReleaseStatus.Inventory` / `History` / `StorageNamespace`, `Snapshot.Status`
  (`snapshot_types.go`)
- `pkg/provisioner/kubernetes/client/client.go`: `PatchResource`, `ListResourcesByLabel` (48). No
  discovery or metadata-list method exists yet; Decision 4's cluster-wide sweep needs one added.
- `k8s.io/client-go/discovery`: `ServerPreferredResources`, the API the cluster-wide sweep enumerates
  resource types with
- `k8s.io/client-go/metadata`: `NewForConfig`, `Getter.List` with `PartialObjectMetadataList`, the
  metadata-only list the sweep uses instead of fetching full objects
- `pkg/provisioner/terraform/stack.go`: `execTerraformDestroyWithRetry` (1195), the retry
  pattern Alternatives rejects for this path
- `pkg/composer/blueprint/composer.go`: `applyCrdLayerBarrier` (960)
- `core`: 27 of 31 install tiers HelmRelease-wrapped, 12 of 14 resources tiers raw;
  `kustomize/lb/install/aws-lb-controller/helm-release.yaml:20-22` the sole uninstall config;
  `terraform/gitops/flux/variables.tf` (Flux 2.9.5);
  `kustomize/provisioning/resources/crossplane/database-credentials/composition.yaml` and
  `xrd.yaml` — the `DatabaseCredentials` composition that produced the stuck `ProviderConfig`
- `github.com/crossplane/crossplane-runtime`, package `pkg/reconciler/providerconfig`:
  `Reconciler.Reconcile`, `releaseUsage`, `deleteOrphanedUsage` (`reconciler.go:156-353`) — the
  deterministic finalizer-removal logic Decision 6 is grounded in
- `github.com/crossplane-contrib/provider-sql`,
  `pkg/controller/namespaced/postgresql/config/config.go:41-47`: the controller wiring confirming
  no predicate filters a metadata-only update to `ProviderConfig`
- `github.com/fluxcd/pkg/apis/meta`: `ReconcileRequestAnnotation`
  (`reconcile.fluxcd.io/requestedAt`), Flux's own documented, predicate-matched reconcile trigger
  for its own object kinds — the guaranteed case Decision 6 distinguishes from the best-effort one
