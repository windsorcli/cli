package virt_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

// recorder is a presenter that stores each event it receives.
type recorder struct {
	mu     sync.Mutex
	events []presenter.Event
}

// fixture holds a runtime with a mock shell and a mock config handler, and stub host shims.
type fixture struct {
	runtime  *runtime.Runtime
	shell    *shell.MockShell
	config   *config.MockConfigHandler
	values   map[string]any
	shims    *virt.Shims
	home     string
	recorder *recorder
	ctx      context.Context
}

// newFixture returns a fixture for the context "local". The host has 8 CPUs, 16GB of memory, and
// the arm64 architecture. The home directory is a temporary directory. The fixture is also the
// default presenter until the test ends. Tests change config values through f.values.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		shell:    shell.NewMockShell(),
		config:   config.NewMockConfigHandler(),
		values:   map[string]any{},
		home:     t.TempDir(),
		recorder: &recorder{},
	}
	f.config.GetContextFunc = func() string { return "local" }
	f.config.GetStringFunc = func(key string, defaultValue ...string) string {
		if v, ok := f.values[key].(string); ok {
			return v
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	f.config.GetIntFunc = func(key string, defaultValue ...int) int {
		if v, ok := f.values[key].(int); ok {
			return v
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	f.config.GetBoolFunc = func(key string, defaultValue ...bool) bool {
		if v, ok := f.values[key].(bool); ok {
			return v
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	f.config.SetFunc = func(key string, value any) error {
		f.values[key] = value
		return nil
	}
	f.config.GetContextValuesFunc = func() (map[string]any, error) { return map[string]any{}, nil }

	f.shims = virt.NewShims()
	f.shims.UserHomeDir = func() (string, error) { return f.home, nil }
	f.shims.Getenv = func(string) string { return "" }
	f.shims.GOARCH = func() string { return "arm64" }
	f.shims.NumCPU = func() int { return 8 }
	f.shims.VirtualMemory = func() (*mem.VirtualMemoryStat, error) {
		return &mem.VirtualMemoryStat{Total: 16 * 1024 * 1024 * 1024}, nil
	}
	f.shims.NewYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) virt.YAMLEncoder {
		return yaml.NewEncoder(w, opts...)
	}

	f.runtime = &runtime.Runtime{
		ProjectRoot:   "/work/project",
		ContextName:   "local",
		ConfigHandler: f.config,
		Shell:         f.shell,
	}
	f.ctx = presenter.NewContext(context.Background(), f.recorder)
	presenter.SetDefault(f.recorder)
	t.Cleanup(func() { presenter.SetDefault(nil) })
	return f
}

// =============================================================================
// Test Helpers
// =============================================================================

// Emit stores e.
func (r *recorder) Emit(_ context.Context, e presenter.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// steps returns each operation event as "kind:message", in order.
func (r *recorder) steps() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == presenter.KindApplying || e.Kind == presenter.KindApplied || e.Kind == presenter.KindFailed {
			out = append(out, string(e.Kind)+":"+e.Message)
		}
	}
	return out
}

// warnings returns the message of each warning event, in order.
func (r *recorder) warnings() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Kind == presenter.KindMessage && e.Level >= slog.LevelWarn {
			out = append(out, e.Message)
		}
	}
	return out
}

// commandLine joins a command and its arguments with spaces.
func commandLine(command string, args ...string) string {
	return strings.TrimSpace(command + " " + strings.Join(args, " "))
}
