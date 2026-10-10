package shims_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Test Setup
// =============================================================================

type packageShims struct {
	shims.Shims
	Exit func(int)
}

type sample struct {
	Name  string `json:"name" yaml:"name"`
	Count int    `json:"count" yaml:"count"`
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewShims(t *testing.T) {
	t.Run("SetsEveryField", func(t *testing.T) {
		// Given the default shims
		s := shims.NewShims()

		// When each field is inspected
		v := reflect.ValueOf(s).Elem()

		// Then no function field is nil
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).IsNil() {
				t.Errorf("expected field %s to be set", v.Type().Field(i).Name)
			}
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestShims_Defaults(t *testing.T) {
	t.Run("FileFunctionsUseTheFilesystem", func(t *testing.T) {
		// Given the default shims and an empty directory
		s := shims.NewShims()
		dir := t.TempDir()
		sub := filepath.Join(dir, "a", "b")
		file := filepath.Join(sub, "f.txt")
		moved := filepath.Join(sub, "g.txt")

		// When a file is created, moved, listed, and removed
		if err := s.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := s.WriteFile(file, []byte("hi"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := s.Rename(file, moved); err != nil {
			t.Fatalf("Rename: %v", err)
		}
		data, err := s.ReadFile(moved)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		entries, err := s.ReadDir(sub)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		matches, err := s.Glob(filepath.Join(sub, "*.txt"))
		if err != nil {
			t.Fatalf("Glob: %v", err)
		}
		if err := s.Remove(moved); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if err := s.RemoveAll(filepath.Join(dir, "a")); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}
		_, statErr := s.Stat(sub)

		// Then each step reflects the real filesystem state
		if string(data) != "hi" {
			t.Errorf("expected content %q, got %q", "hi", data)
		}
		if len(entries) != 1 || entries[0].Name() != "g.txt" {
			t.Errorf("expected one entry g.txt, got %v", entries)
		}
		if len(matches) != 1 || matches[0] != moved {
			t.Errorf("expected glob match %s, got %v", moved, matches)
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("expected not-exist after RemoveAll, got %v", statErr)
		}
	})

	t.Run("EnvironmentFunctionsUseTheProcessEnvironment", func(t *testing.T) {
		// Given the default shims and a variable scoped to this test
		s := shims.NewShims()
		t.Setenv("WINDSOR_SHIMS_TEST", "")

		// When the variable is set through the shim
		if err := s.Setenv("WINDSOR_SHIMS_TEST", "value"); err != nil {
			t.Fatalf("Setenv: %v", err)
		}
		got := s.Getenv("WINDSOR_SHIMS_TEST")
		looked, ok := s.LookupEnv("WINDSOR_SHIMS_TEST")

		// Then both readers return the new value
		if got != "value" || looked != "value" || !ok {
			t.Errorf("expected value from Getenv and LookupEnv, got %q, %q, %v", got, looked, ok)
		}
	})

	t.Run("ProcessFunctionsMatchTheStandardLibrary", func(t *testing.T) {
		// Given the default shims
		s := shims.NewShims()

		// When the process functions are called
		wd, wdErr := s.Getwd()
		home, homeErr := s.UserHomeDir()
		wantWd, _ := os.Getwd()
		wantHome, _ := os.UserHomeDir()

		// Then they return what the standard library returns
		if wdErr != nil || wd != wantWd {
			t.Errorf("expected Getwd %q, got %q (%v)", wantWd, wd, wdErr)
		}
		if homeErr != nil || home != wantHome {
			t.Errorf("expected UserHomeDir %q, got %q (%v)", wantHome, home, homeErr)
		}
		if s.Goos() != runtime.GOOS {
			t.Errorf("expected Goos %q, got %q", runtime.GOOS, s.Goos())
		}
	})

	t.Run("MarshalFunctionsRoundTrip", func(t *testing.T) {
		// Given the default shims and a value
		s := shims.NewShims()
		in := sample{Name: "x", Count: 2}

		// When the value goes through YAML and JSON
		y, err := s.YamlMarshal(in)
		if err != nil {
			t.Fatalf("YamlMarshal: %v", err)
		}
		var fromYaml sample
		if err := s.YamlUnmarshal(y, &fromYaml); err != nil {
			t.Fatalf("YamlUnmarshal: %v", err)
		}
		j, err := s.JsonMarshal(in)
		if err != nil {
			t.Fatalf("JsonMarshal: %v", err)
		}
		var fromJson sample
		if err := s.JsonUnmarshal(j, &fromJson); err != nil {
			t.Fatalf("JsonUnmarshal: %v", err)
		}

		// Then both decode to the original value
		if fromYaml != in || fromJson != in {
			t.Errorf("expected %+v, got yaml %+v and json %+v", in, fromYaml, fromJson)
		}
	})

	t.Run("EmbeddingStructOverridesAPromotedField", func(t *testing.T) {
		// Given a package shims struct that embeds the shared shims
		p := &packageShims{Shims: *shims.NewShims(), Exit: func(int) {}}
		p.ReadFile = func(string) ([]byte, error) { return []byte("fake"), nil }

		// When the promoted field is called
		data, err := p.ReadFile("/does/not/exist")

		// Then the override runs instead of the filesystem
		if err != nil || string(data) != "fake" {
			t.Errorf("expected override result, got %q (%v)", data, err)
		}
	})
}
