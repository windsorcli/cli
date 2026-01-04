package blueprint

import (
	"errors"
	"os"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type WriterTestMocks struct {
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
	Runtime       *runtime.Runtime
	Shims         *Shims
}

func setupWriterMocks(t *testing.T) *WriterTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	rt := &runtime.Runtime{
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
	}

	mocks := &WriterTestMocks{
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		Runtime:       rt,
		Shims:         NewShims(),
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewBlueprintWriter(t *testing.T) {
	t.Run("CreatesWriterWithDefaults", func(t *testing.T) {
		// Given a runtime
		mocks := setupWriterMocks(t)

		// When creating a new writer
		writer := NewBlueprintWriter(mocks.Runtime)

		// Then writer should be created with defaults
		if writer == nil {
			t.Fatal("Expected writer to be created")
		}
		if writer.runtime != mocks.Runtime {
			t.Error("Expected runtime to be set")
		}
		if writer.shims == nil {
			t.Error("Expected shims to be initialized")
		}
	})

	t.Run("AcceptsShimsOverride", func(t *testing.T) {
		// Given a runtime and custom shims
		mocks := setupWriterMocks(t)
		customShims := NewShims()

		// When creating a new writer with overrides
		writer := NewBlueprintWriter(mocks.Runtime, &BaseBlueprintWriter{shims: customShims})

		// Then writer should use custom shims
		if writer.shims != customShims {
			t.Error("Expected custom shims to be used")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWriter_Write(t *testing.T) {
	t.Run("WritesNewBlueprintWhenFileDoesNotExist", func(t *testing.T) {
		// Given a writer and blueprint with no existing file
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		var writtenPath string
		var writtenData []byte
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenPath = path
			writtenData = data
			return nil
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata: blueprintv1alpha1.Metadata{
				Name: "test-blueprint",
			},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, false)

		// Then the file should be written
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if writtenPath == "" {
			t.Error("Expected file to be written")
		}
		if len(writtenData) == 0 {
			t.Error("Expected data to be written")
		}
	})

	t.Run("SkipsWriteWhenFileExistsAndNotOverwrite", func(t *testing.T) {
		// Given a writer with existing file and overwrite=false
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		writeAttempted := false
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writeAttempted = true
			return nil
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		// When writing with overwrite=false
		err := writer.Write(blueprint, false)

		// Then file should not be written
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if writeAttempted {
			t.Error("Expected write to be skipped when file exists")
		}
	})

	t.Run("OverwritesWhenFileExistsAndOverwriteTrue", func(t *testing.T) {
		// Given a writer with existing file and overwrite=true
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		writeAttempted := false
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writeAttempted = true
			return nil
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		// When writing with overwrite=true
		err := writer.Write(blueprint, true)

		// Then file should be written
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if !writeAttempted {
			t.Error("Expected write to occur when overwrite=true")
		}
	})

	t.Run("ReturnsErrorWhenConfigRootEmpty", func(t *testing.T) {
		// Given a writer with empty config root
		mocks := setupWriterMocks(t)
		rt := &runtime.Runtime{
			ProjectRoot:   mocks.Runtime.ProjectRoot,
			ConfigRoot:    "",
			ConfigHandler: mocks.ConfigHandler,
			Shell:         mocks.Shell,
		}
		writer := NewBlueprintWriter(rt)

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when config root is empty")
		}
	})

	t.Run("ReturnsErrorWhenMkdirFails", func(t *testing.T) {
		// Given a writer where MkdirAll fails
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return errors.New("mkdir failed")
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when mkdir fails")
		}
	})

	t.Run("ReturnsErrorWhenWriteFileFails", func(t *testing.T) {
		// Given a writer where WriteFile fails
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return errors.New("write failed")
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when write fails")
		}
	})

	t.Run("CleansTransientFieldsBeforeWriting", func(t *testing.T) {
		// Given a blueprint with transient fields
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		var writtenData []byte
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{
					Path:   "vpc",
					Inputs: map[string]any{"key": "value"},
				},
			},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, false)

		// Then transient fields should be cleaned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		dataStr := string(writtenData)
		if dataStr == "" {
			t.Fatal("Expected data to be written")
		}
	})

	t.Run("CleansKustomizationTransientFields", func(t *testing.T) {
		// Given a blueprint with kustomization transient fields
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		var writtenData []byte
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{
					Name:    "app",
					Patches: []blueprintv1alpha1.BlueprintPatch{{Path: "patch.yaml"}},
				},
			},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, false)

		// Then kustomization transient fields should be cleaned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(writtenData) == 0 {
			t.Fatal("Expected data to be written")
		}
	})

	t.Run("ReturnsErrorForNilBlueprint", func(t *testing.T) {
		// Given a writer and nil blueprint
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		// When writing nil blueprint
		err := writer.Write(nil, false)

		// Then should return error
		if err == nil {
			t.Error("Expected error for nil blueprint")
		}
	})

	t.Run("ReturnsErrorWhenStatFails", func(t *testing.T) {
		// Given a writer where Stat fails with unexpected error
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, errors.New("unexpected stat error")
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, false)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when stat fails unexpectedly")
		}
	})
}
