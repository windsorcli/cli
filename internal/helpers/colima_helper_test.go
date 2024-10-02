package helpers

import (
	"errors"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/windsor-hotel/cli/internal/config"
	"github.com/windsor-hotel/cli/internal/context"
	"github.com/windsor-hotel/cli/internal/shell"
)

type mockFile struct {
	writeStringFunc func(s string) (n int, err error)
	writeFunc       func(p []byte) (n int, err error)
}

func (m *mockFile) WriteString(s string) (n int, err error) {
	return m.writeStringFunc(s)
}

func (m *mockFile) Write(p []byte) (n int, err error) {
	return m.writeFunc(p)
}

func (m *mockFile) Close() error {
	return nil
}

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
	shell := &shell.MockShell{}
	ctx := &context.MockContext{}

	helper := NewColimaHelper(configHandler, shell, ctx)

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
	shell := &shell.MockShell{}
	ctx := &context.MockContext{}

	helper := NewColimaHelper(configHandler, shell, ctx)

	err := helper.PostEnvExec()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestColimaHelper_SetConfig(t *testing.T) {
	t.Run("SuccessfulSetConfig", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				if key != "contexts.test-context.vm.driver" || value != "colima" {
					t.Fatalf("unexpected key/value: %s/%s", key, value)
				}
				return nil
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("driver", "colima")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorRetrievingContext", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "", errors.New("context error")
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error retrieving context: context error" {
			t.Fatalf("expected context error, got %v", err)
		}
	})

	t.Run("ErrorSettingConfigValue", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return errors.New("set config error")
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error setting colima config: set config error" {
			t.Fatalf("expected set config error, got %v", err)
		}
	})

	t.Run("UnsupportedConfigKey", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		err := helper.SetConfig("unsupported", "value")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "unsupported config key: unsupported" {
			t.Fatalf("expected unsupported config key error, got %v", err)
		}
	})

	t.Run("ArchConversion", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		NewColimaHelper(configHandler, shell, ctx)

		// Override the goArch function for testing
		originalGoArch := goArch
		defer func() { goArch = originalGoArch }()

		tests := []struct {
			mockArch string
			expected string
		}{
			{"amd64", "x86_64"},
			{"arm64", "aarch64"},
			{"unknown", "unknown"}, // Default case
		}

		for _, tt := range tests {
			goArch = func() string { return tt.mockArch }
			_, _, _, _, arch := getDefaultValues("test-context")
			if arch != tt.expected {
				t.Fatalf("expected arch to be %v, got %v", tt.expected, arch)
			}
		}
	})

	t.Run("ErrorCreatingConfigDirectory", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		// Inject a mock mkdirAll function that returns an error
		mockMkdirAll := func(path string, perm os.FileMode) error {
			return errors.New("mock error creating directory")
		}

		// Temporarily replace the mkdirAll variable
		originalMkdirAll := mkdirAll
		mkdirAll = mockMkdirAll
		defer func() { mkdirAll = originalMkdirAll }()

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error creating colima config directory: mock error creating directory" {
			t.Fatalf("expected error 'error creating colima config directory: mock error creating directory', got %v", err)
		}
	})

	t.Run("OverrideValues", func(t *testing.T) {
		tests := []struct {
			key       string
			mockKey   string
			mockValue string
			expected  int
		}{
			{"cpu", "contexts.test-context.vm.cpu", "4", 4},
			{"disk", "contexts.test-context.vm.disk", "100", 100},
			{"memory", "contexts.test-context.vm.memory", "8", 8},
		}

		for _, tt := range tests {
			configHandler := &config.MockConfigHandler{
				SetConfigValueFunc: func(key, value string) error {
					return nil
				},
				GetConfigValueFunc: func(key string) (string, error) {
					if key == tt.mockKey {
						return tt.mockValue, nil
					}
					return "", errors.New("unknown key")
				},
			}
			shell := &shell.MockShell{}
			ctx := &context.MockContext{
				GetContextFunc: func() (string, error) {
					return "test-context", nil
				},
			}

			helper := NewColimaHelper(configHandler, shell, ctx)

			err := helper.SetConfig("driver", "colima")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Verify that the value was overridden
			val, err := strconv.Atoi(tt.mockValue)
			if err != nil {
				t.Fatalf("expected no error converting value, got %v", err)
			}
			if val != tt.expected {
				t.Fatalf("expected value to be %d, got %d", tt.expected, val)
			}
		}
	})

	t.Run("ErrorWritingHeaderComment", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
			GetConfigValueFunc: func(key string) (string, error) {
				return "", errors.New("unknown key")
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		// Mock os.Create to return a file with a mocked WriteString method
		originalCreate := osCreate
		mockFile := &mockFile{
			writeStringFunc: func(s string) (n int, err error) {
				return 0, errors.New("mock error writing header comment")
			},
		}
		osCreate = func(name string) (FileWriter, error) {
			return mockFile, nil
		}
		defer func() { osCreate = originalCreate }()

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error writing header comment: mock error writing header comment" {
			t.Fatalf("expected error 'error writing header comment: mock error writing header comment', got %v", err)
		}
	})

	t.Run("ErrorCreatingColimaConfigFile", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
			GetConfigValueFunc: func(key string) (string, error) {
				return "", errors.New("unknown key")
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		// Mock osCreate to return an error
		originalCreate := osCreate
		osCreate = func(name string) (FileWriter, error) {
			return nil, errors.New("mock error creating file")
		}
		defer func() { osCreate = originalCreate }()

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error creating colima config file: mock error creating file" {
			t.Fatalf("expected error 'error creating colima config file: mock error creating file', got %v", err)
		}
	})

	t.Run("ErrorCreatingTemporaryFile", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
			GetConfigValueFunc: func(key string) (string, error) {
				return "", errors.New("unknown key")
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		// Mock os.CreateTemp to return an error
		originalCreateTemp := osCreateTemp
		osCreateTemp = func(dir, pattern string) (*os.File, error) {
			return nil, errors.New("mock error creating temporary file")
		}
		defer func() { osCreateTemp = originalCreateTemp }()

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error creating temporary file: mock error creating temporary file" {
			t.Fatalf("expected error 'error creating temporary file: mock error creating temporary file', got %v", err)
		}
	})

	t.Run("ErrorEncodingYAML", func(t *testing.T) {
		configHandler := &config.MockConfigHandler{
			SetConfigValueFunc: func(key, value string) error {
				return nil
			},
			GetConfigValueFunc: func(key string) (string, error) {
				return "", errors.New("unknown key")
			},
		}
		shell := &shell.MockShell{}
		ctx := &context.MockContext{
			GetContextFunc: func() (string, error) {
				return "test-context", nil
			},
		}

		helper := NewColimaHelper(configHandler, shell, ctx)

		// Mock yaml.NewEncoder to return an encoder that fails on Encode
		originalNewEncoder := newYAMLEncoder
		newYAMLEncoder = func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
			return &mockYAMLEncoder{
				encodeFunc: func(v interface{}) error {
					return errors.New("mock error encoding yaml")
				},
				closeFunc: func() error {
					return nil
				},
			}
		}
		defer func() { newYAMLEncoder = originalNewEncoder }()

		err := helper.SetConfig("driver", "colima")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if err.Error() != "error encoding yaml: mock error encoding yaml" {
			t.Fatalf("expected error 'error encoding yaml: mock error encoding yaml', got %v", err)
		}
	})
}
