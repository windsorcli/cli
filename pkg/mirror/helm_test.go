package mirror

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// =============================================================================
// Test Setup
// =============================================================================

// newHTTPResponse builds a minimal *http.Response suitable for injection into
// the HttpGet shim.
func newHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		Header:     make(http.Header),
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestHelm_resolveHelmChart(t *testing.T) {
	t.Run("FindsChartAndVersion", func(t *testing.T) {
		// Given an index.yaml with two versions of a chart
		index := `entries:
  mychart:
    - name: mychart
      version: 1.2.3
      apiVersion: v2
      appVersion: "1.2.3"
      urls: [https://cdn.example.com/mychart-1.2.3.tgz]
    - name: mychart
      version: 1.2.2
      urls: [https://cdn.example.com/mychart-1.2.2.tgz]
`
		shims := NewShims()
		shims.HttpGet = func(url string) (*http.Response, error) {
			if !strings.HasSuffix(url, "/index.yaml") {
				t.Fatalf("unexpected URL %s", url)
			}
			return newHTTPResponse(200, index), nil
		}
		shims.YamlUnmarshal = yaml.Unmarshal

		// When resolving version 1.2.3
		tgz, entry, err := resolveHelmChart(shims, "https://helm.example.com", "mychart", "1.2.3")

		// Then the canonical URL and entry are returned
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if tgz != "https://cdn.example.com/mychart-1.2.3.tgz" {
			t.Errorf("unexpected tgz %q", tgz)
		}
		if entry.Name != "mychart" || entry.Version != "1.2.3" {
			t.Errorf("entry mismatch: %+v", entry)
		}
	})

	t.Run("ErrorsWhenVersionMissing", func(t *testing.T) {
		shims := NewShims()
		shims.HttpGet = func(url string) (*http.Response, error) {
			return newHTTPResponse(200, "entries: {}\n"), nil
		}
		shims.YamlUnmarshal = yaml.Unmarshal

		_, _, err := resolveHelmChart(shims, "https://helm.example.com", "mychart", "9.9.9")
		if err == nil {
			t.Fatal("expected error for missing chart, got nil")
		}
	})
}

func TestHelm_absoluteURL(t *testing.T) {
	t.Run("AbsoluteLeftAlone", func(t *testing.T) {
		got := absoluteURL("https://helm.example.com", "https://cdn.example.com/x.tgz")
		if got != "https://cdn.example.com/x.tgz" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("RelativeResolvedAgainstRepo", func(t *testing.T) {
		got := absoluteURL("https://helm.example.com/charts", "mychart-1.0.0.tgz")
		if got != "https://helm.example.com/charts/mychart-1.0.0.tgz" {
			t.Errorf("got %q", got)
		}
	})
}

func TestHelm_buildHelmOCIImage(t *testing.T) {
	t.Run("ConfigBlobMatchesHelmFormat", func(t *testing.T) {
		// Given a helm index entry and chart content
		entry := helmIndexEntry{Name: "widget", Version: "1.0.0", APIVersion: "v2", AppVersion: "1.0.0"}
		chart := []byte("fake chart tarball bytes")

		// When building the OCI image
		img, err := buildHelmOCIImage(entry, chart, nil)
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		// Then the raw config blob parses as Helm's canonical chart config JSON
		raw, err := img.RawConfigFile()
		if err != nil {
			t.Fatalf("raw config: %v", err)
		}
		var cfg helmChartConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("unmarshal helm config: %v", err)
		}
		if cfg.Name != "widget" || cfg.Version != "1.0.0" || cfg.APIVersion != "v2" {
			t.Errorf("unexpected helm config: %+v", cfg)
		}

		// And the image advertises one layer with the helm chart media type
		layers, err := img.Layers()
		if err != nil {
			t.Fatalf("layers: %v", err)
		}
		if len(layers) != 1 {
			t.Fatalf("expected 1 layer, got %d", len(layers))
		}
		mt, err := layers[0].MediaType()
		if err != nil {
			t.Fatalf("mediatype: %v", err)
		}
		if string(mt) != helmChartMediaType {
			t.Errorf("expected media type %q, got %q", helmChartMediaType, mt)
		}
	})

	t.Run("IncludesProvenanceLayerWhenPresent", func(t *testing.T) {
		entry := helmIndexEntry{Name: "widget", Version: "1.0.0"}
		img, err := buildHelmOCIImage(entry, []byte("chart"), []byte("prov"))
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		layers, err := img.Layers()
		if err != nil {
			t.Fatalf("layers: %v", err)
		}
		if len(layers) != 2 {
			t.Fatalf("expected 2 layers, got %d", len(layers))
		}
		mt, _ := layers[1].MediaType()
		if string(mt) != helmProvenanceMediaType {
			t.Errorf("expected provenance media type, got %q", mt)
		}
	})
}


// =============================================================================
// Test Helpers
// =============================================================================

// _ is a sink for unused v1.Image values in tests.
var _ = v1.Image(nil)
