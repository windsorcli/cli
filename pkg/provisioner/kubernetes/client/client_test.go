package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Helpers
// =============================================================================

// writeTestKubeconfig writes a minimal kubeconfig pointing at server, setting the file's mtime
// explicitly so tests control staleness detection without depending on filesystem mtime
// resolution or real wall-clock delay between writes.
func writeTestKubeconfig(t *testing.T, path, server string, modTime time.Time) {
	t.Helper()
	content := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: %s
contexts:
- name: test
  context:
    cluster: test
current-context: test
`, server)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("failed to set kubeconfig mtime: %v", err)
	}
}

// newCountingServer returns a test server that answers every request with a minimal valid
// resource body, incrementing hits so a test can tell which server actually received a call.
func newCountingServer(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"apiVersion":"example.com/v1","kind":"Widget","metadata":{"name":"x"}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// =============================================================================
// Private Methods
// =============================================================================

func TestDynamicKubernetesClient_ensureClient(t *testing.T) {
	widgetGVR := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}

	t.Run("RebuildsWhenTheKubeconfigFileChanges", func(t *testing.T) {
		// Given a client built from a kubeconfig pointing at one server
		var hitsA, hitsB int32
		serverA := newCountingServer(t, &hitsA)
		serverB := newCountingServer(t, &hitsB)
		path := filepath.Join(t.TempDir(), "kubeconfig")
		base := time.Now()
		writeTestKubeconfig(t, path, serverA.URL, base)
		t.Setenv("KUBECONFIG", path)

		c := NewDynamicKubernetesClient(nil)
		if _, err := c.GetResource(widgetGVR, "default", "x"); err != nil {
			t.Fatalf("expected no error against the first server, got: %v", err)
		}
		if got := atomic.LoadInt32(&hitsA); got != 1 {
			t.Fatalf("expected 1 hit on the first server, got %d", got)
		}

		// When the kubeconfig file changes to point at a different server
		writeTestKubeconfig(t, path, serverB.URL, base.Add(time.Hour))
		if _, err := c.GetResource(widgetGVR, "default", "x"); err != nil {
			t.Fatalf("expected no error against the second server, got: %v", err)
		}

		// Then the client rebuilt and reached the new server, not the stale one
		if got := atomic.LoadInt32(&hitsB); got != 1 {
			t.Fatalf("expected 1 hit on the second server after the kubeconfig changed, got %d", got)
		}
		if got := atomic.LoadInt32(&hitsA); got != 1 {
			t.Fatalf("expected no further hits on the first server after the kubeconfig changed, got %d", got)
		}
	})

	t.Run("KeepsTheLastKnownGoodClientWhenARebuildAttemptFails", func(t *testing.T) {
		// Given a working client built from a valid kubeconfig
		var hits int32
		server := newCountingServer(t, &hits)
		path := filepath.Join(t.TempDir(), "kubeconfig")
		base := time.Now()
		writeTestKubeconfig(t, path, server.URL, base)
		t.Setenv("KUBECONFIG", path)

		c := NewDynamicKubernetesClient(nil)
		if _, err := c.GetResource(widgetGVR, "default", "x"); err != nil {
			t.Fatalf("expected no error on the first call, got: %v", err)
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("expected 1 hit, got %d", got)
		}

		// When the kubeconfig file changes to something unparsable, simulating a caller
		// catching it mid-rewrite
		if err := os.WriteFile(path, []byte("not: [valid: kubeconfig"), 0600); err != nil {
			t.Fatalf("failed to corrupt kubeconfig: %v", err)
		}
		if err := os.Chtimes(path, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
			t.Fatalf("failed to set kubeconfig mtime: %v", err)
		}

		// Then GetResource still succeeds, served by the last known-good client, rather
		// than failing outright over a rebuild attempt that could not even parse the file
		if _, err := c.GetResource(widgetGVR, "default", "x"); err != nil {
			t.Fatalf("expected the rebuild failure to fall back to the working client, got: %v", err)
		}
		if got := atomic.LoadInt32(&hits); got != 2 {
			t.Fatalf("expected a second hit on the original server, got %d", got)
		}
	})

	t.Run("SurfacesAnErrorWhenTheKubeconfigIsConfirmedDeleted", func(t *testing.T) {
		// Given a working client built from a valid kubeconfig
		var hits int32
		server := newCountingServer(t, &hits)
		path := filepath.Join(t.TempDir(), "kubeconfig")
		writeTestKubeconfig(t, path, server.URL, time.Now())
		t.Setenv("KUBECONFIG", path)

		c := NewDynamicKubernetesClient(nil)
		if _, err := c.GetResource(widgetGVR, "default", "x"); err != nil {
			t.Fatalf("expected no error on the first call, got: %v", err)
		}

		// When the kubeconfig file is removed entirely, rather than merely unreadable for a
		// moment
		if err := os.Remove(path); err != nil {
			t.Fatalf("failed to remove kubeconfig: %v", err)
		}

		// Then the next call surfaces the failure instead of silently reusing a client
		// pointed at a cluster whose kubeconfig is now confirmed gone
		if _, err := c.GetResource(widgetGVR, "default", "x"); err == nil {
			t.Fatal("expected an error once the kubeconfig is confirmed gone, got nil")
		}
	})

	t.Run("RebuildsWhenTheEndpointOverrideChanges", func(t *testing.T) {
		// Given a client whose CheckHealth call set an explicit endpoint
		var hitsA, hitsB int32
		serverA := newCountingServer(t, &hitsA)
		serverB := newCountingServer(t, &hitsB)

		c := NewDynamicKubernetesClient(nil)
		if err := c.CheckHealth(t.Context(), serverA.URL); err != nil {
			t.Fatalf("expected no error against the first endpoint, got: %v", err)
		}
		if got := atomic.LoadInt32(&hitsA); got != 1 {
			t.Fatalf("expected 1 hit on the first endpoint, got %d", got)
		}

		// When CheckHealth runs again against a different endpoint
		if err := c.CheckHealth(t.Context(), serverB.URL); err != nil {
			t.Fatalf("expected no error against the second endpoint, got: %v", err)
		}

		// Then the client rebuilt and reached the new endpoint, not the stale one
		if got := atomic.LoadInt32(&hitsB); got != 1 {
			t.Fatalf("expected 1 hit on the second endpoint after the override changed, got %d", got)
		}
		if got := atomic.LoadInt32(&hitsA); got != 1 {
			t.Fatalf("expected no further hits on the first endpoint after the override changed, got %d", got)
		}
	})

	t.Run("DoesNotFallBackWhenANewEndpointFailsToBuild", func(t *testing.T) {
		// Given a client already checked against a valid endpoint
		var hits int32
		server := newCountingServer(t, &hits)

		c := NewDynamicKubernetesClient(nil)
		if err := c.CheckHealth(t.Context(), server.URL); err != nil {
			t.Fatalf("expected no error against the first endpoint, got: %v", err)
		}

		// When CheckHealth is asked to check a different, malformed endpoint
		err := c.CheckHealth(t.Context(), "not a url at all with spaces")

		// Then it fails outright rather than silently reporting the old endpoint's health:
		// the caller asked about this endpoint specifically, so falling back would misreport it
		if err == nil {
			t.Fatal("expected an error for the malformed endpoint, got nil")
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("expected no further hits on the original server, got %d", got)
		}
	})

	t.Run("ConcurrentCallsAcrossARebuildDoNotRace", func(t *testing.T) {
		// Given a client mid-rebuild, with several goroutines calling it at once
		var hits int32
		server := newCountingServer(t, &hits)
		path := filepath.Join(t.TempDir(), "kubeconfig")
		base := time.Now()
		writeTestKubeconfig(t, path, server.URL, base)
		t.Setenv("KUBECONFIG", path)

		c := NewDynamicKubernetesClient(nil)
		if _, err := c.GetResource(widgetGVR, "default", "x"); err != nil {
			t.Fatalf("expected no error on the first call, got: %v", err)
		}
		writeTestKubeconfig(t, path, server.URL, base.Add(time.Hour))

		// When many goroutines call GetResource while the rebuild is triggered
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = c.GetResource(widgetGVR, "default", "x")
			}()
		}
		wg.Wait()

		// Then nothing races (run with -race to check); this test's assertion is the absence
		// of a race report, not a specific hit count
	})
}

func TestWarningHandlerFor(t *testing.T) {
	t.Run("SuppressesWhenNotVerbose", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.IsVerboseFunc = func() bool { return false }

		handler := warningHandlerFor(mockShell)

		if _, ok := handler.(rest.NoWarnings); !ok {
			t.Errorf("Expected rest.NoWarnings, got %T", handler)
		}
	})

	t.Run("PassesThroughWhenVerbose", func(t *testing.T) {
		mockShell := shell.NewMockShell()
		mockShell.IsVerboseFunc = func() bool { return true }

		handler := warningHandlerFor(mockShell)

		if handler != nil {
			t.Errorf("Expected nil handler, got %T", handler)
		}
	})

	t.Run("SuppressesWhenShellNil", func(t *testing.T) {
		handler := warningHandlerFor(nil)

		if _, ok := handler.(rest.NoWarnings); !ok {
			t.Errorf("Expected rest.NoWarnings, got %T", handler)
		}
	})
}

func TestDynamicKubernetesClient_GetNodeReadyStatus(t *testing.T) {
	newNodeListServer := func(t *testing.T) *httptest.Server {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"apiVersion": "v1",
				"kind": "NodeList",
				"items": [
					{
						"metadata": {"name": "controlplane-2"},
						"status": {
							"addresses": [
								{"type": "InternalIP", "address": "192.168.3.23"},
								{"type": "Hostname", "address": "controlplane-2"}
							],
							"conditions": [{"type": "Ready", "status": "True"}]
						}
					},
					{
						"metadata": {"name": "worker-1"},
						"status": {
							"addresses": [
								{"type": "InternalIP", "address": "192.168.3.24"},
								{"type": "ExternalIP", "address": "203.0.113.10"}
							],
							"conditions": [{"type": "Ready", "status": "False"}]
						}
					}
				]
			}`))
		}))
		t.Cleanup(server.Close)
		return server
	}

	t.Run("MatchesInternalAndExternalIPAddresses", func(t *testing.T) {
		// Given a cluster whose node names differ from their IP addresses
		server := newNodeListServer(t)
		path := filepath.Join(t.TempDir(), "kubeconfig")
		writeTestKubeconfig(t, path, server.URL, time.Now())
		t.Setenv("KUBECONFIG", path)
		c := NewDynamicKubernetesClient(nil)

		// When checking readiness by InternalIP, ExternalIP, and an unmatched address
		status, err := c.GetNodeReadyStatus(t.Context(), []string{"192.168.3.23", "203.0.113.10", "192.168.3.99"})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Then matched IP addresses report Ready status keyed by each address and unknown ones are omitted
		if len(status) != 2 {
			t.Fatalf("expected 2 entries, got %v", status)
		}
		if ready, ok := status["192.168.3.23"]; !ok || !ready {
			t.Errorf("expected 192.168.3.23 to be true, got %v (present=%v)", ready, ok)
		}
		if ready, ok := status["203.0.113.10"]; !ok || ready {
			t.Errorf("expected 203.0.113.10 to be false, got %v (present=%v)", ready, ok)
		}
		if _, ok := status["192.168.3.99"]; ok {
			t.Errorf("expected unknown node 192.168.3.99 to be omitted")
		}
	})

	t.Run("MatchesByNodeNameAndListsAllWhenEmpty", func(t *testing.T) {
		// Given a cluster with named nodes
		server := newNodeListServer(t)
		path := filepath.Join(t.TempDir(), "kubeconfig")
		writeTestKubeconfig(t, path, server.URL, time.Now())
		t.Setenv("KUBECONFIG", path)
		c := NewDynamicKubernetesClient(nil)

		// When querying by node name or with an empty filter
		byName, err := c.GetNodeReadyStatus(t.Context(), []string{"controlplane-2"})
		if err != nil {
			t.Fatalf("expected no error querying by name, got: %v", err)
		}
		if len(byName) != 1 || !byName["controlplane-2"] {
			t.Fatalf("expected controlplane-2=true, got %v", byName)
		}

		all, err := c.GetNodeReadyStatus(t.Context(), nil)
		if err != nil {
			t.Fatalf("expected no error listing all nodes, got: %v", err)
		}
		if len(all) != 2 || !all["controlplane-2"] || all["worker-1"] {
			t.Fatalf("expected controlplane-2=true and worker-1=false, got %v", all)
		}
	})
}
