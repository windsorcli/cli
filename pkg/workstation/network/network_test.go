package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type NetworkTestMocks struct {
	Runtime                  *runtime.Runtime
	ConfigHandler            config.ConfigHandler
	Shell                    *shell.MockShell
	NetworkInterfaceProvider *MockNetworkInterfaceProvider
	Shims                    *Shims
}

func setupDefaultShims() *Shims {
	return &Shims{
		Stat:      func(path string) (os.FileInfo, error) { return nil, nil },
		WriteFile: func(path string, data []byte, perm os.FileMode) error { return nil },
		ReadFile:  func(path string) ([]byte, error) { return nil, nil },
		ReadLink:  func(path string) (string, error) { return "", nil },
		MkdirAll:  func(path string, perm os.FileMode) error { return nil },
		MkdirTemp: func(dir, pattern string) (string, error) { return "/tmp/windsor-test", nil },
		RemoveAll: func(path string) error { return nil },
	}
}

func setupNetworkMocks(t *testing.T, opts ...func(*NetworkTestMocks)) *NetworkTestMocks {
	t.Helper()

	// Store original directory and create temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set project root environment variable
	t.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	// Create a mock shell first (needed for config handler)
	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	// Create config handler
	configHandler := config.NewConfigHandler(mockShell)

	configYAML := `
version: v1alpha1
contexts:
  mock-context:
    network:
      cidr_block: "192.168.1.0/24"
    vm:
      address: "192.168.1.10"
    dns:
      domain: "example.com"
      address: "1.2.3.4"
`
	if err := configHandler.LoadConfigString(configYAML); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Configure mock shell functions
	mockShell.ExecFunc = func(command string, args ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecSilentFunc = func(command string, args ...string) (string, error) {
		return "", nil
	}
	mockShell.ExecSilentWithTimeoutFunc = func(command string, args []string, timeout time.Duration) (string, error) {
		if command == "colima" && len(args) > 0 && args[0] == "ssh" {
			cmdStr := strings.Join(args, " ")
			if strings.Contains(cmdStr, "ls /sys/class/net") {
				return "br-bridge0\neth0\nlo", nil
			}
			if strings.Contains(cmdStr, "sysctl") && strings.Contains(cmdStr, "ip_forward") {
				return "", nil
			}
			if strings.Contains(cmdStr, "iptables") && strings.Contains(cmdStr, "-C") {
				cmd := exec.Command("sh", "-c", "exit 1")
				err := cmd.Run()
				if err != nil {
					return "", err
				}
				return "", fmt.Errorf("unexpected success")
			}
			if strings.Contains(cmdStr, "iptables") && strings.Contains(cmdStr, "-A") {
				return "", nil
			}
		}
		return "", nil
	}

	// Create a mock network interface provider with mock functions
	mockNetworkInterfaceProvider := &MockNetworkInterfaceProvider{
		InterfacesFunc: func() ([]net.Interface, error) {
			return []net.Interface{
				{Name: "eth0"},
				{Name: "lo"},
				{Name: "br-bridge0"}, // Include a "br-" interface to simulate a docker bridge
			}, nil
		},
		InterfaceAddrsFunc: func(iface net.Interface) ([]net.Addr, error) {
			switch iface.Name {
			case "br-bridge0":
				return []net.Addr{
					&net.IPNet{
						IP:   net.ParseIP("192.168.1.1"),
						Mask: net.CIDRMask(24, 32),
					},
				}, nil
			case "eth0":
				return []net.Addr{
					&net.IPNet{
						IP:   net.ParseIP("10.0.0.2"),
						Mask: net.CIDRMask(24, 32),
					},
				}, nil
			case "lo":
				return []net.Addr{
					&net.IPNet{
						IP:   net.ParseIP("127.0.0.1"),
						Mask: net.CIDRMask(8, 32),
					},
				}, nil
			default:
				return nil, fmt.Errorf("no addresses found for interface %s", iface.Name)
			}
		},
	}

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  tmpDir,
		ContextName:   "mock-context",
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}

	mocks := &NetworkTestMocks{
		Runtime:                  rt,
		ConfigHandler:            configHandler,
		Shell:                    mockShell,
		NetworkInterfaceProvider: mockNetworkInterfaceProvider,
		Shims:                    setupDefaultShims(),
	}

	configHandler.SetContext("mock-context")
	configHandler.Set("workstation.dns.address", "1.2.3.4")

	// Apply any overrides
	for _, opt := range opts {
		opt(mocks)
	}

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNetworkManager_NewNetworkManager(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a runtime
		rt := &runtime.Runtime{
			ConfigHandler: config.NewMockConfigHandler(),
			Shell:         shell.NewMockShell(),
		}

		// When creating a new BaseNetworkManager
		nm := NewBaseNetworkManager(rt)

		// Then the NetworkManager should not be nil
		if nm == nil {
			t.Fatalf("expected NetworkManager to be created, got nil")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestNetworkManager_ConfigureGuest(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a properly configured network manager
		manager, _ := setup(t)

		// When configuring the guest
		err := manager.ConfigureGuest()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestNetworkManager_NeedsPrivilegeForCluster(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("FalseOnDockerDesktop", func(t *testing.T) {
		// Given a docker-desktop runtime (cluster reachable via loopback, no host route needed)
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		mocks.ConfigHandler.Set("workstation.address", "192.168.1.10")

		// Then no cluster-privilege work is required
		if manager.NeedsPrivilegeForCluster() {
			t.Errorf("expected NeedsPrivilegeForCluster to be false on docker-desktop")
		}
	})

	t.Run("FalseOnColimaWithoutGuestAddress", func(t *testing.T) {
		// Given colima with no workstation.address yet (typical before workstation TF applies)
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.ConfigHandler.Set("workstation.address", "")

		// Then no cluster-privilege work is required — there's nothing to route to yet
		if manager.NeedsPrivilegeForCluster() {
			t.Errorf("expected NeedsPrivilegeForCluster to be false without guest address")
		}
	})

	t.Run("TrueOnColimaWhenRouteAbsent", func(t *testing.T) {
		// Given colima with a guest address and no matching host route
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.10")
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && len(args) > 0 && args[0] == "route" {
				return "", nil // empty route table → host route absent
			}
			if command == "route" && len(args) > 0 && args[0] == "-n" {
				return "", nil // darwin: empty output → route absent
			}
			return "", nil
		}

		// Then cluster-privilege work is required
		if !manager.NeedsPrivilegeForCluster() {
			t.Errorf("expected NeedsPrivilegeForCluster to be true when host route is absent")
		}
	})
}

func TestNetworkManager_IsHostRouteInstalled(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("FalseOnDockerDesktop", func(t *testing.T) {
		// Given a docker-desktop runtime — host routes never installed
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.runtime", "docker-desktop")
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.10")

		// Then the probe reports nothing installed
		if manager.IsHostRouteInstalled() {
			t.Errorf("expected IsHostRouteInstalled to be false on docker-desktop")
		}
	})

	t.Run("FalseOnColimaWithoutGuestAddress", func(t *testing.T) {
		// Given colima with no guest address (configure-network couldn't have installed anything)
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.ConfigHandler.Set("workstation.address", "")

		// Then the probe reports nothing installed
		if manager.IsHostRouteInstalled() {
			t.Errorf("expected IsHostRouteInstalled to be false without guest address")
		}
	})

	t.Run("FalseOnColimaWhenRouteAbsent", func(t *testing.T) {
		// Given colima with a guest address and the route check shows no matching route
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.ConfigHandler.Set("workstation.address", "192.168.5.10")
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "ip" && len(args) > 0 && args[0] == "route" {
				return "", nil
			}
			if command == "route" && len(args) > 0 && args[0] == "-n" {
				return "", nil
			}
			return "", nil
		}

		// Then the probe reports nothing installed
		if manager.IsHostRouteInstalled() {
			t.Errorf("expected IsHostRouteInstalled to be false when route is absent")
		}
	})
}

func TestNetworkManager_IsResolverInstalled(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("FalseWhenDomainUnset", func(t *testing.T) {
		// Given no DNS domain in config — nothing could have been installed
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "")

		// Then the probe reports nothing installed
		if manager.IsResolverInstalled() {
			t.Errorf("expected IsResolverInstalled to be false when domain is unset")
		}
	})

	t.Run("FalseWhenResolverIPUnderivable", func(t *testing.T) {
		// Given a domain but no resolver IP — nothing matchable to install
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.ConfigHandler.Set("workstation.dns.address", "")

		// Then the probe reports nothing installed
		if manager.IsResolverInstalled() {
			t.Errorf("expected IsResolverInstalled to be false when resolver IP is underivable")
		}
	})
}

func TestNetworkManager_NeedsPrivilegeForDNS(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("FalseWhenDomainUnset", func(t *testing.T) {
		// Given no DNS domain in config (no cluster DNS service to point at)
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "")

		// Then no DNS-privilege work is required
		if manager.NeedsPrivilegeForDNS() {
			t.Errorf("expected NeedsPrivilegeForDNS to be false when domain is unset")
		}
	})

	t.Run("FalseWhenResolverIPUnderivable", func(t *testing.T) {
		// Given a domain but no resolver IP — neither localhost mode nor workstation.dns.address
		manager, mocks := setup(t)
		mocks.ConfigHandler.Set("dns.domain", "example.com")
		mocks.ConfigHandler.Set("workstation.runtime", "colima")
		mocks.ConfigHandler.Set("workstation.dns.address", "")

		// Then no DNS-privilege work is required — there's no IP to write into the resolver entry
		if manager.NeedsPrivilegeForDNS() {
			t.Errorf("expected NeedsPrivilegeForDNS to be false when resolver IP is underivable")
		}
	})
}

// =============================================================================
// Test Private Methods
// =============================================================================

func TestValidateDomain(t *testing.T) {
	t.Run("AcceptsRFC1123LabelCharsetDomains", func(t *testing.T) {
		// Given domains using only RFC 1123 label characters (letters, digits, hyphen, dot)
		cases := []string{"example.com", "local.test", "dev.example.io", "a", "x-y-z.com", "12345.test", "a-b.c-d.e"}

		// When validating each
		for _, d := range cases {
			// Then validation passes
			if err := validateDomain(d); err != nil {
				t.Errorf("expected %q to be accepted, got %v", d, err)
			}
		}
	})

	t.Run("RejectsEmptyLabels", func(t *testing.T) {
		// Given domains whose character set is allowlisted but whose dot structure produces empty
		// labels — dot-only strings collapse to a parent-directory traversal when interpolated
		// into a filesystem path (e.g. /etc/resolver/.. resolves to /etc/ on darwin).
		cases := []string{
			".",        // single dot
			"..",       // double dot — the darwin traversal exploit
			"...",      // triple dot
			".foo",     // leading dot
			"foo.",     // trailing dot
			"foo..bar", // consecutive dots
			"..bar",    // leading double dot
			"bar..",    // trailing double dot
		}

		// When validating each
		for _, d := range cases {
			// Then validation rejects with the empty-label error
			err := validateDomain(d)
			if err == nil {
				t.Errorf("expected %q to be rejected, got nil", d)
				continue
			}
			if !strings.Contains(err.Error(), "contains empty label") {
				t.Errorf("expected empty-label error for %q, got %v", d, err)
			}
		}
	})

	t.Run("RejectsCharactersOutsideAllowlist", func(t *testing.T) {
		// Given domains containing characters that would let configuration escape downstream
		// interpolation contexts — filesystem paths, PowerShell single-quoted strings, shell command lines
		cases := []string{
			"evil/../etc/passwd",       // path separator
			"a\\b",                     // backslash
			"/leading-slash",           // leading slash
			"x/y",                      // embedded slash
			"a'; calc; '",              // single quote → PowerShell injection
			`a"b`,                      // double quote
			"a b",                      // whitespace
			"a;b",                      // shell statement separator
			"a$b",                      // shell variable
			"a`b`",                     // shell command substitution
			"a&b",                      // shell background / chain
			"a|b",                      // shell pipe
			"with_underscore.test",     // underscore not in RFC 1123 label set
			"unicödé.com",              // non-ASCII
		}

		// When validating each
		for _, d := range cases {
			// Then validation rejects with the allowlist error
			err := validateDomain(d)
			if err == nil {
				t.Errorf("expected %q to be rejected, got nil", d)
				continue
			}
			if !strings.Contains(err.Error(), "must contain only letters, digits, hyphen, and dot") {
				t.Errorf("expected allowlist error for %q, got %v", d, err)
			}
		}
	})
}

func TestValidateCIDR(t *testing.T) {
	t.Run("AcceptsValidCIDRsAndReturnsCanonicalForm", func(t *testing.T) {
		// Given well-formed CIDRs (some intentionally non-canonical to verify normalization)
		cases := map[string]string{
			"192.168.5.0/24":   "192.168.5.0/24",
			"10.0.0.0/8":       "10.0.0.0/8",
			"172.16.0.0/12":    "172.16.0.0/12",
			"192.168.5.42/24":  "192.168.5.0/24", // host bits dropped by ParseCIDR
			"fd00::/8":         "fd00::/8",
		}
		for input, want := range cases {
			got, err := validateCIDR(input)
			if err != nil {
				t.Errorf("expected %q to validate, got %v", input, err)
				continue
			}
			if got != want {
				t.Errorf("validateCIDR(%q) = %q, want %q", input, got, want)
			}
		}
	})

	t.Run("RejectsInputsWithShellMetacharacters", func(t *testing.T) {
		// Given strings shaped to escape PowerShell -Command or sh -c context
		cases := []string{
			"",
			"not-a-cidr",
			"192.168.5.0/24'; calc; #",         // PowerShell injection
			"192.168.5.0/24; rm -rf /",         // shell statement separator
			"192.168.5.0/24 -and (rm)",         // PowerShell operator
			"192.168.5.0/24`whoami`",           // PowerShell subexpression
			"$(rm -rf /)",                      // shell command substitution
			"192.168.5.0",                      // missing mask
			"192.168.5.0/33",                   // out-of-range mask
			"...",
		}
		for _, input := range cases {
			if _, err := validateCIDR(input); err == nil {
				t.Errorf("expected %q to be rejected, got nil error", input)
			}
		}
	})
}

func TestValidateIPAddress(t *testing.T) {
	t.Run("AcceptsValidIPv4AndIPv6", func(t *testing.T) {
		cases := map[string]string{
			"192.168.5.10":  "192.168.5.10",
			"10.5.0.2":      "10.5.0.2",
			"::1":           "::1",
			"fe80::1":       "fe80::1",
		}
		for input, want := range cases {
			got, err := validateIPAddress(input)
			if err != nil {
				t.Errorf("expected %q to validate, got %v", input, err)
				continue
			}
			if got != want {
				t.Errorf("validateIPAddress(%q) = %q, want %q", input, got, want)
			}
		}
	})

	t.Run("RejectsInputsWithShellMetacharacters", func(t *testing.T) {
		cases := []string{
			"",
			"not-an-ip",
			"192.168.5.10'; calc; #",     // PowerShell injection
			"192.168.5.10; rm -rf /",     // shell statement separator
			"$(whoami)",                  // shell command substitution
			"10.5.0.2`id`",               // PowerShell subexpression
			"10.5.0.999",                 // out-of-range octet
			"10.5.0",                     // truncated
		}
		for _, input := range cases {
			if _, err := validateIPAddress(input); err == nil {
				t.Errorf("expected %q to be rejected, got nil error", input)
			}
		}
	})
}

func TestNetworkManager_writeFileWithSudo(t *testing.T) {
	setup := func(t *testing.T) (*BaseNetworkManager, *NetworkTestMocks) {
		t.Helper()
		mocks := setupNetworkMocks(t)
		manager := NewBaseNetworkManager(mocks.Runtime)
		manager.shims = mocks.Shims
		return manager, mocks
	}

	t.Run("StagesAndInstallsContent", func(t *testing.T) {
		// Given a happy-path environment
		manager, mocks := setup(t)

		var stagedDir, stagedPath string
		var stagedContent []byte
		mocks.Shims.MkdirTemp = func(_, _ string) (string, error) {
			stagedDir = "/tmp/windsor-net-mock"
			return stagedDir, nil
		}
		mocks.Shims.WriteFile = func(path string, data []byte, _ os.FileMode) error {
			stagedPath = path
			stagedContent = data
			return nil
		}
		var mvSrc, mvDest, chmodMode, chmodTarget string
		mocks.Shell.ExecSudoFunc = func(_, command string, args ...string) (string, error) {
			switch command {
			case "mv":
				mvSrc, mvDest = args[0], args[1]
			case "chmod":
				chmodMode, chmodTarget = args[0], args[1]
			}
			return "", nil
		}

		// When writing a file
		if err := manager.writeFileWithSudo("/etc/some-dest", []byte("payload")); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the helper stages inside the private temp dir, mvs to the destination, and chmods 0644.
		// filepath.Join uses the host separator (backslash on Windows) — assert with the same primitive.
		expectedStagedPath := filepath.Join(stagedDir, "drop-in")
		if stagedPath != expectedStagedPath {
			t.Errorf("expected staged path %q, got %q", expectedStagedPath, stagedPath)
		}
		if string(stagedContent) != "payload" {
			t.Errorf("expected staged content %q, got %q", "payload", string(stagedContent))
		}
		if mvSrc != expectedStagedPath || mvDest != "/etc/some-dest" {
			t.Errorf("expected mv %q -> %q, got %q -> %q", expectedStagedPath, "/etc/some-dest", mvSrc, mvDest)
		}
		if chmodMode != "0644" || chmodTarget != "/etc/some-dest" {
			t.Errorf("expected chmod 0644 %q, got chmod %q %q", "/etc/some-dest", chmodMode, chmodTarget)
		}
	})

	t.Run("RemovesTempDirOnFailureAfterStaging", func(t *testing.T) {
		// Given mv into place fails after a successful temp-file stage
		manager, mocks := setup(t)
		mocks.Shims.MkdirTemp = func(_, _ string) (string, error) {
			return "/tmp/windsor-net-fail", nil
		}
		var removedPath string
		mocks.Shims.RemoveAll = func(path string) error {
			removedPath = path
			return nil
		}
		mocks.Shell.ExecSudoFunc = func(_, command string, _ ...string) (string, error) {
			if command == "mv" {
				return "", fmt.Errorf("mv boom")
			}
			return "", nil
		}

		// When writing a file
		err := manager.writeFileWithSudo("/etc/some-dest", []byte("payload"))

		// Then the helper surfaces the install error and the deferred cleanup runs
		if err == nil || !strings.Contains(err.Error(), "failed to install file: mv boom") {
			t.Fatalf("expected install error, got %v", err)
		}
		if removedPath != "/tmp/windsor-net-fail" {
			t.Errorf("expected cleanup of %q, got %q", "/tmp/windsor-net-fail", removedPath)
		}
	})
}
