# ADR 0008 — Airgap media and hydration

- Status: Proposed
- Date: 2026-08-09
- Deciders: Ryan VanGundy

Goal: produce a single body of media — writable once, readable forever — that carries 100% of what
a Windsor blueprint needs, and from which a disconnected workstation can run `windsor bootstrap`
to completion with no egress at any point. This ADR fixes the media layout, the bill-of-materials
(BOM) contract, the command surface, and the layer ownership; it names the eight feasibility
questions that must be answered by spike before the sequence below is committed to.

## Context

### What already exists, verified against the tree

Windsor is closer to this than it looks. Five mechanisms already in place do most of the structural
work, and each was checked directly rather than assumed.

**1. A BOM already ships inside every blueprint artifact.** `pkg/composer/artifact/manifest.go`
defines `ArtifactManifest` / `ManifestEntry`, written as `artifact-manifest.yaml` into every bundle
(`artifact.go:602`, regenerated at `artifact.go:801`). Its own header states the intent this ADR is
picking up: *"bundled into the blueprint OCI artifact so downstream consumers can hydrate a local
mirror without re-evaluating adaptive composition logic or rendering Helm charts."* It already
models `ArtifactType` (docker / helm / github-release / git-tag) and `ArtifactTransport`
(oci / tarball / git), and already records `helmRepo` provenance.

It is populated by `pkg/composer/artifact/scanner.go`, which walks bundled files for Renovate
annotations — deterministic and offline by construction. Against `windsorcli/core` today that scan
finds annotations across 64 files: 94 `datasource=docker`, 33 `datasource=helm`, 27
`datasource=github-releases`, 4 `git-refs`, 2 `github-tags`. Core's Renovate hygiene is good enough
that this is a genuine inventory, not a token one.

**2. Local registries already run, and Talos already pulls through them.** `windsorcli/core`'s
`terraform/workstation/docker` runs one `ghcr.io/distribution/distribution:3.0.0` container per
upstream registry, on fixed IPs in the reserved block `[4, node_start_offset)`, with storage
bind-mounted from `.windsor/cache/docker/registries/<host>/` (`main.tf:260`; the Incus module does
the same at `main.tf:217`). `contexts/_template/facets/option-workstation.yaml` then maps the
module's `registries` output straight into `machine.registries.mirrors` on the Talos machine patch.
The default map covers `gcr.io`, `ghcr.io`, `quay.io`, `reg.kyverno.io`, `registry-1.docker.io`,
`registry.k8s.io`.

Today those containers run in pull-through mode (`REGISTRY_PROXY_REMOTEURL`, `main.tf:243`). The
airgap change is to run the same containers, at the same IPs and hostnames, in plain storage mode
against pre-seeded content. Nothing in the Talos mirror wiring has to change.

**3. Blueprint sources and Terraform modules already resolve from a local cache.** `Pull` checks
`.windsor/cache/oci/<key>` before reaching the network (`artifact.go:517`) and only downloads on a
cache miss or `NO_CACHE`. `FluxStack.resolveSourceRoot` (`pkg/provisioner/flux/stack.go:546`)
derives the same path. A pre-populated cache directory is therefore already a working offline path
for the blueprint artifact and every Terraform module it carries.

**4. A local git server already runs.** The workstation stack runs a `git-livereload` container at
`git.<domain>`, which is the in-enclave Git origin Flux `GitRepository` sources need when the
blueprint is not consumed over OCI.

**5. Windsor checks tools; it does not install them.** `pkg/runtime/tools/tools_manager.go` has
`Install()` and `WriteManifest()` as explicit placeholders returning nil; the real surface is
`CheckRequirements`, which does `execLookPath` plus a minimum-version comparison against
`registry.go`'s `toolRegistry`. This is load-bearing for airgap: Windsor never has to acquire a
tool, only to find an acceptable one on `PATH`. Supplying tools becomes a `PATH` question, not an
installer question.

### What does not exist, and what actually breaks

**No Terraform provider mirror.** `grep` for `TF_PLUGIN_CACHE_DIR`, `provider_installation`,
`filesystem_mirror`, `TF_CLI_CONFIG_FILE` across both repos returns nothing. Core's modules require
15 distinct providers — `hashicorp/{aws,azurerm,helm,kubernetes,local,null,random,tls}`,
`kreuzwerker/docker`, `lxc/incus`, `siderolabs/talos`, `vmware/vsphere`, `hetznercloud/hcloud`,
`hcloud-talos/imager`, `windsorcli/hyperv`. Every `terraform init` in the enclave hits
`registry.terraform.io` today and fails.

**Helm charts are pulled by Flux over HTTPS, and containerd mirrors do not cover that.** Core
declares 24 distinct HTTP `HelmRepository` URLs (`charts.jetstack.io`, `helm.cilium.io`,
`prometheus-community.github.io`, and so on) plus `oci://docker.io/envoyproxy`. Those fetches come
from source-controller inside the cluster over HTTPS to the public internet. `machine.registries.
mirrors` redirects containerd image pulls only; it does nothing for a chart download. Every HTTP
`HelmRepository` must be republished as OCI and its URL rewritten, or the enclave install stalls at
the first `HelmRelease`.

**The declared BOM is incomplete by construction.** A Renovate annotation covers what the blueprint
author pinned. It does not cover images a chart's own defaults introduce — sidecars,
`kube-rbac-proxy`, init containers, an operator's operand image chosen at runtime. `kustomize/database/
install/cloudnativepg/helm-release.yaml` is representative: the annotation pins the operator image,
while the Postgres image the operator later pulls appears nowhere in the blueprint source. A media
set built from the declared BOM alone will fail somewhere in the middle of a bootstrap, which is the
worst possible failure mode for write-once media.

**Two Terraform paths call the internet at apply time.** `terraform/cluster/talos/extensions/main.tf:34`
resolves a schematic through `talos_image_factory_schematic` against `factory.talos.dev`, and
`terraform/compute/incus/main.tf:33` registers an image remote at `https://images.windsorcli.dev`.
Both need an offline substitute.

**Flux installs itself from OCI charts.** `terraform/gitops/flux/main.tf:150,163` installs
`flux-operator` and `flux-instance` from `oci://ghcr.io/controlplaneio-fluxcd/charts` via the Helm
provider, and pins controller images with `registry = "ghcr.io/fluxcd"` (`main.tf:175`). The
registry field is already parameterized, which is the hook airgap needs; the chart repository is not.

**The container runtime itself is not a binary problem.** Docker Desktop, Colima, and Incus are
daemons and VMs. On macOS, `colima start` provisions a Lima guest that is downloaded from the
internet on first run. A media set that carries every CLI but assumes a working local container
runtime is only honest if that assumption is written down as a prerequisite.

### The two-workstation model

Everything below assumes two roles, because conflating them is where airgap designs usually go
wrong.

- **Staging workstation** — connected. Runs `windsor pack`. Has egress, a container runtime, and the
  same tool versions the enclave will get.
- **Enclave workstation** — disconnected. Has the media mounted read-only and nothing else. Runs
  `windsor verify`, `windsor hydrate`, then `windsor bootstrap`.

The enclave workstation may already have Docker or Terraform installed, at versions Windsor did not
choose. That is a supported and expected condition, not an error, and decision 7 handles it.

## Decision

### 1. Media is a content-addressed directory tree with a signed root index, not an archive

The unit of distribution is a directory that can be burned to optical media, written to a
write-protected volume, or served from a read-only mount. It is usable in place — `windsor hydrate
--media /Volumes/WINDSOR_AIRGAP` — with no unpack step required.

```
windsor-airgap-<blueprint>-<version>/
  index.yaml              # root: schema version, blueprint refs, platforms, arches, digests
  index.sig               # detached signature over index.yaml
  checksums.txt           # sha256 of every file, for readers without the CLI
  oci/                    # ONE OCI image layout (spec v1.1) for all OCI content
    index.json
    oci-layout
    blobs/sha256/...
  providers/              # terraform filesystem-mirror layout, per platform
  bin/<os>_<arch>/        # pinned CLI binaries + per-file checksums
  images/                 # machine images: talos iso/raw/ova, incus image files
  talos/                  # image-cache.oci, schematic pins, boot assets
  runbook.md              # generated, blueprint-specific, human-readable
```

Rationale, in order of weight:

- **A single tarball is unusable on WORM optical media.** ISO 9660 caps a single file at 4 GiB
  without multi-extent support that many readers do not implement. A content-addressed blob tree
  keeps every file at layer size, well under the cap, and stays readable from an ISO 9660 filesystem
  without requiring UDF. (Spike S7 confirms this against the target readers.)
- **One `oci/` layout for everything OCI.** Container images, Helm charts republished as OCI, the
  Windsor blueprint artifact, and Talos installer images all share one blob store, so shared base
  layers are stored once. Against core's image set this is not a marginal saving.
- **Partial verification and resumable copy.** A blob tree can be verified, diffed, and
  incrementally re-copied. A 20 GB tarball cannot.
- **The user can point at folders.** Serving the registry directly off the mounted media, with no
  copy, becomes possible (decision 3).

`index.yaml` is the manifest of record: schema version, the blueprint reference and digest it was
built from, the platform/arch matrix, the full resolved BOM with digests, and the provenance of each
entry (declared, observed, or manually added).

### 2. The BOM is declared ∪ observed, and pack fails closed on the delta

Two independent passes produce the BOM, and `windsor pack` refuses to write media when they
disagree in the dangerous direction.

- **Declared** — the existing Renovate scan (`scanner.go`), extended to resolve every entry to a
  digest and to expand chart references into their `helm template` render so chart-default images
  are captured. This runs offline against blueprint source.
- **Observed** — captured from a reference deployment of the same blueprint at the same version on
  the staging workstation. The capture sources are the registry cache trees at
  `.windsor/cache/docker/registries/<host>/`, which by construction contain exactly what was pulled,
  plus the node-side image list read through the existing cluster client.

`windsor pack` computes `observed \ declared`. A non-empty delta is an error naming each missing
reference, its digest, and the node or repository that pulled it. `--allow-undeclared` records them
in `index.yaml` with `provenance: observed` and proceeds. The default is to fail, because a missing
image discovered after the media is burned is unrecoverable in the enclave.

This is the single most important decision in the ADR. A static scan is not sufficient for a
promise of 100% completeness, and no amount of scanner improvement makes it sufficient, because
chart defaults and operator-chosen operand images are not statically present in the blueprint at
all.

### 3. The airgap registry is the existing workstation registry with proxying off, seeded by push

`windsor hydrate` starts the same per-upstream `distribution` containers core already runs, at the
same IPs and hostnames, with `REGISTRY_PROXY_REMOTEURL` unset, and pushes the media's `oci/` layout
into them. `docker.registries` and the generated `machine.registries.mirrors` are unchanged, so
neither core's Terraform nor the Talos machine patch needs an airgap branch.

Two things are deliberately rejected here:

- **Copying the proxy cache directory and re-serving it as a proxy.** Distribution's
  `proxyTagService.Get` tries the remote first and falls back to the local association only on
  error, so every tag resolution in the enclave pays a DNS or connect timeout before succeeding, and
  the fallback path is not a supported offline mode. Issue distribution/distribution#4046 documents
  the cache failing to serve content it demonstrably holds. Depending on that behavior for a
  disconnected install is not defensible.
- **A single hub registry with `overridePath`.** Talos supports pointing many upstreams at one
  endpoint with distinct paths, which is how Harbor is used (and how Manager's
  [ADR-0008](../../../manager/docs/adr/0008-harbor.md) intends to use it). It is the better shape at
  fleet scale, and it is not the shape core's mirror map generates today. Adopting it here would mean
  changing the mirror derivation in `option-workstation.yaml` as part of an airgap change. Deferred
  to the point where Harbor is the enclave registry; see Deferred.

Where the registry storage lives is a hydrate-time choice: copied into the enclave work directory
(default, needs disk equal to the image set), or served read-only directly from the mounted media
via `REGISTRY_STORAGE_MAINTENANCE_READONLY_ENABLED` (no copy, gated on spike S2).

### 4. Every Helm chart is republished as OCI, and core parameterizes the repository base

`windsor pack` converts all 24 HTTP `HelmRepository` sources into OCI artifacts inside the media's
`oci/` layout, keyed by chart name and version. In the enclave they are served from the local
registry, and Flux consumes them through `HelmRepository` with `type: oci`.

Ownership of the URL rewrite goes to **core, not the CLI**. Core's chart repository URLs become
config-derived — a facet value with the public URL as its default and the enclave registry as the
airgap override — the same pattern `flux/main.tf:175`'s `registry` field already uses for controller
images. This keeps the rewrite declarative and visible in `windsor explain`, and it keeps the CLI
out of the business of mutating third-party YAML.

For blueprints Windsor does not own, `windsor hydrate` will apply a transitional source rewrite
driven by `index.yaml`. That mechanism is explicitly transitional and is not the path core takes.

### 5. Terraform providers ship as a filesystem mirror, and `pkg/runtime/terraform` owns pointing at it

`windsor pack` synthesizes an aggregate `required_providers` block from every Terraform component
the composed blueprint references, runs `terraform providers mirror -platform=<os>_<arch>` per
target platform into `providers/`, and runs `terraform providers lock -platform=...` so that
`.terraform.lock.hcl` carries hashes valid for the enclave's platform rather than the staging
workstation's.

`windsor hydrate` writes a CLI configuration file with a `provider_installation` /
`filesystem_mirror` block, and `pkg/runtime/terraform` exports `TF_CLI_CONFIG_FILE` alongside the
`TF_DATA_DIR` and `TF_VAR_*` variables it already assembles. The architecture skill assigns
"provider policy" to that package, so this needs no new ownership.

OpenTofu is a separate mirror namespace (`registry.opentofu.org`), and
`BaseToolsManager.detectTerraformDriver` already chooses between the two at runtime. `windsor pack`
mirrors for the driver the context is configured for, and `index.yaml` records which. Packing both
is a flag, not a default, because it roughly doubles the largest single component of the media.

### 6. Talos gets two independent offline paths, and schematics are pinned, never resolved

- **Registry mirrors** are the primary path, and already work once decision 3 lands. Nodes pull
  system images from the workstation registries.
- **Talos Image Cache** covers the case where no registry can precede the node — bare metal, and the
  bootstrap window before the workstation stack is up. `windsor pack` runs `talosctl images
  cache-create` over the Talos default image list plus the blueprint's own images into
  `talos/image-cache.oci`, and builds boot media with `imager --image-cache`. The enclave machine
  config sets `machine.features.imageCache.localEnabled: true` with an `IMAGECACHE` volume. The 1 GiB
  default volume size is far too small for core's image set and must be sized from the actual cache
  (spike S5).

Schematic IDs are **pinned in blueprint config, never resolved at apply time**. `platform-vsphere`
and `platform-hetzner` already do this with a literal schematic ID; `cluster/talos/extensions`
resolving one through `talos_image_factory_schematic` does not. That module gains an input that
accepts a pre-computed ID and skips the resource — a core change this ADR depends on. `windsor pack`
downloads the boot assets for the pinned ID and Talos version into `images/`.

Two platform-specific notes. On `platform: docker`, Talos runs as a container image
(`ghcr.io/siderolabs/talos:v<version>`) pulled by the local Docker daemon, which does not consult
`machine.registries.mirrors` — hydrate must load or retag that image into the daemon directly. On
`platform: incus`, `terraform/compute/incus/main.tf:340` already resolves an image that is a local
file path through `incus_image.local`, so shipping the Incus image as a file in `images/` needs no
module change; only the remote registration at `main.tf:33` needs to become conditional.

### 7. Tools are supplied by PATH prepend, and an existing tool is respected if it qualifies

`bin/<os>_<arch>/` carries pinned binaries for the tools `toolRegistry` knows about, plus `windsor`
itself. `windsor hydrate` does not install anything and does not modify the system. It records the
media's `bin` directory in the context, and the existing env printer prepends it to `PATH` through
the shell hook that already manages Windsor's environment.

If the enclave workstation already has a qualifying tool, `--prefer-system-tools` skips the prepend
for that tool. The `CheckRequirements` minimum-version gate applies either way, so a stale system
Terraform fails the same check it fails today, with the same actionable error — except that in the
enclave the error can now name the bundled binary as the remedy instead of a vendor download page
that is unreachable.

**Explicit non-goal: Windsor does not supply the container runtime.** Docker Desktop, Colima, Incus,
and their guest images are prerequisites of the enclave workstation, documented in the generated
runbook and asserted by `windsor verify`. The Colima case on macOS is the sharpest instance —
`colima start` fetches a Lima guest image — and is spike S4.

### 8. The media is signed, verification is mandatory, and it does not depend on a transparency log

`index.yaml` is covered by a detached signature in `index.sig`, over a key whose public half travels
out of band. Keyless signing is not usable here: Fulcio and Rekor both require egress at verify
time, which is precisely what the enclave does not have.

`windsor verify --media <path>` checks the signature, then every file against `checksums.txt`, then
that every BOM entry in `index.yaml` is present in `oci/`, `providers/`, `bin/`, or `images/`.
`windsor hydrate` runs verification first and refuses to proceed on failure unless
`--insecure-skip-verify` is passed. `verify` is also the read-only pre-flight an operator can run
against media before committing an enclave to it.

In-cluster image signature policy (a Kyverno `verifyImages` rule, for instance) has the same
transparency-log problem and must either be configured against a local key or disabled for the
enclave. That is a core policy decision, flagged here rather than made here.

### 9. Airgap is scoped to the workstation and metal platforms, not to cloud

`platform: docker`, `platform: incus`, and `platform: metal` are in scope. `aws`, `azure`, `gcp`,
`hetzner`, and `vsphere` are not, because provisioning against a cloud control plane requires egress
to that control plane by definition. The media set is still useful there — provider mirror, tool
binaries, image mirror all apply to a restricted-egress environment — but "runs `windsor bootstrap`
with no egress at any point" is only claimed for the first three.

### 10. No new architectural layer

Work lands in existing packages, per the ownership table in
`docs/adrs/0002-target-architecture-and-package-topology.md`:

| Concern | Package |
|---|---|
| BOM model, resolution, media index, pack/verify | `pkg/composer/artifact` (already owns the manifest, `Bundle`, `Push`, `Pull`) |
| Registry seeding, enclave registry lifecycle | `pkg/workstation/registry` (new companion package under the workstation layer) |
| Provider mirror config and `TF_CLI_CONFIG_FILE` | `pkg/runtime/terraform` |
| Tool PATH resolution against media | `pkg/runtime/tools` |
| Hydrate orchestration | `Composer` for media-side work, `Provisioner` for cluster-side |
| Command surface | `cmd/pack.go`, `cmd/verify.go`, `cmd/hydrate.go` — Cobra glue only |

### 11. Command surface

Verb-first, consistent with the house convention.

```
# Staging workstation (connected)
windsor pack --output ./media --platform docker --arch amd64
windsor pack --output ./media --observe            # capture from a live reference deployment
windsor pack --output ./media --dry-run            # print the resolved BOM, write nothing

# Enclave workstation (disconnected)
windsor verify --media /Volumes/WINDSOR_AIRGAP
windsor hydrate --media /Volumes/WINDSOR_AIRGAP
windsor bootstrap local
```

`windsor bootstrap --media <path>` is sugar for hydrate-then-bootstrap, since that is the sequence
an operator runs once and then never thinks about again.

`pack` flags: `--output`, `--platform`, `--arch` (repeatable), `--include` / `--exclude` by BOM
entry, `--observe`, `--allow-undeclared`, `--sign-key`, `--tofu` / `--terraform`, `--dry-run`.
`hydrate` flags: `--media`, `--work-dir`, `--registry-mode=copy|readonly`, `--prefer-system-tools`,
`--insecure-skip-verify`.

## Feasibility spikes — gates on the sequence below

Each spike has one question and one pass criterion. None of the implementation phases start before
its listed spikes pass.

| # | Question | Pass criterion |
|---|---|---|
| S1 | Can a plain (non-proxy) `distribution:3.0.0` serve a storage tree written by the same image in proxy mode? | A cache tree produced by a real core bootstrap serves `docker pull` by tag and by digest with `REGISTRY_PROXY_REMOTEURL` unset. Determines whether cache harvesting is a viable seeding path at all, or whether push-only is mandatory. |
| S2 | Can `distribution` serve read-only directly from a mounted ISO 9660 volume, bind-mounted into the Docker Desktop / Colima VM? | Full core image set pulls from a registry whose storage is the mounted media, with zero copies. Determines whether `--registry-mode=readonly` ships. |
| S3 | `terraform providers mirror` across all 15 providers, per platform. | Mirror builds; `terraform init` succeeds offline for every core component; lockfile hashes validate. Records total size — `hashicorp/aws` alone is the single largest artifact in the media. Repeat for OpenTofu. |
| S4 | Colima on macOS with no egress. | `colima start` completes against a Lima guest image supplied from the media. If it cannot, macOS enclave support is Docker Desktop only and the runbook says so. |
| S5 | Talos Image Cache at core's actual image volume. | `talosctl images cache-create` over the full BOM, `imager --image-cache` builds bootable media, node provisions with a correctly sized `IMAGECACHE` volume. Records the required size against the 1 GiB default. |
| S6 | How large is `observed \ declared` for a full core bootstrap? | Measured, enumerated, and categorized by cause. If it is small and confined to chart defaults, `helm template` expansion in the declared pass may close it and `--observe` becomes optional rather than required. If it is large, `--observe` is mandatory and decision 2 is load-bearing. |
| S7 | Total media size for core at one platform and one arch, and does the layout survive an ISO 9660 round trip? | Size measured against BD-R capacities (25 / 50 / 100 GB). Every file under the 4 GiB single-file cap. Filenames survive the filesystem's constraints. |
| S8 | Chart republish and rewrite end to end. | All 24 HTTP `HelmRepository` sources convert to OCI, and a `HelmRelease` reconciles from the enclave registry through `type: oci`. `fluxcd/flux-mirror` is evaluated here as an existing implementation before writing one. |

S6 is the one that can change the design rather than just the parameters, and should run first.

## Implementation sequence

**Phase 1 — BOM and pack, no enclave yet.** Extend `scanner.go` to resolve digests and expand chart
renders. Add the media index model and `windsor pack` / `windsor verify` against
`platform: docker`. Gate: S3, S7. Deliverable is media that verifies, with no hydration path yet.

**Phase 2 — Observed capture and the completeness gate.** Cache-tree and node-side harvest,
`observed \ declared` computation, fail-closed default. Gate: S6.

**Phase 3 — Hydrate and the closed loop.** Enclave registry lifecycle, provider mirror wiring, tool
PATH, `windsor hydrate`, and `bootstrap --media`. Gate: S1, S2, S4, S8. Deliverable is a full
`windsor bootstrap` on a disconnected workstation at `platform: docker`, running in CI on a
network-namespaced runner. This is the point at which the ADR's central claim is proven or not.

**Phase 4 — Metal and Incus.** Talos Image Cache media, pinned schematics, Incus image files,
conditional image remote. Gate: S5. Requires the core changes named in decisions 4 and 6.

Phases 1 and 2 land in the CLI alone. Phase 3 depends on core parameterizing chart repository URLs.
Phase 4 depends on core accepting a pre-computed schematic ID and making the Incus remote
conditional. Those core changes are tracked as blockers, not assumed.

## Consequences

- **Media is version-locked to a blueprint version, and correctly so.** A media set is built from
  one blueprint at one version for one platform/arch matrix. Upgrading the enclave means new media.
  This matches the direction in [[project_windsor_lifecycle]] — bootstrap once, then upgrade per
  blueprint version — and makes `windsor upgrade` in an enclave a media-swap operation, which is a
  separate design that this ADR does not attempt.
- **`windsor pack` becomes a first-class CI artifact producer.** It is slow, large, and needs a
  container runtime, egress, and (for `--observe`) a real cluster. That is a heavier build job than
  anything in the repo today.
- **The fail-closed default will be annoying before it is valuable.** Every new chart-default image
  in an upstream bump surfaces as a pack failure. That is the intended trade: an annoying failure on
  the staging workstation instead of an unrecoverable one in the enclave.
- **Registry storage is duplicated at rest by default.** `--registry-mode=copy` needs enclave disk
  equal to the image set. S2 decides whether the read-only path removes that.
- **Two Terraform paths in core stop being able to call the internet at apply time**, which is a
  strict improvement in determinism for connected installs too — pinned schematics do not drift.
- **Cloud platforms are explicitly excluded from the completeness claim.** The media remains useful
  in restricted-egress cloud environments, and the ADR does not promise more than that.
- **Signing introduces key management this repo does not have.** A signing key for pack, and a
  distribution path for its public half, are new operational surface.

## Alternatives considered

- **Extend `windsor bundle` rather than adding `windsor pack`.** Rejected: `bundle` produces a
  blueprint tarball consumable by Flux `OCIRepository` and is a stable published contract. Airgap
  media is a different artifact with a different lifecycle, different size class, and a different
  consumer. Overloading the verb would make both harder to reason about.
- **Ship a single signed tarball.** Rejected on the 4 GiB ISO 9660 single-file limit, on the loss of
  blob-level dedupe across images and charts, and on the user requirement that the media be usable
  by pointing at folders.
- **Harvest the pull-through cache directories and re-serve them as proxies.** Rejected per decision
  3 — the remote-first tag resolution and the documented cache-serving failures make it unsuitable
  as the primary mechanism. S1 keeps it alive as a possible *seeding* optimization for the push
  path, not as the serving mode.
- **A single hub registry with `overridePath`.** Deferred rather than rejected. It is the right
  answer once Harbor is the enclave registry (Manager ADR-0008), and adopting it now would mean
  rewriting core's mirror derivation as part of an airgap change.
- **Rely on `helm template` alone to close the image gap, with no observed pass.** Rejected as the
  sole mechanism: it does not cover images an operator selects at runtime from its own logic rather
  than from chart values, which is exactly the CloudNativePG case. S6 may show it closes enough of
  the gap to make `--observe` optional; it cannot make it unnecessary.
- **Have Windsor install missing tools in the enclave.** Rejected: `Install()` has been a deliberate
  placeholder since the tools manager was written, and modifying a locked-down enclave workstation's
  system state is exactly the wrong default for that environment. PATH prepend achieves the same
  outcome without mutating the host.
- **Vendor charts into the blueprint at build time instead of republishing them as OCI.** Rejected:
  it discards chart provenance and version metadata, and it diverges the enclave install path from
  the connected one. Republishing as OCI keeps both paths on the same Flux mechanism.
- **A self-hosted Talos Image Factory in the enclave.** Rejected for this ADR. Sidero's own guidance
  is that an air-gapped factory needs its images and Talos versions pre-seeded and signed, so it adds
  a service to operate without removing the pre-seeding work. Pinned schematics plus pre-downloaded
  boot assets achieve the same result with no running service. Revisit if the enclave needs to
  generate new schematics rather than consume pinned ones.

## Deferred

- `windsor upgrade` inside an enclave — media-to-media transition, what is diffed, and whether
  incremental media (deltas against a previously hydrated set) is worth the complexity.
- Harbor as the enclave registry, and the `overridePath` mirror layout that follows from it.
- Multi-blueprint media carrying core plus manager plus a tenant blueprint in one set.
- Enclave-side secrets. SOPS with a local age key works offline; 1Password and cloud KMS backends do
  not. The blueprint's secrets configuration constrains which contexts can be airgapped at all, and
  that constraint deserves its own record.
- In-cluster image signature verification policy against a local key.

## References

- `pkg/composer/artifact/manifest.go` — the existing BOM model and its stated hydration intent
- `pkg/composer/artifact/scanner.go` — Renovate-annotation scan
- `pkg/composer/artifact/artifact.go:481` — `Pull` and the OCI disk cache
- `pkg/provisioner/flux/stack.go:546` — `resolveSourceRoot` and the same cache path
- `pkg/runtime/tools/tools_manager.go`, `pkg/runtime/tools/registry.go` — check-don't-install
- `windsorcli/core` `terraform/workstation/docker/main.tf:213-266` — registry containers and cache volumes
- `windsorcli/core` `contexts/_template/facets/option-workstation.yaml` — mirror map to Talos machine patch
- `windsorcli/core` `terraform/gitops/flux/main.tf:150-176` — Flux install and the parameterized image registry
- `windsorcli/core` `terraform/cluster/talos/extensions/main.tf:34` — online schematic resolution
- `docs/adrs/0002-target-architecture-and-package-topology.md` — layer ownership table
- Sidero Labs, [Air-Gapped Kubernetes With Talos Linux](https://www.siderolabs.com/blog/air-gapped-kubernetes-with-talos-linux) — Image Cache workflow
- Sidero Labs, [Pull-through cache](https://docs.siderolabs.com/talos/v1.11/configure-your-talos-cluster/images-container-runtime/pull-through-cache) — `mirrors`, `overridePath`, `skipFallback`
- Sidero Labs, [Run Image Factory On-Prem](https://docs.siderolabs.com/omni/self-hosted/run-image-factory-on-prem)
- Flux, [Air-gapped installation](https://fluxcd.io/flux/installation/configuration/air-gapped/)
- [`fluxcd/flux-mirror`](https://github.com/fluxcd/flux-mirror) — chart and artifact mirroring, evaluated in S8
- CNCF Distribution, [Registry as a pull through cache](https://distribution.github.io/distribution/recipes/mirror/)
- [distribution/distribution#4046](https://github.com/distribution/distribution/issues/4046) — proxy cache failing to serve held content
- HashiCorp, [`terraform providers mirror`](https://developer.hashicorp.com/terraform/cli/commands/providers/mirror)
- `windsorcli/manager` [ADR-0008 — Harbor](../../../manager/docs/adr/0008-harbor.md)
