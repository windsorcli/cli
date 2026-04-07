package blueprint

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type mockFileInfo struct{}

func (m *mockFileInfo) Name() string       { return "blueprint.yaml" }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{}   { return nil }

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

	t.Run("CreatesWriterWithDefaultShims", func(t *testing.T) {
		// Given a writer
		mocks := setupWriterMocks(t)

		// When creating a writer
		writer := NewBlueprintWriter(mocks.Runtime)

		// Then writer should have default shims
		if writer.shims == nil {
			t.Error("Expected default shims to be set")
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
		err := writer.Write(blueprint, true)

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

	t.Run("WritesMinimalBlueprintWithInitURLsWhenFileDoesNotExist", func(t *testing.T) {
		// Given a writer, no existing file, and initBlueprintURLs
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
			Kind:       "Blueprint",
			ApiVersion: "v1alpha1",
			Metadata:   blueprintv1alpha1.Metadata{Name: "test-blueprint"},
			Sources:    []blueprintv1alpha1.Source{},
		}
		initURL := "oci://ghcr.io/windsorcli/core:v1.0.0"

		// When writing with file not existing and initBlueprintURLs
		err := writer.Write(blueprint, true, initURL)

		// Then minimal blueprint should be written with source from init URL
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(writtenData) == 0 {
			t.Fatal("Expected data to be written")
		}
		writtenStr := string(writtenData)
		if !strings.Contains(writtenStr, "name: core") {
			t.Error("Expected written blueprint to contain source name 'core' from init URL")
		}
		if !strings.Contains(writtenStr, "oci://ghcr.io/windsorcli/core:v1.0.0") {
			t.Error("Expected written blueprint to contain init URL")
		}
		if !strings.Contains(writtenStr, "install: true") {
			t.Error("Expected written blueprint to contain install: true for init source")
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

	t.Run("WritesMinimalWhenFileExistsAndOverwriteTrueWithInitURLs", func(t *testing.T) {
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		var writtenData []byte
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, nil
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			writtenData = data
			return nil
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "local"},
			Sources:  []blueprintv1alpha1.Source{{Name: "core", Url: "oci://ghcr.io/windsorcli/core:latest"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Name: "cluster", Path: "cluster/talos", Source: "core"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "csi", Path: "csi", Source: "core"},
			},
		}

		err := writer.Write(blueprint, true, "oci://ghcr.io/windsorcli/core:latest")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		dataStr := string(writtenData)
		if strings.Contains(dataStr, "cluster/talos") || strings.Contains(dataStr, "path: csi") {
			t.Error("Expected minimal blueprint (no terraform/kustomize expansion) when overwrite=true with init URLs")
		}
		if !strings.Contains(dataStr, "sources:") || !strings.Contains(dataStr, "core") {
			t.Error("Expected minimal blueprint to contain sources")
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
		err := writer.Write(blueprint, true)

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
		err := writer.Write(blueprint, true)

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
		err := writer.Write(blueprint, true)

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
		err := writer.Write(blueprint, true)

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
		err := writer.Write(blueprint, true)

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
		err := writer.Write(blueprint, true)

		// Then an error should be returned
		if err == nil {
			t.Error("Expected error when stat fails unexpectedly")
		}
	})

	t.Run("StripsValuesCommonFromConfigMaps", func(t *testing.T) {
		// Given a blueprint with only values-common ConfigMap
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		var marshalledBlueprint *blueprintv1alpha1.Blueprint
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}
		writer.shims.YamlMarshal = func(v interface{}) ([]byte, error) {
			if bp, ok := v.(*blueprintv1alpha1.Blueprint); ok {
				marshalledBlueprint = bp
			}
			return []byte("test yaml"), nil
		}

		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			ConfigMaps: map[string]map[string]string{
				"values-common": {
					"DOMAIN":  "example.com",
					"CONTEXT": "test",
				},
			},
		}

		// When writing the blueprint
		err := writer.Write(blueprint, true)

		// Then values-common should be stripped and ConfigMaps should be nil
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if marshalledBlueprint == nil {
			t.Fatal("Expected blueprint to be marshalled")
		}
		if marshalledBlueprint.ConfigMaps != nil {
			t.Error("Expected ConfigMaps to be nil when only values-common exists")
		}
	})

	t.Run("StripsInstallFalseFromSources", func(t *testing.T) {
		mocks := setupWriterMocks(t)
		writer := NewBlueprintWriter(mocks.Runtime)

		var marshalledBlueprint *blueprintv1alpha1.Blueprint
		writer.shims.Stat = func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		writer.shims.MkdirAll = func(path string, perm os.FileMode) error {
			return nil
		}
		writer.shims.WriteFile = func(path string, data []byte, perm os.FileMode) error {
			return nil
		}
		writer.shims.YamlMarshal = func(v interface{}) ([]byte, error) {
			if bp, ok := v.(*blueprintv1alpha1.Blueprint); ok {
				marshalledBlueprint = bp
			}
			return []byte("test yaml"), nil
		}

		falseVal := false
		blueprint := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "test"},
			Sources: []blueprintv1alpha1.Source{
				{
					Name:    "test-source",
					Url:     "oci://example.com/repo:tag",
					Install: &blueprintv1alpha1.BoolExpression{Value: &falseVal, IsExpr: false},
				},
			},
		}

		err := writer.Write(blueprint, true)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if marshalledBlueprint == nil {
			t.Fatal("Expected blueprint to be marshalled")
		}
		if len(marshalledBlueprint.Sources) != 1 {
			t.Fatalf("Expected 1 source, got %d", len(marshalledBlueprint.Sources))
		}
		if marshalledBlueprint.Sources[0].Install != nil {
			t.Error("Expected install: false to be stripped from source")
		}
	})
}
