package terraform

import (
	"archive/tar"
	"io"
	"testing"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("CreatesShimsWithDefaultImplementations", func(t *testing.T) {
		// Given a new shims instance
		shims := NewShims()

		// Then all function fields should be set
		if shims.MkdirAll == nil {
			t.Error("Expected MkdirAll to be set")
		}
		if shims.WriteFile == nil {
			t.Error("Expected WriteFile to be set")
		}
		if shims.ReadFile == nil {
			t.Error("Expected ReadFile to be set")
		}
		if shims.Stat == nil {
			t.Error("Expected Stat to be set")
		}
		if shims.Chdir == nil {
			t.Error("Expected Chdir to be set")
		}
		if shims.FilepathRel == nil {
			t.Error("Expected FilepathRel to be set")
		}
		if shims.JsonUnmarshal == nil {
			t.Error("Expected JsonUnmarshal to be set")
		}
		if shims.NewBytesReader == nil {
			t.Error("Expected NewBytesReader to be set")
		}
		if shims.NewTarReader == nil {
			t.Error("Expected NewTarReader to be set")
		}
		if shims.EOFError == nil {
			t.Error("Expected EOFError to be set")
		}
		if shims.TypeDir == nil {
			t.Error("Expected TypeDir to be set")
		}
		if shims.Create == nil {
			t.Error("Expected Create to be set")
		}
		if shims.Copy == nil {
			t.Error("Expected Copy to be set")
		}
		if shims.Chmod == nil {
			t.Error("Expected Chmod to be set")
		}
		if shims.Setenv == nil {
			t.Error("Expected Setenv to be set")
		}
	})

	t.Run("ShimsProvideBasicFunctionality", func(t *testing.T) {
		// Given real shims
		shims := NewShims()

		// When testing basic functionality
		// Then all function fields should work

		// And NewBytesReader should work
		reader := shims.NewBytesReader([]byte("test"))
		if reader == nil {
			t.Error("Expected NewBytesReader to create a reader")
		}

		// And NewTarReader should work
		tarReader := shims.NewTarReader(reader)
		if tarReader == nil {
			t.Error("Expected NewTarReader to create a tar reader")
		}

		// And EOFError should return io.EOF
		if shims.EOFError() != io.EOF {
			t.Error("Expected EOFError to return io.EOF")
		}

		// And TypeDir should return tar.TypeDir
		if shims.TypeDir() != tar.TypeDir {
			t.Error("Expected TypeDir to return tar.TypeDir")
		}

		// And FilepathRel should work
		rel, err := shims.FilepathRel("/base", "/base/path")
		if err != nil {
			t.Errorf("Expected FilepathRel to work, got error: %v", err)
		}
		if rel != "path" {
			t.Errorf("Expected relative path 'path', got: %s", rel)
		}
	})
}
