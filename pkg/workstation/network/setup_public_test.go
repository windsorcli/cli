package network_test

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/network"
)

// =============================================================================
// Test Setup
// =============================================================================

// recorder is a presenter that stores each event it receives.
type recorder struct {
	mu     sync.Mutex
	events []presenter.Event
}

// fixture holds a runtime with a mock shell and a mock config handler, a mock interface provider, and shims.
type fixture struct {
	runtime  *runtime.Runtime
	shell    *shell.MockShell
	config   *config.MockConfigHandler
	values   map[string]string
	provider *network.MockNetworkInterfaceProvider
	shims    *network.Shims
	commands []string
	recorder *recorder
	ctx      context.Context
}

// newFixture returns a fixture for the context "local". The mock shell records each command in
// f.commands and succeeds. The host has the interface bridge100 with the address 192.168.5.1/24.
// Tests change config values through f.values.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		shell:    shell.NewMockShell(),
		config:   config.NewMockConfigHandler(),
		values:   map[string]string{},
		provider: network.NewMockNetworkInterfaceProvider(),
		shims:    network.NewShims(),
		recorder: &recorder{},
	}
	f.config.GetContextFunc = func() string { return "local" }
	f.config.GetStringFunc = func(key string, defaultValue ...string) string {
		if v, ok := f.values[key]; ok {
			return v
		}
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	record := func(command string, args ...string) {
		f.commands = append(f.commands, strings.TrimSpace(command+" "+strings.Join(args, " ")))
	}
	f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		record(command, args...)
		return "", nil
	}
	f.shell.ExecSilentWithTimeoutFunc = func(command string, args []string, _ time.Duration) (string, error) {
		record(command, args...)
		return "", nil
	}
	f.shell.ExecSudoFunc = func(_ string, command string, args ...string) (string, error) {
		record(command, args...)
		return "", nil
	}
	f.provider.InterfacesFunc = func() ([]net.Interface, error) {
		return []net.Interface{{Name: "bridge100"}}, nil
	}
	f.provider.InterfaceAddrsFunc = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.5.1"), Mask: net.CIDRMask(24, 32)}}, nil
	}
	f.runtime = &runtime.Runtime{ContextName: "local", ConfigHandler: f.config, Shell: f.shell}
	f.ctx = presenter.NewContext(context.Background(), f.recorder)
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

// commandsContaining returns each recorded command that contains part.
func (f *fixture) commandsContaining(part string) []string {
	var out []string
	for _, c := range f.commands {
		if strings.Contains(c, part) {
			out = append(out, c)
		}
	}
	return out
}
