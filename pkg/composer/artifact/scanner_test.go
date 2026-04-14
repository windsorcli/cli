package artifact

import (
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestScanRenovateAnnotations_YAML(t *testing.T) {
	t.Run("DockerImage_WithTagAndDigest_ExtractsReferenceAndDigest", func(t *testing.T) {
		// Given a YAML snippet with a docker Renovate annotation followed by
		// a tag line containing an @sha256 digest
		content := []byte(`          repository: longhornio/longhorn-engine
          # renovate: datasource=docker depName=longhornio/longhorn-engine
          tag: v1.11.1@sha256:5ec00ddbc8f66911b2c8154f2c7ebf15342b4ee4508d6d2f5435aec2537e596f
`)

		// When the scanner is invoked on the snippet
		got := scanRenovateAnnotations("kustomize/csi/longhorn/helm-release.yaml", content)

		// Then exactly one entry is returned with reference, tag, and digest
		// populated from the annotation plus the adjacent tag line
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		e := got[0]
		if e.Type != ArtifactTypeDocker {
			t.Errorf("Type: got %q, want %q", e.Type, ArtifactTypeDocker)
		}
		if e.Reference != "longhornio/longhorn-engine:v1.11.1" {
			t.Errorf("Reference: got %q", e.Reference)
		}
		if e.Version != "v1.11.1" {
			t.Errorf("Version: got %q", e.Version)
		}
		if e.Digest != "sha256:5ec00ddbc8f66911b2c8154f2c7ebf15342b4ee4508d6d2f5435aec2537e596f" {
			t.Errorf("Digest: got %q", e.Digest)
		}
		if e.Repository != "longhornio/longhorn-engine" {
			t.Errorf("Repository: got %q", e.Repository)
		}
		if e.Source.Datasource != "docker" {
			t.Errorf("Source.Datasource: got %q", e.Source.Datasource)
		}
	})

	t.Run("HelmChart_InHelmReleaseSpec_ExtractsNameVersionRepo", func(t *testing.T) {
		// Given a YAML HelmRelease chart spec with a helm Renovate annotation
		content := []byte(`  chart:
    spec:
      chart: longhorn
      # renovate: datasource=helm depName=longhorn package=longhorn helmRepo=https://charts.longhorn.io
      version: 1.11.1
      sourceRef:
        kind: HelmRepository
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("kustomize/csi/longhorn/helm-release.yaml", content)

		// Then a single helm entry is produced with name, version, and repo
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		e := got[0]
		if e.Type != ArtifactTypeHelm {
			t.Errorf("Type: got %q", e.Type)
		}
		if e.Reference != "longhorn" {
			t.Errorf("Reference: got %q", e.Reference)
		}
		if e.Version != "1.11.1" {
			t.Errorf("Version: got %q", e.Version)
		}
		if e.Repository != "https://charts.longhorn.io" {
			t.Errorf("Repository: got %q", e.Repository)
		}
	})

	t.Run("MultipleAnnotations_InOneFile_ReturnsAllEntries", func(t *testing.T) {
		// Given a YAML file with two separate Renovate annotations
		content := []byte(`  chart:
    spec:
      chart: longhorn
      # renovate: datasource=helm depName=longhorn helmRepo=https://charts.longhorn.io
      version: 1.11.1
  values:
    image:
      repository: longhornio/longhorn-engine
      # renovate: datasource=docker depName=longhornio/longhorn-engine
      tag: v1.11.1@sha256:5ec00ddbc8f66911b2c8154f2c7ebf15342b4ee4508d6d2f5435aec2537e596f
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then both annotations produce entries
		if len(got) != 2 {
			t.Fatalf("entries: got %d, want 2", len(got))
		}
		if got[0].Type != ArtifactTypeHelm {
			t.Errorf("entry[0].Type: got %q", got[0].Type)
		}
		if got[1].Type != ArtifactTypeDocker {
			t.Errorf("entry[1].Type: got %q", got[1].Type)
		}
	})

	t.Run("AnnotationWithoutFollowingValue_IsSkipped", func(t *testing.T) {
		// Given a Renovate annotation that is not followed by a pinned value
		// within the lookahead window
		content := []byte(`# renovate: datasource=docker depName=example/foo
# some other comment
# and another
# still nothing
# no value here
# way past lookahead
tag: v1.0.0
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then no entries are emitted because no pin was located
		if len(got) != 0 {
			t.Fatalf("entries: got %d, want 0", len(got))
		}
	})

	t.Run("NonRenovateComment_IsIgnored", func(t *testing.T) {
		// Given a regular YAML comment that does not mention renovate
		content := []byte(`# this is just a note
tag: v1.0.0
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then no entries are produced
		if len(got) != 0 {
			t.Fatalf("entries: got %d, want 0", len(got))
		}
	})
}

func TestScanRenovateAnnotations_Terraform(t *testing.T) {
	t.Run("DockerImage_InHelmReleaseValues_ExtractsTagAndDigest", func(t *testing.T) {
		// Given a terraform yamlencode block with a docker Renovate annotation
		// followed by an HCL assignment with an @sha256 digest
		content := []byte(`    kustomizeController = {
      image = "ghcr.io/fluxcd/kustomize-controller"
      # renovate: datasource=docker depName=ghcr.io/fluxcd/kustomize-controller package=ghcr.io/fluxcd/kustomize-controller
      tag = "v1.8.3@sha256:c59e81059330a55203bf60806229a052617134d8b557c1bd83cdc69a8ece7ea2"
    }
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("terraform/gitops/flux/main.tf", content)

		// Then a single docker entry is produced with tag, digest, and package
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		e := got[0]
		if e.Type != ArtifactTypeDocker {
			t.Errorf("Type: got %q", e.Type)
		}
		if e.Version != "v1.8.3" {
			t.Errorf("Version: got %q", e.Version)
		}
		if e.Digest != "sha256:c59e81059330a55203bf60806229a052617134d8b557c1bd83cdc69a8ece7ea2" {
			t.Errorf("Digest: got %q", e.Digest)
		}
		if e.Source.Package != "ghcr.io/fluxcd/kustomize-controller" {
			t.Errorf("Source.Package: got %q", e.Source.Package)
		}
	})

	t.Run("HelmChartVersion_InVariableDefault_ExtractsVersionAndRepo", func(t *testing.T) {
		// Given a terraform variable block declaring a helm chart version
		content := []byte(`variable "cilium_version" {
  description = "Version of the Cilium Helm chart to install."
  type        = string
  # renovate: datasource=helm depName=cilium package=cilium helmRepo=https://helm.cilium.io
  default = "1.16.3"
}
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("terraform/cni/cilium/variables.tf", content)

		// Then one helm entry is produced pointing at the configured repo
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		e := got[0]
		if e.Type != ArtifactTypeHelm {
			t.Errorf("Type: got %q", e.Type)
		}
		if e.Reference != "cilium" {
			t.Errorf("Reference: got %q", e.Reference)
		}
		if e.Version != "1.16.3" {
			t.Errorf("Version: got %q", e.Version)
		}
		if e.Repository != "https://helm.cilium.io" {
			t.Errorf("Repository: got %q", e.Repository)
		}
	})

	t.Run("TrailingCommaInHCLAssignment_IsStrippedFromVersion", func(t *testing.T) {
		// Given a terraform assignment with a trailing comma (as happens in
		// tuple/list contexts: `tags = ["v1.0.0",]` or multi-line blocks)
		content := []byte(`variable "foo_version" {
  # renovate: datasource=github-releases depName=foo package=org/foo
  default = "v1.0.0",
}
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("terraform/variables.tf", content)

		// Then the captured version does not absorb the trailing comma
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Version != "v1.0.0" {
			t.Errorf("Version: got %q, want %q", got[0].Version, "v1.0.0")
		}
	})

	t.Run("GitHubRelease_ExtractsPackageAndVersion", func(t *testing.T) {
		// Given a terraform variable declaring a github release pin
		content := []byte(`variable "flux_cli_version" {
  # renovate: datasource=github-releases depName=flux package=fluxcd/flux2
  default = "v2.3.0"
}
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("terraform/gitops/flux/variables.tf", content)

		// Then one github-release entry is produced keyed on the package path
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		e := got[0]
		if e.Type != ArtifactTypeGitHubRelease {
			t.Errorf("Type: got %q", e.Type)
		}
		if e.Reference != "fluxcd/flux2" {
			t.Errorf("Reference: got %q", e.Reference)
		}
		if e.Version != "v2.3.0" {
			t.Errorf("Version: got %q", e.Version)
		}
	})
}

func TestScanRenovateAnnotations_Transport(t *testing.T) {
	t.Run("DockerDatasource_AssignsOCITransport", func(t *testing.T) {
		// Given a docker Renovate annotation
		content := []byte(`# renovate: datasource=docker depName=nginx
tag: 1.27.0
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then the entry carries transport=oci since hydrate publishes
		// container images to an OCI registry
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Transport != ArtifactTransportOCI {
			t.Errorf("Transport: got %q, want %q", got[0].Transport, ArtifactTransportOCI)
		}
	})

	t.Run("HelmDatasource_AssignsOCITransport", func(t *testing.T) {
		// Given a helm Renovate annotation pointing at a traditional HTTP repo
		content := []byte(`# renovate: datasource=helm depName=cilium helmRepo=https://helm.cilium.io
version: 1.16.3
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then transport is still oci since Windsor always re-hosts charts to OCI
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Transport != ArtifactTransportOCI {
			t.Errorf("Transport: got %q, want %q", got[0].Transport, ArtifactTransportOCI)
		}
	})

	t.Run("GitHubReleaseDatasource_AssignsTarballTransport", func(t *testing.T) {
		// Given a github-releases Renovate annotation
		content := []byte(`# renovate: datasource=github-releases depName=flux package=fluxcd/flux2
default = "v2.3.0"
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.tf", content)

		// Then transport is tarball because release assets are raw HTTP downloads
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Transport != ArtifactTransportTarball {
			t.Errorf("Transport: got %q, want %q", got[0].Transport, ArtifactTransportTarball)
		}
	})

	t.Run("GitTagsDatasource_AssignsGitTransport", func(t *testing.T) {
		// Given a git-tags Renovate annotation
		content := []byte(`# renovate: datasource=git-tags depName=siderolabs/talos
default = "1.12.6"
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.tf", content)

		// Then transport is git
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Transport != ArtifactTransportGit {
			t.Errorf("Transport: got %q, want %q", got[0].Transport, ArtifactTransportGit)
		}
	})
}

func TestScanRenovateAnnotations_ReferenceComposition(t *testing.T) {
	t.Run("TagValueIsBareVersion_PrependsDepName", func(t *testing.T) {
		// Given a docker annotation whose tag is a bare version
		content := []byte(`# renovate: datasource=docker depName=longhornio/longhorn-engine
tag: v1.11.1@sha256:5ec00ddbc8f66911b2c8154f2c7ebf15342b4ee4508d6d2f5435aec2537e596f
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then the reference is depName:tag, no duplication
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Reference != "longhornio/longhorn-engine:v1.11.1" {
			t.Errorf("Reference: got %q", got[0].Reference)
		}
	})

	t.Run("TagValueContainsFullReference_UsedVerbatim", func(t *testing.T) {
		// Given a docker annotation whose value is already a full image ref
		// (as happens with `image: alpine:3.21.5` in Helm values)
		content := []byte(`# renovate: datasource=docker depName=alpine
image: alpine:3.21.5@sha256:5405e8f36ce1878720f71217d664aa3dea32e5e5df11acbf07fc78ef5661465b
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then the reference is the value verbatim, not "alpine:alpine:3.21.5"
		// and version carries only the tag portion, not the whole ref string
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Reference != "alpine:3.21.5" {
			t.Errorf("Reference: got %q, want %q", got[0].Reference, "alpine:3.21.5")
		}
		if got[0].Version != "3.21.5" {
			t.Errorf("Version: got %q, want %q", got[0].Version, "3.21.5")
		}
		if got[0].Digest != "sha256:5405e8f36ce1878720f71217d664aa3dea32e5e5df11acbf07fc78ef5661465b" {
			t.Errorf("Digest: got %q", got[0].Digest)
		}
	})

	t.Run("TagValueContainsRegistryPath_UsedVerbatim", func(t *testing.T) {
		// Given a docker annotation whose value contains a registry path
		content := []byte(`# renovate: datasource=docker depName=kubectl
tag: alpine/k8s:1.35.3
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then the reference is the value verbatim
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
		if got[0].Reference != "alpine/k8s:1.35.3" {
			t.Errorf("Reference: got %q, want %q", got[0].Reference, "alpine/k8s:1.35.3")
		}
	})
}

func TestScanRenovateAnnotations_IncusAliasFiltering(t *testing.T) {
	t.Run("IncusStyleAliasReference_IsExcluded", func(t *testing.T) {
		// Given a docker Renovate annotation whose value is an Incus remote
		// alias reference (e.g. "ghcr:path/name:version"). Incus aliases are
		// user-defined and not mirrorable via a generic OCI transport.
		content := []byte(`# renovate: datasource=docker depName=ghcr.io/distribution/distribution
tag: ghcr:distribution/distribution:3.0.0
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("terraform/workstation/incus/main.tf", content)

		// Then the entry is silently dropped from the manifest
		if len(got) != 0 {
			t.Fatalf("entries: got %d, want 0 (entry should be filtered)", len(got))
		}
	})

	t.Run("OCIReferenceWithSingleColon_IsRetained", func(t *testing.T) {
		// Given a docker annotation whose value is an ordinary OCI tag with
		// exactly one colon (no registry-alias prefix)
		content := []byte(`# renovate: datasource=docker depName=alpine
tag: 3.21.5@sha256:5405e8f36ce1878720f71217d664aa3dea32e5e5df11acbf07fc78ef5661465b
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then the entry is preserved — the Incus heuristic does not
		// misclassify standard tag references
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
	})

	t.Run("LocalhostPortReference_IsRetained", func(t *testing.T) {
		// Given a docker annotation whose value is a host:port/repo:tag OCI
		// reference. This pattern shares surface structure with Incus aliases
		// (two colons, bare prefix) but must not be filtered.
		content := []byte(`# renovate: datasource=docker depName=localhost:5000/foo
tag: localhost:5000/foo:v1.0.0
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then the entry is preserved because the segment after the first
		// colon is a numeric port followed by "/"
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
	})

	t.Run("OCIReferenceWithRegistryAndTag_IsRetained", func(t *testing.T) {
		// Given a docker annotation whose value is a full OCI reference with
		// a hostname (contains "."), path, and tag
		content := []byte(`# renovate: datasource=docker depName=docker.elastic.co/beats/filebeat
tag: docker.elastic.co/beats/filebeat:9.3.3
`)

		// When the scanner is invoked
		got := scanRenovateAnnotations("f.yaml", content)

		// Then the entry is preserved because the prefix contains a "."
		if len(got) != 1 {
			t.Fatalf("entries: got %d, want 1", len(got))
		}
	})
}

func TestDedupeManifestEntries(t *testing.T) {
	t.Run("SameReferenceFromMultipleFiles_CollapsesToOneEntry", func(t *testing.T) {
		// Given two entries with identical type, reference, and digest
		entries := []ManifestEntry{
			{
				Type:      ArtifactTypeDocker,
				Reference: "ghcr.io/fluxcd/kustomize-controller:v1.8.3",
				Digest:    "sha256:abc",
				Source:    ArtifactSource{File: "a.tf"},
			},
			{
				Type:      ArtifactTypeDocker,
				Reference: "ghcr.io/fluxcd/kustomize-controller:v1.8.3",
				Digest:    "sha256:abc",
				Source:    ArtifactSource{File: "b.tf"},
			},
			{
				Type:      ArtifactTypeDocker,
				Reference: "other:v1.0",
				Source:    ArtifactSource{File: "c.tf"},
			},
		}

		// When dedupeManifestEntries is invoked
		got := dedupeManifestEntries(entries)

		// Then the duplicate is removed and the first occurrence is preserved
		if len(got) != 2 {
			t.Fatalf("entries: got %d, want 2", len(got))
		}
		if got[0].Source.File != "a.tf" {
			t.Errorf("expected first occurrence preserved, got %q", got[0].Source.File)
		}
		if got[1].Reference != "other:v1.0" {
			t.Errorf("second entry Reference: got %q", got[1].Reference)
		}
	})
}
