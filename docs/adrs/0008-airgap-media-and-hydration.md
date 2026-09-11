# ADR 0008 — OCI registry mirror for images, charts, and Terraform providers

- Status: Proposed
- Date: 2026-08-09
- Deciders: Ryan VanGundy
- Rescoped: 2026-09-01 — narrowed from the original "Airgap media and hydration" design (write-once
  physical media, a disconnected two-workstation `windsor bootstrap`) down to the piece actually
  targeted for v0.10.0: extending the existing image/blueprint OCI mirror mode to also cover
  Terraform providers, over a network-reachable registry. Full write-once airgap media — physical
  distribution, BOM completeness via declared/observed reconciliation, offline Talos provisioning,
  signing, the two-workstation model — is out of scope here. That design was fully drafted under
  this same ADR number before the rescope and remains recoverable from this file's git history
  (`git log -p -- docs/adrs/0008-airgap-media-and-hydration.md`) as the starting point for a future
  ADR once airgap work is actually scoped, rather than something to re-derive from nothing.

Goal: close the one gap in Windsor's existing OCI mirror story. Blueprint sources and container
images already have a mirror path; Terraform providers don't, because HashiCorp's `terraform` has no
OCI client and never fetches providers any way but straight from `registry.terraform.io`. This ADR
gives providers the same mirror path the other two artifact types already have.

## Context

### What already exists, verified against the tree

- **Blueprint OCI sources already pull through a local cache.** `ArtifactBuilder.Pull`
  (`pkg/composer/artifact/artifact.go:481`) checks `.windsor/cache/oci/<key>` before reaching the
  network, downloading only on a cache miss. Refs are used verbatim — pointing a blueprint source
  `url` at a private mirror registry already works today, with no separate "mirror mode" flag.
- **Container images have a separate pull-through mechanism**, `docker.registries`, which runs local
  registry-proxy containers the container runtime pulls through. Unrelated to the blueprint OCI
  path — a second, already-working mirror mechanism, not a gap.
- **`terraform.driver` already selects `terraform` or `tofu`** (`BaseToolsManager.GetTerraformCommand`,
  `pkg/runtime/tools/tools_manager.go:326`, `getTerraformDriver` at `:340`), both invoked as external
  binaries — no Go SDK, no `terraform-exec` dependency. `runTerraformInit`
  (`pkg/provisioner/terraform/stack.go:1566`) is the sole call into `terraform init` / `tofu init`,
  and `selectTerraformCommandEnv` (`stack.go:1919`) is the one place that assembles the env passed
  to it — the natural wiring point for anything provider-mirror-related.
- **`.terraformrc` is already excluded from `windsor bundle` output**
  (`shouldSkipTerraformFile`, `artifact.go:896-918`) — correctly, since it would encode local paths
  that don't survive a bundle. Any generated CLI config needs to be produced at consumption time,
  not shipped.

### The actual gap

**No Terraform provider mirror mechanism exists at all.** A repo-wide grep for
`TF_PLUGIN_CACHE_DIR`, `provider_installation`, `filesystem_mirror`, and `TF_CLI_CONFIG_FILE`
returns nothing. Every `terraform init` / `tofu init` resolves providers straight from
`registry.terraform.io` / `registry.opentofu.org`, with zero Windsor involvement.

**HashiCorp's `terraform` has no OCI client for providers, and there's no signal that's changing.**
Providers are distributed over the Terraform Registry Protocol (HTTPS + JSON, zip packages,
SHA256SUMS + GPG signature) — there is no OCI presence to mirror from the way there is for images
and OCI-native Helm charts. The feature request
([hashicorp/terraform#31463](https://github.com/hashicorp/terraform/issues/31463)) has been open
since July 2022 with no roadmap commitment. OpenTofu 1.10 added native, still-experimental OCI
provider mirroring (`provider_installation { oci_mirror { ... } }`), but this organization is
terraform-primary and needs both drivers supported, so the design can't center on tofu's feature.

**Provider packages are OS/arch-specific, and a naive mirror breaks every platform but the
publisher's own.** `terraform providers mirror` — the only existing tool that actually fetches
provider packages, verifying SHA256SUMS and signature — only downloads the platforms passed via
repeated `-platform=os_arch` flags, defaulting to the invoking machine's platform if none are given.

**OpenTofu's own OCI provider-mirror layout already solves the multi-platform representation
problem**, using the same primitive as Docker multi-arch images (checked live against
[opentofu.org/docs/cli/oci_registries/provider-mirror](https://opentofu.org/docs/cli/oci_registries/provider-mirror/),
not recalled from memory). One version tag resolves to an OCI Image Index
(`artifactType: application/vnd.opentofu.provider`) whose `manifests[]` holds one descriptor per
platform (`artifactType: application/vnd.opentofu.provider-target`, a `platform: {os, architecture}`
field), each pointing at an image manifest wrapping a single `archive/zip` layer. A client resolves
by fetching the index and matching its own OS/arch against the descriptors.

**"Platform" is already an overloaded word in this CLI** — it names the aws/incus/docker/metal/azure
deployment-target dimension elsewhere. Any OS/arch selector this feature introduces needs a visibly
distinct name.

## Decision

### 1. One registry-population mechanism serves both drivers

`filesystem_mirror` (in `provider_installation`) is wire-compatible across both `terraform` and
`tofu`; `oci_mirror` is tofu-only and experimental. Windsor owns the full pipeline for both drivers
rather than branching on `terraform.driver`:

- **Publish** (from any environment with egress): fetch each requested platform's package via
  `terraform providers mirror -platform=<os_arch>` — still the real, only origin-side fetch and
  verification mechanism — then wrap the results into an OCI Image Index per (2) and push to the
  mirror registry.
- **Consume** (any environment, either driver): for each required provider, pull only its Image
  Index (small — JSON only), pick the one descriptor whose `platform` matches the consuming
  machine's own `runtime.GOOS`/`runtime.GOARCH` (no flag needed here — unlike publish, the consuming
  machine already knows its own platform), and pull only that descriptor's `archive/zip` layer.
  Write it to `<mirror-dir>/<HOSTNAME>/<NAMESPACE>/<TYPE>/terraform-provider-<TYPE>_<VERSION>_
  <TARGET>.zip` — Terraform's documented **packed filesystem mirror** layout, verified directly
  against [developer.hashicorp.com/terraform/cli/config/config-file](https://developer.hashicorp.com/terraform/cli/config/config-file):
  no `index.json`, no version manifest required, Terraform infers everything from the nesting and
  filename alone. Generate a CLI config with `provider_installation { filesystem_mirror { path =
  <mirror-dir> } }`, and export `TF_CLI_CONFIG_FILE` alongside the `TF_DATA_DIR` and `TF_VAR_*`
  variables `selectTerraformCommandEnv` already assembles, ahead of `runTerraformInit`. Terraform
  itself never resolves multi-arch — it only ever sees a directory already narrowed to its own
  platform, so `init` runs against the identical filesystem-mirror code path it already has, unmodified.

This works unmodified for `terraform` and `tofu` alike.

### 2. Providers are stored as OCI Image Indexes — OpenTofu's layout, not a bespoke one

Adopting OpenTofu's exact media types and structure (Context) means the registry content stays
legible to standard OCI tooling and, once `oci_mirror` graduates past experimental, is directly
consumable by `tofu` with zero Windsor involvement — interoperability that a Windsor-specific format
would forfeit for no benefit. `go-containerregistry` — already a dependency via `ArtifactBuilder` —
builds and pushes the index directly (`v1.IndexManifest`, `remote.WriteIndex`); no new library.

### 3. Platform/arch selection is explicit, never inferred from the publishing machine

The publish path takes a repeatable OS/arch selector, deliberately not named `--platform` (Context —
collides with the existing deployment-platform dimension). Defaults to a configured set rather than
the invoking machine's own platform; publishing from one developer's laptop must never silently
produce a mirror that only works on that laptop's architecture. The exact flag name and where its
default set is configured (a new `windsor.yaml` key, most likely) is an open follow-up, not decided
here.

### 4. Command structure: `windsor push` gains objects; `windsor pull providers` is new

`windsor push <registry/repo[:tag]>` already exists (`cmd/push.go`), pushing only the blueprint with
no object argument — the "object optional when singular" case, since there was only one pushable
thing. Adding two more artifact types makes the object necessary going forward; the bare form is
preserved unchanged:

- `windsor push <registry>` — **unchanged.** Still blueprint-only.
- `windsor push images <registry>` — new object. OCI-to-OCI copy of the image refs the existing
  scanner already extracts (`pkg/composer/artifact/scanner.go`) for the manifest.
- `windsor push providers <registry> [--arch=<os_arch>]...` — new object. Runs the publish pipeline
  in (1)/(2)/(3).
- `windsor push mirror <registry>` — convenience object equivalent to running all three together
  (blueprint + images + providers), with `--include`/`--exclude` to scope down. Same three calls,
  not a fourth mechanism.
- `windsor pull providers` — new command family (no `windsor pull` exists today). Resolves the
  current blueprint's provider requirements and runs the consume path in (1). No bare `windsor pull`
  form yet: blueprint sources and image references stay purely config-driven and need no explicit
  pull step, so `providers` is the only object today.

Cobra dispatches to a child command only when the first positional token exactly matches a
registered subcommand name (`images`, `providers`, `mirror`) — none of which collide with a valid
`registry/repo[:tag]` token — so `pushCmd`'s existing `RunE` keeps handling the bare form unchanged.

Split objects plus a `mirror` convenience wrapper were chosen over a single umbrella command because
providers and images have different natural re-push cadences — a provider version bump shouldn't
require re-pushing images, and forcing every re-push through `--include`/`--exclude` flags on one
command is worse ergonomics than a dedicated object for the common surgical case.
`windsor mirror push` / `windsor mirror pull` (mirror as a shared noun root) was considered and
rejected against this codebase's verb-first command convention (`windsor unlock`, not
`windsor stack lock`).

### 5. `.terraformrc` stays out of `windsor bundle`, unchanged

The generated CLI config encodes local materialized paths and is regenerated by
`windsor pull providers` at consumption time — `shouldSkipTerraformFile`'s existing exclusion
(`artifact.go:896-918`) needs no change.

## Consequences

- `windsor push` gains three subcommands; the existing single-argument invocation is preserved
  exactly.
- A new `windsor pull` command family exists with exactly one object today (`providers`); a bare
  `windsor pull` form isn't built until something else needs it.
- The provider OCI layout matches OpenTofu's `oci_mirror` convention exactly, so no format migration
  is needed if tofu users later want to bypass Windsor's own pull path.
- The publish-side conversion (`terraform providers mirror` → OCI index) is genuinely new code with
  no existing tool to lean on — unlike the blueprint/image push paths, which are pure OCI-to-OCI
  copies. This is the real scope of the build.
- The platform-set default (what feeds the OS/arch selector when unset) is an open follow-up.
- Full airgap media is explicitly not delivered by this ADR. Anyone reaching for "run
  `windsor bootstrap` with zero egress" from this design will not find it here — see Deferred.

## Alternatives considered

- **tofu-native `oci_mirror` as the sole mechanism, `terraform` left unsupported.** Rejected — this
  org is terraform-primary, and HashiCorp shows no movement after four years (#31463 still open).
- **A Windsor-specific OCI packaging format for providers.** Rejected in favor of OpenTofu's
  `oci_mirror` layout — an existing, documented spec, consumable by standard OCI tooling and
  directly by `tofu` itself.
- **Per-platform tags instead of an OCI Image Index.** Rejected — loses at-pull-time platform
  resolution and doesn't match the standard OpenTofu already established.
- **A single `windsor push mirror` as the only push object.** Rejected for ergonomics — forces
  `--include`/`--exclude` flags for the common "just re-push providers" case.
- **Keep the full write-once airgap media scope in this ADR.** Rejected for v0.10.0: that design
  (content-addressed media tree, declared/observed BOM reconciliation, Talos Image Cache, signing,
  the two-workstation model) is real and was fully drafted, but isn't being built this cycle. Kept
  out rather than left half-decided alongside a decision that is shipping, per Deferred below.

## Deferred

Everything below was decided in this ADR's original 2026-08-09 draft and is recoverable in full from
git history (`git log -p -- docs/adrs/0008-airgap-media-and-hydration.md`) rather than needing to be
re-derived when it's actually scoped:

- Write-once physical media (content-addressed directory tree, ISO 9660 constraints, BD-R capacity
  planning) as the distribution mechanism for a fully disconnected enclave.
- BOM completeness via declared-vs-observed reconciliation, fail-closed on the delta.
- Talos Image Cache, pinned schematic IDs, and the two independent offline paths for node
  provisioning.
- Media signing and mandatory verification (`windsor verify`), without dependence on a transparency
  log.
- The two-workstation (staging/enclave) operational model and a `windsor bootstrap --media` entry
  point.
- Comprehensive Helm chart republishing as OCI with core-side config-derived repository URLs.
- Container runtime provisioning in a disconnected environment (Colima's Lima guest fetch on macOS,
  specifically).
- Enclave-side secrets handling (SOPS with a local age key works offline; 1Password/cloud KMS don't).

## References

- `pkg/composer/artifact/artifact.go:382,481` — `ArtifactBuilder.Push`/`.Pull`, the existing OCI
  push/pull mechanism this design extends
- `pkg/composer/artifact/artifact.go:896-918` — `shouldSkipTerraformFile`, existing `.terraformrc`
  bundle exclusion
- `pkg/composer/artifact/scanner.go` — existing image-reference extraction, reused for `push images`
- `pkg/runtime/tools/tools_manager.go:326,340` — `GetTerraformCommand`/`getTerraformDriver`
- `pkg/provisioner/terraform/stack.go:1566,1919` — `runTerraformInit`, `selectTerraformCommandEnv`
- `cmd/push.go` — existing `windsor push` command this ADR extends
- [hashicorp/terraform#31463](https://github.com/hashicorp/terraform/issues/31463) — OCI registry
  support requested 2022, still open, no roadmap commitment
- OpenTofu, [Provider Mirrors in OCI Registries](https://opentofu.org/docs/cli/oci_registries/provider-mirror/) —
  Image Index / per-platform manifest layout adopted verbatim in decision 2
- HashiCorp, [`terraform providers mirror`](https://developer.hashicorp.com/terraform/cli/commands/providers/mirror) —
  origin-side fetch mechanism and `-platform` flag behavior
- HashiCorp, [Provider network mirror protocol](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol) —
  the non-OCI mirror protocol HashiCorp does support, referenced for contrast
- HashiCorp, [CLI Configuration File — `filesystem_mirror`](https://developer.hashicorp.com/terraform/cli/config/config-file) —
  the packed-layout naming convention (`HOSTNAME/NAMESPACE/TYPE/terraform-provider-TYPE_VERSION_TARGET.zip`)
  decision 1's consume path writes
