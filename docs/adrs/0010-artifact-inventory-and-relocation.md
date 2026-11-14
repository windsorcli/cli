# ADR 0010 — Artifact inventory: list what a context pulls and where each artifact goes

- Status: Proposed. Supersedes the single HelmRepository patch proposed in cli#3376.
- Date: 2026-10-28; revised 2026-11-05 (artifact tags replace rendering); revised 2026-11-12
  (blueprint sources follow the image mirrors, and `registries` replaces `docker.registries`);
  revised 2026-11-13 (the CycloneDX BOM ships in v0.10.0 and lists the artifacts of each `oci://`
  source, and `windsor bundle` reads the digest of each tag from its registry)
- Deciders: Ryan VanGundy
- Surfaced while designing core's chart registry
  ([core ADR 0020](https://github.com/windsorcli/core/blob/main/docs/adr/0020-helm-chart-registry.md)),
  which needs a list of every chart a context installs.
- The cross-repo contract is sdlc ADR 0001 (chart registry contract), sections 3 to 6.

## Context

An air-gapped site, or one that reviews every artifact before a cluster pulls it, copies each
chart and image a context uses into registries it controls. The copy itself is solved.
[Flux Mirror](https://fluxcd.io/flux/cli-plugins/flux-mirror/) (`flux mirror sync`, a Flux CLI
plugin at v0.10.0 and marked preview) republishes an HTTP chart as OCI at
`<destination>/<chart name>:<version>` and copies an OCI chart or image byte for byte, with its
referrers. It reads an explicit list of sources and destinations and discovers nothing itself.

The list is the missing piece. The CLI can build it in two ways:

- It can find artifacts by computation: render each Kustomization and HelmRelease, evaluate
  Terraform defaults, and run an extractor for each operator kind. Each mechanism is large and
  none is complete. Operators select images at runtime, Terraform modules compute values, and the
  Talos node image appears in no manifest.
- It can read a comment tag that the blueprint author puts on each artifact reference. CI in
  registry mode finds a reference that has no tag.

This ADR uses the tags. The CLI renders nothing, evaluates no Terraform, and runs no extractor.

### What the CLI has today

Before v0.10.0, `windsor bundle` wrote `artifact-manifest.yaml` into every blueprint artifact
(`pkg/composer/artifact`). It scanned Renovate comments in `kustomize/` and `terraform/` and
recorded each artifact's type, transport, reference, version, digest, and declaring file. The
scanner worked offline and needed no rendering. This ADR keeps that approach for the `artifact`
tag, and Decision 7 replaces the file.

### Charts and images go to different places

Core ADR 0020 puts every chart at `<chart registry>/<chart name>:<chart version>`, the reference
`helm push` writes, and swaps the Flux source objects to point there. Images use containerd's
per-host mirrors (`registries`, Decision 9), which keep the upstream path and leave manifests
alone. A mirror maps a host, not a repository, so an image cannot move into the flat chart
layout. The two kinds need different destinations from the same list.

### cli#3376

cli#3376 proposed one composer-generated patch that turns every HelmRepository into `type: oci`
pointing at a single registry. Core ADR 0020 rejected that approach: the `oci` HelmRepository
type is in maintenance mode and carries no status, and a patch cannot change an HTTP
HelmRepository into the OCIRepository the chart registry needs. Core does the swap in its own
manifests, so the CLI has no chart rewrite to do.

## Decision

### 1. `windsor show artifacts` lists the tagged artifacts of a context

The command prints every artifact that a tag names in the files that the current context uses
(Decision 3): kind (`chart` or `image`), upstream reference, version, destination when one is
configured, and the file that declares it. It also lists each `oci://` blueprint source as kind
`blueprint` (Decision 8). It copies nothing. Copying stays with Flux Mirror or
any tool that reads the list.

```sh
windsor show artifacts --charts-to registry.example.com/windsor -o flux-mirror | flux mirror sync -
```

`-o yaml` (the default) prints one YAML entry per artifact with a `destination` field.
`-o flux-mirror` prints a Flux Mirror `v1beta2` config. The config needs Flux Mirror v0.16.0 or
later. `-o list` prints one `destination<TAB>upstream` line per artifact for other tools.
`-o cyclonedx` prints the CycloneDX BOM of Decision 7.

The command is not named `mirror`. It covers charts, which go to a chart registry rather than a
mirror, and it never copies.

### 2. An `artifact` tag names each artifact

The tag is a comment, `# artifact: type=<image|chart> [name=<name>] [repo=<chart repository URL>] [referrers=<true|false>]`,
in YAML or HCL, with `#` or `//`. A block comment is not a tag. The syntax is in sdlc ADR 0001,
section 3.

```yaml
image: ghcr.io/fluxcd/source-controller:v1.6.0  # artifact: type=image

# artifact: type=chart name=cert-manager repo=https://charts.jetstack.io
version: v1.17.0
```

- A tag on a comment line of its own binds to the next line that is not blank and not a comment.
  A tag at the end of a line binds to that line. A tag never binds to a line above it.
- The value is the last scalar on the bound line, without quotes. An `@sha256` suffix gives a
  digest, and the list keeps it.
- An image `name` is the full repository with the registry. It MAY be omitted when the value is a
  full reference. A chart `name` is the `Chart.yaml` name, and `repo` is an `https://` Helm
  repository or an `oci://` path.
- `referrers` applies only to an image and defaults to `true`. With `referrers=false`, the Flux
  Mirror config sets `includeReferrers: false` for that image. Use it for a registry that fails the
  referrers check. When two tags name the same image, `false` wins.
- One tag names one artifact.
- A tag with no value, an unknown `type`, no `name` and no full reference, a chart with no `repo`,
  a `referrers` value that is not `true` or `false`, or a chart with `referrers` is an error. `windsor show artifacts` and `windsor bundle` both fail on it and name the
  file and line.
- The tag scanner of `pkg/composer/artifact` reads the tag. It replaces the Renovate scan.
- Renovate reads the same tag through a regex `customManagers` entry, so one tag gives the list
  and the version updates.

### 3. The scan covers the files that the composed blueprint uses

The CLI scans the blueprint's own source in the project root and the OCI cache of each `oci://`
source, nested sources included. For a typical context, that is the blueprint and `core`. In each
source, it reads the `.yaml`, `.yml`, `.tf`, and `.tfvars` files that the composed blueprint of the
context uses. The composed blueprint already has the result of each facet `when` condition, so the list
follows the context.

The scan renders nothing, evaluates no Terraform, and reads no operator spec. It reads the plain
paths in the blueprint, the fields of `kustomization.yaml` files that name other files, and the
`source` of Terraform `module` blocks:

- For each kustomization and each `flux:` tier, the scan starts at `kustomize/<path>` in the source
  that the entry names. It reads that directory and each selected component directory. It does not
  enter a subdirectory that has its own kustomization file, unless an entry reaches it. It follows
  local entries of `resources`, `components`, `bases`, patch paths, `transformers`, `generators`,
  `validators`, and `replacements`, also `../` entries. It reads patch files in a reached directory.
  Flux generates a kustomization for a directory that has none, so the scan reads all of that
  directory.
- For each Terraform component, the scan starts at `terraform/<path>` in its source, or at the
  `pathPrefix` of that source. It reads that directory and each local module. It does not enter a
  subdirectory that has `.tf` files, unless a `module` block reaches it.
- An entry with `enabled: false` adds no files.
- The scan always reads all of `contexts/` in the project, all of `_template/` in a bundled source,
  and all of `kustomize/crds/` in each source. These directories hold facets, values files, and the
  CRD catalog. A facet can install an image that a catalog entry tags, with no kustomization.
- The list holds a tag in a facet file only when the composed blueprint uses the tagged value,
  directly or through a `config` block. The CLI reads this from the composition trace of
  `windsor explain`. A facet tag MUST be on a value that the composed blueprint uses.

Under-including causes a failed pull, and over-including costs copy time. Thus a name that the CLI
cannot resolve MUST NOT remove a tag silently. A path or a component name with a `${` expression
widens the scan to the whole directory and gives a note on stderr. A name that the source does not
hold, and a file that the scan cannot parse, give a note on stderr. The composer fails on a deferred
kustomization path or component name, so an unresolved name reaches the scan only as literal text
in a blueprint.

The same rule applies to a facet tag. The trace walks the raw expressions of each input and
substitution of an enabled entry, each `flux:` tier, and each blueprint file, and it follows each
reference through the `config` blocks that are active in the last config round. A literal value
that another active block overrides is not used. When the trace
cannot resolve a facet tag, the CLI MUST list it and give a note on stderr. A tag on a list item
below a key, an expression that does not parse, and a `jsonnet()` call are not resolved, because
the walk cannot name the value that they read. A composition error stops the command, so the CLI
never filters from a partial trace.

`windsor show artifacts --all` reads whole sources: the `kustomize/`, `terraform/`, and `contexts/`
directories in the project, and the `kustomize/`, `terraform/`, and `_template/` directories in a
bundled source. `windsor bundle` also reads whole sources, because a bundle is not specific to one
context.

The earlier whole-source scan was too large for a site to copy. A CI run on the default core
context copied 170 artifacts in 37.5 minutes, including Elastic images that the context does not
enable.

A site MUST sync with the same values and host architecture as the deployment, or sync the `--all`
output. Composition fills `workstation.arch` from the host, so an arm64 host can select other images
than an amd64 deployment. A site MUST sync again before it enables a component that the context did
not use.

### 4. Destinations follow each kind's layout

- A chart goes to `<--charts-to>/<chart name>:<chart version>`, the layout `helm push` and Flux
  Mirror both write. The CLI does not read a chart registry from blueprint values, since the
  key's name belongs to the blueprint's schema. Core passes its `charts.registry` value.
- An image goes to the mirror the context's `registries` configures for its upstream
  host, under its upstream path and tag. An entry is a mirror only when it sets `endpoint`.
  The key `*` applies to each host with no entry. The command fails when two
  different images have the same destination.
- A blueprint source goes to the same mirror as an image from its host (Decision 8).
- An image with no mirror is listed with no destination. `-o flux-mirror` leaves it out and
  names it on stderr.
- A chart whose tag names a repository under `--charts-to` stays out of `-o flux-mirror`, and the
  command names it on stderr, so Flux Mirror never copies a registry onto itself.

### 5. The CLI writes no artifact list into the cluster

The registry contents are the list, as sdlc ADR 0001 section 6 states. A relocated context
enforces its own registries, and the CLI applies no ConfigMap for a policy.

### 6. Flux Mirror output selects exact tags

`-o flux-mirror` writes HTTP charts as `charts` entries with an exact `version`. It writes OCI
charts and images as `artifacts` entries whose `selector.tags` lists the one tag. It sets
`includeReferrers: true` so signatures and SBOMs travel with each artifact. With a tags selector,
Flux Mirror does not list the tags of the source repository. That listing is slow on a repository
with many tags, such as `docker.elastic.co`. No semver match runs either, so Flux Mirror never
copies a sibling tag such as `v0.25.0` for `0.25.0`. The output has no `hosts` section. Flux
Mirror reads credentials from the Docker config, and a site that needs `hosts` merges its own.

### 7. `windsor bundle` publishes the same list as a CycloneDX BOM

This decision ships in v0.10.0. `windsor bundle` writes `artifact-manifest.cdx.json` at the root
of the blueprint artifact. The file is a [CycloneDX](https://cyclonedx.org) 1.6 BOM, so SBOM tools
read it without the CLI. `windsor bundle` writes no `artifact-manifest.yaml`. It builds the BOM
from the tag scan of Decision 2 over the whole bundled source, the scope of `--all` in Decision 3.
Thus the BOM and `windsor show artifacts` read one source. `windsor show artifacts -o cyclonedx`
prints the same BOM for the scope that the command reads.

- An image is a `container` component, a chart is an `application` component, and a blueprint
  source is a `data` component.
- An OCI image, chart or blueprint has the package URL
  `pkg:oci/<name>?repository_url=<path>&tag=<tag>`, with the digest as the version when the
  artifact has one. An HTTP chart has `pkg:generic/<name>@<version>?download_url=<repository>`,
  because the package URL specification has no chart type. The package URL is also the `bom-ref`,
  so the BOM has one component for each package URL. A chart wins over an image tag with the same
  reference.
- A digest, when the artifact has one, is in `hashes` as SHA-256.
- `windsor bundle` gives a digest to each chart and image tag with an exact version and no
  `@sha256` suffix. It reads the digest of the manifest or index of an OCI artifact from its
  registry. It reads the digest of an HTTP chart from the `index.yaml` of its repository, or
  hashes the chart archive when the index gives none. A digest that the tag line gives wins, and
  the CLI reads nothing for it. A failed lookup stops the bundle and names the artifact.
- The metadata of the bundle BOM names the blueprint and its version, with the blueprint name as
  its `bom-ref`. The BOM has no serial number and no timestamp, so the same source gives the same
  file.

`windsor bundle` has no render step. It fails on a malformed tag. The BOM has no edge from a
Kustomization to its artifacts, because the tag scan does not know which component a tag serves. A
bundle holds `contexts/_template` and not the other context directories, so its BOM leaves out a
tag in a context directory that `--all` reads in the project.

A bundle BOM lists each artifact that the bundle can pull, also the artifacts of its `oci://`
sources. A site that copies a blueprint that consumes core reads one BOM:

- Each `oci://` source in the `_template/blueprint.yaml` of the bundle is a `data` component with
  the tag of its URL. The component has the digest of the URL, or else the manifest digest that
  the pull wrote to the `digest` file of the cache. A tag can move, so this digest names the exact
  artifact that the bundle read. A cache from an older CLI has no `digest` file, and the component
  then has no digest.
- A source in a source BOM matches a source of the blueprint by its package URL with no version.
  A digest thus does not make the bundle read one source twice, or list it twice.
- The components of a source come from `artifact-manifest.cdx.json` in the source artifact. That
  BOM lists the sources of the source, so the list continues to every depth.
- The `dependencies` graph tells which blueprint pulls each component. The blueprint depends on its
  own artifacts and on each source. A source depends on the artifacts and the sources that its BOM
  gives to its own blueprint. The CLI copies the other edges of a source BOM as they are.
- An artifact that an older CLI wrote has no BOM. For such a source, the CLI reads the tags of the
  whole cached source and the `oci://` sources of its `_template/blueprint.yaml`. A source BOM can
  lack a source that its blueprint names, because the first v0.10.0 BOM listed its own tags only.
  The CLI reads that source in the same way.
- A pulled artifact chooses the sources below it and writes its own BOM, so the CLI limits what it
  takes from a source. It stops at a source that is already above it in the walk. It fails when
  sources nest more than 16 deep, or when a bundle reads more than 256 sources. A component of the
  bundle wins over a source component with the same `bom-ref`. A source BOM gives only edges
  between its own components, and none from a component of the bundle. The CLI drops an edge to a
  `bom-ref` that is not a component.
- A source that is not `oci://` is not in the BOM, as in Decision 1. Flux pulls it, and the CLI has
  no cache for it.

When the bundle and core pull one image, the BOM has one component and two edges to it. A scanner
reads the flat component list and matches each package URL. A copy tool reads the same list. A
tool that needs to know where an artifact comes from reads the graph.

Nested components (`components[].components`) were rejected. CycloneDX defines nesting as an
assembly: the child is part of the parent. A blueprint artifact does not hold the images that it
pulls. Nesting also repeats an artifact under each source that pulls it, so its bom-refs need a
source prefix to stay unique. A tool that reads only the top level also misses the nested
components.

A published BOM MUST NOT leave out a source without a sign. Thus `windsor bundle` pulls each
`oci://` source that is not in the OCI cache, through the pull that other commands use, mirrors
included. A failed pull stops the bundle and names the source. These pulls and the digest lookups
are the only registry calls of `windsor bundle`.

The digest records the artifact that each tag names when the bundle is made. A site that copies
the artifacts by these digests holds the artifacts that the BOM lists, and its cluster pulls them.
A tag can move, so a site that pulls by tag from the upstream can get another artifact. Only a
digest on the tag line prevents that. The CLI does not compare the BOM with a running cluster.

A chart with a version range, a chart in a Git repository, and a component that the CLI reads
from a source BOM get no lookup. A source BOM keeps the digests that its own bundle wrote.

`windsor show artifacts -o cyclonedx` gives the same graph. It reads no digest from a registry, so
its components have a digest only when the tag line gives one. A source depends on each source whose
name its cached `_template/blueprint.yaml` gives, and on the tags of its own files in the scope of
the command. The output has no metadata, so it has no edges from the project. The `-o yaml`,
`-o list`, and `-o flux-mirror` outputs stay as they are, and the facet filter of Decision 3 still
applies.

### 8. Blueprint sources follow the image mirrors

A copy of a blueprint artifact is not sufficient. Two clients pull an `oci://` source from the
literal `sources[].url`:

- The CLI pulls each source into its OCI cache, nested sources included. The Terraform module
  resolver reads modules from that cache, so Terraform pulls nothing itself.
- Flux source-controller pulls the `OCIRepository` that the CLI applies for each source.

The CLI loads sources before it composes the blueprint, so a blueprint value cannot redirect them.
Core's `charts.registry` works because core writes its own chart sources. The CLI writes the
blueprint `OCIRepository`, so the CLI does this redirect.

A blueprint source is a full OCI reference with a host and a path, like an image. Thus it uses
the image rule of Decision 4:

- A blueprint entry goes to the mirror that `registries` configures for its upstream host,
  under its upstream path and tag. The check for two images with one destination also covers
  blueprint entries.
- `-o flux-mirror` writes a blueprint entry as an `artifacts` entry with `selector.tags: [<tag>]`
  and `includeReferrers: true`. Signatures and SBOMs travel with the artifact.
- A source with no exact tag, such as a digest only or a `semver` range, gets no config entry.
  The command names it on stderr, as for an image with no tag.

When the context has a mirror for the host of an `oci://` source, the CLI pulls from the mirror:

- The CLI rewrites the URL at the pull in `pkg/composer/artifact`. Direct sources, nested sources,
  and Terraform modules all go through this pull.
- The CLI sets the mirror URL in `spec.url` of the `OCIRepository`. When the mirror endpoint uses
  `http://`, the CLI also sets `spec.insecure: true`.
- The composed blueprint, the OCI cache key, `windsor show blueprint`, and the inventory keep the
  upstream URL. Thus `windsor show artifacts` still names the source to copy from.
- A digest stays valid, because Flux Mirror copies the artifact byte for byte.

A `registries` entry with `endpoint` turns the redirect on for its host, for images and
blueprint sources together. An entry with no `endpoint` turns it off. A workstation registry proxy
sets no `endpoint`, so it does not change a pull.

### 9. A top-level `registries` key replaces `docker.registries`

The `docker.registries` map now does three jobs. Its `remote` and `hostport` fields configure the
workstation registry containers. Its `endpoint` field sets the containerd mirror on the cluster
nodes, and Decisions 4 and 8 read it for image and blueprint destinations. Only the first job
belongs to Docker. Thus the map moves to a top-level `registries` key, with the same entries and
fields. The cli config API types `endpoint` on the new key.

`endpoint` is new in v0.10.0. The rename ships in the same release, so registry mode starts on
`registries`. The deprecation path is:

1. The CLI reads `registries`. When only `docker.registries` is set, the CLI reads it and gives a
   warning that names `registries`. A context that sets both keys fails.
2. The CLI gives one `registries` map to the composed values. A facet reads `registries`, whatever
   key the user wrote.
3. Core moves its facets, its schema, and its tests to `registries`. `windsor init` writes
   `registries`.
4. A later release removes `docker.registries`.

## Consequences

Core deletes `flux-mirror.yaml` and its comparison check once `-o flux-mirror` ships. Any
blueprint that tags its artifacts gets the same air-gap list without writing one.

`windsor show artifacts` needs no `flux` CLI and no network access. It reads files that the
context already has. It is fast and its output is the same on every machine.

The list is only as complete as the tags. CI in registry mode finds a missing tag on a chart or
an image that a Pod pulls, because the pull fails. Nothing finds a missing tag on an artifact that
no Pod pulls, such as core's Talos node image. An operator that selects an image at runtime needs
that image tagged by hand, and the pull fails until then.

A tag records the pinned version, not the version a cluster runs. A chart default or a value
that overrides a tag is not in the list. CI in registry mode catches an image that differs from a tag, because the registry does not hold it.

The default list holds the tags of the files that the context uses, each tag in `contexts/`,
`_template/`, and `kustomize/crds/` outside a facet file, and each facet tag on a value that the
composed blueprint uses. A tag on a facet value that no composed value reads is not in the list, so
a facet author MUST put the tag on the value that the blueprint consumes. A site that enables
another component syncs again first.

A site configures one mirror for each upstream host, and images and blueprint sources share it. A
context cannot redirect the images of a host and keep its blueprint sources on the upstream. The
host that runs the CLI MUST reach the mirror at the address in `endpoint`. On an air-gapped site
that host is inside the network. A workstation can fail when the endpoint name resolves only in
the cluster.

A context that sets `docker.registries` keeps working until the later release removes it, and
each command gives the warning until the user moves the map.

The CLI removes the Flux render, the Helm chart render, the FluxInstance extractor, and the
flux-operator dependency that the first version of `windsor show artifacts` needed. It also
removes the Renovate scan. It takes on the CycloneDX and package URL libraries as dependencies.

`windsor bundle` needs access to the registry of each tagged artifact and to each Helm repository,
to read the digests. It needs registry access for `oci://` sources when the cache does not hold
them. A CI job that bundles a blueprint MUST have read credentials for each of these registries.
`windsor bundle` does not work without network access. In return, a site copies the artifacts of a bundle from one BOM.

## Alternatives considered

### Render each Kustomization and HelmRelease

The CLI would run `flux build kustomization`, render each chart with the Helm SDK, and read
images from the pod templates. Rejected: it is precise but large. It needs the flux and
flux-operator CLIs, network access, and a chart cache, and it still misses runtime images and
anything outside a manifest.

### Evaluate Terraform

The CLI would parse each Terraform component and read `helm_release` and workload resources, with
or without a declared subset of the language. Rejected: no Go library exports Terraform's
evaluator, and a reader of a subset still needs the author to shape the code to fit.

### Extractors for each operator kind

The CLI would read an operator's spec to find the images it selects. Rejected: the set of kinds
has no end, and each one is a mechanism to maintain.

### Terraform plan JSON

The CLI would run `terraform plan` and read `helm_release` values from `terraform show -json`.
Rejected: a plan needs init, providers, backend credentials, and upstream outputs. An air-gap
list must exist before the first bootstrap, when every component is new.

### A declared `images` list in the blueprint

A blueprint would carry a second copy of each pin. Rejected: a second copy drifts from the file
that uses it. The tag sits beside the pin.

### A `windsor mirror` command that copies

The CLI would run the copy itself. Rejected for now: Flux Mirror already handles registry
authentication, workload identity, signature checks before copying, drift reporting, and
re-runs. The CLI's job is the list.

### One composer patch over every HelmRepository (cli#3376)

Rejected, for the reasons in Context.

### A separate key for the blueprint source registry

A CLI key in `windsor.yaml`, with a `--blueprints-to` flag, would name the registry for blueprint
sources. Rejected: it usually holds the same registry as `registries`. A user who sets one
and forgets the other gets a failed pull.

### Mirror URLs written in `sources[].url`

The user would write the mirror URL into the blueprint of each context. Rejected: it does not reach
nested sources. The inventory would also read the mirror as the upstream, so Flux Mirror would copy
the registry onto itself.

### Digests from a running cluster

CI would read the images on each node and the Flux sources after `windsor up`, compare them with
the tags, and write the observed digests into the tag lines. Rejected: it needs a cluster, a CI
script, and a new input format for the CLI. It sees only the artifacts that one run of one
context pulls.

### Digest pins written by Renovate

Renovate would add an `@sha256` suffix to each tag line and update it with each version. Rejected:
each tag line then holds a digest that a person must review on every bump, and an HTTP chart
gets no digest.

## References

- [Flux Mirror](https://fluxcd.io/blog/2026/08/flux-mirror/)
- [Flux Mirror config](https://fluxcd.io/flux/cli-plugins/flux-mirror/config/)
- [core ADR 0020: Helm charts from a single chart registry](https://github.com/windsorcli/core/blob/main/docs/adr/0020-helm-chart-registry.md)
- sdlc ADR 0001: chart registry contract between cli and core
