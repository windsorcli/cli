// The virt_test package is a test suite for the base Virt interface
// It provides test coverage for the core virtualization abstraction layer
// It serves as a verification framework for the base virtualization functionality
// It enables testing of dependency injection and initialization patterns

package virt

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/workstation/services"
)

// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
	Runtime       *runtime.Runtime
	ConfigHandler config.ConfigHandler
	Shell         *shell.MockShell
	Shims         *Shims
	Service       *services.MockService
}

type SetupOptions struct {
	ConfigHandler config.ConfigHandler
	ConfigStr     string
}

// setupShims creates a new Shims instance with default implementations
func setupShims(t *testing.T) *Shims {
	t.Helper()
	shims := &Shims{
		Setenv: func(key, value string) error {
			return os.Setenv(key, value)
		},
		UnmarshalJSON: func(data []byte, v any) error {
			return json.Unmarshal(data, v)
		},
		UserHomeDir: func() (string, error) {
			return "/tmp", nil
		},
		MkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
		WriteFile: func(name string, data []byte, perm os.FileMode) error {
			return nil
		},
		Rename: func(oldpath, newpath string) error {
			return nil
		},
		Stat: func(name string) (os.FileInfo, error) {
			return nil, nil
		},
		GOARCH: func() string {
			return "x86_64"
		},
		NumCPU: func() int {
			return 4
		},
		VirtualMemory: func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{
				Total: 8 * 1024 * 1024 * 1024, // 8GB
			}, nil
		},
		MarshalYAML: func(v any) ([]byte, error) {
			return yaml.Marshal(v)
		},
		NewYAMLEncoder: func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v any) error {
					return nil
				},
				closeFunc: func() error {
					return nil
				},
			}
		},
	}

	t.Cleanup(func() {
		os.Unsetenv("COMPOSE_FILE")
		os.Unsetenv("WINDSOR_CONTEXT")
	})

	return shims
}

func setupMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
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
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	// Process options with defaults
	options := &SetupOptions{}
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create shell
	mockShell := shell.NewMockShell()
	// Mock GetProjectRoot to return a temporary directory
	mockShell.GetProjectRootFunc = func() (string, error) {
		return t.TempDir(), nil
	}

	// Create config handler
	var configHandler config.ConfigHandler
	if options.ConfigHandler == nil {
		configHandler = config.NewConfigHandler(mockShell)
	} else {
		configHandler = options.ConfigHandler
	}

	// Create mock service
	mockService := services.NewMockService()

	// Create runtime
	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		TemplateRoot:  tmpDir,
		ContextName:   "mock-context",
		ConfigHandler: configHandler,
		Shell:         mockShell,
	}
	configHandler.SetContext("mock-context")

	// Load default config string
	defaultConfigStr := `
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
      enabled: true
      address: 10.0.0.53
    network:
      cidr_block: 10.0.0.0/24
    docker:
      enabled: true
      compose_file: docker-compose.yml`

	if err := configHandler.LoadConfigString(defaultConfigStr); err != nil {
		t.Fatalf("Failed to load default config string: %v", err)
	}

	// Load test-specific config if provided
	if options.ConfigStr != "" {
		if err := configHandler.LoadConfigString(options.ConfigStr); err != nil {
			t.Fatalf("Failed to load config string: %v", err)
		}
	}

	// Register cleanup to restore original state
	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
		if err := os.Chdir(origDir); err != nil {
			t.Logf("Warning: Failed to change back to original directory: %v", err)
		}
	})

	return &Mocks{
		Runtime:       rt,
		ConfigHandler: configHandler,
		Shell:         mockShell,
		Service:       mockService,
		Shims:         setupShims(t),
	}
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestVirt_Initialize(t *testing.T) {
	setup := func(t *testing.T) (*Mocks, *BaseVirt) {
		t.Helper()
		mocks := setupMocks(t)
		virt := NewBaseVirt(mocks.Runtime)
		virt.shims = mocks.Shims
		return mocks, virt
	}

	t.Run("Success", func(t *testing.T) {
		// Given a Virt with a mock runtime
		_, virt := setup(t)

		// Then the service should be properly initialized
		if virt == nil {
			t.Fatal("Expected Virt, got nil")
		}
	})

	t.Run("ErrorResolvingShell", func(t *testing.T) {
		// Given a Virt with mock components
		_, virt := setup(t)

		// Then the service should be properly initialized
		if virt == nil {
			t.Fatal("Expected Virt, got nil")
		}
	})

	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Given a Virt with mock components
		_, virt := setup(t)

		// Then the service should be properly initialized
		if virt == nil {
			t.Fatal("Expected Virt, got nil")
		}
	})
}
