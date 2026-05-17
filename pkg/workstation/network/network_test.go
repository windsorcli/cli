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

// =============================================================================
// Test Private Methods
// =============================================================================

func TestValidateDomain(t *testing.T) {
	t.Run("AcceptsTypicalDomains", func(t *testing.T) {
		// Given domains that span the realistic config surface
		cases := []string{"example.com", "local.test", "dev.example.io", "a", "x-y_z.com"}

		// When validating each
		for _, d := range cases {
			// Then validation passes
			if err := validateDomain(d); err != nil {
				t.Errorf("expected %q to be accepted, got %v", d, err)
			}
		}
	})

	t.Run("RejectsPathSeparators", func(t *testing.T) {
		// Given domains containing path separators that would let configuration escape filesystem layout
		cases := []string{"evil/../etc/passwd", "a\\b", "/leading-slash", "x/y"}

		// When validating each
		for _, d := range cases {
			// Then validation rejects with the path-separator error
			err := validateDomain(d)
			if err == nil {
				t.Errorf("expected %q to be rejected, got nil", d)
				continue
			}
			if !strings.Contains(err.Error(), "contains path separator") {
				t.Errorf("expected path-separator error for %q, got %v", d, err)
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
