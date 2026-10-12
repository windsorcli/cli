package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewConfigHandler(t *testing.T) {
	t.Run("PanicsWithoutAShell", func(t *testing.T) {
		// Given no shell
		var sh shell.Shell

		// When a handler is created
		panicked := func() (p bool) {
			defer func() { p = recover() != nil }()
			config.NewConfigHandler(sh)
			return false
		}()

		// Then the constructor panics
		if !panicked {
			t.Error("expected a panic")
		}
	})

	t.Run("StartsUnloadedAndEmpty", func(t *testing.T) {
		// Given a new handler
		f := newFixture(t)

		// When its state is read
		loaded := f.handler.IsLoaded()
		value := f.handler.Get("platform")

		// Then nothing is loaded
		if loaded || value != nil {
			t.Errorf("expected unloaded and empty, got loaded=%v platform=%v", loaded, value)
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLoadConfig(t *testing.T) {
	t.Run("SucceedsWithNoFilesAndStaysUnloaded", func(t *testing.T) {
		// Given an empty project
		f := newFixture(t)

		// When config is loaded
		err := f.handler.LoadConfig()

		// Then it succeeds and reports nothing loaded
		if err != nil || f.handler.IsLoaded() {
			t.Errorf("expected no error and unloaded, got err=%v loaded=%v", err, f.handler.IsLoaded())
		}
	})

	t.Run("ReadsTheCurrentContextFromTheRootFile", func(t *testing.T) {
		// Given a root windsor.yaml with two contexts
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\ncontexts:\n  local:\n    platform: docker\n  other:\n    platform: aws\n")

		// When config is loaded for the default context
		f.load(t)

		// Then only the local context's values are read
		if got := f.handler.GetString("platform"); got != "docker" {
			t.Errorf("expected platform docker, got %q", got)
		}
		if !f.handler.IsLoaded() {
			t.Error("expected loaded")
		}
	})

	t.Run("RootFileKeepsOnlyTypedContextFields", func(t *testing.T) {
		// Given a root windsor.yaml context with a typed and an untyped key
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\ncontexts:\n  local:\n    platform: docker\n    custom: kept\n")

		// When config is loaded
		f.load(t)

		// Then the untyped key is dropped
		if got := f.handler.Get("custom"); got != nil {
			t.Errorf("expected untyped key dropped, got %v", got)
		}
	})

	t.Run("RootFileWithoutTheContextMarksLoaded", func(t *testing.T) {
		// Given a root windsor.yaml with no entry for the current context
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\n")

		// When config is loaded
		f.load(t)

		// Then the handler reports loaded with no values
		if !f.handler.IsLoaded() || f.handler.Get("platform") != nil {
			t.Errorf("expected loaded and empty, got loaded=%v", f.handler.IsLoaded())
		}
	})

	t.Run("ContextFileKeepsUntypedFields", func(t *testing.T) {
		// Given a context-level windsor.yaml with an untyped key
		f := newFixture(t)
		f.write(t, "contexts/local/windsor.yaml", "custom:\n  nested: kept\n")

		// When config is loaded
		f.load(t)

		// Then the untyped key is read
		if got := f.handler.GetString("custom.nested"); got != "kept" {
			t.Errorf("expected custom.nested kept, got %q", got)
		}
	})

	t.Run("ReadsTheYmlExtensionWhenYamlIsAbsent", func(t *testing.T) {
		// Given a context-level windsor.yml only
		f := newFixture(t)
		f.write(t, "contexts/local/windsor.yml", "platform: incus\n")

		// When config is loaded
		f.load(t)

		// Then its values are read
		if got := f.handler.GetString("platform"); got != "incus" {
			t.Errorf("expected platform incus, got %q", got)
		}
	})

	t.Run("PrefersYamlOverYml", func(t *testing.T) {
		// Given both context-level windsor.yaml and windsor.yml
		f := newFixture(t)
		f.write(t, "contexts/local/windsor.yaml", "platform: docker\n")
		f.write(t, "contexts/local/windsor.yml", "platform: incus\n")

		// When config is loaded
		f.load(t)

		// Then windsor.yaml wins
		if got := f.handler.GetString("platform"); got != "docker" {
			t.Errorf("expected platform docker, got %q", got)
		}
	})

	t.Run("LayersSourcesWithValuesWinning", func(t *testing.T) {
		// Given every source sets the same keys at increasing precedence
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\ncontexts:\n  local:\n    platform: root\n    provider: root\n    id: root\n")
		f.write(t, "contexts/local/windsor.yaml", "platform: context\nprovider: context\nid: context\n")
		f.write(t, ".windsor/contexts/local/workstation.yaml", "platform: workstation\nprovider: workstation\n")
		f.write(t, "contexts/local/values.yaml", "platform: values\n")

		// When config is loaded
		f.load(t)

		// Then each key comes from the highest source that sets it
		want := map[string]string{"platform": "values", "provider": "workstation", "id": "context"}
		for key, value := range want {
			if got := f.handler.GetString(key); got != value {
				t.Errorf("%s: expected %q, got %q", key, value, got)
			}
		}
	})

	t.Run("MergesNestedMapsAcrossSources", func(t *testing.T) {
		// Given two sources that set different keys of one map
		f := newFixture(t)
		f.write(t, "contexts/local/windsor.yaml", "cluster:\n  driver: talos\n")
		f.write(t, "contexts/local/values.yaml", "cluster:\n  workers:\n    count: 2\n")

		// When config is loaded
		f.load(t)

		// Then both keys survive
		if f.handler.GetString("cluster.driver") != "talos" || f.handler.GetInt("cluster.workers.count") != 2 {
			t.Errorf("expected merged cluster map, got %v", f.handler.Get("cluster"))
		}
	})

	t.Run("WorkstationStateAloneDoesNotMarkLoaded", func(t *testing.T) {
		// Given only a workstation.yaml
		f := newFixture(t)
		f.write(t, ".windsor/contexts/local/workstation.yaml", "workstation:\n  runtime: colima\n")

		// When config is loaded
		f.load(t)

		// Then its values are read but the handler reports unloaded
		if got := f.handler.GetString("workstation.runtime"); got != "colima" {
			t.Errorf("expected workstation.runtime colima, got %q", got)
		}
		if f.handler.IsLoaded() {
			t.Error("expected unloaded")
		}
	})

	t.Run("LoadsTheTemplateSchema", func(t *testing.T) {
		// Given a template schema with a default
		f := newFixture(t)
		f.write(t, "contexts/_template/schema.yaml", testSchemaHeader+"type: object\nproperties:\n  region:\n    type: string\n    default: us-east-1\n")

		// When config is loaded
		f.load(t)

		// Then the schema is available and its default resolves
		if f.handler.GetSchema() == nil {
			t.Fatal("expected a schema")
		}
		if got := f.handler.GetString("region"); got != "us-east-1" {
			t.Errorf("expected region default, got %q", got)
		}
	})

	t.Run("LoadsTheAPISchemasForAVersionAfterV1alpha1", func(t *testing.T) {
		// Given a root windsor.yaml at v1alpha2
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\n")

		// When config is loaded
		f.load(t)

		// Then the embedded API schemas are loaded
		if f.handler.GetSchema() == nil {
			t.Error("expected API schemas to load")
		}
	})

	t.Run("SkipsTheAPISchemasForV1alpha1", func(t *testing.T) {
		// Given a root windsor.yaml at v1alpha1
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\n")

		// When config is loaded
		f.load(t)

		// Then no schema is loaded
		if f.handler.GetSchema() != nil {
			t.Error("expected no schema")
		}
	})

	t.Run("LoadsInvalidValuesWithAWarning", func(t *testing.T) {
		// Given values.yaml that violates the schema
		f := newFixture(t)
		f.loadSchema(t, "  count:\n    type: integer\n")
		f.write(t, "contexts/local/values.yaml", "count: many\n")

		// When config is loaded
		err := f.handler.LoadConfig()

		// Then the invalid value is still loaded
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got := f.handler.GetString("count"); got != "many" {
			t.Errorf("expected count many, got %q", got)
		}
	})

	t.Run("ReturnsAnErrorForMalformedFiles", func(t *testing.T) {
		// Given each source with malformed YAML
		files := map[string]string{
			"windsor.yaml":                             "version: [",
			"contexts/local/windsor.yaml":              "platform: [",
			".windsor/contexts/local/workstation.yaml": "workstation: [",
			"contexts/local/values.yaml":               "platform: [",
			"contexts/_template/schema.yaml":           "type: [",
		}
		for rel, content := range files {
			f := newFixture(t)
			f.write(t, rel, content)

			// When config is loaded
			err := f.handler.LoadConfig()

			// Then an error is returned
			if err == nil {
				t.Errorf("%s: expected an error", rel)
			}
		}
	})

	t.Run("ReturnsAnErrorWhenTheProjectRootFails", func(t *testing.T) {
		// Given a shell that cannot find the project root
		sh := shell.NewMockShell()
		sh.GetProjectRootFunc = func() (string, error) { return "", errors.New("no root") }
		handler := config.NewConfigHandler(sh)

		// When config is loaded
		err := handler.LoadConfig()

		// Then the error names the project root
		if err == nil || !strings.Contains(err.Error(), "project root") {
			t.Errorf("expected a project root error, got %v", err)
		}
	})
}

func TestLoadConfigForContext(t *testing.T) {
	t.Run("LoadsTheNamedContextWithoutSwitching", func(t *testing.T) {
		// Given two contexts with values
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "platform: docker\n")
		f.write(t, "contexts/prod/values.yaml", "platform: aws\n")

		// When the other context is loaded by name
		if err := f.handler.LoadConfigForContext("prod"); err != nil {
			t.Fatalf("LoadConfigForContext: %v", err)
		}

		// Then its values are read and the current context is unchanged
		if got := f.handler.GetString("platform"); got != "aws" {
			t.Errorf("expected platform aws, got %q", got)
		}
		if got := f.handler.GetContext(); got != "local" {
			t.Errorf("expected context local, got %q", got)
		}
		if f.exists(".windsor/context") {
			t.Error("expected no context file")
		}
	})
}

func TestLoadConfigString(t *testing.T) {
	t.Run("IgnoresEmptyContent", func(t *testing.T) {
		// Given a handler
		f := newFixture(t)

		// When empty content is loaded
		err := f.handler.LoadConfigString("")

		// Then nothing changes
		if err != nil || f.handler.IsLoaded() {
			t.Errorf("expected no-op, got err=%v loaded=%v", err, f.handler.IsLoaded())
		}
	})

	t.Run("SelectsTheCurrentContextFromAContextsMap", func(t *testing.T) {
		// Given content with a contexts map
		f := newFixture(t)

		// When it is loaded
		err := f.handler.LoadConfigString("contexts:\n  local:\n    platform: docker\n    custom: kept\n  other:\n    platform: aws\n")

		// Then the current context's values, typed or not, are read
		if err != nil {
			t.Fatalf("LoadConfigString: %v", err)
		}
		if f.handler.GetString("platform") != "docker" || f.handler.GetString("custom") != "kept" || !f.handler.IsLoaded() {
			t.Errorf("unexpected values platform=%q custom=%q", f.handler.GetString("platform"), f.handler.GetString("custom"))
		}
	})

	t.Run("MergesTheWholeMapWithoutAContextsKey", func(t *testing.T) {
		// Given content without a contexts key
		f := newFixture(t)

		// When it is loaded twice with different keys
		_ = f.handler.LoadConfigString("platform: docker\n")
		_ = f.handler.LoadConfigString("dns:\n  domain: test\n")

		// Then both loads are merged
		if f.handler.GetString("platform") != "docker" || f.handler.GetString("dns.domain") != "test" {
			t.Errorf("expected merged values, got platform=%q dns.domain=%q", f.handler.GetString("platform"), f.handler.GetString("dns.domain"))
		}
	})

	t.Run("MergesTheWholeMapWhenTheContextIsMissing", func(t *testing.T) {
		// Given content whose contexts map lacks the current context
		f := newFixture(t)

		// When it is loaded
		_ = f.handler.LoadConfigString("contexts:\n  other:\n    platform: aws\n")

		// Then the whole map, contexts key included, is merged
		if got := f.handler.GetString("contexts.other.platform"); got != "aws" {
			t.Errorf("expected contexts.other.platform aws, got %q", got)
		}
	})

	t.Run("ReturnsAnErrorForMalformedYAML", func(t *testing.T) {
		// Given a handler
		f := newFixture(t)

		// When malformed content is loaded
		err := f.handler.LoadConfigString("platform: [")

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})
}
