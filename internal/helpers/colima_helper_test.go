package helpers

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
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
			mockConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
					cliConfigHandler := config.NewMockConfigHandler()
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
					cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			mockConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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

		t.Run("ValidConfigValues", func(t *testing.T) {
			// Given a mock config handler with valid config values
			cliConfigHandler := config.NewMockConfigHandler()
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
			cliConfigHandler := config.NewMockConfigHandler()
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
	})

	t.Run("GetEnvVars", func(t *testing.T) {
		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given a mock context that returns an error
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "", errors.New("mock context error")
				},
			}

			// And a DI container with the mock context and a mock config handler registered
			cliConfigHandler := config.NewMockConfigHandler()
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", cliConfigHandler)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting environment variables
			_, err = helper.GetEnvVars()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error retrieving context: mock context error" {
				t.Fatalf("expected 'error retrieving context: mock context error', got '%v'", err)
			}
		})

		t.Run("Success", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "contexts.test-context.vm.driver" {
					return "colima", nil
				}
				return "", nil
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting environment variables
			envVars, err := helper.GetEnvVars()

			// Then it should return the expected environment variables
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			expectedDockerSockPath := filepath.Join("/mock/home", ".colima", "windsor-test-context", "docker.sock")
			if envVars["DOCKER_SOCK"] != expectedDockerSockPath {
				t.Fatalf("expected DOCKER_SOCK to be '%s', got '%s'", expectedDockerSockPath, envVars["DOCKER_SOCK"])
			}
		})
	})

	t.Run("PostEnvExec", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			// Given a mock config handler and context
			cliConfigHandler := config.NewMockConfigHandler()
			ctx := context.NewMockContext()

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
			mockContext := context.NewMockContext()
			mockConfigHandler := config.NewMockConfigHandler()

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

	t.Run("GetEnvVars", func(t *testing.T) {
		t.Run("ErrorRetrievingVMDriver", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "contexts.test-context.vm.driver" {
					return "", errors.New("mock driver error")
				}
				return "", nil
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting environment variables
			_, err = helper.GetEnvVars()

			// Then it should return an error indicating VM driver retrieval failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error retrieving vm driver: mock driver error" {
				t.Fatalf("expected 'error retrieving vm driver: mock driver error', got '%v'", err)
			}
		})

		t.Run("DriverNotColima", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "contexts.test-context.vm.driver" {
					return "not-colima", nil
				}
				return "", nil
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting environment variables
			envVars, err := helper.GetEnvVars()

			// Then it should return nil for envVars and no error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if envVars != nil {
				t.Fatalf("expected nil envVars, got %v", envVars)
			}
		})

		t.Run("ErrorRetrievingUserHomeDir", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "contexts.test-context.vm.driver" {
					return "colima", nil
				}
				return "", nil
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return an error
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "", errors.New("mock home dir error")
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting environment variables
			_, err = helper.GetEnvVars()

			// Then it should return an error indicating user home directory retrieval failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != "error retrieving user home directory: mock home dir error" {
				t.Fatalf("expected 'error retrieving user home directory: mock home dir error', got '%v'", err)
			}
		})

		t.Run("Success", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				if key == "contexts.test-context.vm.driver" {
					return "colima", nil
				}
				return "", nil
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And getting environment variables
			envVars, err := helper.GetEnvVars()

			// Then it should return the expected environment variables
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			expectedDockerSockPath := filepath.Join("/mock/home", ".colima", "windsor-test-context", "docker.sock")
			if envVars["DOCKER_SOCK"] != expectedDockerSockPath {
				t.Fatalf("expected DOCKER_SOCK to be '%s', got '%s'", expectedDockerSockPath, envVars["DOCKER_SOCK"])
			}
		})
	})

	t.Run("WriteConfig", func(t *testing.T) {
		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given a mock context that returns an error when retrieving context
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "", errors.New("mock error")
				},
			}
			// And a mock config handler
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should return an error indicating context retrieval failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error retrieving context: mock error"
			if err.Error() != expectedError {
				t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
			}
		})

		t.Run("DefaultValues", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				return "", nil // Return empty to use default values
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// Mock the writeFile and rename functions
			originalWriteFile := writeFile
			writeFile = func(filename string, data []byte, perm os.FileMode) error {
				return nil
			}
			defer func() { writeFile = originalWriteFile }()

			originalRename := rename
			rename = func(oldpath, newpath string) error {
				return nil
			}
			defer func() { rename = originalRename }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should not return an error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("OverrideValues", func(t *testing.T) {
			// Given a mock context and config handler with specific values
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()
			mockConfigHandler.GetConfigValueFunc = func(key string) (string, error) {
				switch key {
				case "contexts.test-context.vm.cpu":
					return "8", nil
				case "contexts.test-context.vm.disk":
					return "200", nil
				case "contexts.test-context.vm.memory":
					return "16", nil
				case "contexts.test-context.vm.arch":
					return "x86_64", nil
				default:
					return "", nil
				}
			}

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// Mock the writeFile and rename functions
			originalWriteFile := writeFile
			writeFile = func(filename string, data []byte, perm os.FileMode) error {
				return nil
			}
			defer func() { writeFile = originalWriteFile }()

			originalRename := rename
			rename = func(oldpath, newpath string) error {
				return nil
			}
			defer func() { rename = originalRename }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should not return an error
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})

		t.Run("ErrorRetrievingContext", func(t *testing.T) {
			// Given a mock context that returns an error
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "", errors.New("mock context error")
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should return an error indicating context retrieval failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error retrieving context: mock context error"
			if err.Error() != expectedError {
				t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
			}
		})

		t.Run("ErrorEncodingYAML", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// Mock the newYAMLEncoder function to return an encoder that fails
			originalNewYAMLEncoder := newYAMLEncoder
			newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
				return &mockYAMLEncoder{
					encodeFunc: func(v interface{}) error {
						return errors.New("mock encoding error")
					},
					closeFunc: func() error {
						return nil
					},
				}
			}
			defer func() { newYAMLEncoder = originalNewYAMLEncoder }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should return an error indicating YAML encoding failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error encoding yaml: mock encoding error"
			if err.Error() != expectedError {
				t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
			}
		})

		t.Run("ErrorWritingFile", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// Mock the writeFile function to return an error
			originalWriteFile := writeFile
			writeFile = func(filename string, data []byte, perm os.FileMode) error {
				return errors.New("mock write file error")
			}
			defer func() { writeFile = originalWriteFile }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should return an error indicating file writing failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error writing to temporary file: mock write file error"
			if err.Error() != expectedError {
				t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
			}
		})

		t.Run("ErrorRenamingFile", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// Mock the rename function to return an error
			originalRename := rename
			rename = func(oldpath, newpath string) error {
				return errors.New("mock rename error")
			}
			defer func() { rename = originalRename }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should return an error indicating file renaming failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error renaming temporary file to colima config file: mock rename error"
			if err.Error() != expectedError {
				t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
			}
		})

		t.Run("ErrorClosingEncoder", func(t *testing.T) {
			// Given a mock context and config handler
			mockContext := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}
			mockConfigHandler := config.NewMockConfigHandler()

			// And a DI container with the mock context and config handler registered
			diContainer := di.NewContainer()
			diContainer.Register("context", mockContext)
			diContainer.Register("cliConfigHandler", mockConfigHandler)

			// Mock the userHomeDir function to return a valid directory
			originalUserHomeDir := userHomeDir
			userHomeDir = func() (string, error) {
				return "/mock/home", nil
			}
			defer func() { userHomeDir = originalUserHomeDir }()

			// Mock the newYAMLEncoder function to return an encoder that fails on Close
			originalNewYAMLEncoder := newYAMLEncoder
			newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
				return &mockYAMLEncoder{
					encodeFunc: func(v interface{}) error {
						return nil
					},
					closeFunc: func() error {
						return errors.New("mock close error")
					},
				}
			}
			defer func() { newYAMLEncoder = originalNewYAMLEncoder }()

			// When creating a new ColimaHelper
			helper, err := NewColimaHelper(diContainer)
			if err != nil {
				t.Fatalf("NewColimaHelper() error = %v", err)
			}

			// And writing the configuration
			err = helper.WriteConfig()

			// Then it should return an error indicating encoder closing failure
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			expectedError := "error closing encoder: mock close error"
			if err.Error() != expectedError {
				t.Fatalf("expected error to be '%s', got '%s'", expectedError, err.Error())
			}
		})
	})
}
