package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGetContext(t *testing.T) {
	t.Run("DefaultsToLocal", func(t *testing.T) {
		// Given no context file and no environment variable
		f := newFixture(t)

		// When the context is read
		got := f.handler.GetContext()

		// Then it is local
		if got != "local" {
			t.Errorf("expected local, got %q", got)
		}
	})

	t.Run("ReadsTheEnvironmentVariable", func(t *testing.T) {
		// Given WINDSOR_CONTEXT is set and no context file exists
		f := newFixture(t)
		t.Setenv("WINDSOR_CONTEXT", "staging")

		// When the context is read
		got := f.handler.GetContext()

		// Then the variable's value is used
		if got != "staging" {
			t.Errorf("expected staging, got %q", got)
		}
	})

	t.Run("PrefersTheContextFileOverTheEnvironment", func(t *testing.T) {
		// Given a context file and a different WINDSOR_CONTEXT
		f := newFixture(t)
		f.write(t, ".windsor/context", "  prod\n")
		t.Setenv("WINDSOR_CONTEXT", "staging")

		// When the context is read
		got := f.handler.GetContext()

		// Then the trimmed file value is used
		if got != "prod" {
			t.Errorf("expected prod, got %q", got)
		}
	})

	t.Run("KeepsTheFirstResolutionForTheHandlerLifetime", func(t *testing.T) {
		// Given a handler that has resolved its context from the file
		f := newFixture(t)
		f.write(t, ".windsor/context", "prod")
		_ = f.handler.GetContext()

		// When the file changes
		f.write(t, ".windsor/context", "staging")

		// Then the same handler keeps its value and a new handler reads the change
		if got := f.handler.GetContext(); got != "prod" {
			t.Errorf("expected cached prod, got %q", got)
		}
		if got := f.newHandler().GetContext(); got != "staging" {
			t.Errorf("expected new handler to read staging, got %q", got)
		}
	})
}

func TestSetContext(t *testing.T) {
	t.Run("WritesTheFileAndEnvironmentAndUpdatesTheHandler", func(t *testing.T) {
		// Given a handler that has resolved the default context
		f := newFixture(t)
		_ = f.handler.GetContext()

		// When the context is set
		if err := f.handler.SetContext("prod"); err != nil {
			t.Fatalf("SetContext: %v", err)
		}

		// Then the file, the environment, and the handler all name the new context
		if got := f.read(t, ".windsor/context"); got != "prod" {
			t.Errorf("expected file prod, got %q", got)
		}
		if got := os.Getenv("WINDSOR_CONTEXT"); got != "prod" {
			t.Errorf("expected env prod, got %q", got)
		}
		if got := f.handler.GetContext(); got != "prod" {
			t.Errorf("expected handler prod, got %q", got)
		}
	})

	t.Run("LeavesAnotherResolvedHandlerUnchanged", func(t *testing.T) {
		// Given a second handler that has already resolved its context
		f := newFixture(t)
		other := f.newHandler()
		_ = other.GetContext()

		// When the first handler sets the context
		_ = f.handler.SetContext("prod")

		// Then the second handler keeps its value
		if got := other.GetContext(); got != "local" {
			t.Errorf("expected local, got %q", got)
		}
	})

	t.Run("ReturnsAnErrorWhenTheProjectRootFails", func(t *testing.T) {
		// Given a shell that cannot find the project root
		t.Setenv("WINDSOR_CONTEXT", "")
		sh := shell.NewMockShell()
		sh.GetProjectRootFunc = func() (string, error) { return "", errors.New("no root") }

		// When the context is set
		err := config.NewConfigHandler(sh).SetContext("prod")

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})
}

func TestWithContext(t *testing.T) {
	t.Run("OverridesTheContextWithoutTouchingTheOriginal", func(t *testing.T) {
		// Given a handler on the context from the file
		f := newFixture(t)
		f.write(t, ".windsor/context", "prod")

		// When a copy is made for another context
		copied := f.handler.WithContext("test")

		// Then the copy uses the override, the original does not, and no file changes
		if got := copied.GetContext(); got != "test" {
			t.Errorf("expected copy test, got %q", got)
		}
		if got := f.handler.GetContext(); got != "prod" {
			t.Errorf("expected original prod, got %q", got)
		}
		if got := f.read(t, ".windsor/context"); got != "prod" {
			t.Errorf("expected file unchanged, got %q", got)
		}
	})

	t.Run("CopiesValuesWithoutSharingThem", func(t *testing.T) {
		// Given a handler with a value
		f := newFixture(t)
		_ = f.handler.Set("platform", "docker")

		// When a copy changes a value
		copied := f.handler.WithContext("test")
		_ = copied.Set("platform", "aws")

		// Then the copy started with the value and the original keeps its own
		if got := f.handler.GetString("platform"); got != "docker" {
			t.Errorf("expected original docker, got %q", got)
		}
		if got := copied.GetString("platform"); got != "aws" {
			t.Errorf("expected copy aws, got %q", got)
		}
	})

	t.Run("DropsPendingSetsFromTheCopy", func(t *testing.T) {
		// Given an existing values.yaml for the copy's context and a pending Set on the original
		f := newFixture(t)
		f.write(t, "contexts/prod/values.yaml", "kept: true\n")
		_ = f.handler.Set("platform", "docker")

		// When the copy saves with overwrite
		if err := f.handler.WithContext("prod").SaveConfig(true); err != nil {
			t.Fatalf("SaveConfig: %v", err)
		}

		// Then the copy's values.yaml is not patched
		if got := f.read(t, "contexts/prod/values.yaml"); got != "kept: true\n" {
			t.Errorf("expected values.yaml unchanged, got %q", got)
		}
	})
}

func TestIsDevMode(t *testing.T) {
	t.Run("MatchesLocalContextNames", func(t *testing.T) {
		// Given a handler with no dev value
		f := newFixture(t)

		// When context names are checked
		cases := map[string]bool{"local": true, "local-a": true, "localx": false, "prod": false}

		// Then only local and local-* are dev
		for name, want := range cases {
			if got := f.handler.IsDevMode(name); got != want {
				t.Errorf("%s: expected %v, got %v", name, want, got)
			}
		}
	})

	t.Run("PrefersAnExplicitDevValue", func(t *testing.T) {
		// Given dev set to false
		f := newFixture(t)
		_ = f.handler.Set("dev", false)

		// When a local context is checked
		got := f.handler.IsDevMode("local")

		// Then the explicit value wins
		if got {
			t.Error("expected dev mode off")
		}
	})
}

func TestContextPaths(t *testing.T) {
	t.Run("ResolveUnderTheProjectRoot", func(t *testing.T) {
		// Given a handler on context prod
		f := newFixture(t)
		_ = f.handler.SetContext("prod")

		// When the context paths are read
		configRoot, err1 := f.handler.GetConfigRoot()
		scratch, err2 := f.handler.GetWindsorScratchPath()

		// Then they sit under contexts/ and .windsor/contexts/
		if err1 != nil || configRoot != filepath.Join(f.root, "contexts", "prod") {
			t.Errorf("unexpected config root %q (%v)", configRoot, err1)
		}
		if err2 != nil || scratch != filepath.Join(f.root, ".windsor", "contexts", "prod") {
			t.Errorf("unexpected scratch path %q (%v)", scratch, err2)
		}
	})

	t.Run("ReturnErrorsWhenTheProjectRootFails", func(t *testing.T) {
		// Given a shell that cannot find the project root
		t.Setenv("WINDSOR_CONTEXT", "")
		sh := shell.NewMockShell()
		sh.GetProjectRootFunc = func() (string, error) { return "", errors.New("no root") }
		handler := config.NewConfigHandler(sh)

		// When the context paths are read
		_, err1 := handler.GetConfigRoot()
		_, err2 := handler.GetWindsorScratchPath()
		err3 := handler.Clean()

		// Then each returns an error
		if err1 == nil || err2 == nil || err3 == nil {
			t.Errorf("expected errors, got %v, %v, %v", err1, err2, err3)
		}
	})
}

func TestClean(t *testing.T) {
	t.Run("RemovesTerraformCachesOnly", func(t *testing.T) {
		// Given terraform caches and other files in the scratch path
		f := newFixture(t)
		f.write(t, ".windsor/contexts/local/.terraform/plugin", "x")
		f.write(t, ".windsor/contexts/local/.tfstate/state", "x")
		f.write(t, ".windsor/contexts/local/.kube/config", "x")

		// When the context is cleaned
		if err := f.handler.Clean(); err != nil {
			t.Fatalf("Clean: %v", err)
		}

		// Then the caches are gone and the other files remain
		if f.exists(".windsor/contexts/local/.terraform") || f.exists(".windsor/contexts/local/.tfstate") {
			t.Error("expected terraform caches removed")
		}
		if !f.exists(".windsor/contexts/local/.kube/config") {
			t.Error("expected kubeconfig kept")
		}
	})

	t.Run("SucceedsWithNothingToRemove", func(t *testing.T) {
		// Given an empty project
		f := newFixture(t)

		// When the context is cleaned
		err := f.handler.Clean()

		// Then it succeeds
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}
