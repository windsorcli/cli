# ADR 0010 — Artifact inventory: list what a context pulls and where each artifact goes

- Status: Proposed. Supersedes the single HelmRepository patch proposed in cli#3376.
- Date: 2026-10-28; revised 2026-11-05 (artifact tags replace rendering)
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

`windsor bundle` writes `artifact-manifest.yaml` into every blueprint artifact
(`pkg/composer/artifact`). It scans Renovate comments in `kustomize/` and `terraform/` and
records each artifact's type, transport, reference, version, digest, and declaring file. The
scanner works offline and needs no rendering. This ADR reuses it for the `artifact` tag.

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
HelmRepository into the OCIRepository the chart registry needs. Core does the swap in its own
manifests, so the CLI has no chart rewrite to do.

## Decision

### 1. `windsor show artifacts` lists the tagged artifacts of a context

The command prints every artifact that a tag names in the blueprint sources of the current
context: kind (`chart` or `image`), upstream reference, version, destination when one is
configured, and the file that declares it. It copies nothing. Copying stays with Flux Mirror or
any tool that reads the list.

```sh
windsor show artifacts --charts-to registry.example.com/windsor -o flux-mirror | flux mirror sync -
```

`-o yaml` (the default) prints one YAML entry per artifact with a `destination` field.
`-o flux-mirror` prints a Flux Mirror `v1beta2` config. `-o list` prints one
`destination<TAB>upstream` line per artifact for other tools.

The command is not named `mirror`. It covers charts, which go to a chart registry rather than a
mirror, and it never copies.

### 2. An `artifact` tag names each artifact

The tag is a comment, `# artifact: type=<image|chart> [name=<name>] [repo=<chart repository URL>]`,
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
- One tag names one artifact.
- A tag with no value, an unknown `type`, no `name` and no full reference, or a chart with no
  `repo` is an error. `windsor show artifacts` and `windsor bundle` both fail on it and name the
  file and line.
- A new function in the scanner of `pkg/composer/artifact` reads the tag. It shares the file
  filter of the Renovate scan. The Renovate scan stays for tools that a cluster does not pull.
- Renovate reads the same tag through a regex `customManagers` entry, so one tag gives the list
  and the version updates.

### 3. The scan covers every source of the context

The CLI scans the blueprint's own source in the project root and the OCI cache of each `oci://`
source, nested sources included. For a typical context, that is the blueprint and `core`. It reads
the `.yaml`, `.yml`, and `.tf` files under `kustomize/`, `terraform/`, and `contexts/` in the
project, and under `kustomize/`, `terraform/`, and `_template/` in a bundled source. This covers
facets and values files.

The scan reads whole sources. It does not filter by the Kustomizations and Terraform components
that the context enables. An artifact of a disabled facet is in the list. A site copies a few
extra artifacts.

### 4. Destinations follow each kind's layout

- A chart goes to `<--charts-to>/<chart name>:<chart version>`, the layout `helm push` and Flux
  Mirror both write. The CLI does not read a chart registry from blueprint values, since the
  key's name belongs to the blueprint's schema. Core passes its `charts.registry` value.
- An image goes to the mirror the context's `docker.registries` configures for its upstream
  host, under its upstream path and tag. An entry is a mirror only when it sets `endpoint`.
  The key `*` applies to each host with no entry. The command fails when two
  different images have the same destination.
- An image with no mirror is listed with no destination. `-o flux-mirror` leaves it out and
  names it on stderr.
- A chart whose tag names a repository under `--charts-to` stays out of `-o flux-mirror`, and the
  command names it on stderr, so Flux Mirror never copies a registry onto itself.

### 5. The CLI writes no artifact list into the cluster

The registry contents are the list, as sdlc ADR 0001 section 6 states. A relocated context
enforces its own registries, and the CLI applies no ConfigMap for a policy.

### 6. Flux Mirror output selects exact tags

`-o flux-mirror` writes HTTP charts as `charts` entries with an exact `version`. It writes OCI
charts and images as `artifacts` entries whose `selector.regex.pattern` is `^<tag>$` with the
tag escaped, and sets `includeReferrers: true` so signatures and SBOMs travel with each artifact.
An exact tag pattern avoids Flux Mirror's semver selector, which treats `v0.25.0` and `0.25.0` as
equal and copies whichever it sorts first. The output has no `hosts` section. Flux Mirror reads
credentials from the Docker config, and a site that needs `hosts` merges its own.

### 7. `windsor bundle` publishes the same list as a CycloneDX BOM

This decision ships in a later release. In v0.10.0, `windsor bundle` still writes
`artifact-manifest.yaml`, and it fails on a malformed tag. From the later release,
`windsor bundle` writes `artifact-manifest.cdx.json` at the root of the blueprint artifact. The
file is a [CycloneDX](https://cyclonedx.org) 1.6 BOM, so SBOM tools read it without the CLI. It
replaces `artifact-manifest.yaml`. `windsor bundle` builds it from the tag scan of Decision 2, so
the BOM and `windsor show artifacts` read one source.

- An image is a `container` component, and a chart is an `application` component.
- An OCI image or chart has the package URL `pkg:oci/<name>?repository_url=<path>&tag=<tag>`,
  with the digest as the version when the tag line gives one. An HTTP chart has
  `pkg:generic/<name>@<version>?download_url=<repository>`, because the package URL
  specification has no chart type.
- A digest, when the tag line gives one, is in `hashes` as SHA-256.

`windsor bundle` makes no registry calls and has no render step. The BOM has no graph from a
Kustomization to its artifacts, because the tag scan does not know which component a tag serves.

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

The scan reads whole sources, so the list includes artifacts of disabled facets.

The CLI removes the Flux render, the Helm chart render, the FluxInstance extractor, and the
flux-operator dependency that the first version of `windsor show artifacts` needed. It takes on a
CycloneDX library as a dependency when Decision 7 ships.

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

## References

- [Flux Mirror](https://fluxcd.io/blog/2026/08/flux-mirror/)
- [Flux Mirror config](https://fluxcd.io/flux/cli-plugins/flux-mirror/config/)
- [core ADR 0020: Helm charts from a single chart registry](https://github.com/windsorcli/core/blob/main/docs/adr/0020-helm-chart-registry.md)
- sdlc ADR 0001: chart registry contract between cli and core
