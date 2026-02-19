package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
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
