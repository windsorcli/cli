package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewKustomizeGenerator(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// When a new KustomizeGenerator is created
		generator := NewKustomizeGenerator(mocks.Injector)

		// Then the generator should be non-nil
		if generator == nil {
			t.Errorf("Expected NewKustomizeGenerator to return a non-nil value")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestKustomizeGenerator_Write(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osMkdirAll, osStat, and osWriteFile functions are saved
		originalMkdirAll := osMkdirAll
		originalStat := osStat
		originalWriteFile := osWriteFile
		defer func() {
			osMkdirAll = originalMkdirAll
			osStat = originalStat
			osWriteFile = originalWriteFile
		}()

		// And the shell's GetProjectRoot method is mocked to return a predefined path
		expectedProjectRoot := "/mock/project/root"
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return expectedProjectRoot, nil
		}

		// And the osMkdirAll function is mocked to simulate directory creation
		osMkdirAll = func(path string, perm os.FileMode) error {
			if path != filepath.Join(expectedProjectRoot, "kustomize") {
				t.Errorf("Unexpected path for osMkdirAll: %s", path)
			}
			return nil
		}

		// And the osStat function is mocked to simulate the file not existing
		osStat = func(name string) (os.FileInfo, error) {
			if name == filepath.Join(expectedProjectRoot, "kustomize", "kustomization.yaml") {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		// And the osWriteFile function is mocked to simulate file writing
		osWriteFile = func(filename string, data []byte, perm os.FileMode) error {
			expectedFilePath := filepath.Join(expectedProjectRoot, "kustomize", "kustomization.yaml")
			if filename != expectedFilePath {
				t.Errorf("Unexpected filename for osWriteFile: %s", filename)
			}
			expectedContent := []byte("resources: []\n")
			if string(data) != string(expectedContent) {
				t.Errorf("Unexpected content for osWriteFile: %s", string(data))
			}
			return nil
		}

		// And a new KustomizeGenerator is created and initialized
		generator := NewKustomizeGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected KustomizeGenerator.Initialize to return nil, got %v", err)
		}

		// When the Write method is called
		err := generator.Write()

		// Then no error should occur
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original GetProjectRootFunc is saved
		originalGetProjectRootFunc := mocks.MockShell.GetProjectRootFunc
		defer func() {
			mocks.MockShell.GetProjectRootFunc = originalGetProjectRootFunc
		}()

		// And the shell's GetProjectRoot method is mocked to return an error
		mocks.MockShell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mocked error in GetProjectRoot")
		}

		// And a new KustomizeGenerator is created and initialized
		generator := NewKustomizeGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected KustomizeGenerator.Initialize to return nil, got %v", err)
		}

		// When the Write method is called
		err := generator.Write()

		// Then an error should occur
		if err == nil || !strings.Contains(err.Error(), "mocked error in GetProjectRoot") {
			t.Errorf("Expected error containing 'mocked error in GetProjectRoot', got %v", err)
		}
	})

	t.Run("ErrorCreatingDirectory", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osMkdirAll function is saved
		originalMkdirAll := osMkdirAll
		defer func() {
			osMkdirAll = originalMkdirAll
		}()

		// And the osMkdirAll function is mocked to simulate an error when creating the directory
		osMkdirAll = func(path string, perm os.FileMode) error {
			return fmt.Errorf("mocked error in osMkdirAll")
		}

		// And a new KustomizeGenerator is created and initialized
		generator := NewKustomizeGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected KustomizeGenerator.Initialize to return nil, got %v", err)
		}

		// When the Write method is called
		err := generator.Write()

		// Then an error should occur
		if err == nil || !strings.Contains(err.Error(), "mocked error in osMkdirAll") {
			t.Errorf("Expected error containing 'mocked error in osMkdirAll', got %v", err)
		}
	})

	t.Run("FileAlreadyExists", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osStat function is saved
		originalStat := osStat
		defer func() {
			osStat = originalStat
		}()

		// And the osStat function is mocked to simulate the file already existing
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}

		// And a new KustomizeGenerator is created and initialized
		generator := NewKustomizeGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected KustomizeGenerator.Initialize to return nil, got %v", err)
		}

		// When the Write method is called
		err := generator.Write()

		// Then no error should occur because the file already exists
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("ErrorWritingFile", func(t *testing.T) {
		// Given a set of safe mocks
		mocks := setupSafeMocks()

		// And the original osWriteFile function is saved
		originalWriteFile := osWriteFile
		defer func() {
			osWriteFile = originalWriteFile
		}()

		// And the osWriteFile function is mocked to simulate an error when writing the file
		osWriteFile = func(name string, data []byte, perm os.FileMode) error {
			return fmt.Errorf("mocked error in osWriteFile")
		}

		// And a new KustomizeGenerator is created and initialized
		generator := NewKustomizeGenerator(mocks.Injector)
		if err := generator.Initialize(); err != nil {
			t.Errorf("Expected KustomizeGenerator.Initialize to return nil, got %v", err)
		}

		// When the Write method is called
		err := generator.Write()

		// Then an error should occur
		if err == nil || !strings.Contains(err.Error(), "mocked error in osWriteFile") {
			t.Errorf("Expected error containing 'mocked error in osWriteFile', got %v", err)
		}
	})
}
