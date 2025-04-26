package generators

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

		// And ReadFile is mocked to return predefined content
		mocks.Shims.ReadFile = func(filename string) ([]byte, error) {
			if filepath.ToSlash(filename) == gitGenTestMockGitignorePath {
				return []byte(gitGenTestExistingContent), nil
			}
			return []byte{}, nil
		}

		// And MkdirAll is mocked to handle directory creation
		mocks.Shims.MkdirAll = func(path string, perm fs.FileMode) error {
			return nil
		}

		// And WriteFile is mocked to capture parameters
		var capturedFilename string
		var capturedContent []byte
		var capturedPerm fs.FileMode
		mocks.Shims.WriteFile = func(filename string, content []byte, perm fs.FileMode) error {
			capturedFilename = filename
			capturedContent = content
			capturedPerm = perm
			return nil
		}

		// When Write is called
		err := generator.Write()

		// Then no error should be returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And WriteFile should be called with the correct parameters
		if filepath.ToSlash(capturedFilename) != gitGenTestMockGitignorePath {
			t.Errorf("expected filename %s, got %s", gitGenTestMockGitignorePath, capturedFilename)
		}

		// Normalize line endings for cross-platform compatibility
		normalizeLineEndings := func(content string) string {
			return strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", "")
		}

		if normalizeLineEndings(string(capturedContent)) != normalizeLineEndings(gitGenTestExpectedContent) {
			t.Errorf("expected content %s, got %s", gitGenTestExpectedContent, string(capturedContent))
		}

		if capturedPerm != gitGenTestExpectedPerm {
			t.Errorf("expected permission %v, got %v", gitGenTestExpectedPerm, capturedPerm)
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
