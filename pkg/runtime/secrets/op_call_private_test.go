package secrets

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1password/onepassword-sdk-go"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Helpers
// =============================================================================

// syncBuffer is a writer that is safe for the timer goroutine of the approval hint.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newTestCLIProvider returns an unlocked CLI provider whose shims never run a real command,
// never watch a real signal, and report the given timeout.
func newTestCLIProvider(timeout string) *OnePasswordCLIProvider {
	p := NewOnePasswordCLIProvider(OnePasswordVault{ID: "personal", Name: "Employee", URL: "acme.1password.com"})
	p.unlocked = true
	p.shims.Getenv = func(key string) string {
		if key == opTimeoutEnv {
			return timeout
		}
		return ""
	}
	p.shims.Command = func(name string, args ...string) *exec.Cmd { return &exec.Cmd{} }
	p.shims.WatchInterrupt = func() (<-chan struct{}, func()) { return make(chan struct{}), func() {} }
	p.shims.HintWriter = &syncBuffer{}
	return p
}

var testRef = SecretRef{Vault: "personal", Item: "ghcr", Field: "credential"}

// =============================================================================
// opTimeout
// =============================================================================

func TestOpTimeout(t *testing.T) {
	cases := map[string]time.Duration{
		"":    defaultOpTimeout,
		"45s": 45 * time.Second,
		"2m":  2 * time.Minute,
		"abc": defaultOpTimeout,
		"-5s": defaultOpTimeout,
		"0":   defaultOpTimeout,
	}
	for value, want := range cases {
		got := opTimeout(func(string) string { return value })
		if got != want {
			t.Errorf("opTimeout(%q) = %s, want %s", value, got, want)
		}
	}
}

// =============================================================================
// CLI provider
// =============================================================================

func TestOnePasswordCLIProvider_Failures(t *testing.T) {
	cases := []struct {
		name       string
		stderr     string
		wantMsg    string
		wantFix    string
		wantStderr string
	}{
		{"NotSignedIn", "[ERROR] 2026/11/20 You are not signed in. Run op signin.", "isn't signed in for acme.1password.com", "op signin --account acme.1password.com", "You are not signed in"},
		{"AccountNotFound", "[ERROR] no account found for filter \"acme.1password.com\"", "has no account acme.1password.com", "vault url", "no account found"},
		{"DesktopApp", "[ERROR] connecting to desktop app: 1Password CLI couldn't connect to the 1Password desktop app.", "Couldn't reach the 1Password desktop app", "op whoami --account acme.1password.com", "connecting to desktop app"},
		{"ItemNotFound", "[ERROR] \"ghcr\" isn't an item in the \"Employee\" vault.", "no such item or vault", "item name", "isn't an item"},
		{"FieldNotFound", "[ERROR] the item has no field \"credential\"", "no such field", "field name", "no field"},
		{"Unknown", "[ERROR] something odd", "Failed to read", "", "something odd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestCLIProvider("")
			p.shims.CmdOutput = func(context.Context, *exec.Cmd) ([]byte, []byte, error) {
				return nil, []byte(tc.stderr), errors.New("exit status 1")
			}
			_, handled, err := p.Resolve(testRef)
			if !handled || err == nil {
				t.Fatalf("expected handled error, got handled=%v err=%v", handled, err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) || !strings.Contains(err.Error(), tc.wantStderr) {
				t.Errorf("message %q should hold %q and %q", err.Error(), tc.wantMsg, tc.wantStderr)
			}
			we := werror.Find(err)
			if we == nil || !strings.Contains(we.Remediation, tc.wantFix) {
				t.Errorf("remediation should hold %q, got %+v", tc.wantFix, we)
			}
		})
	}
}

func TestOnePasswordCLIProvider_TimesOutWhenOpHangs(t *testing.T) {
	p := newTestCLIProvider("50ms")
	p.shims.CmdOutput = func(ctx context.Context, _ *exec.Cmd) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
	start := time.Now()
	_, _, err := p.Resolve(testRef)
	if err == nil || !strings.Contains(err.Error(), "Timed out after 50ms") || !strings.Contains(err.Error(), "acme.1password.com") {
		t.Fatalf("expected timeout error naming the account, got %v", err)
	}
	if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "op whoami --account acme.1password.com") || !strings.Contains(we.Remediation, opTimeoutEnv) {
		t.Errorf("expected remediation with op whoami and the env var, got %+v", we)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("the call did not return at the timeout")
	}
}

func TestOnePasswordCLIProvider_ReturnsAtTimeoutEvenWhenOpIgnoresContext(t *testing.T) {
	p := newTestCLIProvider("50ms")
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	p.shims.CmdOutput = func(context.Context, *exec.Cmd) ([]byte, []byte, error) {
		<-release
		return nil, nil, errors.New("late")
	}
	_, _, err := p.Resolve(testRef)
	if err == nil || !strings.Contains(err.Error(), "Timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestOnePasswordCLIProvider_InterruptNamesTheSecret(t *testing.T) {
	p := newTestCLIProvider("")
	interrupted := make(chan struct{}, 1)
	p.shims.WatchInterrupt = func() (<-chan struct{}, func()) { return interrupted, func() {} }
	p.shims.CmdOutput = func(ctx context.Context, _ *exec.Cmd) ([]byte, []byte, error) {
		interrupted <- struct{}{}
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
	_, _, err := p.Resolve(testRef)
	if err == nil || !strings.Contains(err.Error(), "Interrupted") || !strings.Contains(err.Error(), "op://Employee/ghcr/credential") {
		t.Fatalf("expected interrupt error naming the secret, got %v", err)
	}
}

func TestOnePasswordCLIProvider_PrintsApprovalHintWhenSlow(t *testing.T) {
	old := opApprovalHintDelay
	opApprovalHintDelay = 10 * time.Millisecond
	t.Cleanup(func() { opApprovalHintDelay = old })

	p := newTestCLIProvider("")
	out := &syncBuffer{}
	p.shims.HintWriter = out
	p.shims.CmdOutput = func(context.Context, *exec.Cmd) ([]byte, []byte, error) {
		time.Sleep(100 * time.Millisecond)
		return []byte("value"), nil, nil
	}
	value, _, err := p.Resolve(testRef)
	if err != nil || value != "value" {
		t.Fatalf("unexpected result %q, %v", value, err)
	}
	if !strings.Contains(out.String(), "Approve the request in the 1Password app") {
		t.Errorf("expected approval hint, got %q", out.String())
	}
}

func TestOnePasswordCLIProvider_NoHintWhenFast(t *testing.T) {
	p := newTestCLIProvider("")
	out := &syncBuffer{}
	p.shims.HintWriter = out
	p.shims.CmdOutput = func(context.Context, *exec.Cmd) ([]byte, []byte, error) {
		return []byte("value"), nil, nil
	}
	if _, _, err := p.Resolve(testRef); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != "" {
		t.Errorf("expected no hint, got %q", out.String())
	}
}

func TestOnePasswordCLIProvider_Check(t *testing.T) {
	t.Run("RunsWhoamiForTheAccount", func(t *testing.T) {
		p := newTestCLIProvider("")
		var args []string
		p.shims.Command = func(name string, a ...string) *exec.Cmd {
			args = append([]string{name}, a...)
			return &exec.Cmd{}
		}
		p.shims.CmdOutput = func(context.Context, *exec.Cmd) ([]byte, []byte, error) { return []byte("ok"), nil, nil }
		if err := p.Check(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := strings.Join(args, " "); got != "op whoami --account acme.1password.com" {
			t.Errorf("unexpected command %q", got)
		}
		if p.Account() != "acme.1password.com" {
			t.Errorf("Account() = %q", p.Account())
		}
	})

	t.Run("ReportsNotSignedIn", func(t *testing.T) {
		p := newTestCLIProvider("")
		p.shims.CmdOutput = func(context.Context, *exec.Cmd) ([]byte, []byte, error) {
			return nil, []byte("[ERROR] account is not signed in"), errors.New("exit status 1")
		}
		err := p.Check()
		if err == nil || !strings.Contains(err.Error(), "isn't signed in for acme.1password.com") {
			t.Errorf("unexpected error %v", err)
		}
	})
}

// =============================================================================
// SDK provider
// =============================================================================

func TestOnePasswordSDKProvider_Check(t *testing.T) {
	setup := func(t *testing.T, timeout string) *OnePasswordSDKProvider {
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "tok")
		resetGlobalClient()
		t.Cleanup(resetGlobalClient)
		p := NewOnePasswordSDKProvider(OnePasswordVault{ID: "personal", Name: "Employee"})
		p.shims.Getenv = func(key string) string {
			switch key {
			case "OP_SERVICE_ACCOUNT_TOKEN":
				return "tok"
			case opTimeoutEnv:
				return timeout
			}
			return ""
		}
		p.shims.WatchInterrupt = func() (<-chan struct{}, func()) { return make(chan struct{}), func() {} }
		p.shims.NewOnePasswordClient = func(context.Context, ...onepassword.ClientOption) (*onepassword.Client, error) {
			return &onepassword.Client{}, nil
		}
		return p
	}

	t.Run("Succeeds", func(t *testing.T) {
		p := setup(t, "")
		p.shims.ListVaults = func(*onepassword.Client, context.Context) error { return nil }
		if err := p.Check(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ReportsRejectedToken", func(t *testing.T) {
		p := setup(t, "")
		p.shims.ListVaults = func(*onepassword.Client, context.Context) error { return fmt.Errorf("invalid service account token") }
		err := p.Check()
		if err == nil || !strings.Contains(err.Error(), "invalid service account token") {
			t.Fatalf("unexpected error %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "OP_SERVICE_ACCOUNT_TOKEN") {
			t.Errorf("expected token remediation, got %+v", we)
		}
	})

	t.Run("TimesOutWhenTheSDKHangs", func(t *testing.T) {
		p := setup(t, "50ms")
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		p.shims.ListVaults = func(*onepassword.Client, context.Context) error {
			<-release
			return nil
		}
		err := p.Check()
		if err == nil || !strings.Contains(err.Error(), "Timed out after 50ms") || !strings.Contains(err.Error(), sdkAccountLabel) {
			t.Fatalf("expected timeout error, got %v", err)
		}
	})

	t.Run("ResolveTimesOutWhenTheSDKHangs", func(t *testing.T) {
		p := setup(t, "50ms")
		p.unlocked = true
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		p.shims.ResolveSecret = func(*onepassword.Client, context.Context, string) (string, error) {
			<-release
			return "", nil
		}
		_, _, err := p.Resolve(SecretRef{Vault: "personal", Item: "ghcr", Field: "credential"})
		if err == nil || !strings.Contains(err.Error(), "op://Employee/ghcr/credential") {
			t.Fatalf("expected timeout error naming the secret, got %v", err)
		}
	})
}

// =============================================================================
// Resolver
// =============================================================================

type fakeChecker struct {
	MockProvider
	account string
	err     error
	calls   int
}

func (f *fakeChecker) Account() string { return f.account }
func (f *fakeChecker) Check() error    { f.calls++; return f.err }

func TestResolver_CheckReachable(t *testing.T) {
	t.Run("ChecksEachAccountOnce", func(t *testing.T) {
		a := &fakeChecker{account: "a.1password.com"}
		a2 := &fakeChecker{account: "a.1password.com"}
		b := &fakeChecker{account: "b.1password.com"}
		r := NewResolver([]Provider{a, a2, b, NewMockProvider()}, nil)
		if err := r.CheckReachable(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.calls != 1 || a2.calls != 0 || b.calls != 1 {
			t.Errorf("calls = %d %d %d, want 1 0 1", a.calls, a2.calls, b.calls)
		}
	})

	t.Run("ReportsEveryFailingAccount", func(t *testing.T) {
		a := &fakeChecker{account: "a.1password.com", err: errors.New("a down")}
		b := &fakeChecker{account: "b.1password.com", err: errors.New("b down")}
		r := NewResolver([]Provider{a, b}, nil)
		err := r.CheckReachable(context.Background())
		if err == nil || !strings.Contains(err.Error(), "a down") || !strings.Contains(err.Error(), "b down") {
			t.Fatalf("expected both failures, got %v", err)
		}
	})

	t.Run("PassesWithNoCheckers", func(t *testing.T) {
		r := NewResolver([]Provider{NewMockProvider()}, nil)
		if err := r.CheckReachable(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
