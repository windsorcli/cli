# ADR 0010 — Artifact inventory: list what a context pulls and where each artifact goes

- Status: Proposed. Supersedes the single HelmRepository patch proposed in cli#3376.
- Date: 2026-10-28; amended 2026-11-04 (Decisions 3 and 8)
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

`-o yaml` (the default) prints one YAML entry per artifact with a `destination` field. `-o flux-mirror` prints a Flux Mirror `v1beta1` config. `-o list` prints one
`destination<TAB>upstream` line per artifact for other tools. `--all` prints the blueprint-wide
set from each source's manifest (Decision 8), regardless of context, for a publisher
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
blueprint's registry value with `--set`, in the `key=value` format that `windsor up` takes. For
core, that is `--set charts.registry=`. `windsor up` saves its `--set` values to the context.
This command saves nothing, because a `show` command does not change the context. When a rendered chart source is already under `--charts-to`, the command names it
on stderr and leaves it out of `-o flux-mirror`, so Flux Mirror never copies a registry onto
itself.

Some substitution values do not exist before apply, such as a Terraform output. `flux build`
renders a missing value as empty, so the CLI writes a marker value into `postBuild.substitute`
for each one. The command fails only when a marker lands in a chart source's URL, tag, or
digest, or in an image reference. A marker anywhere else does not change the list.

### 3. Terraform artifacts come from a declared subset of each component

For each Terraform component the context enables, the lister parses the `.tf` files in the
component's directory with `hcl/v2`. It does not evaluate the Terraform language. It reads a
value only when the value is in the readable subset:

- A literal.
- `var.<name>`, set from the component's Windsor input or, when no input is set, from the
  variable's default.
- `local.<name>`, when the local's expression is in the readable subset.
- An object, a tuple, or a string template whose parts are all in the readable subset.
- `yamlencode(...)` or `jsonencode(...)` of a value in the readable subset.

Every other expression is unreadable: a function call outside the list, a conditional, a `for`
expression, a resource attribute, a module output, and a value under `count` or `for_each`. The
lister reads three resource types:

- A `helm_release` gives its `repository`, `chart`, and `version`. Its `values` documents pass to
  the chart render of Decision 2. A key whose value is unreadable is left out, so the chart's
  default applies to that key.
- A `kubernetes_*` workload resource gives the `image` of each container in its pod template.
- A `kubernetes_manifest` gives its `manifest` object, which the lister reads like rendered Flux
  output.

The command fails when a chart's `repository`, `chart`, or `version`, or a container's `image`,
is unreadable. The message names the file and the attribute, and tells the author to set the
value from a variable. The lister does not follow `module` blocks. A module that installs a
chart puts that `helm_release` in the component's own directory.

The subset is a contract between the CLI and a blueprint author. An author keeps each artifact
reference readable, and the CLI does not have to know the rest of the Terraform language. The
version stays in the file that uses it, under its Renovate comment.

In core, the subset gives:

- cilium, flux-operator, and flux-instance from `var.chart_repository` and the version variables.
- The flux-instance `distribution.version` from `var.flux_version`, so the FluxInstance extractor
  of Decision 4 resolves the controller images that the context runs.
- `alpine/k8s` from `var.ready_gate_image` in `kubernetes_job_v1.flux_ready_gate`.

Three values are unreadable in core, and none of them changes the list:

- The cilium `values` document is a `merge` of conditionals. The chart renders with its defaults,
  which pin the same image tags. The repositories are the upstream ones, which a mirror needs.
- The flux-instance `components` comes from a `concat` of conditionals. The chart's default
  component set applies.
- The release namespace refers to a resource attribute. The namespace does not change an image.

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

### 8. `windsor bundle` publishes the manifest as a CycloneDX BOM

`windsor bundle` writes `artifact-manifest.cdx.json` at the root of the blueprint artifact. The
file is a [CycloneDX](https://cyclonedx.org) 1.6 BOM, so SBOM tools read it without the CLI.
`windsor bundle` builds it by running the lister of Decisions 2 and 3 at publish time. It no
longer scans Renovate annotations. Renovate keeps its comments to bump versions, and the CLI does
not parse them. A consumer reads the BOM and needs neither the flux CLI nor network access.

The BOM maps the lister's entries as follows:

- An image is a `container` component. A chart and a Terraform component are `application`
  components.
- An OCI image or chart has the package URL `pkg:oci/<name>@<digest>?repository_url=<path>&tag=<tag>`.
  An HTTP chart has `pkg:generic/<name>@<version>?download_url=<tarball>`, because the package URL
  specification has no chart type.
- The digest is in `hashes` as SHA-256. An HTTP chart has the hash of its tarball.
- Each Kustomization and Terraform component is an `application` component with a `bom-ref`. Its
  `dependsOn` entry in `dependencies` lists the artifacts that it declares.
- A component that only some facets activate has `scope: optional`.
- A reference that stays deferred has a `windsor:deferred` property and no digest.

At publish time, `windsor bundle` does these steps:

1. List the render profiles: one for each `platform` value that a facet `when` condition names,
   and one with no platform.
2. For each profile, compose the blueprint with the blueprint's default and test values, and run
   the lister. A deferred value stays deferred.
3. Resolve a digest for each entry. An OCI image or chart gets the manifest digest from its
   registry. An HTTP chart gets the sha256 of the tarball that the render pulled.
4. Merge the profiles. The key is the package URL. Union the `dependsOn` edges.
5. Write the BOM with sorted components. When the registry supports the OCI 1.1 referrers API,
   also attach the BOM to the artifact with `artifactType: application/vnd.cyclonedx+json`.

At consume time, `windsor show artifacts` does these steps:

1. Read the BOM of each source.
2. Compose the context's blueprint and take the names of its active Kustomizations and Terraform
   components.
3. Keep each artifact that an active component depends on.
4. Compute destinations as in Decision 5, and print the result as in Decision 1.

`--render` keeps the path of Decision 2 for a user who wants to check the BOM against the
context. `--all` prints every artifact of the BOMs without the filter. A source that has only the
older `artifact-manifest.yaml` cannot be filtered, so the reader renders instead and prints a
note. `-o cyclonedx` prints the filtered result as a BOM. The SBOM of a deployment is that
result, and the SBOM of a release is the full file.

Three questions stay open:

- Whether a boolean facet toggle needs its own render profile. The `dependsOn` filter handles the
  toggle at consume time. Rendering only by platform covers core today.
- How the BOM carries an artifact that no rendered object names, such as core's Talos node
  image. Options are a Terraform extractor or a short declared list.
- Whether `windsor bundle --no-resolve` writes a BOM without digests and registry calls. It
  would still need the flux CLI.

## Consequences

Core deletes `flux-mirror.yaml` and its comparison check once `-o flux-mirror` ships. Any
blueprint that swaps its own chart sources gets the same air-gap list without writing one.

The image list includes chart defaults and post-renderer changes, because the CLI renders every
chart. It misses an image that an operator chooses at runtime when no built-in extractor
covers the operator's kind. It also misses an image that only a Terraform resource that
Decision 3 does not read. Core's Talos node image is one, and a site copies it without this list.

`windsor show artifacts` needs the flux CLI at the minimum version `windsor plan` checks. A
context with a FluxInstance also needs the flux-operator CLI. The command needs network access
to pull each chart, through the Docker config for OCI charts. The CLI caches pulled charts. The
CLI takes on the Helm v4 SDK and `fluxcd/pkg/chartutil` as dependencies.

`windsor bundle` needs the flux CLI, the flux-operator CLI when a render produces a FluxInstance,
network access, and registry credentials. The publisher carries that cost once, and a consumer
carries none. A publish step that runs offline cannot produce a full manifest.

`artifact-manifest.yaml` is replaced by `artifact-manifest.cdx.json`, which the lister generates.
The annotation scan in `pkg/composer/artifact` is removed. The CLI takes on a CycloneDX library
as a dependency.

The Terraform read is small enough to test in full, and a new core module cannot make it grow.
A blueprint that computes an artifact reference with a function gets an error, not a wrong list.
The author moves that reference to a variable.

An artifact under `count` or `for_each`, or in a nested module, is not listed. An unreadable
`values` key can hide an image that only that key selects. The chart default then decides the
image, and the list can differ from what the cluster runs. The air-gap acceptance test catches
that case.

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

### A custom manifest schema

The CLI would keep its own `artifact-manifest.yaml` and add `digest`, `usedBy`, and `platforms`.
Rejected: no SBOM tool reads it, and CycloneDX already has components, hashes, a dependency
graph, and a scope.

### Keep the annotation scan and check it against a render

`windsor bundle` would keep scanning annotations, and CI would render a test context and fail
when the manifest misses an artifact. Rejected: it keeps two mechanisms that must agree, and the
manifest stays without digests and without chart defaults.

### A manifest that authors write by hand

Each blueprint would commit its `artifact-manifest.yaml`. Rejected for the reason in "A declared
`images` list in the blueprint": a second copy of a pin drifts from the file that uses it.

### Static evaluation of the Terraform language

The lister would evaluate each module with Terraform's function library, `count`, `for_each`,
and local modules. Rejected: no Go library exports
Terraform's evaluator. Terraform and OpenTofu keep it under `internal/`, and the copy in tflint
is under the Business Source License. The CLI would maintain its own copy of the language
semantics. That copy grows with each module a blueprint adds, and a semantic gap gives a wrong
list with no error.

### Renovate annotations for Terraform only

The lister would take Terraform artifacts from the Renovate annotations in each enabled
component's directory, and render everything else. Rejected: Decision 8 removes the annotation
scan, so the BOM has one mechanism. An annotation also records the default, not a component
input that overrides it.

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
