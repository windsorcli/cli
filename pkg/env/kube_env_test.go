package env

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// Test Setup
// =============================================================================

// mockDirEntry is a simple mock implementation of os.DirEntry
type mockDirEntry struct {
	name  string
	isDir bool
}

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return m.isDir }
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
func (m mockFileInfo) Sys() any           { return nil }

// setupKubeEnvMocks creates a base mock setup for Kubernetes environment tests
func setupKubeEnvMocks(t *testing.T, opts ...*SetupOptions) *Mocks {
	t.Helper()
	if len(opts) == 0 {
		opts = []*SetupOptions{{}}
	}

	mocks := setupMocks(t, opts[0])
	projectRoot, err := mocks.Shell.GetProjectRoot()
	if err != nil {
		t.Fatalf("Failed to get project root: %v", err)
	}

	t.Setenv("WINDSOR_PROJECT_ROOT", projectRoot)

	// Mock readDir to return some valid persistent volume folders
	mocks.Shims.ReadDir = func(dirname string) ([]os.DirEntry, error) {
		if strings.HasSuffix(dirname, "volumes") {
			return []os.DirEntry{
				mockDirEntry{name: "pvc-1234", isDir: true},
				mockDirEntry{name: "pvc-5678", isDir: true},
			}, nil
		}
		return nil, errors.New("mock readDir error")
	}

	// Mock stat to return nil for both kubeconfig and volumes
	mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(name, ".kube/config") || strings.HasSuffix(name, "volumes") {
			return mockFileInfo{name: filepath.Base(name)}, nil
		}
		return nil, os.ErrNotExist
	}

	// Mock queryPersistentVolumeClaims to return some PVCs
	queryPersistentVolumeClaims = func(_ string) (*corev1.PersistentVolumeClaimList, error) {
		return &corev1.PersistentVolumeClaimList{
			Items: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-1",
						Namespace: "test-ns",
						UID:       "1234",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-2",
						Namespace: "test-ns",
						UID:       "5678",
					},
				},
			},
		}, nil
	}

	return mocks
}

// =============================================================================
// Test Public Methods
// =============================================================================

// TestKubeEnvPrinter_GetEnvVars tests the GetEnvVars method of the KubeEnvPrinter
func TestKubeEnvPrinter_GetEnvVars(t *testing.T) {
	setup := func(t *testing.T) (*KubeEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupKubeEnvMocks(t)
		printer := NewKubeEnvPrinter(mocks.Injector)
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		printer.shims = mocks.Shims
		return printer, mocks
	}

	t.Run("SuccessWithKubeConfig", func(t *testing.T) {
		printer, mocks := setup(t)

		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedKubeConfig := filepath.Join(configRoot, ".kube/config")
		if envVars["KUBECONFIG"] != expectedKubeConfig {
			t.Errorf("Expected KUBECONFIG=%s, got %s", expectedKubeConfig, envVars["KUBECONFIG"])
		}
	})

	t.Run("SuccessWithVolumes", func(t *testing.T) {
		printer, _ := setup(t)

		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		volumeDir := filepath.Join(projectRoot, ".volumes")

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedPaths := map[string]string{
			"PV_TEST_NS_PVC_1": filepath.Join(volumeDir, "pvc-1234"),
			"PV_TEST_NS_PVC_2": filepath.Join(volumeDir, "pvc-5678"),
		}

		for k, v := range expectedPaths {
			if envVars[k] != v {
				t.Errorf("Expected %s=%s, got %s", k, v, envVars[k])
			}
		}
	})

	t.Run("NoKubeConfig", func(t *testing.T) {
		printer, mocks := setup(t)
		configRoot, err := mocks.ConfigHandler.GetConfigRoot()
		if err != nil {
			t.Fatalf("Failed to get config root: %v", err)
		}

		mocks.Shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		expectedKubeConfig := filepath.Join(configRoot, ".kube/config")
		if envVars["KUBECONFIG"] != expectedKubeConfig {
			t.Errorf("Expected KUBECONFIG=%s, got %s", expectedKubeConfig, envVars["KUBECONFIG"])
		}
	})

	t.Run("GetConfigRootError", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}
		mocks := setupKubeEnvMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})
		printer := NewKubeEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

		envVars, err := printer.GetEnvVars()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if envVars != nil {
			t.Errorf("Expected nil envVars, got %v", envVars)
		}
	})

	t.Run("ErrorReadingVolumes", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock ReadDir to return an error
		mocks.Shims.ReadDir = func(dirname string) ([]os.DirEntry, error) {
			if strings.HasSuffix(dirname, "volumes") {
				return nil, errors.New("mock readDir error")
			}
			return nil, nil
		}

		envVars, err := printer.GetEnvVars()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error reading volume directories") {
			t.Errorf("Expected error about reading volume directories, got %v", err)
		}
		if envVars != nil {
			t.Errorf("Expected nil envVars, got %v", envVars)
		}
	})

	t.Run("ErrorQueryingPVCs", func(t *testing.T) {
		printer, _ := setup(t)

		// Mock queryPersistentVolumeClaims to return an error
		queryPersistentVolumeClaims = func(_ string) (*corev1.PersistentVolumeClaimList, error) {
			return nil, errors.New("mock PVC query error")
		}

		envVars, err := printer.GetEnvVars()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error querying persistent volume claims") {
			t.Errorf("Expected error about querying PVCs, got %v", err)
		}
		if envVars != nil {
			t.Errorf("Expected nil envVars, got %v", envVars)
		}
	})

	t.Run("NilPVCList", func(t *testing.T) {
		printer, _ := setup(t)

		// Mock queryPersistentVolumeClaims to return nil list
		queryPersistentVolumeClaims = func(_ string) (*corev1.PersistentVolumeClaimList, error) {
			return nil, nil
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if envVars == nil {
			t.Error("Expected non-nil envVars")
		}
	})

	t.Run("EmptyPVCList", func(t *testing.T) {
		printer, _ := setup(t)

		// Mock queryPersistentVolumeClaims to return empty list
		queryPersistentVolumeClaims = func(_ string) (*corev1.PersistentVolumeClaimList, error) {
			return &corev1.PersistentVolumeClaimList{Items: []corev1.PersistentVolumeClaim{}}, nil
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if envVars == nil {
			t.Error("Expected non-nil envVars")
		}
	})

	t.Run("VolumeDirStatError", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock Stat to return an error for volume directory
		mocks.Shims.Stat = func(name string) (os.FileInfo, error) {
			if strings.HasSuffix(name, "volumes") {
				return nil, errors.New("mock stat error")
			}
			return mockFileInfo{name: filepath.Base(name)}, nil
		}

		envVars, err := printer.GetEnvVars()
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "error checking volume directory") {
			t.Errorf("Expected error about checking volume directory, got %v", err)
		}
		if envVars != nil {
			t.Errorf("Expected nil envVars, got %v", envVars)
		}
	})

	t.Run("NoPVCDirectories", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock ReadDir to return no PVC directories
		mocks.Shims.ReadDir = func(dirname string) ([]os.DirEntry, error) {
			if strings.HasSuffix(dirname, "volumes") {
				return []os.DirEntry{
					mockDirEntry{name: "other-dir", isDir: true},
				}, nil
			}
			return nil, nil
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if envVars == nil {
			t.Error("Expected non-nil envVars")
		}
	})

	t.Run("UnmatchedPVCDirectories", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock ReadDir to return PVC directories that don't match any PVCs
		mocks.Shims.ReadDir = func(dirname string) ([]os.DirEntry, error) {
			if strings.HasSuffix(dirname, "volumes") {
				return []os.DirEntry{
					mockDirEntry{name: "pvc-9999", isDir: true},
					mockDirEntry{name: "pvc-8888", isDir: true},
				}, nil
			}
			return nil, nil
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if envVars == nil {
			t.Error("Expected non-nil envVars")
		}
	})

	t.Run("ExistingPVEnvVars", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock Environ to return PV_ prefixed variables
		mocks.Shims.Environ = func() []string {
			return []string{
				"PV_TEST_NS_PVC_1=/path/to/pvc-1234",
				"OTHER_VAR=value",
				"PV_TEST_NS_PVC_2=/path/to/pvc-5678",
			}
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if envVars == nil {
			t.Error("Expected non-nil envVars")
		}
		if envVars["PV_TEST_NS_PVC_1"] != "/path/to/pvc-1234" {
			t.Errorf("Expected PV_TEST_NS_PVC_1=/path/to/pvc-1234, got %s", envVars["PV_TEST_NS_PVC_1"])
		}
		if envVars["PV_TEST_NS_PVC_2"] != "/path/to/pvc-5678" {
			t.Errorf("Expected PV_TEST_NS_PVC_2=/path/to/pvc-5678, got %s", envVars["PV_TEST_NS_PVC_2"])
		}
	})

	t.Run("EmptyVolumeDir", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock ReadDir to return empty directory
		mocks.Shims.ReadDir = func(dirname string) ([]os.DirEntry, error) {
			if strings.HasSuffix(dirname, "volumes") {
				return []os.DirEntry{}, nil
			}
			return nil, nil
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if envVars == nil {
			t.Error("Expected non-nil envVars")
		}
	})

	t.Run("PartiallyMatchedPVCDirectories", func(t *testing.T) {
		printer, mocks := setup(t)

		// Mock ReadDir to return mix of matching and non-matching PVC directories
		mocks.Shims.ReadDir = func(dirname string) ([]os.DirEntry, error) {
			if strings.HasSuffix(dirname, "volumes") {
				return []os.DirEntry{
					mockDirEntry{name: "pvc-1234", isDir: true}, // matches
					mockDirEntry{name: "pvc-9999", isDir: true}, // doesn't match
				}, nil
			}
			return nil, nil
		}

		// Mock queryPersistentVolumeClaims to return specific PVCs
		queryPersistentVolumeClaims = func(_ string) (*corev1.PersistentVolumeClaimList, error) {
			return &corev1.PersistentVolumeClaimList{
				Items: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pvc-1",
							Namespace: "test-ns",
							UID:       "1234",
						},
					},
				},
			}, nil
		}

		envVars, err := printer.GetEnvVars()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if envVars == nil {
			t.Error("Expected non-nil envVars")
		}

		// Check that only the matching PVC is in envVars
		projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
		expectedPath := filepath.Join(projectRoot, ".volumes", "pvc-1234")
		if envVars["PV_TEST_NS_PVC_1"] != expectedPath {
			t.Errorf("Expected PV_TEST_NS_PVC_1=%s, got %s", expectedPath, envVars["PV_TEST_NS_PVC_1"])
		}
	})
}

// TestKubeEnvPrinter_Print tests the Print method of the KubeEnvPrinter
func TestKubeEnvPrinter_Print(t *testing.T) {
	setup := func(t *testing.T) (*KubeEnvPrinter, *Mocks) {
		t.Helper()
		mocks := setupKubeEnvMocks(t)
		printer := NewKubeEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}
		return printer, mocks
	}

	t.Run("Success", func(t *testing.T) {
		printer, _ := setup(t)

		err := printer.Print()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("GetConfigError", func(t *testing.T) {
		mockConfigHandler := config.NewMockConfigHandler()
		mockConfigHandler.GetConfigRootFunc = func() (string, error) {
			return "", errors.New("mock config error")
		}
		mocks := setupKubeEnvMocks(t, &SetupOptions{ConfigHandler: mockConfigHandler})
		printer := NewKubeEnvPrinter(mocks.Injector)
		printer.shims = mocks.Shims
		if err := printer.Initialize(); err != nil {
			t.Fatalf("Failed to initialize printer: %v", err)
		}

		err := printer.Print()
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}
