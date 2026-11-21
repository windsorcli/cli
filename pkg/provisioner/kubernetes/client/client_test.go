package client

import (
	"context"
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

// newNodeListServer returns a test server that answers every request with body as a NodeList.
func newNodeListServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// =============================================================================
// Public Methods
// =============================================================================

func TestDynamicKubernetesClient_GetNodeReadyStatus(t *testing.T) {
	nodeList := `{"apiVersion":"v1","kind":"NodeList","items":[
{"apiVersion":"v1","kind":"Node","metadata":{"name":"controlplane-1"},"status":{
"addresses":[{"type":"InternalIP","address":"192.168.3.21"},{"type":"Hostname","address":"controlplane-1"}],
"conditions":[{"type":"Ready","status":"True"}]}},
{"apiVersion":"v1","kind":"Node","metadata":{"name":"controlplane-2"},"status":{
"addresses":[{"type":"InternalIP","address":"10.0.0.22"},{"type":"ExternalIP","address":"192.168.3.22"}],
"conditions":[{"type":"Ready","status":"False"}]}}
]}`

	setup := func(t *testing.T) *DynamicKubernetesClient {
		t.Helper()
		server := newNodeListServer(t, nodeList)
		path := filepath.Join(t.TempDir(), "kubeconfig")
		writeTestKubeconfig(t, path, server.URL, time.Now())
		t.Setenv("KUBECONFIG", path)
		return NewDynamicKubernetesClient(nil)
	}

	t.Run("MatchesNodesByInternalIP", func(t *testing.T) {
		// Given a cluster whose node names differ from their addresses
		c := setup(t)

		// When the status is requested by InternalIP
		status, err := c.GetNodeReadyStatus(context.Background(), []string{"192.168.3.21"})

		// Then the node is found under the requested address
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if ready, ok := status["192.168.3.21"]; !ok || !ready {
			t.Errorf("expected 192.168.3.21 to be Ready, got %v (found=%v)", ready, ok)
		}
		if len(status) != 1 {
			t.Errorf("expected 1 entry, got %v", status)
		}
	})

	t.Run("MatchesNodesByExternalIP", func(t *testing.T) {
		// Given a node with an ExternalIP
		c := setup(t)

		// When the status is requested by ExternalIP
		status, err := c.GetNodeReadyStatus(context.Background(), []string{"192.168.3.22"})

		// Then the node is found and reports its Ready condition
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if ready, ok := status["192.168.3.22"]; !ok || ready {
			t.Errorf("expected 192.168.3.22 to be found and NotReady, got %v (found=%v)", ready, ok)
		}
	})

	t.Run("MatchesNodesByName", func(t *testing.T) {
		// Given a cluster with named nodes
		c := setup(t)

		// When the status is requested by node name
		status, err := c.GetNodeReadyStatus(context.Background(), []string{"controlplane-1"})

		// Then the node is found under its name
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if ready, ok := status["controlplane-1"]; !ok || !ready {
			t.Errorf("expected controlplane-1 to be Ready, got %v (found=%v)", ready, ok)
		}
	})

	t.Run("OmitsValuesThatMatchNoNode", func(t *testing.T) {
		// Given a cluster with no node at the requested address
		c := setup(t)

		// When the status is requested by an unknown address
		status, err := c.GetNodeReadyStatus(context.Background(), []string{"192.168.3.99"})

		// Then the result omits the value
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(status) != 0 {
			t.Errorf("expected no entries, got %v", status)
		}
	})

	t.Run("ReturnsAllNodesByNameWhenNoneAreRequested", func(t *testing.T) {
		// Given a cluster with two nodes
		c := setup(t)

		// When the status is requested with no values
		status, err := c.GetNodeReadyStatus(context.Background(), nil)

		// Then the result has every node under its name
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(status) != 2 || !status["controlplane-1"] || status["controlplane-2"] {
			t.Errorf("expected controlplane-1 Ready and controlplane-2 NotReady, got %v", status)
		}
	})
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
