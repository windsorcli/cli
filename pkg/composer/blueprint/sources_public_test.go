package blueprint_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Test Setup
// =============================================================================

const coreURL = "oci://ghcr.io/acme/core:v1.0.0"

// withCoreSource publishes a core artifact at coreURL and writes a user blueprint that declares it.
func (f *fixture) withCoreSource(t *testing.T) {
	t.Helper()
	f.project(t)
	f.publish(t, coreURL, map[string]string{"_template/blueprint.yaml": blueprintHeader + "metadata:\n  name: core\n"})
	f.write(t, "contexts/local/blueprint.yaml", blueprintHeader+"metadata:\n  name: local\nsources:\n- name: core\n  url: "+coreURL+"\n")
}

// readUserBlueprint returns the content of the local context's blueprint.yaml.
func (f *fixture) readUserBlueprint(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, "contexts", "local", "blueprint.yaml"))
	if err != nil {
		t.Fatalf("read blueprint.yaml: %v", err)
	}
	return string(data)
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWrite(t *testing.T) {
	t.Run("WritesTheReferentialForm", func(t *testing.T) {
		// Given a composed blueprint with a kustomization
		f := newFixture(t)
		f.project(t)
		f.facet(t, "k", "kustomize:\n- name: k\n  path: k\n")
		h := f.mustLoad(t)

		// When the blueprint is written
		if err := h.Write(); err != nil {
			t.Fatalf("Write: %v", err)
		}

		// Then the file holds a header comment, metadata, and sources, but no components
		got := f.readUserBlueprint(t)
		want := "# This file selects and overrides components from underlying blueprint sources.\n" +
			"# To see the fully rendered blueprint, run: windsor show blueprint\n" +
			blueprintHeader +
			"metadata:\n  name: local\n  description: Blueprint for the local context\n" +
			"sources:\n- name: template\n  install: true\n"
		if got != want {
			t.Errorf("expected\n%s\ngot\n%s", want, got)
		}
	})

	t.Run("KeepsAnExistingFileWithoutOverwrite", func(t *testing.T) {
		// Given an existing user blueprint
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)
		before := f.readUserBlueprint(t)

		// When the blueprint is written without overwrite
		_ = h.Write()

		// Then the file is unchanged
		if got := f.readUserBlueprint(t); got != before {
			t.Errorf("expected the file unchanged, got\n%s", got)
		}
	})

	t.Run("ReplacesAnExistingFileWithOverwrite", func(t *testing.T) {
		// Given an existing user blueprint without a description
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)

		// When the blueprint is written with overwrite
		if err := h.Write(true); err != nil {
			t.Fatalf("Write: %v", err)
		}

		// Then the file holds the composed metadata
		if got := f.readUserBlueprint(t); !strings.Contains(got, "description: Blueprint for the local context") {
			t.Errorf("expected the composed description, got\n%s", got)
		}
	})

	t.Run("RecordsInitURLsAsInstalledSources", func(t *testing.T) {
		// Given a published artifact and no user blueprint
		f := newFixture(t)
		f.write(t, "windsor.yaml", "version: v1alpha1\n")
		f.write(t, "contexts/local/values.yaml", "platform: docker\n")
		f.publish(t, coreURL, map[string]string{"_template/blueprint.yaml": blueprintHeader + "metadata:\n  name: core\n"})

		// When the blueprint is loaded from the artifact's URL and written
		h := f.mustLoad(t, coreURL)
		if err := h.Write(); err != nil {
			t.Fatalf("Write: %v", err)
		}

		// Then the file declares the artifact as an installed source
		if got := f.readUserBlueprint(t); !strings.Contains(got, "- name: core\n  url: "+coreURL+"\n  install: true\n") {
			t.Errorf("expected an installed core source, got\n%s", got)
		}
	})

	t.Run("FailsBeforeABlueprintComposes", func(t *testing.T) {
		// Given a handler that has not loaded a blueprint
		f := newFixture(t)
		h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)

		// When the blueprint is written
		err := h.Write()

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})
}

func TestGetDeclaredSources(t *testing.T) {
	t.Run("ReturnsTheUserBlueprintSources", func(t *testing.T) {
		// Given a user blueprint that declares the core source
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)

		// When the declared sources are read
		got, err := h.GetDeclaredSources()

		// Then the core source is returned
		if err != nil || len(got) != 1 || got[0].Name != "core" || got[0].Url != coreURL {
			t.Errorf("expected the core source, got %+v (err=%v)", got, err)
		}
	})
}

func TestRetargetSource(t *testing.T) {
	t.Run("ReturnsThePreviousURLAndRepointsTheSource", func(t *testing.T) {
		// Given a loaded blueprint with the core source
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)

		// When the source is retargeted to a newer tag
		prev, err := h.RetargetSource("core", "oci://ghcr.io/acme/core:v1.2.0")

		// Then the previous URL is returned and the composed source uses the new one
		if err != nil || prev != coreURL {
			t.Fatalf("expected previous URL %s, got %q (err=%v)", coreURL, prev, err)
		}
		if got := h.Generate().Sources[0].Url; got != "oci://ghcr.io/acme/core:v1.2.0" {
			t.Errorf("expected the new URL, got %s", got)
		}
	})

	t.Run("RejectsAnUndeclaredSource", func(t *testing.T) {
		// Given a loaded blueprint with the core source
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)

		// When a source the blueprint does not declare is retargeted
		_, err := h.RetargetSource("other", "oci://ghcr.io/acme/other:v1.0.0")

		// Then an error names the source
		if err == nil || !strings.Contains(err.Error(), `source "other" is not declared`) {
			t.Errorf("expected an undeclared-source error, got %v", err)
		}
	})

	t.Run("RejectsAURLWithoutATag", func(t *testing.T) {
		// Given a loaded blueprint with the core source
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)

		// When the source is retargeted to a URL with no tag
		_, err := h.RetargetSource("core", "oci://ghcr.io/acme/core")

		// Then an error reports the missing version
		if err == nil || !strings.Contains(err.Error(), "missing a version") {
			t.Errorf("expected a missing-version error, got %v", err)
		}
	})

	t.Run("FailsBeforeABlueprintComposes", func(t *testing.T) {
		// Given a handler that has not loaded a blueprint
		f := newFixture(t)
		h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)

		// When a source is retargeted
		_, err := h.RetargetSource("core", coreURL)

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})
}

func TestUpgradeSourcesToLatest(t *testing.T) {
	t.Run("MovesASourceToTheNewestStableTag", func(t *testing.T) {
		// Given the core source at v1.0.0 and newer stable and prerelease tags
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)
		f.tags = []string{"v1.0.0", "v1.1.0", "v2.0.0-rc.1"}

		// When the sources are upgraded
		got, err := h.UpgradeSourcesToLatest()

		// Then the source moves to the newest stable tag, and the prerelease is skipped
		want := []blueprint.SourceUpgrade{{Name: "core", From: coreURL, To: "oci://ghcr.io/acme/core:v1.1.0"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("expected %+v, got %+v (err=%v)", want, got, err)
		}
		if url := h.Generate().Sources[0].Url; url != "oci://ghcr.io/acme/core:v1.1.0" {
			t.Errorf("expected the composed source at v1.1.0, got %s", url)
		}
	})

	t.Run("PersistsTheUpgradeOnAnOverwrite", func(t *testing.T) {
		// Given an upgraded core source
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)
		f.tags = []string{"v1.1.0"}
		_, _ = h.UpgradeSourcesToLatest()

		// When the blueprint is written with overwrite
		if err := h.Write(true); err != nil {
			t.Fatalf("Write: %v", err)
		}

		// Then the file declares the new URL
		if got := f.readUserBlueprint(t); !strings.Contains(got, "url: oci://ghcr.io/acme/core:v1.1.0") {
			t.Errorf("expected the upgraded URL, got\n%s", got)
		}
	})

	t.Run("ChangesNothingWhenTheSourceIsCurrent", func(t *testing.T) {
		// Given the core source at its newest tag
		f := newFixture(t)
		f.withCoreSource(t)
		h := f.mustLoad(t)
		f.tags = []string{"v0.9.0", "v1.0.0"}

		// When the sources are upgraded
		got, err := h.UpgradeSourcesToLatest()

		// Then no upgrade is reported
		if err != nil || len(got) != 0 {
			t.Errorf("expected no upgrades, got %+v (err=%v)", got, err)
		}
	})

	t.Run("SkipsSourcesThatAreNotOCI", func(t *testing.T) {
		// Given only the local template source
		f := newFixture(t)
		f.project(t)
		h := f.mustLoad(t)
		f.tags = []string{"v9.0.0"}

		// When the sources are upgraded
		got, err := h.UpgradeSourcesToLatest()

		// Then no upgrade is reported
		if err != nil || len(got) != 0 {
			t.Errorf("expected no upgrades, got %+v (err=%v)", got, err)
		}
	})

	t.Run("FailsBeforeABlueprintComposes", func(t *testing.T) {
		// Given a handler that has not loaded a blueprint
		f := newFixture(t)
		h := blueprint.NewBlueprintHandler(f.runtime, f.artifact)

		// When the sources are upgraded
		_, err := h.UpgradeSourcesToLatest()

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})
}

// =============================================================================
// Test Public Functions
// =============================================================================

func TestIsDowngrade(t *testing.T) {
	cases := []struct {
		name     string
		previous string
		target   string
		want     bool
	}{
		{"OlderTag", "oci://ghcr.io/acme/core:v1.2.0", "oci://ghcr.io/acme/core:v1.1.0", true},
		{"NewerTag", "oci://ghcr.io/acme/core:v1.1.0", "oci://ghcr.io/acme/core:v1.2.0", false},
		{"SameTag", "oci://ghcr.io/acme/core:v1.1.0", "oci://ghcr.io/acme/core:v1.1.0", false},
		{"DifferentRepository", "oci://ghcr.io/acme/core:v1.2.0", "oci://ghcr.io/acme/other:v1.1.0", false},
		{"NonSemverTag", "oci://ghcr.io/acme/core:main", "oci://ghcr.io/acme/core:v1.1.0", false},
		{"NotOCI", "https://example.com/core", "oci://ghcr.io/acme/core:v1.1.0", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := blueprint.IsDowngrade(c.previous, c.target); got != c.want {
				t.Errorf("IsDowngrade(%q, %q) = %v, want %v", c.previous, c.target, got, c.want)
			}
		})
	}
}
