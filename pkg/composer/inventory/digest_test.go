package inventory_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/windsorcli/cli/pkg/composer/inventory"
)

// =============================================================================
// Test Setup
// =============================================================================

// fakeDigests is a DigestLookup that returns err, or else the digest of each reference:version in digests.
type fakeDigests struct {
	mu      sync.Mutex
	digests map[string]string
	err     error
	calls   []string
}

// Digest records the lookup of e and returns its digest.
func (f *fakeDigests) Digest(e inventory.Entry) (string, error) {
	key := e.Reference + ":" + e.Version
	f.mu.Lock()
	f.calls = append(f.calls, key)
	f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	digest, ok := f.digests[key]
	if !ok {
		return "", fmt.Errorf("no digest for %s", key)
	}
	return digest, nil
}

// sortedCalls returns the lookups that f received, sorted.
func (f *fakeDigests) sortedCalls() []string {
	calls := append([]string(nil), f.calls...)
	sort.Strings(calls)
	return calls
}

// startRegistry starts an in-memory OCI registry and returns its host.
func startRegistry(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

// pushImage writes img to ref and returns its digest.
func pushImage(t *testing.T, ref string, img v1.Image) string {
	t.Helper()
	parsed, err := name.ParseReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(parsed, img); err != nil {
		t.Fatal(err)
	}
	digest, err := img.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest.String()
}

// pushIndex writes idx to ref and returns its digest.
func pushIndex(t *testing.T, ref string, idx v1.ImageIndex) string {
	t.Helper()
	parsed, err := name.ParseReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.WriteIndex(parsed, idx); err != nil {
		t.Fatal(err)
	}
	digest, err := idx.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest.String()
}

// startChartRepo serves files as a Helm repository and counts the requests for index.yaml.
func startChartRepo(t *testing.T, files map[string]string) (string, *atomic.Int32) {
	t.Helper()
	var indexRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			indexRequests.Add(1)
		}
		content, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(server.Close)
	return server.URL, &indexRequests
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRegistryDigests_Digest(t *testing.T) {
	t.Run("ReturnsTheIndexDigestOfAMultiPlatformImage", func(t *testing.T) {
		// Given a registry that holds an image index at example/app:1.0.0
		host := startRegistry(t)
		idx, err := random.Index(64, 1, 2)
		if err != nil {
			t.Fatal(err)
		}
		want := pushIndex(t, host+"/example/app:1.0.0", idx)
		entry := inventory.Entry{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: host + "/example/app", Version: "1.0.0"}

		// When the digest of the image is read
		got, err := inventory.NewRegistryDigests().Digest(entry)

		// Then it is the digest of the index
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	})

	t.Run("ReturnsTheManifestDigestOfAnOCIChart", func(t *testing.T) {
		// Given a registry that holds a chart at charts/openebs:4.6.2
		host := startRegistry(t)
		img, err := random.Image(64, 1)
		if err != nil {
			t.Fatal(err)
		}
		want := pushImage(t, host+"/charts/openebs:4.6.2", img)
		entry := inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportOCI, Reference: host + "/charts/openebs", Name: "openebs", Version: "4.6.2"}

		// When the digest of the chart is read
		got, err := inventory.NewRegistryDigests().Digest(entry)

		// Then it is the digest of the manifest
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	})

	t.Run("ReturnsErrorWhenTheTagIsNotInTheRegistry", func(t *testing.T) {
		// Given an empty registry
		host := startRegistry(t)
		entry := inventory.Entry{Kind: inventory.KindImage, Transport: inventory.TransportOCI, Reference: host + "/example/app", Version: "1.0.0"}

		// When the digest of a tag is read
		_, err := inventory.NewRegistryDigests().Digest(entry)

		// Then the error names the reference
		if err == nil || !strings.Contains(err.Error(), host+"/example/app:1.0.0") {
			t.Errorf("expected an error that names the reference, got %v", err)
		}
	})

	t.Run("ReturnsTheIndexDigestOfAnHTTPChartAndReadsTheIndexOnce", func(t *testing.T) {
		// Given a Helm repository whose index gives cert-manager v1.17.0 and v1.18.0 a digest
		repo, indexRequests := startChartRepo(t, map[string]string{
			"/index.yaml": "apiVersion: v1\nentries:\n  cert-manager:\n  - version: v1.18.0\n    digest: " + strings.Repeat("b", 64) +
				"\n  - version: v1.17.0\n    digest: " + strings.Repeat("a", 64) + "\n",
		})
		digests := inventory.NewRegistryDigests()

		// When the digests of 1.17.0 and v1.18.0 are read
		first, err := digests.Digest(inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportHTTP, Reference: repo, Name: "cert-manager", Version: "1.17.0"})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		second, err := digests.Digest(inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportHTTP, Reference: repo, Name: "cert-manager", Version: "v1.18.0"})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then each is the index digest of its version, and the index is read once
		if first != "sha256:"+strings.Repeat("a", 64) || second != "sha256:"+strings.Repeat("b", 64) {
			t.Errorf("expected the index digests, got %s and %s", first, second)
		}
		if n := indexRequests.Load(); n != 1 {
			t.Errorf("expected one index request, got %d", n)
		}
	})

	t.Run("HashesTheArchiveWhenTheIndexGivesNoDigest", func(t *testing.T) {
		// Given a Helm repository whose index gives a relative archive URL and no digest
		archive := "chart archive bytes"
		repo, _ := startChartRepo(t, map[string]string{
			"/index.yaml":           "entries:\n  app:\n  - version: 1.0.0\n    urls:\n    - charts/app-1.0.0.tgz\n",
			"/charts/app-1.0.0.tgz": archive,
		})
		sum := sha256.Sum256([]byte(archive))

		// When the digest of the chart is read
		got, err := inventory.NewRegistryDigests().Digest(inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportHTTP, Reference: repo, Name: "app", Version: "1.0.0"})

		// Then it is the sha256 of the archive
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if want := "sha256:" + hex.EncodeToString(sum[:]); got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	})

	t.Run("ReturnsErrorWhenTheChartVersionIsNotInTheIndex", func(t *testing.T) {
		// Given a Helm repository whose index has no app 2.0.0
		repo, _ := startChartRepo(t, map[string]string{"/index.yaml": "entries:\n  app:\n  - version: 1.0.0\n    digest: abc\n"})

		// When the digest of app 2.0.0 is read
		_, err := inventory.NewRegistryDigests().Digest(inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportHTTP, Reference: repo, Name: "app", Version: "2.0.0"})

		// Then the error names the chart and its version
		if err == nil || !strings.Contains(err.Error(), "chart app 2.0.0 is not in the index") {
			t.Errorf("expected a not-in-index error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenTheIndexIsMissing", func(t *testing.T) {
		// Given a Helm repository with no index
		repo, _ := startChartRepo(t, nil)

		// When the digest of a chart is read
		_, err := inventory.NewRegistryDigests().Digest(inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportHTTP, Reference: repo, Name: "app", Version: "1.0.0"})

		// Then the error gives the status of the download
		if err == nil || !strings.Contains(err.Error(), "404") {
			t.Errorf("expected a 404 error, got %v", err)
		}
	})

	t.Run("ReturnsErrorForAGitChart", func(t *testing.T) {
		// Given a chart in a Git repository
		entry := inventory.Entry{Kind: inventory.KindChart, Transport: inventory.TransportGit, Reference: "https://github.com/example/charts", Name: "app", Version: "1.0.0"}

		// When its digest is read
		_, err := inventory.NewRegistryDigests().Digest(entry)

		// Then it returns an error
		if err == nil {
			t.Error("expected an error")
		}
	})
}
