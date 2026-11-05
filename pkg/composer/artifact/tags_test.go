package artifact_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestScanTags(t *testing.T) {
	t.Run("ReadsAnImageReferenceInYAML", func(t *testing.T) {
		// Given an image tag above a line that holds a full image reference
		content := []byte("# artifact: type=image name=ghcr.io/fluxcd/source-controller\nimage: ghcr.io/fluxcd/source-controller:v1.6.0\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("kustomize/flux/values.yaml", content)

		// Then the tag has the name, the version and the version line
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		want := artifact.ArtifactTag{Type: artifact.ArtifactTagImage, Name: "ghcr.io/fluxcd/source-controller", Version: "v1.6.0", File: "kustomize/flux/values.yaml", Line: 2}
		if len(tags) != 1 || tags[0] != want {
			t.Fatalf("expected %+v, got %+v", want, tags)
		}
	})

	t.Run("ReadsAChartVersionAndRepo", func(t *testing.T) {
		// Given a chart tag above a version line
		content := []byte("  # artifact: type=chart name=cert-manager repo=https://charts.jetstack.io\n  version: v1.17.0\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("kustomize/cert-manager/release.yaml", content)

		// Then the tag has the repo and the version
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(tags) != 1 || tags[0].Repo != "https://charts.jetstack.io" || tags[0].Version != "v1.17.0" {
			t.Fatalf("unexpected tags %+v", tags)
		}
	})

	t.Run("ReadsHCLWithEitherCommentStyle", func(t *testing.T) {
		// Given two tags in HCL, one with // and one with #
		content := []byte("variable \"a\" {\n  // artifact: type=image name=alpine/k8s\n  default = \"1.30.0\"\n}\nvariable \"b\" {\n  # artifact: type=chart name=cilium repo=oci://quay.io/cilium/charts\n  default = \"1.17.2\"\n}\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("terraform/gate/variables.tf", content)

		// Then both tags have their versions
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(tags) != 2 || tags[0].Version != "1.30.0" || tags[1].Version != "1.17.2" {
			t.Fatalf("unexpected tags %+v", tags)
		}
	})

	t.Run("SplitsADigestFromTheVersion", func(t *testing.T) {
		// Given a version line with an @sha256 digest
		content := []byte("# artifact: type=image name=ghcr.io/example/app\ntag: v2.0.0@sha256:abc123\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("kustomize/app/values.yaml", content)

		// Then the version and the digest are separate
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if tags[0].Version != "v2.0.0" || tags[0].Digest != "sha256:abc123" {
			t.Fatalf("unexpected tag %+v", tags[0])
		}
	})

	t.Run("SkipsCommentsAndBlankLinesBelowTheTag", func(t *testing.T) {
		// Given a tag followed by a comment and a blank line before the version
		content := []byte("# artifact: type=chart name=redis repo=https://charts.example.com\n# renovate: datasource=helm depName=redis\n\nversion: \"20.6.1\" # pinned\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("kustomize/redis/release.yaml", content)

		// Then the version is the value on the first other line, without quotes or comment
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if tags[0].Version != "20.6.1" || tags[0].Line != 4 {
			t.Fatalf("unexpected tag %+v", tags[0])
		}
	})

	t.Run("BindsATrailingTagToItsOwnLine", func(t *testing.T) {
		// Given a tag at the end of a line, and a value on the line above it
		content := []byte("other: v0.0.1\nimage: ghcr.io/fluxcd/source-controller:v1.6.0  # artifact: type=image\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("kustomize/flux/values.yaml", content)

		// Then the tag binds to its own line and takes the name from the reference
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(tags) != 1 || tags[0].Name != "ghcr.io/fluxcd/source-controller" || tags[0].Version != "v1.6.0" || tags[0].Line != 2 {
			t.Fatalf("unexpected tags %+v", tags)
		}
	})

	t.Run("NeverBindsToALineAboveTheTag", func(t *testing.T) {
		// Given a tag-only comment with a value line above it and nothing below it
		content := []byte("image: ghcr.io/example/app:v1\n# artifact: type=image name=ghcr.io/example/app\n")

		// When the content is scanned
		_, err := artifact.ScanTags("kustomize/app/values.yaml", content)

		// Then the tag has no value to bind to
		if err == nil || !strings.Contains(err.Error(), "kustomize/app/values.yaml:2") {
			t.Fatalf("expected an error for line 2, got %v", err)
		}
	})

	t.Run("KeepsTheDigestOfAFullReference", func(t *testing.T) {
		// Given a tag with no name above a full reference with a digest
		content := []byte("# artifact: type=image\nimage: ghcr.io/example/app:v2.0.0@sha256:abc123\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("kustomize/app/values.yaml", content)

		// Then the name, version and digest are separate
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		got := tags[0]
		if got.Name != "ghcr.io/example/app" || got.Version != "v2.0.0" || got.Digest != "sha256:abc123" {
			t.Fatalf("unexpected tag %+v", got)
		}
	})

	t.Run("ReadsTheNewTagOfAKustomizeImageEntry", func(t *testing.T) {
		// Given a kustomize images entry with the tag above newTag
		content := []byte("images:\n  - name: ghcr.io/fluxcd/source-controller\n    # artifact: type=image name=ghcr.io/fluxcd/source-controller\n    newTag: v1.6.0\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("kustomize/flux/kustomization.yaml", content)

		// Then the version is the newTag value
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(tags) != 1 || tags[0].Version != "v1.6.0" {
			t.Fatalf("unexpected tags %+v", tags)
		}
	})

	t.Run("IgnoresABlockComment", func(t *testing.T) {
		// Given a tag inside a block comment
		content := []byte("/* artifact: type=image name=ghcr.io/example/app */\nversion = \"v1\"\n")

		// When the content is scanned
		tags, err := artifact.ScanTags("terraform/app/main.tf", content)

		// Then there are no tags
		if err != nil || len(tags) != 0 {
			t.Fatalf("expected no tags and no error, got %+v, %v", tags, err)
		}
	})

	t.Run("ReturnsAnErrorForAMalformedTag", func(t *testing.T) {
		cases := map[string]string{
			"UnknownType":                 "# artifact: type=binary name=tool\nversion: 1.0.0\n",
			"ImageWithoutNameOrReference": "# artifact: type=image\ntag: v1\n",
			"ChartWithoutName":            "# artifact: type=chart repo=https://charts.example.com\nversion: 1.0.0\n",
			"ChartWithoutRepo":            "# artifact: type=chart name=redis\nversion: 1.0.0\n",
			"ChartWithBadRepoScheme":      "# artifact: type=chart name=redis repo=charts.example.com\nversion: 1.0.0\n",
			"NoValueLine":                 "# artifact: type=image name=a/b\n",
			"ReferenceWithoutTag":         "# artifact: type=image\nimage: ghcr.io/example/app\n",
		}
		for name, content := range cases {
			t.Run(name, func(t *testing.T) {
				// Given a malformed tag
				// When the content is scanned
				_, err := artifact.ScanTags("kustomize/x.yaml", []byte(content))

				// Then the error names the file and line
				if err == nil || !strings.Contains(err.Error(), "kustomize/x.yaml:1") {
					t.Fatalf("expected an error that names the file and line, got %v", err)
				}
			})
		}
	})
}

func TestScanTagsDir(t *testing.T) {
	t.Run("ReadsKustomizeAndTerraformFilesAndSkipsTheRest", func(t *testing.T) {
		// Given a source tree with tags in kustomize, terraform, a .terraform cache and a template
		root := t.TempDir()
		write := func(rel, content string) {
			path := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		write("kustomize/a/values.yaml", "# artifact: type=image name=ghcr.io/example/a\nimage: ghcr.io/example/a:v1\n")
		write("terraform/b/main.tf", "# artifact: type=image name=ghcr.io/example/b\ndefault = \"v2\"\n")
		write("terraform/b/.terraform/modules/c.tf", "# artifact: type=image name=ghcr.io/example/c\ndefault = \"v3\"\n")
		write("contexts/_template/values.yaml", "# artifact: type=image name=ghcr.io/example/d\nimage: ghcr.io/example/d:v4\n")

		// When the tree is scanned
		tags, err := artifact.ScanTagsDir(root)

		// Then only the kustomize and terraform tags are returned, sorted by file
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(tags) != 2 || tags[0].File != "kustomize/a/values.yaml" || tags[1].File != "terraform/b/main.tf" {
			t.Fatalf("unexpected tags %+v", tags)
		}
	})

	t.Run("ReturnsNoTagsForARootWithoutSourceDirectories", func(t *testing.T) {
		// Given an empty root
		// When the root is scanned
		tags, err := artifact.ScanTagsDir(t.TempDir())

		// Then there are no tags and no error
		if err != nil || len(tags) != 0 {
			t.Fatalf("expected no tags and no error, got %+v, %v", tags, err)
		}
	})

	t.Run("ReturnsAnErrorForAMalformedTag", func(t *testing.T) {
		// Given a file with a tag that has no type
		root := t.TempDir()
		path := filepath.Join(root, "kustomize", "x.yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# artifact: name=a\nimage: a:1\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		// When the root is scanned
		_, err := artifact.ScanTagsDir(root)

		// Then the error names the file
		if err == nil || !strings.Contains(err.Error(), "kustomize/x.yaml:1") {
			t.Fatalf("expected an error that names the file, got %v", err)
		}
	})
}
