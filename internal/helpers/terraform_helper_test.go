package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsor-hotel/cli/internal/context"
)

func setup() {
	// Initialization code
}

func teardown() {
	// Cleanup code
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func TestTerraformHelper_GenerateTerraformTfvarsFlags(t *testing.T) {
	// Given: a mock context and config handler
	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return "/mock/config/root", nil
		},
	}
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		nil,
	)
	mockShell := createMockShell(func() (string, error) {
		return "/mock/project/root", nil
	})
	helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

	t.Run("ValidProjectPath", func(t *testing.T) {
		// Given: a valid project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root/terraform", nil
		}
		defer func() { getwd = originalGetwd }()

		// Create mock tfvars file
		os.MkdirAll("/mock/project/root/terraform", os.ModePerm)
		os.WriteFile("/mock/project/root/terraform/terraform.tfvars", []byte(""), os.ModePerm)
		defer os.RemoveAll("/mock/project/root/terraform")

		// When: GenerateTerraformTfvarsFlags is called
		flags, err := helper.GenerateTerraformTfvarsFlags()

		// Then: it should return the correct flags
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedFlags := "-var-file=/mock/project/root/terraform/terraform.tfvars"
		if flags != expectedFlags {
			t.Errorf("Expected %s, got %s", expectedFlags, flags)
		}
	})

	t.Run("InvalidProjectPath", func(t *testing.T) {
		// Given: an invalid project path
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "", errors.New("project path not found")
		}
		defer func() { getwd = originalGetwd }()

		// When: GenerateTerraformTfvarsFlags is called
		_, err := helper.GenerateTerraformTfvarsFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
	})
}

func TestTerraformHelper_GenerateTerraformInitBackendFlags(t *testing.T) {
	// Given: a mock context and config handler
	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return "/mock/config/root", nil
		},
	}
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		func(key string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"backend": "local",
			}, nil
		},
	)
	mockShell := createMockShell(func() (string, error) {
		return "/mock/project/root", nil
	})
	helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

	t.Run("ValidBackend", func(t *testing.T) {
		// Given: a valid backend
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root/terraform", nil
		}
		defer func() { getwd = originalGetwd }()

		// When: GenerateTerraformInitBackendFlags is called
		flags, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return the correct flags
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedFlags := "-backend=true -backend-config=path=/mock/config/root/.tfstate/terraform/terraform.tfstate"
		if flags != expectedFlags {
			t.Errorf("Expected %s, got %s", expectedFlags, flags)
		}
	})

	t.Run("InvalidBackend", func(t *testing.T) {
		// Given: an invalid backend
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "test-context", nil
			},
			func(key string) (map[string]interface{}, error) {
				return nil, errors.New("backend not found")
			},
		)
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GenerateTerraformInitBackendFlags is called
		_, err := helper.GenerateTerraformInitBackendFlags()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
	})
}

func TestTerraformHelper_GenerateBackendOverrideTf(t *testing.T) {
	// Override getwd for the test
	originalGetwd := getwd
	getwd = func() (string, error) {
		return "/mock/working/dir", nil
	}
	defer func() { getwd = originalGetwd }() // Restore original function after test

	// Given: a mock context and config handler
	mockContext := &context.MockContext{
		GetConfigRootFunc: func() (string, error) {
			return "/mock/config/root", nil
		},
	}
	mockConfigHandler := createMockConfigHandler(
		func(key string) (string, error) {
			return "test-context", nil
		},
		func(key string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"backend": "local",
			}, nil
		},
	)
	mockShell := createMockShell(func() (string, error) {
		return "/mock/project/root", nil
	})
	helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

	t.Run("ValidBackend", func(t *testing.T) {
		// Given: a valid backend
		originalGetwd := getwd
		getwd = func() (string, error) {
			return "/mock/project/root/terraform", nil
		}
		defer func() { getwd = originalGetwd }()

		// When: GenerateBackendOverrideTf is called
		err := helper.GenerateBackendOverrideTf()

		// Then: it should create the backend_override.tf file
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		expectedPath := filepath.Join("/mock/working/dir", "backend_override.tf")
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Fatalf("Expected file %s to be created", expectedPath)
		}
	})

	t.Run("InvalidBackend", func(t *testing.T) {
		// Given: an invalid backend
		mockConfigHandler := createMockConfigHandler(
			func(key string) (string, error) {
				return "test-context", nil
			},
			func(key string) (map[string]interface{}, error) {
				return nil, errors.New("backend not found")
			},
		)
		helper := NewTerraformHelper(mockConfigHandler, mockShell, mockContext)

		// When: GenerateBackendOverrideTf is called
		err := helper.GenerateBackendOverrideTf()

		// Then: it should return an error
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
	})
}
