package env

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KubeEnvPrinterMocks struct {
	Injector      di.Injector
	ConfigHandler *config.MockConfigHandler
	Shell         *shell.MockShell
}

func setupSafeKubeEnvPrinterMocks(injector ...di.Injector) *KubeEnvPrinterMocks {
	var mockInjector di.Injector
	if len(injector) > 0 {
		mockInjector = injector[0]
	} else {
		mockInjector = di.NewMockInjector()
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return filepath.FromSlash("/mock/config/root"), nil
	}

	mockShell := shell.NewMockShell()

	mockInjector.Register("configHandler", mockConfigHandler)
	mockInjector.Register("shell", mockShell)

	// Mock readDir to return some valid persistent volume folders
	readDir = func(dirname string) ([]os.DirEntry, error) {
		if strings.HasSuffix(dirname, ".volumes") {
			return []os.DirEntry{
				mockDirEntry{name: "pvc-1234"},
				mockDirEntry{name: "pvc-5678"},
			}, nil
		}
		return nil, errors.New("mock readDir error")
	}

	// Mock stat to return nil
	stat = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(name, ".kube/config") || strings.HasSuffix(name, ".volumes") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	// Mock queryPersistentVolumeClaims to return appropriate PVC claims
	queryPersistentVolumeClaims = func(kubeConfigPath string) (*corev1.PersistentVolumeClaimList, error) {
		return &corev1.PersistentVolumeClaimList{
			Items: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{UID: "1234", Namespace: "default", Name: "claim1"}},
				{ObjectMeta: metav1.ObjectMeta{UID: "5678", Namespace: "default", Name: "claim2"}},
			},
		}, nil
	}

	return &KubeEnvPrinterMocks{
		Injector:      mockInjector,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}
}

// mockDirEntry is a simple mock implementation of os.DirEntry
type mockDirEntry struct {
	name string
}

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return true }
func (m mockDirEntry) Type() os.FileMode          { return os.ModeDir }
func (m mockDirEntry) Info() (os.FileInfo, error) { return mockFileInfo{name: m.name}, nil }

// mockFileInfo is a simple mock implementation of os.FileInfo
type mockFileInfo struct {
	name string
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return os.ModeDir }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return true }
func (m mockFileInfo) Sys() interface{}   { return nil }

func TestKubeEnvPrinter_GetEnvVars(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mocks := setupSafeKubeEnvPrinterMocks()

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
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
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

	t.Run("ErrorReadingVolumes", func(t *testing.T) {
		mocks := setupSafeKubeEnvPrinterMocks()
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "/mock/config/root", nil
		}

		originalReadDir := readDir
		defer func() { readDir = originalReadDir }()
		readDir = func(dirname string) ([]os.DirEntry, error) {
			return nil, errors.New("mock readDir error")
		}

		kubeEnvPrinter := NewKubeEnvPrinter(mocks.Injector)
		kubeEnvPrinter.Initialize()

		_, err := kubeEnvPrinter.GetEnvVars()
		expectedError := "error reading volume directories: mock readDir error"
		if err == nil || err.Error() != expectedError {
			t.Errorf("error = %v, want %v", err, expectedError)
		}
	})

	t.Run("SuccessWithExistingPVCEnvVars", func(t *testing.T) {
		// Use setupSafeKubeEnvPrinterMocks to create mocks
		mocks := setupSafeKubeEnvPrinterMocks()
		kubeEnvPrinter := NewKubeEnvPrinter(mocks.Injector)
		kubeEnvPrinter.Initialize()

		// Set up environment variables to simulate existing PVC environment variables
		os.Setenv("PV_NAMESPACE_PVCNAME", "/mock/volume/dir/pvc-12345")
		defer os.Unsetenv("PV_NAMESPACE_PVCNAME")

		// Mock the readDir function to simulate reading the volume directory
		originalReadDir := readDir
		defer func() { readDir = originalReadDir }()
		readDir = func(dirname string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				mockDirEntry{name: "pvc-12345"},
			}, nil
		}

		// Call GetEnvVars and check for errors
		envVars, err := kubeEnvPrinter.GetEnvVars()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that GetEnvVars returns the correct envVars
		expectedEnvVars := map[string]string{
			"KUBECONFIG":           filepath.FromSlash("/mock/config/root/.kube/config"),
			"KUBE_CONFIG_PATH":     filepath.FromSlash("/mock/config/root/.kube/config"),
			"PV_NAMESPACE_PVCNAME": "/mock/volume/dir/pvc-12345",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
		}
	})

	t.Run("AllVolumesAccountedFor", func(t *testing.T) {
		mocks := setupSafeKubeEnvPrinterMocks()
		kubeEnvPrinter := NewKubeEnvPrinter(mocks.Injector)
		kubeEnvPrinter.Initialize()

		// Set up environment variables to simulate all PVCs being accounted for
		os.Setenv("PV_DEFAULT_CLAIM1", "/mock/volume/dir/pvc-1234")
		os.Setenv("PV_DEFAULT_CLAIM2", "/mock/volume/dir/pvc-5678")
		defer os.Unsetenv("PV_DEFAULT_CLAIM1")
		defer os.Unsetenv("PV_DEFAULT_CLAIM2")

		// Mock the readDir function to simulate reading the volume directory
		originalReadDir := readDir
		defer func() { readDir = originalReadDir }()
		readDir = func(dirname string) ([]os.DirEntry, error) {
			return []os.DirEntry{
				mockDirEntry{name: "pvc-1234"},
				mockDirEntry{name: "pvc-5678"},
			}, nil
		}

		// Mock queryPersistentVolumeClaims to verify it is not called
		originalQueryPVCs := queryPersistentVolumeClaims
		defer func() { queryPersistentVolumeClaims = originalQueryPVCs }()
		queryPersistentVolumeClaims = func(kubeConfigPath string) (*corev1.PersistentVolumeClaimList, error) {
			t.Error("queryPersistentVolumeClaims should not be called")
			return nil, nil
		}

		// Call GetEnvVars and check for errors
		envVars, err := kubeEnvPrinter.GetEnvVars()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Verify that GetEnvVars returns the correct envVars without calling queryPersistentVolumeClaims
		expectedEnvVars := map[string]string{
			"KUBECONFIG":        filepath.FromSlash("/mock/config/root/.kube/config"),
			"KUBE_CONFIG_PATH":  filepath.FromSlash("/mock/config/root/.kube/config"),
			"PV_DEFAULT_CLAIM1": "/mock/volume/dir/pvc-1234",
			"PV_DEFAULT_CLAIM2": "/mock/volume/dir/pvc-5678",
		}
		if !reflect.DeepEqual(envVars, expectedEnvVars) {
			t.Errorf("envVars = %v, want %v", envVars, expectedEnvVars)
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
		originalStat := stat
		defer func() { stat = originalStat }()
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
		mocks.ConfigHandler.GetConfigRootFunc = func() (string, error) {
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
