package generators

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewKustomizeGenerator(t *testing.T) {
	t.Run("NewKustomizeGenerator", func(t *testing.T) {
		// Given a set of mocks
		mocks := setupMocks(t)

		// When a new KustomizeGenerator is created
		generator := NewKustomizeGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize KustomizeGenerator: %v", err)
		}

		// Then the KustomizeGenerator should be created correctly
		if generator == nil {
			t.Fatalf("expected KustomizeGenerator to be created, got nil")
		}

		// And the KustomizeGenerator should have the correct injector
		if generator.injector != mocks.Injector {
			t.Errorf("expected KustomizeGenerator to have the correct injector")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestKustomizeGenerator_Write(t *testing.T) {
	setup := func(t *testing.T) (*KustomizeGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewKustomizeGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize KustomizeGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a KustomizeGenerator with mocks
		generator, mocks := setup(t)

		// And the project root is set
		expectedProjectRoot := "/mock/project/root"
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return expectedProjectRoot, nil
		}

		// And the shims are configured for file operations
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			expectedPath := filepath.Join(expectedProjectRoot, "kustomize")
			if path != expectedPath {
				t.Errorf("expected path %s, got %s", expectedPath, path)
			}
			return nil
		}

		mocks.Shims.Stat = func(name string) (fs.FileInfo, error) {
			expectedPath := filepath.Join(expectedProjectRoot, "kustomize", "kustomization.yaml")
			if name == expectedPath {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}

		mocks.Shims.WriteFile = func(filename string, data []byte, perm fs.FileMode) error {
			expectedPath := filepath.Join(expectedProjectRoot, "kustomize", "kustomization.yaml")
			if filename != expectedPath {
				t.Errorf("expected filename %s, got %s", expectedPath, filename)
			}
			expectedContent := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources: []\n"
			if string(data) != expectedContent {
				t.Errorf("expected content %s, got %s", expectedContent, string(data))
			}
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of mocks
		mocks := setupMocks(t)

		// And GetProjectRoot is mocked to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// And a new KustomizeGenerator is created
		generator := NewKustomizeGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize KustomizeGenerator: %v", err)
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "mock error getting project root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadingKustomization", func(t *testing.T) {
		// Given a KustomizeGenerator with mocks
		generator, mocks := setup(t)

		// And MkdirAll is mocked to return an error
		mocks.Shims.MkdirAll = func(_ string, _ fs.FileMode) error {
			return fmt.Errorf("mock error reading kustomization.yaml")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "mock error reading kustomization.yaml"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("KustomizationDoesNotExist", func(t *testing.T) {
		// Given a KustomizeGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to simulate kustomization.yaml does not exist
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And WriteFile is mocked to simulate successful file creation
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("KustomizationExists", func(t *testing.T) {
		// Given a KustomizeGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to simulate kustomization.yaml exists
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, nil
		}

		// And WriteFile should not be called
		writeFileCalled := false
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			writeFileCalled = true
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And WriteFile should not have been called
		if writeFileCalled {
			t.Error("expected WriteFile not to be called when file exists")
		}
	})

	t.Run("ErrorWritingKustomization", func(t *testing.T) {
		// Given a KustomizeGenerator with mocks
		generator, mocks := setup(t)

		// And Stat is mocked to simulate kustomization.yaml does not exist
		mocks.Shims.Stat = func(_ string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		}

		// And WriteFile is mocked to simulate an error during file writing
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing kustomization.yaml")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "mock error writing kustomization.yaml"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}
