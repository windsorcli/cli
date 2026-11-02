# ADR 0010 — Artifact inventory: list what a context pulls and where each artifact goes

- Status: Proposed. Supersedes the single HelmRepository patch proposed in cli#3376.
- Date: 2026-10-28
- Deciders: Ryan VanGundy
- Surfaced while designing core's chart registry
  ([core ADR 0020](https://github.com/windsorcli/core/blob/main/docs/adr/0020-helm-chart-registry.md)),
  which needs a list of every chart a context installs and found that only the CLI can produce
  it for one context of an arbitrary blueprint.

## Context

An air-gapped site, or one that reviews every artifact before a cluster pulls it, copies each
chart and image a context uses into registries it controls. The copy itself is solved.
[Flux Mirror](https://fluxcd.io/flux/cli-plugins/flux-mirror/) (`flux mirror sync`, a Flux CLI
plugin at v0.10.0 and marked preview) republishes an HTTP chart as OCI at
`<destination>/<chart name>:<version>` and copies an OCI chart or image byte for byte, with its
referrers. It reads an explicit list of sources and destinations and discovers nothing itself.

The list is the missing piece, and only the CLI can build it for one context. A blueprint's
facets decide per context which Kustomizations, components, and Terraform components exist, and
an overlay blueprint can patch another source's Kustomizations. A list maintained by hand inside
a blueprint covers everything that blueprint could install and nothing an overlay adds. Core
keeps one at `flux-mirror.yaml` until the CLI replaces it.

### What the CLI has today

- `windsor bundle` writes `artifact-manifest.yaml` into every blueprint artifact
  (`pkg/composer/artifact`). It scans Renovate annotations in `kustomize/` and `terraform/` and
  records each artifact's type, transport, reference, version, digest, and declaring file. For
  core on 2026-10-28 it lists 105 `docker`, 19 `helm`, and 12 `github-release` entries. It covers
  the whole blueprint rather than one context, has no destinations, and has no consumer. An OCI
  chart is recorded as `docker`, because its annotation uses Renovate's `docker` datasource, so
  the kyverno chart and the kyverno controller image get the same type.
- `windsor plan` renders a new Kustomization locally with `kustomize build`, wrapping its path
  and components in a synthetic kustomization (`pkg/provisioner/flux/stack.go`,
  `runFromScratch`). It applies neither the Kustomization's inline patches nor its postBuild
  substitutions.
- Composition merges Kustomizations of the same name across facets and sources
  (`MergeKustomizationFields` in `api/v1alpha1/blueprint_merge.go`). Components union and
  resolve against the Kustomization's one source. An overlay's `source` replaces that source.
  Inline patches append, and a context's `patches/<name>/` directory adds more.

### Charts and images go to different places

Core ADR 0020 puts every chart at `<chart registry>/<chart name>:<chart version>`, the reference
`helm push` writes, and swaps the Flux source objects to point there. Images use containerd's
per-host mirrors (`docker.registries`), which keep the upstream path and leave manifests alone. A
mirror maps a host, not a repository, so an image cannot move into the flat chart layout. The
two kinds need different destinations from the same list.

### cli#3376

cli#3376 proposed one composer-generated patch that turns every HelmRepository into `type: oci`
pointing at a single registry. Core ADR 0020 rejected that approach: the `oci` HelmRepository
type is in maintenance mode and carries no status, and a patch cannot change an HTTP
HelmRepository into the OCIRepository the chart registry needs. Core now does the swap in its own
manifests, gated on its `chart_registry` substitution, so the CLI has no chart rewrite to do.

## Decision

### 1. `windsor show artifacts` lists what one context pulls

The command composes the current context and prints every external artifact it pulls: kind
(`chart`, `image`, or `blueprint`), upstream reference, version, digest when pinned, destination
when one is configured, and the file that declares it. It copies nothing. Copying stays with
Flux Mirror or any tool that reads the list.

```sh
windsor show artifacts --charts-to registry.example.com/windsor -o flux-mirror | flux mirror sync -
```

`-o yaml` (the default) prints the `artifact-manifest.yaml` schema with a `destination` field
added. `-o flux-mirror` prints a Flux Mirror `v1beta1` config. `-o list` prints one
`destination<TAB>upstream` line per artifact for other tools. `--all` prints the blueprint-wide
set from each source's `artifact-manifest.yaml`, regardless of context, for a publisher
building a full set.

The command is not named `mirror`. It covers charts, which go to a chart registry rather than a
mirror, and it never copies.

### 2. Flux artifacts come from the rendered Kustomizations

For each Kustomization the context applies, the CLI runs `flux build kustomization` with
`--kustomization-file` and `--dry-run`. The flux CLI builds with `fluxcd/pkg/kustomize`, the
package kustomize-controller uses. The output therefore has the path, components, inline
patches, and postBuild substitutions applied, including `${var:=default}`. Dry-run mode reads no
`substituteFrom` objects from a cluster. The CLI writes the context's substitution values into
`postBuild.substitute` in the Kustomization file it passes to the flux CLI. Rendering after
patches means an overlay's replacement chart or image appears and the one it replaced does not.

The CLI reads three things from the rendered output:

- Every chart source: each `OCIRepository` carrying the Helm chart layer media type, each
  HelmRelease whose `spec.chart` names a HelmRepository (chart, version, and repository URL),
  and each chart-producing GitRepository.
- Every image in a pod template: `containers`, `initContainers`, and `ephemeralContainers` in
  Pods, Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs, and CronJobs.
- Every HelmRelease, which the CLI renders in turn.

For each HelmRelease, the CLI renders the chart the way helm-controller would:

1. It pulls the chart that the rendered source object names, and caches it.
2. It merges values in helm-controller's order: chart defaults, then `valuesFrom`, then
   `spec.values`. The `valuesFrom` ConfigMaps and Secrets come from the same render.
   `ChartValuesFromReferences` in `fluxcd/pkg/chartutil` does the merge against a fake client
   that holds the rendered objects.
3. It renders the chart client-side with the Helm SDK, hooks included. The CLI uses Helm v4,
   the major version helm-controller v1.6 uses, so the output matches the cluster's.
4. It applies `spec.postRenderers`, because a Kustomize post-renderer can change an image.
5. It reads images from the pod templates in the result, and recurses into any HelmRelease or
   FluxInstance the chart renders.

`.Capabilities.KubeVersion` is the context's Kubernetes version. `.Capabilities.APIVersions`
holds the built-in APIs plus every CRD the context's render installs. A chart that gates a
ServiceMonitor on `monitoring.coreos.com/v1` then renders as it does in the cluster. A `lookup`
call returns nothing, as it does in a Helm dry run.

The CLI renders with the context's values as they are, and knows no blueprint's value names.
A context that already points its chart sources at a chart registry renders the registry's URLs
in place of each vendor's upstream. To list the upstreams, the operator overrides the
blueprint's registry value for one run with `--set`, the flag `windsor up` takes. For core, that
is `--set charts.registry=`. The command applies `--set` to that run only and writes nothing to
the context. When a rendered chart source is already under `--charts-to`, the command names it
on stderr and leaves it out of `-o flux-mirror`, so Flux Mirror never copies a registry onto
itself.

A Kustomization whose substitutions include a value that does not exist before apply, such as a
Terraform output, renders with that value left in place. The command fails only when such a
value lands in a chart source's URL, tag, or digest, or in an image reference.

### 3. Terraform artifacts come from static evaluation of each component

For each Terraform component the context enables, the CLI parses the module with `hcl/v2` and
follows local `module` sources. It evaluates each resource block below with `var.*` set from
the component's Windsor inputs and the variable defaults, and with `local.*` resolved in the
same pass. A reference to a resource attribute or a module output evaluates as unknown.

- A `helm_release` gives its repository, chart, version, `values`, and `set` entries. The CLI
  renders the chart as in Decision 2.
- A `kubernetes_*` workload resource gives the images in its pod template.
- A `kubernetes_manifest` gives its object, which the CLI reads like rendered Flux output.

These resources produce charts or Kubernetes objects, so the CLI reads them the same way for any
blueprint. The CLI reads no other resource type, because Terraform has no common attribute that
marks an image. An image stays in the file that uses it, and the blueprint declares no second
copy.

The command fails only when an unknown value lands in a chart's repository, name, or version,
or in an image reference. In core, this finds cilium, flux-operator, and flux-instance from
`var.chart_repository` and the version variables.

### 4. The object that declares an artifact sets its kind, and extractors cover runtime images

An artifact found through a chart source or a `helm_release` is a chart. An artifact found in a
pod template is an image. The CLI reads no manifest to classify either.

Some images appear in no manifest, because an operator chooses them at runtime. The CLI has a
built-in extractor for each kind whose images follow from its spec. The first is FluxInstance,
whose operator derives each controller image from `distribution` and `components`. The
extractor runs `flux-operator build instance` on the rendered FluxInstance and reads the images
from its output. The flux-operator CLI resolves a `distribution.version` range as the operator
does.

The blueprint schema does not change. The CLI does not read Renovate annotations.

### 5. Destinations follow each kind's layout

- A chart goes to `<--charts-to>/<chart name>:<chart version>`, the layout `helm push` and Flux
  Mirror both write. The CLI does not read a chart registry from blueprint values, since the
  key's name belongs to the blueprint's schema. Core passes its `charts.registry` value.
- An image goes to the mirror the context's `docker.registries` configures for its upstream
  host, under its upstream path and tag. An image whose host has no mirror is listed with no
  destination. `-o flux-mirror` leaves it out and names it on stderr.
- A blueprint artifact (an `oci://` source) is listed with no destination. Relocating it already
  works through the source's `url` in the context's `blueprint.yaml`.

### 6. Flux Mirror output selects exact tags

`-o flux-mirror` writes HTTP charts as `charts` entries with an exact `version`. It writes OCI
charts and images as `artifacts` entries whose `selector.regex.pattern` is `^<tag>$` with the
tag escaped, and sets `includeReferrers: true` so signatures and SBOMs travel with each artifact.
An exact tag pattern avoids Flux Mirror's semver selector, which treats `v0.25.0` and `0.25.0` as
equal and copies whichever it sorts first. The output has no `hosts` section. Flux Mirror reads
credentials from the Docker config, and a site that needs `hosts` merges its own.

### 7. Overlays replace charts through inline patches

An overlay blueprint replaces another source's chart, for example Chainguard's grafana in place
of core's, by appending inline patches to the core Kustomization of the same name. The patches
change the source object's URL, digest, and credentials, and the HelmRelease's values. That works
across sources today, because patches carry their content.

The CLI does not add cross-source components. A Kustomization resolves every component against
its one source, and Flux builds it from one artifact, so a component from the overlay's source
cannot join a core Kustomization. Setting `source` on the overlay's entry would move the whole
Kustomization, core's paths included.

## Consequences

Core deletes `flux-mirror.yaml` and its comparison check once `-o flux-mirror` ships. Any
blueprint that swaps its own chart sources gets the same air-gap list without writing one.

The image list includes chart defaults and post-renderer changes, because the CLI renders every
chart. It misses an image that an operator chooses at runtime when no built-in extractor
covers the operator's kind. It also misses an image that only a Terraform resource outside
Decision 3 names. Core's Talos node image is one, and a site copies it without this list.

`windsor show artifacts` needs the flux CLI at the minimum version `windsor plan` checks. A
context with a FluxInstance also needs the flux-operator CLI. The command needs network access
to pull each chart, through the Docker config for OCI charts. The CLI caches pulled charts. The
CLI takes on the Helm v4 SDK and `fluxcd/pkg/chartutil` as dependencies.

`artifact-manifest.yaml` keeps its place in the bundle as the blueprint-wide set from Renovate
annotations. `windsor show artifacts` does not read it.

## Alternatives considered

### A Kustomization render inside the CLI

The CLI would extend `windsor plan`'s `kustomize build` with inline patches and its own
envsubst. Rejected: each substitution rule the CLI gets wrong gives a wrong list with no error.
`flux build kustomization` runs the controller's own package, and `windsor plan` already needs
the flux CLI.

### `helm template` in place of the Helm SDK

The CLI would run `helm template` with the merged values, `--kube-version`, and one
`--api-versions` flag per CRD. That keeps the Helm SDK out of the binary. Rejected: the CLI
would then merge `valuesFrom` itself, with `targetPath`, `optional`, and helm-controller's merge
order. That merge is the step most likely to drift from the cluster, and `fluxcd/pkg/chartutil`
exists to do it.

### A declared `images` list in the blueprint

Each Kustomization and Terraform component would carry an `images` list for images that no
rendered object names. Rejected: an image belongs in the file that uses it. A second copy in
the blueprint can drift from the image the cluster runs, as core's `talos_node_image` pin can.

### Flux Mirror config as the source of truth

A blueprint would declare `flux-mirror.yaml`, and the CLI would generate the Flux source objects
from it. Rejected: the config lacks the namespace, release, fallback HelmRepository, and
registry swap each chart needs. `kustomize build` and the facet tests would stop working
without the CLI. It cannot vary per context, and it would tie every blueprint's manifests to a
preview plugin's config API.

### Images from Renovate annotations

The CLI would scan Renovate annotations in the files each active Kustomization and Terraform
component reaches. Rejected: an annotation records what an author pinned, not what the context
runs. It misses chart defaults and post-renderer changes. An overlay that replaces an image by
patching values leaves the old annotation in the base file, so both images appear.

### Terraform plan JSON

The CLI would run `terraform plan` and read `helm_release` values from `terraform show -json`.
Rejected: a plan needs init, providers, backend credentials, and upstream outputs. An air-gap
list must exist before the first bootstrap, when every component is new and `windsor plan` runs
no plan for it.

### A `windsor mirror` command that copies

The CLI would run the copy itself. Rejected for now: Flux Mirror already handles registry
authentication, workload identity, signature checks before copying, drift reporting, and
re-runs. The CLI's job is the list.

### One composer patch over every HelmRepository (cli#3376)

Rejected, for the reasons in Context.

## References

- [Flux Mirror](https://fluxcd.io/blog/2026/08/flux-mirror/)
- [Flux Mirror config](https://fluxcd.io/flux/cli-plugins/flux-mirror/config/)
- [core ADR 0020: Helm charts from a single chart registry](https://github.com/windsorcli/core/blob/main/docs/adr/0020-helm-chart-registry.md)
- [How to use Chainguard Helm charts](https://edu.chainguard.dev/chainguard/containers/how-to-use/use-chainguard-helm-charts/)
