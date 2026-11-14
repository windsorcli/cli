package paths_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/runtime/paths"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNew(t *testing.T) {
	t.Run("DefaultsToTheStandardLayout", func(t *testing.T) {
		// Given a project root and no paths block
		root := filepath.Join(string(filepath.Separator), "project")

		// When the resolver is built
		r, err := paths.New(root, nil, nil)

		// Then each directory sits at the project root
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		assertPath(t, "Base", r.Base(), root)
		assertPath(t, "Terraform", r.Terraform(), filepath.Join(root, "terraform"))
		assertPath(t, "Kustomize", r.Kustomize(), filepath.Join(root, "kustomize"))
		assertPath(t, "Contexts", r.Contexts(), filepath.Join(root, "contexts"))
		assertPath(t, "ContextDir", r.ContextDir("dev"), filepath.Join(root, "contexts", "dev"))
		assertPath(t, "TemplateDir", r.TemplateDir(), filepath.Join(root, "contexts", "_template"))
	})

	t.Run("BaseMovesTheThreeDirectories", func(t *testing.T) {
		// Given a paths block with base only
		root := filepath.Join(string(filepath.Separator), "project")

		// When the resolver is built
		r, err := paths.New(root, &v1alpha2.Paths{Base: "infra"}, nil)

		// Then the three directories sit under base
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		assertPath(t, "Base", r.Base(), filepath.Join(root, "infra"))
		assertPath(t, "Terraform", r.Terraform(), filepath.Join(root, "infra", "terraform"))
		assertPath(t, "Kustomize", r.Kustomize(), filepath.Join(root, "infra", "kustomize"))
		assertPath(t, "TemplateDir", r.TemplateDir(), filepath.Join(root, "infra", "contexts", "_template"))
	})

	t.Run("KeysAreRelativeToBase", func(t *testing.T) {
		// Given base and a per-key path
		root := filepath.Join(string(filepath.Separator), "project")

		// When the resolver is built
		r, err := paths.New(root, &v1alpha2.Paths{Base: "infra", Terraform: "tf/modules"}, nil)

		// Then the key joins base and the other keys keep their defaults
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		assertPath(t, "Terraform", r.Terraform(), filepath.Join(root, "infra", "tf", "modules"))
		assertPath(t, "Contexts", r.Contexts(), filepath.Join(root, "infra", "contexts"))
	})

	t.Run("RejectsInvalidValues", func(t *testing.T) {
		root := filepath.Join(string(filepath.Separator), "project")
		cases := map[string]*v1alpha2.Paths{
			"AbsoluteBase":            {Base: "/infra"},
			"AbsoluteKey":             {Terraform: "/tf"},
			"ParentElementInBase":     {Base: "../infra"},
			"ParentElementInKey":      {Kustomize: "a/../../b"},
			"BaseInsideWindsor":       {Base: ".windsor/x"},
			"BaseIsWindsor":           {Base: ".windsor"},
			"KeyInsideWindsor":        {Contexts: ".windsor/contexts"},
			"BaseIsProjectRoot":       {Base: "."},
			"BaseCleansToProjectRoot": {Base: "a/.."},
			"KeyIsBase":               {Base: "infra", Terraform: "."},
			"KeysEqual":               {Terraform: "src", Kustomize: "src"},
			"KeyContainsKey":          {Terraform: "src", Kustomize: "src/k"},
			"KeyInsideKey":            {Contexts: "a/b", Terraform: "a"},
		}
		for name, block := range cases {
			t.Run(name, func(t *testing.T) {
				// Given an invalid paths block
				// When the resolver is built
				_, err := paths.New(root, block, nil)

				// Then it fails
				if err == nil {
					t.Fatal("expected an error, got none")
				}
			})
		}
	})

	t.Run("RejectsASymbolicLinkOutsideTheProject", func(t *testing.T) {
		// Given a project whose infra directory links outside the project
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "infra")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		// When the resolver is built with the symbolic link check
		_, err := paths.New(root, &v1alpha2.Paths{Base: "infra"}, paths.NewShims())

		// Then it fails and names the link
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("expected a symbolic link error, got %v", err)
		}
	})

	t.Run("AcceptsASymbolicLinkInsideTheProject", func(t *testing.T) {
		// Given a project whose infra directory links to another directory in the project
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "real"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "infra")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		// When the resolver is built with the symbolic link check
		_, err := paths.New(root, &v1alpha2.Paths{Base: "infra"}, paths.NewShims())

		// Then it succeeds
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestLoad(t *testing.T) {
	loadWith := func(t *testing.T, content string) (*paths.Resolver, error) {
		t.Helper()
		root := t.TempDir()
		if content != "" {
			if err := os.WriteFile(filepath.Join(root, "windsor.yaml"), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
		return paths.Load(root, nil)
	}

	t.Run("MissingRootFileGivesDefaults", func(t *testing.T) {
		// Given no root file
		// When the resolver loads
		r, err := loadWith(t, "")

		// Then the layout is the default
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		assertPath(t, "Terraform", r.Terraform(), filepath.Join(r.ProjectRoot(), "terraform"))
	})

	t.Run("V1alpha1RootIgnoresPaths", func(t *testing.T) {
		// Given a v1alpha1 root file
		// When the resolver loads
		r, err := loadWith(t, "version: v1alpha1\ncontexts:\n  local: {}\n")

		// Then the layout is the default
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		assertPath(t, "Contexts", r.Contexts(), filepath.Join(r.ProjectRoot(), "contexts"))
	})

	t.Run("V1alpha2RootGivesConfiguredPaths", func(t *testing.T) {
		// Given a v1alpha2 root file with paths
		r, err := loadWith(t, "version: v1alpha2\npaths:\n  base: infra\n  contexts: ctx\n")

		// Then the directories follow the block
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		assertPath(t, "Kustomize", r.Kustomize(), filepath.Join(r.ProjectRoot(), "infra", "kustomize"))
		assertPath(t, "Contexts", r.Contexts(), filepath.Join(r.ProjectRoot(), "infra", "ctx"))
	})

	t.Run("InvalidBlockNamesTheFile", func(t *testing.T) {
		// Given a v1alpha2 root file with an absolute path
		_, err := loadWith(t, "version: v1alpha2\npaths:\n  base: /abs\n")

		// Then the error names the root file and the key
		if err == nil || !strings.Contains(err.Error(), "windsor.yaml") || !strings.Contains(err.Error(), "paths.base") {
			t.Fatalf("expected an error that names windsor.yaml and paths.base, got %v", err)
		}
	})

	t.Run("ReadErrorFails", func(t *testing.T) {
		// Given a read that fails with an error other than not-exist
		s := paths.NewShims()
		s.ReadFile = func(string) ([]byte, error) { return nil, errors.New("boom") }

		// When the resolver loads
		_, err := paths.Load("/project", s)

		// Then it returns the error
		if err == nil {
			t.Fatal("expected an error, got none")
		}
	})

	t.Run("NotExistReadGivesDefaults", func(t *testing.T) {
		// Given a read that reports not-exist
		s := paths.NewShims()
		s.ReadFile = func(string) ([]byte, error) { return nil, fs.ErrNotExist }

		// When the resolver loads
		r, err := paths.Load("/project", s)

		// Then the layout is the default
		if err != nil || r == nil {
			t.Fatalf("expected defaults, got %v", err)
		}
	})
}

func TestResolver_RebaseKustomize(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "project")
	r, err := paths.New(root, &v1alpha2.Paths{Base: "infra", Kustomize: "k8s"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("MapsTheStandardPrefix", func(t *testing.T) {
		// Given a path under kustomize/
		// When it is rebased
		got := r.RebaseKustomize("kustomize/ingress/base")

		// Then it moves under the configured directory
		if got != "infra/k8s/ingress/base" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("MapsTheBareDirectory", func(t *testing.T) {
		if got := r.RebaseKustomize("kustomize"); got != "infra/k8s" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("LeavesOtherPathsAlone", func(t *testing.T) {
		if got := r.RebaseKustomize("kustomized/x"); got != "kustomized/x" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("DefaultLayoutKeepsThePath", func(t *testing.T) {
		if got := paths.Default(root).RebaseKustomize("kustomize/a"); got != "kustomize/a" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestResolver_IsUnder(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "project")
	r, _ := paths.New(root, &v1alpha2.Paths{Base: "infra"}, nil)

	t.Run("TemplateAndTerraformFollowTheConfiguredDirectories", func(t *testing.T) {
		// Given a resolver with base infra
		// Then the checks follow the moved directories
		if !r.IsUnderTemplate(filepath.Join(root, "infra", "contexts", "_template", "x")) {
			t.Error("expected the template directory to match")
		}
		if r.IsUnderTemplate(filepath.Join(root, "contexts", "_template")) {
			t.Error("the standard template directory must not match")
		}
		if !r.IsUnderTerraform(filepath.Join(root, "infra", "terraform", "vpc")) {
			t.Error("expected the terraform directory to match")
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// assertPath fails when got differs from want.
func assertPath(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", name, got, want)
	}
}
