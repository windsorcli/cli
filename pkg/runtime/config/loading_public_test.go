package config_test

import (
	"errors"
	"path/filepath"
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

	t.Run("RootFileKeepsUntypedContextFields", func(t *testing.T) {
		// Given a root windsor.yaml context with a typed and an untyped key
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\ncontexts:\n  local:\n    platform: docker\n    custom: kept\n")

		// When config is loaded
		f.load(t)

		// Then both keys are read
		if f.handler.GetString("platform") != "docker" || f.handler.GetString("custom") != "kept" {
			t.Errorf("expected both keys, got platform=%v custom=%v", f.handler.Get("platform"), f.handler.Get("custom"))
		}
	})

	t.Run("ConvertsProviderToPlatform", func(t *testing.T) {
		// Given a v1alpha1 context file that sets provider and no platform
		f := newFixture(t)
		f.write(t, "contexts/local/windsor.yaml", "provider: aws\n")

		// When config is loaded
		f.load(t)

		// Then platform holds the provider value, and provider is gone
		if got := f.handler.GetString("platform"); got != "aws" {
			t.Errorf("expected platform aws, got %q", got)
		}
		if got := f.handler.Get("provider"); got != nil {
			t.Errorf("expected no provider, got %v", got)
		}
	})

	t.Run("KeepsPlatformOverProvider", func(t *testing.T) {
		// Given a root context that sets provider, and values.yaml that sets platform
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\ncontexts:\n  local:\n    provider: aws\n")
		f.write(t, "contexts/local/values.yaml", "platform: docker\n")

		// When config is loaded
		f.load(t)

		// Then platform keeps its value, and provider is gone
		if got := f.handler.GetString("platform"); got != "docker" {
			t.Errorf("expected platform docker, got %q", got)
		}
		if got := f.handler.Get("provider"); got != nil {
			t.Errorf("expected no provider, got %v", got)
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
		f.write(t, "windsor.yaml", "version: v1alpha1\ncontexts:\n  local:\n    platform: root\n    custom: root\n    id: root\n")
		f.write(t, "contexts/local/windsor.yaml", "platform: context\ncustom: context\nid: context\n")
		f.write(t, ".windsor/contexts/local/workstation.yaml", "platform: workstation\ncustom: workstation\n")
		f.write(t, "contexts/local/values.yaml", "platform: values\n")

		// When config is loaded
		f.load(t)

		// Then each key comes from the highest source that sets it
		want := map[string]string{"platform": "values", "custom": "workstation", "id": "context"}
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

	t.Run("LoadsNoSchemaFromTheRootVersionAlone", func(t *testing.T) {
		// Given root files at each version and no template schema
		for _, version := range []string{"v1alpha1", "v1alpha2"} {
			f := newFixture(t)
			f.write(t, "windsor.yaml", "version: "+version+"\n")

			// When config is loaded
			f.load(t)

			// Then no schema is loaded
			if f.handler.GetSchema() != nil {
				t.Errorf("%s: expected no schema", version)
			}
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

func TestLoadConfig_RootEnvironment(t *testing.T) {
	t.Run("LoadsRootEnvironmentIntoTheMergedMap", func(t *testing.T) {
		// Given a v1alpha2 root file with an environment map
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\nenvironment:\n  REGION: us-east-2\n  TEAM: core\n")

		// When config is loaded
		f.load(t)

		// Then the root variables are in the merged map
		got := f.handler.GetStringMap("environment")
		if got["REGION"] != "us-east-2" || got["TEAM"] != "core" {
			t.Errorf("expected root variables, got %v", got)
		}
	})

	t.Run("ContextEnvironmentOverridesRootPerKey", func(t *testing.T) {
		// Given a root variable and a context that overrides one of two
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\nenvironment:\n  REGION: us-east-2\n  TEAM: core\n")
		f.write(t, "contexts/local/values.yaml", "environment:\n  REGION: eu-west-1\n  EXTRA: yes\n")

		// When config is loaded
		f.load(t)

		// Then the context wins for REGION, and the other root and context keys stay
		got := f.handler.GetStringMap("environment")
		if got["REGION"] != "eu-west-1" || got["TEAM"] != "core" || got["EXTRA"] != "yes" {
			t.Errorf("expected per-key merge, got %v", got)
		}
	})

	t.Run("EmptyContextValueOverridesRootValue", func(t *testing.T) {
		// Given a context that sets a root variable to the empty string
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\nenvironment:\n  REGION: us-east-2\n")
		f.write(t, "contexts/local/values.yaml", "environment:\n  REGION: \"\"\n")

		// When config is loaded
		f.load(t)

		// Then the empty value replaces the root value and the key stays set
		got, ok := f.handler.GetStringMap("environment")["REGION"]
		if !ok || got != "" {
			t.Errorf("expected an empty REGION, got %q (present=%v)", got, ok)
		}
	})

	t.Run("KeepsExpressionsUnevaluated", func(t *testing.T) {
		// Given a root value that holds an expression
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\nenvironment:\n  TOKEN: ${secret.value}\n")

		// When config is loaded
		f.load(t)

		// Then the loader leaves the text for windsor env to evaluate in the context scope
		if got := f.handler.GetStringMap("environment")["TOKEN"]; got != "${secret.value}" {
			t.Errorf("expected the raw expression, got %q", got)
		}
	})

	t.Run("IgnoresEnvironmentInAV1alpha1Root", func(t *testing.T) {
		// Given a v1alpha1 root file with a top-level environment key
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\nenvironment:\n  REGION: us-east-2\n")

		// When config is loaded
		f.load(t)

		// Then the loader does not read it
		if got := f.handler.GetStringMap("environment"); len(got) != 0 {
			t.Errorf("expected no environment, got %v", got)
		}
	})

	t.Run("RootEnvironmentAloneDoesNotMakeTheContextExist", func(t *testing.T) {
		// Given a root file with an environment and no context files
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\nenvironment:\n  REGION: us-east-2\n")

		// When the handler checks for a new context
		isNew, err := f.handler.IsNewContext()

		// Then the context is still new
		if err != nil || !isNew {
			t.Errorf("expected a new context, got isNew=%v err=%v", isNew, err)
		}
	})
}

func TestLoadConfig_RootPaths(t *testing.T) {
	t.Run("ReadsContextValuesFromTheConfiguredContextsDirectory", func(t *testing.T) {
		// Given a root file with paths.base and values in the moved contexts directory
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\npaths:\n  base: infra\n")
		f.write(t, "infra/contexts/local/values.yaml", "environment:\n  REGION: moved\n")
		f.write(t, "contexts/local/values.yaml", "environment:\n  REGION: standard\n")

		// When config is loaded
		f.load(t)

		// Then the moved directory supplies the values
		if got := f.handler.GetStringMap("environment")["REGION"]; got != "moved" {
			t.Errorf("expected the moved values, got %q", got)
		}
	})

	t.Run("GetConfigRootFollowsTheContextsKey", func(t *testing.T) {
		// Given a root file with a per-key contexts path
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\npaths:\n  contexts: ctx\n")
		f.load(t)

		// When the config root is read
		got, err := f.handler.GetConfigRoot()

		// Then it sits under the configured directory
		if err != nil {
			t.Fatalf("GetConfigRoot: %v", err)
		}
		if want := filepath.Join(f.root, "ctx", "local"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("SaveConfigWritesValuesInTheConfiguredDirectory", func(t *testing.T) {
		// Given a root file with paths.base and a loaded config
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\npaths:\n  base: infra\n")
		f.load(t)
		if err := f.handler.Set("environment.REGION", "saved"); err != nil {
			t.Fatalf("Set: %v", err)
		}

		// When the config is saved
		if err := f.handler.SaveConfig(); err != nil {
			t.Fatalf("SaveConfig: %v", err)
		}

		// Then values.yaml is in the moved directory only
		if !f.exists("infra/contexts/local/values.yaml") || f.exists("contexts/local/values.yaml") {
			t.Errorf("expected values.yaml under infra/contexts only")
		}
	})

	t.Run("RejectsAnInvalidPathsBlock", func(t *testing.T) {
		// Given a root file whose paths block goes up a directory
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\npaths:\n  base: ../up\n")

		// When config is loaded
		err := f.handler.LoadConfig()

		// Then the load fails and names the key
		if err == nil || !strings.Contains(err.Error(), "paths.base") {
			t.Fatalf("expected an error that names paths.base, got %v", err)
		}
	})

	t.Run("RejectsAnEmptyPathsValue", func(t *testing.T) {
		// Given a root file that sets a paths key to an empty string
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\npaths:\n  terraform: \"\"\n")

		// When config is loaded
		err := f.handler.LoadConfig()

		// Then the load fails and names the key
		if err == nil || !strings.Contains(err.Error(), "paths.terraform") {
			t.Fatalf("expected an error that names paths.terraform, got %v", err)
		}
	})

	t.Run("RejectsContextFilesInTheConfiguredDirectory", func(t *testing.T) {
		// Given a v1alpha2 root with a moved contexts directory that holds a v1alpha1 context file
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\npaths:\n  base: infra\n")
		f.write(t, "infra/contexts/local/windsor.yaml", "provider: aws\n")

		// When config is loaded
		err := f.handler.LoadConfig()

		// Then the load fails and names the file in the moved directory
		if err == nil || !strings.Contains(err.Error(), filepath.Join("infra", "contexts", "local", "windsor.yaml")) {
			t.Fatalf("expected an error that names the context file, got %v", err)
		}
	})
}
