package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
)

type KubeEnvPrinterMocks struct {
	Injector       di.Injector
	ContextHandler *context.MockContext
	Shell          *shell.MockShell
}

func setupSafeKubeEnvPrinterMocks(injector ...di.Injector) *KubeEnvPrinterMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockContext := context.NewMockContext()
	mockContext.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	mockShell := shell.NewMockShell()

	mockInjector.Register("contextHandler", mockContext)
	mockInjector.Register("shell", mockShell)

	return &KubeEnvPrinterMocks{
		Injector:       mockInjector,
		ContextHandler: mockContext,
		Shell:          mockShell,
	}
}

func TestKubeEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeKubeEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.kube/config") {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}

		kubeEnvPrinter := NewKubeEnvPrinter(mocks.Injector)
		kubeEnvPrinter.Initialize()
		kubeEnvPrinter.Initialize()

		envVars, err := kubeEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedPath := filepath.FromSlash("/mock/config/root/.kube/config")
		if envVars["KUBECONFIG"] != expectedPath || envVars["KUBE_CONFIG_PATH"] != expectedPath {
			t.Errorf("KUBECONFIG = %v, KUBE_CONFIG_PATH = %v, want both to be %v", envVars["KUBECONFIG"], envVars["KUBE_CONFIG_PATH"], expectedPath)
		}
	})

	t.Run("NoKubeConfig", func(t *testing.T) {
		mocks := setupSafeKubeEnvPrinterMocks()

		originalStat := stat
		defer func() { stat = originalStat }()
		stat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		kubeEnvPrinter := NewKubeEnvPrinter(mocks.Injector)
		kubeEnvPrinter.Initialize()

		envVars, err := kubeEnvPrinter.GetEnvVars()
		if err != nil {
			t.Fatalf("GetEnvVars returned an error: %v", err)
		}

		expectedPath := filepath.FromSlash("/mock/config/root/.kube/config")
		if envVars["KUBECONFIG"] != expectedPath || envVars["KUBE_CONFIG_PATH"] != expectedPath {
			t.Errorf("KUBECONFIG = %v, KUBE_CONFIG_PATH = %v, want both to be %v", envVars["KUBECONFIG"], envVars["KUBE_CONFIG_PATH"], expectedPath)
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mocks := setupSafeKubeEnvPrinterMocks()
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock context error")
		}

		kubeEnvPrinter := NewKubeEnvPrinter(mocks.Injector)
		kubeEnvPrinter.Initialize()

		_, err := kubeEnvPrinter.GetEnvVars()
		expectedError := "error retrieving configuration root directory: mock context error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})
}

func TestKubeEnvPrinter_Print(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Use setupSafeKubeEnvPrinterMocks to create mocks
		mocks := setupSafeKubeEnvPrinterMocks()
		mockInjector := mocks.Injector
		kubeEnvPrinter := NewKubeEnvPrinter(mockInjector)
		kubeEnvPrinter.Initialize()

		// Mock the stat function to simulate the existence of the kubeconfig file
		stat = func(name string) (os.FileInfo, error) {
			if name == filepath.FromSlash("/mock/config/root/.kube/config") {
				return nil, nil // Simulate that the file exists
			}
			return nil, os.ErrNotExist
		}

		// Mock the PrintEnvVarsFunc to verify it is called with the correct envVars
		var capturedEnvVars map[string]string
		mocks.Shell.PrintEnvVarsFunc = func(envVars map[string]string) error {
			capturedEnvVars = envVars
			return nil
		}

		// Call Print and check for errors
		err := kubeEnvPrinter.Print()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that PrintEnvVarsFunc was called with the correct envVars
		expectedEnvVars := map[string]string{
			"KUBECONFIG":       filepath.FromSlash("/mock/config/root/.kube/config"),
			"KUBE_CONFIG_PATH": filepath.FromSlash("/mock/config/root/.kube/config"),
		}
		if !reflect.DeepEqual(capturedEnvVars, expectedEnvVars) {
			t.Errorf("capturedEnvVars = %v, want %v", capturedEnvVars, expectedEnvVars)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		// Use setupSafeKubeEnvPrinterMocks to create mocks
		mocks := setupSafeKubeEnvPrinterMocks()

		// Override the GetConfigFunc to simulate an error
		mocks.ContextHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}

		mockInjector := mocks.Injector

		kubeEnvPrinter := NewKubeEnvPrinter(mockInjector)
		kubeEnvPrinter.Initialize()
		// Call Print and check for errors
		err := kubeEnvPrinter.Print()
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "mock config error") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
