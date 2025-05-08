package generators

import (
	"fmt"
	"io/fs"
	"os"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

const (
	gitGenTestMockGitignorePath = "/mock/project/root/.gitignore"
	gitGenTestExistingContent   = "existing content\n"
	gitGenTestExpectedContent   = `existing content

# managed by windsor cli
.windsor/
.volumes/
terraform/**/backend_override.tf
contexts/**/.terraform/
contexts/**/.tfstate/
contexts/**/.kube/
contexts/**/.talos/
contexts/**/.omni/
contexts/**/.aws/
`
	gitGenTestExpectedPerm = fs.FileMode(0644)
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestGitGenerator_NewGitGenerator(t *testing.T) {
	t.Run("NewGitGenerator", func(t *testing.T) {
		// Given a set of mocks
		mocks := setupMocks(t)

		// When a new GitGenerator is created
		generator := NewGitGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// Then the GitGenerator should be created correctly
		if generator == nil {
			t.Fatalf("expected GitGenerator to be created, got nil")
		}

		// And the GitGenerator should have the correct injector
		if generator.injector != mocks.Injector {
			t.Errorf("expected GitGenerator to have the correct injector")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGitGenerator_Write(t *testing.T) {
	setup := func(t *testing.T) (*GitGenerator, *Mocks) {
		mocks := setupMocks(t)
		generator := NewGitGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}
		return generator, mocks
	}

	t.Run("Success", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And GetProjectRoot is mocked to return a specific path
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "/mock/project/root", nil
		}

		// And ReadFile is mocked to return existing content
		mocks.Shims.ReadFile = func(path string) ([]byte, error) {
			if path == "/mock/project/root/.gitignore" {
				return []byte(gitGenTestExistingContent), nil
			}
			return nil, fmt.Errorf("unexpected file read: %s", path)
		}

		// And WriteFile is mocked to verify the content
		var writtenPath string
		var writtenContent []byte
		mocks.Shims.WriteFile = func(path string, content []byte, _ fs.FileMode) error {
			writtenPath = path
			writtenContent = content
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should occur
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// And the file should be written to the correct path
		expectedPath := "/mock/project/root/.gitignore"
		if writtenPath != expectedPath {
			t.Errorf("expected filename %s, got %s", expectedPath, writtenPath)
		}

		// And the content should be correct
		expectedContent := gitGenTestExpectedContent
		if string(writtenContent) != expectedContent {
			t.Errorf("expected content %s, got %s", expectedContent, string(writtenContent))
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		// Given a set of mocks
		mocks := setupMocks(t)

		// And GetProjectRoot is mocked to return an error
		mocks.Shell.GetProjectRootFunc = func() (string, error) {
			return "", fmt.Errorf("mock error getting project root")
		}

		// And a new GitGenerator is created
		generator := NewGitGenerator(mocks.Injector)
		generator.shims = mocks.Shims
		if err := generator.Initialize(); err != nil {
			t.Fatalf("failed to initialize GitGenerator: %v", err)
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to get project root: mock error getting project root"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("ErrorReadingGitignore", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to return an error
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
			return nil, fmt.Errorf("mock error reading .gitignore")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to read .gitignore: mock error reading .gitignore"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})

	t.Run("GitignoreDoesNotExist", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And ReadFile is mocked to simulate .gitignore does not exist
		mocks.Shims.ReadFile = func(_ string) ([]byte, error) {
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

	t.Run("ErrorWritingGitignore", func(t *testing.T) {
		// Given a GitGenerator with mocks
		generator, mocks := setup(t)

		// And WriteFile is mocked to simulate an error during file writing
		mocks.Shims.WriteFile = func(_ string, _ []byte, _ fs.FileMode) error {
			return fmt.Errorf("mock error writing .gitignore")
		}

		// When Write is called
		err := generator.Write()

		// Then an error should be returned
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		// And the error should match the expected error
		expectedError := "failed to write to .gitignore: mock error writing .gitignore"
		if err.Error() != expectedError {
			t.Errorf("expected error %s, got %s", expectedError, err.Error())
		}
	})
}
