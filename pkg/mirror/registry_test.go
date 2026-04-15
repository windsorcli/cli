package mirror

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

// newRegistryTestShims returns shims that track HttpGet + Sleep calls and
// return an ever-advancing time so waitReady completes deterministically.
func newRegistryTestShims(getFn func(string) (*http.Response, error)) *Shims {
	s := NewShims()
	s.HttpGet = getFn
	s.Sleep = func(d time.Duration) {}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { t0 = t0.Add(100 * time.Millisecond); return t0 }
	return s
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestRegistry_EnsureRunning(t *testing.T) {
	t.Run("StartsContainerWhenAbsent", func(t *testing.T) {
		// Given docker inspect reports the container does not exist
		m := shell.NewMockShell()
		var ranContainer bool
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && len(args) > 0 && args[0] == "inspect" {
				return "", errors.New("Error: No such object: windsor-mirror")
			}
			if cmd == "docker" && len(args) > 0 && args[0] == "run" {
				ranContainer = true
				return "", nil
			}
			return "", nil
		}
		shims := newRegistryTestShims(func(url string) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})
		r := NewRegistry(m, shims, "/tmp/cache", 0, "test")

		// When ensuring running
		err := r.EnsureRunning()

		// Then a new container is started and readiness probed
		if err != nil {
			t.Fatalf("EnsureRunning: %v", err)
		}
		if !ranContainer {
			t.Error("expected docker run to be invoked")
		}
	})

	t.Run("RecreatesStoppedContainer", func(t *testing.T) {
		// Given docker inspect reports an exited container
		m := shell.NewMockShell()
		var removed, ran bool
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && args[0] == "inspect" {
				return "exited\n", nil
			}
			if cmd == "docker" && args[0] == "rm" {
				removed = true
				return "", nil
			}
			if cmd == "docker" && args[0] == "run" {
				ran = true
				return "", nil
			}
			return "", nil
		}
		shims := newRegistryTestShims(func(url string) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})
		r := NewRegistry(m, shims, "/tmp/cache", 0, "test")

		// When ensuring running
		err := r.EnsureRunning()

		// Then the old container is removed and a new one is run
		if err != nil {
			t.Fatalf("EnsureRunning: %v", err)
		}
		if !removed || !ran {
			t.Errorf("expected rm + run, got removed=%v ran=%v", removed, ran)
		}
	})

	t.Run("SkipsStartWhenAlreadyRunning", func(t *testing.T) {
		// Given inspect reports running state with matching port
		m := shell.NewMockShell()
		var runCalled, rmCalled bool
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && args[0] == "inspect" {
				for _, a := range args {
					if strings.Contains(a, "HostPort") {
						return "5000 \n", nil
					}
				}
				return "running\n", nil
			}
			if cmd == "docker" && args[0] == "run" {
				runCalled = true
			}
			if cmd == "docker" && args[0] == "rm" {
				rmCalled = true
			}
			return "", nil
		}
		shims := newRegistryTestShims(func(url string) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})
		r := NewRegistry(m, shims, "/tmp/cache", 0, "test")

		// When ensuring running
		if err := r.EnsureRunning(); err != nil {
			t.Fatalf("EnsureRunning: %v", err)
		}
		// Then neither rm nor run is invoked when the container is healthy on the right port
		if runCalled || rmCalled {
			t.Errorf("expected no rm/run on healthy match, got run=%v rm=%v", runCalled, rmCalled)
		}
	})

	t.Run("ReturnsErrorWhenReadinessTimesOut", func(t *testing.T) {
		// Given an HTTP probe that always fails and a time source that jumps
		// past the readiness deadline on second call
		m := shell.NewMockShell()
		m.ExecSilentFunc = func(cmd string, args ...string) (string, error) {
			if cmd == "docker" && args[0] == "inspect" {
				return "running\n", nil
			}
			return "", nil
		}
		shims := NewShims()
		shims.Sleep = func(d time.Duration) {}
		var calls int
		shims.Now = func() time.Time {
			calls++
			if calls > 2 {
				return time.Now().Add(time.Hour)
			}
			return time.Time{}
		}
		shims.HttpGet = func(url string) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}
		r := NewRegistry(m, shims, "/tmp/cache", 0, "test")

		err := r.EnsureRunning()
		if err == nil || !strings.Contains(err.Error(), "did not become ready") {
			t.Errorf("expected readiness timeout error, got %v", err)
		}
	})
}

func TestRegistry_Endpoint(t *testing.T) {
	t.Run("ReturnsLocalhostPort", func(t *testing.T) {
		r := NewRegistry(shell.NewMockShell(), NewShims(), "/tmp", 0, "test")
		if got := r.Endpoint(); got != "http://localhost:5000" {
			t.Errorf("got %q", got)
		}
	})
}
