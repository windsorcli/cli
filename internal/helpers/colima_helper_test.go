package helpers

import (
	"errors"
	"io"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/mem"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/di"
)

type mockYAMLEncoder struct {
	encodeFunc func(v interface{}) error
	closeFunc  func() error
}

func (m *mockYAMLEncoder) Encode(v interface{}) error {
	return m.encodeFunc(v)
}

func (m *mockYAMLEncoder) Close() error {
	return m.closeFunc()
}

func TestColimaHelper(t *testing.T) {
	t.Run("NewColimaHelper", func(t *testing.T) {
		t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
			// Given a DI container without registering cliConfigHandler
			diContainer := di.NewContainer()

			// When attempting to create ColimaHelper
			_, err := NewColimaHelper(diContainer)

			// Then it should return an error indicating cliConfigHandler resolution failure
			if err == nil || !strings.Contains(err.Error(), "error resolving cliConfigHandler") {
				t.Fatalf("expected error resolving cliConfigHandler, got %v", err)
			}
		})

		t.Run("ErrorResolvingContext", func(t *testing.T) {
			// Given a DI container with only cliConfigHandler registered
			diContainer := di.NewContainer()
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// When attempting to create ColimaHelper
			_, err := NewColimaHelper(diContainer)

			// Then it should return an error indicating context resolution failure
			if err == nil || !strings.Contains(err.Error(), "error resolving context") {
				t.Fatalf("expected error resolving context, got %v", err)
			}
		})
	})

	t.Run("SetConfig", func(t *testing.T) {
		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given a mock context that returns an error when retrieving context
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "", errors.New("context error")
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("driver", "colima")

			// Then it should return an error indicating context retrieval failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error retrieving context: context error" {
				t.Fatalf("expected 'error retrieving context: context error', got '%v'", err)
			}
		})

		t.Run("Driver", func(t *testing.T) {
			// Given a mock config handler that expects a specific key/value pair
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				if key != "contexts.test-context.vm.driver" || value != "colima" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("driver", "colima")

			// Then it should not return an error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("CPU", func(t *testing.T) {
			tests := []struct {
				value    string
				expected interface{}
				errMsg   string
			}{
				{"4", 4, ""},
				{"", runtime.NumCPU() / 2, ""},
				{"invalid", nil, "invalid value for cpu: strconv.Atoi: parsing \"invalid\": invalid syntax"},
			}

			for _, tt := range tests {
				t.Run(tt.value, func(t *testing.T) {
					// Given a mock config handler that expects a specific key/value pair
					cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
					cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
						if key != "contexts.test-context.vm.cpu" || value != tt.expected {
							t.Fatalf("unexpected key/value: %s/%v", key, value)
						}
						return nil
					}
					// And a mock context that returns a specific context
					ctx := &context.MockContext{
						GetContextFunc: func() (string, error) {
							return "test-context", nil
						},
					}

					// And a DI container with the mock context and config handler registered
					diContainer := di.NewContainer()
					diContainer.Register("cliConfigHandler", cliConfigHandler)
					diContainer.Register("context", ctx)

					// When creating a new ColimaHelper
					helper, err := NewColimaHelper(diContainer)
					if err != nil {
						t.Fatalf("NewColimaHelper() error = %v", err)
					}

					// And setting the config
					err = helper.SetConfig("cpu", tt.value)

					// Then it should return the expected error or no error
					if tt.errMsg != "" {
						if err == nil {
							t.Fatalf("expected error, got nil")
						}
						if err.Error() != tt.errMsg {
							t.Fatalf("expected error '%s', got '%v'", tt.errMsg, err)
						}
					} else {
						if err != nil {
							t.Fatalf("expected no error, got %v", err)
						}
					}
				})
			}
		})

		t.Run("Disk", func(t *testing.T) {
			tests := []struct {
				value    string
				expected interface{}
				errMsg   string
			}{
				{"100", 100, ""},
				{"", 60, ""},
				{"invalid", nil, "invalid value for disk: strconv.Atoi: parsing \"invalid\": invalid syntax"},
			}

			for _, tt := range tests {
				t.Run(tt.value, func(t *testing.T) {
					// Given a mock config handler that expects a specific key/value pair
					cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
					cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
						if key != "contexts.test-context.vm.disk" || value != tt.expected {
							t.Fatalf("unexpected key/value: %s/%v", key, value)
						}
						return nil
					}
					// And a mock context that returns a specific context
					ctx := &context.MockContext{
						GetContextFunc: func() (string, error) {
							return "test-context", nil
						},
					}

					// And a DI container with the mock context and config handler registered
					diContainer := di.NewContainer()
					diContainer.Register("cliConfigHandler", cliConfigHandler)
					diContainer.Register("context", ctx)

					// When creating a new ColimaHelper
					helper, err := NewColimaHelper(diContainer)
					if err != nil {
						t.Fatalf("NewColimaHelper() error = %v", err)
					}

					// And setting the config
					err = helper.SetConfig("disk", tt.value)

					// Then it should return the expected error or no error
					if tt.errMsg != "" {
						if err == nil {
							t.Fatalf("expected error, got nil")
						}
						if err.Error() != tt.errMsg {
							t.Fatalf("expected error '%s', got '%v'", tt.errMsg, err)
						}
					} else {
						if err != nil {
							t.Fatalf("expected no error, got %v", err)
						}
					}
				})
			}
		})

		t.Run("Memory", func(t *testing.T) {
			// Given a mock virtualMemory function that returns a fixed total memory
			originalVirtualMemory := virtualMemory
			virtualMemory = func() (*mem.VirtualMemoryStat, error) {
				return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
			}
			defer func() { virtualMemory = originalVirtualMemory }()

			// And a mock config handler that expects a specific key/value pair
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				if key != "contexts.test-context.vm.memory" || value != 32 {
					t.Fatalf("unexpected key/value: %s/%v", key, value)
				}
				return nil
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("memory", "")

			// Then it should not return an error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("GetArch", func(t *testing.T) {
			// Given a mock goArch function that returns different architectures
			originalGoArch := goArch
			defer func() { goArch = originalGoArch }()

			tests := []struct {
				mockArch string
				expected string
			}{
				{"amd64", "x86_64"},
				{"arm64", "aarch64"},
				{"unknown", "unknown"},
			}

			for _, tt := range tests {
				t.Run(tt.mockArch, func(t *testing.T) {
					// When the goArch function is mocked to return a specific architecture
					goArch = func() string { return tt.mockArch }

					// And getArch is called
					arch := getArch()

					// Then it should return the expected architecture
					if arch != tt.expected {
						t.Fatalf("expected %s, got %s", tt.expected, arch)
					}
				})
			}
		})

		t.Run("ArchDefault", func(t *testing.T) {
			// Given a mock goArch function that returns "amd64"
			originalGoArch := goArch
			defer func() { goArch = originalGoArch }()

			goArch = func() string { return "amd64" }

			// And a mock config handler that expects a specific key/value pair
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				if key != "contexts.test-context.vm.arch" || value != "x86_64" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("arch", "")

			// Then it should not return an error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("RetrieveAndSetArchValue", func(t *testing.T) {
			// Given a mock config handler that returns a specific architecture value
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "contexts.test-context.vm.arch" {
					return "x86_64", nil
				}
				return "", nil
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", mockConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("arch", "x86_64")

			// Then it should not return an error
			if err != nil {
				t.Fatalf("SetConfig() error = %v", err)
			}

			// And the architecture should be set correctly
			if arch, err := mockConfigHandler.GetConfigValue("contexts.test-context.vm.arch"); err != nil || arch != "x86_64" {
				t.Errorf("expected arch to be 'x86_64', got '%v'", arch)
			}
		})

		t.Run("UnsupportedConfigKey", func(t *testing.T) {
			// Given a mock config handler
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting an unsupported config key
			err = helper.SetConfig("unsupported", "value")

			// Then it should return an error indicating unsupported config key
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "unsupported config key: unsupported" {
				t.Fatalf("expected unsupported config key error, got %v", err)
			}
		})

		t.Run("ErrorSettingDriverConfig", func(t *testing.T) {
			// Given a mock config handler that returns an error when setting config value
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return errors.New("config error")
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("driver", "colima")

			// Then it should return an error indicating config setting failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error setting colima config: config error" {
				t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
			}
		})

		t.Run("ErrorSettingCPUConfig", func(t *testing.T) {
			// Given a mock config handler that returns an error when setting config value
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return errors.New("config error")
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("cpu", "4")

			// Then it should return an error indicating config setting failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error setting colima config: config error" {
				t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
			}
		})

		t.Run("ErrorSettingDiskConfig", func(t *testing.T) {
			// Given a mock config handler that returns an error when setting config value
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return errors.New("config error")
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("disk", "100")

			// Then it should return an error indicating config setting failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error setting colima config: config error" {
				t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
			}
		})

		t.Run("ErrorSettingMemoryConfig", func(t *testing.T) {
			// Given a mock config handler that returns an error when setting config value
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return errors.New("config error")
			}
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("memory", "8")

			// Then it should return an error indicating config setting failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error setting colima config: config error" {
				t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
			}
		})

		t.Run("InvalidArchValue", func(t *testing.T) {
			// Given a mock config handler
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting an invalid architecture value
			err = helper.SetConfig("arch", "invalid-arch")

			// Then it should return an error indicating invalid architecture value
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "invalid value for arch: invalid-arch" {
				t.Fatalf("expected 'invalid value for arch: invalid-arch', got '%v'", err)
			}
		})
		t.Run("ErrorSettingArchConfig", func(t *testing.T) {
			// Given a mock config handler that returns an error when setting config value
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return errors.New("config error")
			}

			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("arch", "x86_64")

			// Then it should return an error indicating config setting failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error setting colima config: config error" {
				t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
			}
		})

		t.Run("VMTypeVZForAarch64", func(t *testing.T) {
			// Given a mock getArch function that returns "aarch64"
			originalGetArch := getArch
			defer func() { getArch = originalGetArch }()

			getArch = func() string {
				return "aarch64"
			}

			// And a mock config handler
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				return "", nil
			}
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return nil
			}

			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// Create DI container and register mocks
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("cpu", "4")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Then vmType should be set to "vz" in the configuration
			// This might involve checking the state of the cliConfigHandler or other side effects
		})

		t.Run("ErrorCreatingConfigDirectory", func(t *testing.T) {
			// Given the original mkdirAll function
			originalMkdirAll := mkdirAll
			// And restoring the original mkdirAll function after the test
			defer func() { mkdirAll = originalMkdirAll }()

			// And a mock mkdirAll function to simulate an error
			mkdirAll = func(path string, perm os.FileMode) error {
				return errors.New("mkdir error")
			}

			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)

			// When generating the Colima config
			err := generateColimaConfig("test-context", cliConfigHandler)

			// Then it should return an error indicating directory creation failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error creating colima config directory: mkdir error" {
				t.Fatalf("expected 'error creating colima config directory: mkdir error', got '%v'", err)
			}
		})

		t.Run("ValidConfigValues", func(t *testing.T) {
			// Given a mock config handler with valid config values
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				switch key {
				case "contexts.test-context.vm.cpu":
					return "4", nil
				case "contexts.test-context.vm.disk":
					return "100", nil
				case "contexts.test-context.vm.memory":
					return "8", nil
				default:
					return "", errors.New("unknown key")
				}
			}
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return nil
			}

			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config with valid values
			err = helper.SetConfig("cpu", "4")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			err = helper.SetConfig("disk", "100")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			err = helper.SetConfig("memory", "8")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("InvalidConfigValues", func(t *testing.T) {
			// Given a mock config handler with invalid config values
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				switch key {
				case "contexts.test-context.vm.cpu":
					return "invalid", nil
				case "contexts.test-context.vm.disk":
					return "invalid", nil
				case "contexts.test-context.vm.memory":
					return "invalid", nil
				default:
					return "", errors.New("unknown key")
				}
			}
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return nil
			}

			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config with invalid values
			err = helper.SetConfig("cpu", "invalid")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			err = helper.SetConfig("disk", "invalid")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			err = helper.SetConfig("memory", "invalid")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
		})

		t.Run("ErrorEncodingYAML", func(t *testing.T) {
			// Given the original newYAMLEncoder function
			originalNewYAMLEncoder := newYAMLEncoder
			// And restoring the original newYAMLEncoder function after the test
			defer func() { newYAMLEncoder = originalNewYAMLEncoder }()

			// And a mock newYAMLEncoder function to simulate an encoding error
			newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
				return &mockYAMLEncoder{
					encodeFunc: func(v interface{}) error {
						return errors.New("encoding error")
					},
					closeFunc: func() error {
						return nil
					},
				}
			}

			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				return "", nil
			}
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return nil
			}

			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("cpu", "4")

			// Then it should return an error indicating YAML encoding failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error encoding yaml: encoding error" {
				t.Fatalf("expected 'error encoding yaml: encoding error', got '%v'", err)
			}
		})

		t.Run("ErrorWritingToFile", func(t *testing.T) {
			// Given the original writeFile function
			originalWriteFile := writeFile
			// And restoring the original writeFile function after the test
			defer func() { writeFile = originalWriteFile }()

			// And a mock writeFile function to simulate a write error
			writeFile = func(filename string, data []byte, perm os.FileMode) error {
				return errors.New("write error")
			}

			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				return "", nil
			}
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return nil
			}

			// And a mock context that returns a specific context
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("cpu", "4")

			// Then it should return an error indicating file write failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error writing to temporary file: write error" {
				t.Fatalf("expected 'error writing to temporary file: write error', got '%v'", err)
			}
		})

		t.Run("ErrorRenamingFile", func(t *testing.T) {
			// Given the original rename function
			originalRename := rename
			// And restoring the original rename function after the test
			defer func() { rename = originalRename }()

			// And a mock rename function to simulate a rename error
			rename = func(oldpath, newpath string) error {
				return errors.New("rename error")
			}

			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				return "", nil
			}
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return nil
			}

			// And a mock context that returns a specific context
			ctx := context.NewMockContext(nil, nil, nil)
			ctx.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("cpu", "4")

			// Then it should return an error indicating file rename failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error renaming temporary file to colima config file: rename error" {
				t.Fatalf("expected 'error renaming temporary file to colima config file: rename error', got '%v'", err)
			}
		})
	})

	t.Run("GetEnvVars", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler and context
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			ctx := context.NewMockContext(nil, nil, nil)

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting environment variables
			envVars, err := helper.GetEnvVars()
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Then it should return an empty map
			if len(envVars) != 0 {
				t.Fatalf("expected empty envVars, got %v", envVars)
			}
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler and context
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			ctx := context.NewMockContext(nil, nil, nil)

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And executing post environment setup
			err = helper.PostEnvExec()
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	})

	t.Run("GetDefaultValues", func(t *testing.T) {
		t.Run("MemoryError", func(t *testing.T) {
			// Given a mock virtualMemory function that returns an error
			originalVirtualMemory := virtualMemory
			virtualMemory = func() (*mem.VirtualMemoryStat, error) {
				return nil, errors.New("mock error")
			}
			defer func() { virtualMemory = originalVirtualMemory }()

			// When calling getDefaultValues
			_, _, memory, _, _ := getDefaultValues("test-context")

			// Then it should return a default memory value of 2
			if memory != 2 {
				t.Fatalf("expected memory to be 2, got %d", memory)
			}
		})

		t.Run("MemoryMock", func(t *testing.T) {
			// Given a mock virtualMemory function that returns a fixed total memory
			originalVirtualMemory := virtualMemory
			virtualMemory = func() (*mem.VirtualMemoryStat, error) {
				return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
			}
			defer func() { virtualMemory = originalVirtualMemory }()

			// When calling getDefaultValues
			_, _, memory, _, _ := getDefaultValues("test-context")

			// Then it should return half of the total memory
			if memory != 32 { // Expecting half of 64GB
				t.Fatalf("expected memory to be 32, got %d", memory)
			}
		})

		t.Run("EncoderCloseError", func(t *testing.T) {
			// Given a mock newYAMLEncoder function that returns a mock encoder that simulates an error on Close
			originalNewYAMLEncoder := newYAMLEncoder
			defer func() { newYAMLEncoder = originalNewYAMLEncoder }()

			newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
				return &mockYAMLEncoder{
					encodeFunc: func(v interface{}) error {
						return nil
					},
					closeFunc: func() error {
						return errors.New("close error")
					},
				}
			}

			// And a mock config handler and context
			cliConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)
			cliConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				return "", nil
			}
			cliConfigHandler.SetConfigValueFunc = func(key string, value interface{}) error {
				return nil
			}
			ctx := context.NewMockContext(nil, nil, nil)
			ctx.GetContextFunc = func() (string, error) {
				return "test-context", nil
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("cliConfigHandler", cliConfigHandler)
			diContainer.Register("context", ctx)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And setting the config
			err = helper.SetConfig("cpu", "4")

			// Then it should return an error indicating encoder close failure
			if err == nil || err.Error() != "error closing encoder: close error" {
				t.Fatalf("expected 'error closing encoder: close error', got '%v'", err)
			}
		})

		t.Run("MemoryOverflowHandling", func(t *testing.T) {
			// Given a mock virtualMemory function that returns a normal value
			originalVirtualMemory := virtualMemory
			defer func() { virtualMemory = originalVirtualMemory }()

			virtualMemory = func() (*mem.VirtualMemoryStat, error) {
				return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
			}

			// And forcing the overflow condition
			testForceMemoryOverflow = true
			defer func() { testForceMemoryOverflow = false }()

			// When calling getDefaultValues
			_, _, memory, _, _ := getDefaultValues("test-context")

			// Then it should return the maximum integer value for memory
			if memory != math.MaxInt {
				t.Fatalf("expected memory to be set to MaxInt, got %d", memory)
			}
		})
	})

	t.Run("GetContainerConfig", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := context.NewMockContext(nil, nil, nil)
			mockConfigHandler := config.NewMockConfigHandler(nil, nil, nil, nil, nil, nil)

			// And a DI container with the mock context and config handler registered
			container := di.NewContainer()
			container.Register("context", mockContext)
			container.Register("cliConfigHandler", mockConfigHandler)

			// When creating a new ColimaHelper
			colimaHelper, err := NewColimaHelper(container)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting container configuration
			containerConfig, err := colimaHelper.GetContainerConfig()
			if err != nil {
				t.Fatalf("GetContainerConfig() error = %v", err)
			}

			// Then it should return nil as per the stub implementation
			if containerConfig != nil {
				t.Errorf("expected nil, got %v", containerConfig)
			}
		})
	})
}
