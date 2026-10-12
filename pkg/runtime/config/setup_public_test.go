package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type fixture struct {
	root    string
	shell   *shell.MockShell
	handler config.ConfigHandler
}

const testSchemaHeader = "$schema: https://json-schema.org/draft/2020-12/schema\n"

// newFixture returns a handler whose project root is a fresh temporary directory. It clears
// WINDSOR_CONTEXT for the test, so context resolution starts from the file and the default.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("WINDSOR_CONTEXT", "")
	root := t.TempDir()
	sh := shell.NewMockShell()
	sh.GetProjectRootFunc = func() (string, error) { return root, nil }
	return &fixture{root: root, shell: sh, handler: config.NewConfigHandler(sh)}
}

// newHandler returns a second handler over the same project root.
func (f *fixture) newHandler() config.ConfigHandler {
	return config.NewConfigHandler(f.shell)
}

// write creates the file at rel under the project root with content.
func (f *fixture) write(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// read returns the content of the file at rel under the project root.
func (f *fixture) read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// exists reports whether the file at rel under the project root exists.
func (f *fixture) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(f.root, rel))
	return err == nil
}

// readYAML decodes the file at rel under the project root into a map.
func (f *fixture) readYAML(t *testing.T, rel string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := yaml.Unmarshal([]byte(f.read(t, rel)), &out); err != nil {
		t.Fatalf("decode %s: %v", rel, err)
	}
	return out
}

// load runs LoadConfig and fails the test on error.
func (f *fixture) load(t *testing.T) {
	t.Helper()
	if err := f.handler.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
}

// loadSchema loads a schema from properties YAML, indented under "properties:".
func (f *fixture) loadSchema(t *testing.T, properties string) {
	t.Helper()
	if err := f.handler.LoadSchemaFromBytes([]byte(testSchemaHeader + "type: object\nproperties:\n" + properties)); err != nil {
		t.Fatalf("LoadSchemaFromBytes: %v", err)
	}
}
