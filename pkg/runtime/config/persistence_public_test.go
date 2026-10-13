package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSaveConfig(t *testing.T) {
	t.Run("WritesANewValuesFileAndRootFile", func(t *testing.T) {
		// Given values in a non-dev context
		f := newFixture(t)
		_ = f.handler.SetContext("prod")
		_ = f.handler.Set("platform", "aws")
		_ = f.handler.Set("dns.domain", "example.com")

		// When config is saved
		if err := f.handler.SaveConfig(); err != nil {
			t.Fatalf("SaveConfig: %v", err)
		}

		// Then values.yaml holds the values and a root windsor.yaml is created
		want := map[string]any{"platform": "aws", "dns": map[string]any{"domain": "example.com"}}
		if got := f.readYAML(t, "contexts/prod/values.yaml"); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if got := f.read(t, "windsor.yaml"); got != "version: v1alpha1\n" {
			t.Errorf("unexpected root file %q", got)
		}
	})

	t.Run("KeepsAnExistingRootFile", func(t *testing.T) {
		// Given an existing root windsor.yaml
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha2\n")
		_ = f.handler.Set("id", "abc")

		// When config is saved
		_ = f.handler.SaveConfig()

		// Then the root file is unchanged
		if got := f.read(t, "windsor.yaml"); got != "version: v1alpha2\n" {
			t.Errorf("unexpected root file %q", got)
		}
	})

	t.Run("SkipsTheRootFileInGlobalMode", func(t *testing.T) {
		// Given a shell in global mode
		f := newFixture(t)
		f.shell.IsGlobalFunc = func() bool { return true }
		_ = f.handler.Set("id", "abc")

		// When config is saved
		_ = f.handler.SaveConfig()

		// Then no root file is created
		if f.exists("windsor.yaml") {
			t.Error("expected no root file")
		}
	})

	t.Run("RoutesManagedKeysAwayFromValuesInDevMode", func(t *testing.T) {
		// Given workstation, platform, provider, and network values in the local context
		f := newFixture(t)
		_ = f.handler.Set("workstation.runtime", "colima")
		_ = f.handler.Set("platform", "docker")
		_ = f.handler.Set("provider", "docker")
		_ = f.handler.Set("network.cidr_block", "10.5.0.0/16")
		_ = f.handler.Set("id", "abc")

		// When config is saved
		_ = f.handler.SaveConfig()

		// Then values.yaml keeps only network and id
		want := map[string]any{"id": "abc", "network": map[string]any{"cidr_block": "10.5.0.0/16"}}
		if got := f.readYAML(t, "contexts/local/values.yaml"); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("LeavesAnExistingFileWithoutOverwrite", func(t *testing.T) {
		// Given an existing values.yaml
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "id: old\n")
		_ = f.handler.Set("id", "new")

		// When config is saved without overwrite
		_ = f.handler.SaveConfig()

		// Then the file is unchanged
		if got := f.read(t, "contexts/local/values.yaml"); got != "id: old\n" {
			t.Errorf("expected unchanged file, got %q", got)
		}
	})

	t.Run("PatchesOnlySetPathsAndKeepsComments", func(t *testing.T) {
		// Given an existing values.yaml with comments and other keys
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "# operator notes\nid: old\ndns:\n  # resolver\n  domain: test\n  enabled: true\nextra: kept\n")
		f.load(t)
		_ = f.handler.Set("dns.domain", "example.com")

		// When config is saved with overwrite
		if err := f.handler.SaveConfig(true); err != nil {
			t.Fatalf("SaveConfig: %v", err)
		}

		// Then only the set path changes and comments survive
		got := f.read(t, "contexts/local/values.yaml")
		for _, want := range []string{"# operator notes", "# resolver", "id: old", "domain: example.com", "enabled: true", "extra: kept"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected %q in %q", want, got)
			}
		}
	})

	t.Run("DeletesATopLevelKeySetToNil", func(t *testing.T) {
		// Given an existing values.yaml with two keys
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "id: abc\nextra: gone\n")
		f.load(t)
		_ = f.handler.Set("extra", nil)

		// When config is saved with overwrite
		_ = f.handler.SaveConfig(true)

		// Then the key is removed
		if got := f.readYAML(t, "contexts/local/values.yaml"); !reflect.DeepEqual(got, map[string]any{"id": "abc"}) {
			t.Errorf("unexpected values %v", got)
		}
	})

	t.Run("SavesAValueConvertedAgainAfterASchemaLoads", func(t *testing.T) {
		// Given a string value saved before a schema typed its path
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "id: abc\n")
		_ = f.handler.Set("version", "1.30")
		_ = f.handler.SaveConfig(true)

		// When a schema loads and config is saved again with overwrite
		f.loadSchema(t, "  version:\n    type: string\n")
		_ = f.handler.SaveConfig(true)

		// Then values.yaml holds the original string
		if got := f.readYAML(t, "contexts/local/values.yaml")["version"]; got != "1.30" {
			t.Errorf("expected \"1.30\", got %#v", got)
		}
	})

	t.Run("ClearsPendingSetsAfterAWrite", func(t *testing.T) {
		// Given a saved Set followed by a manual edit
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "id: abc\n")
		_ = f.handler.Set("id", "def")
		_ = f.handler.SaveConfig(true)
		f.write(t, "contexts/local/values.yaml", "id: manual\n")

		// When config is saved again with overwrite
		_ = f.handler.SaveConfig(true)

		// Then the manual edit is kept
		if got := f.read(t, "contexts/local/values.yaml"); got != "id: manual\n" {
			t.Errorf("expected manual edit kept, got %q", got)
		}
	})

	t.Run("RewritesAnUnparsableFileWhole", func(t *testing.T) {
		// Given a values.yaml that is not a mapping
		f := newFixture(t)
		f.write(t, "contexts/local/values.yaml", "- a\n- b\n")
		_ = f.handler.Set("id", "abc")

		// When config is saved with overwrite
		_ = f.handler.SaveConfig(true)

		// Then the file holds the full values
		if got := f.readYAML(t, "contexts/local/values.yaml"); !reflect.DeepEqual(got, map[string]any{"id": "abc"}) {
			t.Errorf("unexpected values %v", got)
		}
	})

	t.Run("WritesNothingForEmptyData", func(t *testing.T) {
		// Given a handler with no values
		f := newFixture(t)

		// When config is saved
		err := f.handler.SaveConfig()

		// Then no values file is written
		if err != nil || f.exists("contexts/local/values.yaml") {
			t.Errorf("expected no values file, got err=%v", err)
		}
	})

	t.Run("ReturnsAnErrorWhenTheProjectRootFails", func(t *testing.T) {
		// Given a shell that cannot find the project root
		t.Setenv("WINDSOR_CONTEXT", "")
		sh := shell.NewMockShell()
		sh.GetProjectRootFunc = func() (string, error) { return "", errors.New("no root") }
		handler := config.NewConfigHandler(sh)

		// When config and workstation state are saved
		err1 := handler.SaveConfig()
		err2 := handler.SaveWorkstationState()

		// Then both return errors
		if err1 == nil || err2 == nil {
			t.Errorf("expected errors, got %v and %v", err1, err2)
		}
	})
}

func TestSaveWorkstationState(t *testing.T) {
	t.Run("WritesManagedKeysInDevMode", func(t *testing.T) {
		// Given workstation, platform, and network values in the local context
		f := newFixture(t)
		_ = f.handler.Set("workstation.runtime", "colima")
		_ = f.handler.Set("platform", "docker")
		_ = f.handler.Set("network.cidr_block", "10.5.0.0/16")
		_ = f.handler.Set("network.loadbalancer_ips.start", "10.5.1.1")
		_ = f.handler.Set("id", "abc")

		// When workstation state is saved
		if err := f.handler.SaveWorkstationState(); err != nil {
			t.Fatalf("SaveWorkstationState: %v", err)
		}

		// Then the file holds the workstation map, platform, and only the network CIDR
		want := map[string]any{
			"workstation": map[string]any{"runtime": "colima"},
			"platform":    "docker",
			"network":     map[string]any{"cidr_block": "10.5.0.0/16"},
		}
		if got := f.readYAML(t, ".windsor/contexts/local/workstation.yaml"); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("WritesOnlyTheWorkstationMapOutsideDevMode", func(t *testing.T) {
		// Given workstation and platform values in a non-dev context with no runtime
		f := newFixture(t)
		_ = f.handler.SetContext("prod")
		_ = f.handler.Set("workstation.arch", "arm64")
		_ = f.handler.Set("platform", "aws")

		// When workstation state is saved
		_ = f.handler.SaveWorkstationState()

		// Then platform stays out of workstation state
		want := map[string]any{"workstation": map[string]any{"arch": "arm64"}}
		if got := f.readYAML(t, ".windsor/contexts/prod/workstation.yaml"); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("KeepsPlatformWhenAWorkstationRuntimeIsSet", func(t *testing.T) {
		// Given a non-dev context with a workstation runtime
		f := newFixture(t)
		_ = f.handler.SetContext("prod")
		_ = f.handler.Set("workstation.runtime", "docker-desktop")
		_ = f.handler.Set("platform", "docker")

		// When workstation state is saved
		_ = f.handler.SaveWorkstationState()

		// Then platform is written to workstation state
		if got := f.readYAML(t, ".windsor/contexts/prod/workstation.yaml")["platform"]; got != "docker" {
			t.Errorf("expected platform docker, got %v", got)
		}
	})

	t.Run("RemovesTheFileWhenNothingIsManaged", func(t *testing.T) {
		// Given an existing workstation state and no managed values
		f := newFixture(t)
		f.write(t, ".windsor/contexts/local/workstation.yaml", "workstation:\n  runtime: colima\n")
		_ = f.handler.Set("id", "abc")

		// When workstation state is saved
		_ = f.handler.SaveWorkstationState()

		// Then the file is removed
		if f.exists(".windsor/contexts/local/workstation.yaml") {
			t.Error("expected workstation state removed")
		}
	})
}

func TestSetDefault(t *testing.T) {
	t.Run("FillsOnlyMissingValues", func(t *testing.T) {
		// Given a handler with a platform already set
		f := newFixture(t)
		_ = f.handler.Set("platform", "docker")
		id := "abc"
		platform := "none"

		// When a default with platform and id is applied
		if err := f.handler.SetDefault(v1alpha1.Context{ID: &id, Platform: &platform}); err != nil {
			t.Fatalf("SetDefault: %v", err)
		}

		// Then the existing platform wins and the id is filled in
		if f.handler.GetString("platform") != "docker" || f.handler.GetString("id") != "abc" {
			t.Errorf("unexpected values platform=%q id=%q", f.handler.GetString("platform"), f.handler.GetString("id"))
		}
	})

	t.Run("AppliesThePackageDefaults", func(t *testing.T) {
		// Given an empty handler
		f := newFixture(t)

		// When the non-dev default is applied
		_ = f.handler.SetDefault(config.DefaultConfig)

		// Then the platform is none
		if got := f.handler.GetString("platform"); got != "none" {
			t.Errorf("expected platform none, got %q", got)
		}
	})
}

func TestGetConfig(t *testing.T) {
	t.Run("ReturnsTypedFieldsAndDropsTheRest", func(t *testing.T) {
		// Given typed and untyped values
		f := newFixture(t)
		_ = f.handler.LoadConfigString("platform: docker\ndns:\n  domain: test\ncustom: dropped\n")

		// When the typed config is read
		got := f.handler.GetConfig()

		// Then typed fields are set
		if got.Platform == nil || *got.Platform != "docker" || got.DNS == nil || got.DNS.Domain == nil || *got.DNS.Domain != "test" {
			t.Errorf("unexpected typed config %+v", got)
		}
	})

	t.Run("ReturnsAnEmptyContextForATypeMismatch", func(t *testing.T) {
		// Given a value that cannot decode into its typed field
		f := newFixture(t)
		_ = f.handler.LoadConfigString("dns: not-a-map\n")

		// When the typed config is read
		got := f.handler.GetConfig()

		// Then an empty context is returned
		if !reflect.DeepEqual(got, &v1alpha1.Context{}) {
			t.Errorf("expected empty context, got %+v", got)
		}
	})
}
