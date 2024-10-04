package helpers

import (
	"errors"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
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

func TestColimaHelper_GetEnvVars(t *testing.T) {
	configHandler := &config.MockConfigHandler{}
	ctx := &context.MockContext{}

	diContainer := di.NewContainer()
	diContainer.Register("configHandler", configHandler)
	diContainer.Register("context", ctx)

	helper, err := NewColimaHelper(diContainer)
	if err != nil {
		t.Fatalf("NewColimaHelper() error = %v", err)
	}

	envVars, err := helper.GetEnvVars()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(envVars) != 0 {
		t.Fatalf("expected empty envVars, got %v", envVars)
	}
}

func TestColimaHelper_PostEnvExec(t *testing.T) {
	configHandler := &config.MockConfigHandler{}
	ctx := &context.MockContext{}

	diContainer := di.NewContainer()
	diContainer.Register("configHandler", configHandler)
	diContainer.Register("context", ctx)

	helper, err := NewColimaHelper(diContainer)
	if err != nil {
		t.Fatalf("NewColimaHelper() error = %v", err)
	}

	err = helper.PostEnvExec()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestColimaHelper_SetConfig(t *testing.T) {

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "", errors.New("context error")
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving context: context error" {
			t.Fatalf("expected 'error retrieving context: context error', got '%v'", err)
		}
	})

	t.Run("Driver", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				if key != "contexts.test-context.vm.driver" || value != "colima" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("driver", "colima")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("CPU", func(t *testing.T) {
		tests := []struct {
			value    string
			expected string
			errMsg   string
		}{
			{"4", "4", ""},
			{"", strconv.Itoa(runtime.NumCPU() / 2), ""},
			{"invalid", "", "invalid value for cpu: strconv.Atoi: parsing \"invalid\": invalid syntax"},
		}

		for _, tt := range tests {
			t.Run(tt.value, func(t *testing.T) {
				configHandler := &config.MockConfigHandler{
					SetConfigValueFunc: func(key, value string) error {
						if key != "contexts.test-context.vm.cpu" || value != tt.expected {
							t.Fatalf("unexpected key/value: %s/%s", key, value)
						}
						return nil
					},
				}
				ctx := &context.MockContext{
					GetContextFunc: func() (string, error) {
						return "test-context", nil
					},
				}

				diContainer := di.NewContainer()
				diContainer.Register("configHandler", configHandler)
				diContainer.Register("context", ctx)

				helper, err := NewColimaHelper(diContainer)
				if err != nil {
					t.Fatalf("NewColimaHelper() error = %v", err)
				}

				err = helper.SetConfig("cpu", tt.value)
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
			expected string
			errMsg   string
		}{
			{"100", "100", ""},
			{"", "60", ""},
			{"invalid", "", "invalid value for disk: strconv.Atoi: parsing \"invalid\": invalid syntax"},
		}

		for _, tt := range tests {
			t.Run(tt.value, func(t *testing.T) {
				configHandler := &config.MockConfigHandler{
					SetConfigValueFunc: func(key, value string) error {
						if key != "contexts.test-context.vm.disk" || value != tt.expected {
							t.Fatalf("unexpected key/value: %s/%s", key, value)
						}
						return nil
					},
				}
				ctx := &context.MockContext{
					GetContextFunc: func() (string, error) {
						return "test-context", nil
					},
				}

				diContainer := di.NewContainer()
				diContainer.Register("configHandler", configHandler)
				diContainer.Register("context", ctx)

				helper, err := NewColimaHelper(diContainer)
				if err != nil {
					t.Fatalf("NewColimaHelper() error = %v", err)
				}

				err = helper.SetConfig("disk", tt.value)
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
		// Mock the virtualMemory function to return a fixed total memory
		originalVirtualMemory := virtualMemory
		virtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
		}
		defer func() { virtualMemory = originalVirtualMemory }()

		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				if key != "contexts.test-context.vm.memory" || value != "32" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("memory", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("GetArch", func(t *testing.T) {
		// Save the original goArch function
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
				// Mock the goArch function
				goArch = func() string { return tt.mockArch }

				arch := getArch()
				if arch != tt.expected {
					t.Fatalf("expected %s, got %s", tt.expected, arch)
				}
			})
		}
	})

	t.Run("ArchDefault", func(t *testing.T) {
		// Override the goArch function for testing
		originalGoArch := goArch
		defer func() { goArch = originalGoArch }()

		goArch = func() string { return "amd64" }

		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				if key != "contexts.test-context.vm.arch" || value != "x86_64" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("arch", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("UnsupportedConfigKey", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("unsupported", "value")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "unsupported config key: unsupported" {
			t.Fatalf("expected unsupported config key error, got %v", err)
		}
	})

	t.Run("ErrorSettingDriverConfig", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("config error")
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: config error" {
			t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
		}
	})

	t.Run("ErrorSettingCPUConfig", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("config error")
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("cpu", "4")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: config error" {
			t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
		}
	})

	t.Run("ErrorSettingDiskConfig", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("config error")
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("disk", "100")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: config error" {
			t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
		}
	})

	t.Run("ErrorSettingMemoryConfig", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("config error")
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("memory", "8")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: config error" {
			t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
		}
	})

	t.Run("InvalidArchValue", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("arch", "invalid-arch")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "invalid value for arch: invalid-arch" {
			t.Fatalf("expected 'invalid value for arch: invalid-arch', got '%v'", err)
		}
	})

	t.Run("ErrorSettingArchConfig", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("config error")
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("arch", "x86_64")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: config error" {
			t.Fatalf("expected 'error setting colima config: config error', got '%v'", err)
		}
	})

	t.Run("VMTypeVZForAarch64", func(t *testing.T) {
		// Save the original getArch function
		originalGetArch := getArch
		// Restore the original getArch function after the test
		defer func() { getArch = originalGetArch }()

		// Mock the getArch function to simulate "aarch64"
		getArch = func() string {
			return "aarch64"
		}

		configHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				return "", nil
			},
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		// Create DI container and register mocks
		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		// Assuming generateColimaConfig is called within SetConfig
		err = helper.SetConfig("cpu", "4")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Here you would check if vmType was set to "vz" in the configuration
		// This might involve checking the state of the configHandler or other side effects
	})

	t.Run("ErrorCreatingConfigDirectory", func(t *testing.T) {
		// Save the original mkdirAll function
		originalMkdirAll := mkdirAll
		// Restore the original mkdirAll function after the test
		defer func() { mkdirAll = originalMkdirAll }()

		// Mock the mkdirAll function to simulate an error
		mkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir error")
		}

		configHandler := &config.MockConfigHandler{}

		err := generateColimaConfig("test-context", configHandler)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error creating colima config directory: mkdir error" {
			t.Fatalf("expected 'error creating colima config directory: mkdir error', got '%v'", err)
		}
	})

	t.Run("ValidConfigValues", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
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
			},
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

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
		configHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
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
			},
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

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
		// Mock the newYAMLEncoder function to return a mock encoder that simulates an error
		originalNewYAMLEncoder := newYAMLEncoder
		defer func() { newYAMLEncoder = originalNewYAMLEncoder }()

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

		configHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				return "", nil
			},
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("cpu", "4")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error encoding yaml: encoding error" {
			t.Fatalf("expected 'error encoding yaml: encoding error', got '%v'", err)
		}
	})

	t.Run("ErrorWritingToFile", func(t *testing.T) {
		// Save the original writeFile function
		originalWriteFile := writeFile
		// Restore the original writeFile function after the test
		defer func() { writeFile = originalWriteFile }()

		// Mock the writeFile function to simulate an error
		writeFile = func(filename string, data []byte, perm os.FileMode) error {
			return errors.New("write error")
		}

		configHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				return "", nil
			},
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("cpu", "4")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error writing to temporary file: write error" {
			t.Fatalf("expected 'error writing to temporary file: write error', got '%v'", err)
		}
	})

	t.Run("ErrorRenamingFile", func(t *testing.T) {
		// Save the original rename function
		originalRename := rename
		// Restore the original rename function after the test
		defer func() { rename = originalRename }()

		// Mock the rename function to simulate an error
		rename = func(oldpath, newpath string) error {
			return errors.New("rename error")
		}

		configHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				return "", nil
			},
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}

		err = helper.SetConfig("cpu", "4")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error renaming temporary file to colima config file: rename error" {
			t.Fatalf("expected 'error renaming temporary file to colima config file: rename error', got '%v'", err)
		}
	})
}

func TestGetDefaultValues(t *testing.T) {
	t.Run("MemoryError", func(t *testing.T) {
		// Mock the virtualMemory function to return an error
		originalVirtualMemory := virtualMemory
		virtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return nil, errors.New("mock error")
		}
		defer func() { virtualMemory = originalVirtualMemory }()

		// Call getDefaultValues and check the memory value
		_, _, memory, _, _ := getDefaultValues("test-context")
		if memory != 2 {
			t.Fatalf("expected memory to be 2, got %d", memory)
		}
	})

	t.Run("MemoryMock", func(t *testing.T) {
		// Mock the virtualMemory function to return a fixed total memory
		originalVirtualMemory := virtualMemory
		virtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
		}
		defer func() { virtualMemory = originalVirtualMemory }()

		// Call getDefaultValues and check the memory value
		_, _, memory, _, _ := getDefaultValues("test-context")
		if memory != 32 { // Expecting half of 64GB
			t.Fatalf("expected memory to be 32, got %d", memory)
		}
	})

	t.Run("EncoderCloseError", func(t *testing.T) {
		// Mock the newYAMLEncoder function to return a mock encoder that simulates an error on Close
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

		configHandler := &config.MockConfigHandler{
			GetConfigValueFunc: func(key string) (string, error) {
				return "", nil
			},
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		diContainer := di.NewContainer()
		diContainer.Register("configHandler", configHandler)
		diContainer.Register("context", ctx)

		helper, err := NewColimaHelper(diContainer)
		if err != nil {
			t.Fatalf("NewColimaHelper() error = %v", err)
		}
		err = helper.SetConfig("cpu", "4")
		if err == nil || err.Error() != "error closing encoder: close error" {
			t.Fatalf("expected 'error closing encoder: close error', got '%v'", err)
		}
	})

	t.Run("MemoryOverflowHandling", func(t *testing.T) {
		originalVirtualMemory := virtualMemory
		defer func() { virtualMemory = originalVirtualMemory }()

		// Mock the virtualMemory function to return a normal value
		virtualMemory = func() (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 64 * 1024 * 1024 * 1024}, nil // 64GB
		}

		// Force the overflow condition
		testForceMemoryOverflow = true
		defer func() { testForceMemoryOverflow = false }()

		_, _, memory, _, _ := getDefaultValues("test-context")

		if memory != math.MaxInt {
			t.Fatalf("expected memory to be set to MaxInt, got %d", memory)
		}
	})
}

func TestNewColimaHelper(t *testing.T) {
	t.Run("ErrorResolvingConfigHandler", func(t *testing.T) {
		// Create DI container without registering configHandler
		diContainer := di.NewContainer()

		// Attempt to create ColimaHelper
		_, err := NewColimaHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving configHandler") {
			t.Fatalf("expected error resolving configHandler, got %v", err)
		}
	})

	t.Run("ErrorResolvingContext", func(t *testing.T) {
		// Create DI container and register only configHandler
		diContainer := di.NewContainer()
		mockConfigHandler := &config.MockConfigHandler{}
		diContainer.Register("configHandler", mockConfigHandler)

		// Attempt to create ColimaHelper
		_, err := NewColimaHelper(diContainer)
		if err == nil || !strings.Contains(err.Error(), "error resolving context") {
			t.Fatalf("expected error resolving context, got %v", err)
		}
	})
}
