package blueprint

import (
	"path/filepath"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Private Methods
// =============================================================================

func TestHandler_getRepositoryScopeValues(t *testing.T) {
	t.Run("ReturnsNilWhenRepositoryIsEmpty", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.runtime.ConfigHandler = nil
		handler.runtime.Shell = nil
		handler.userBlueprintLoader = &mockLoaderImpl{
			getSourceNameFunc: func() string { return "user" },
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{}
			},
		}

		values := handler.getRepositoryScopeValues()
		if values != nil {
			t.Errorf("Expected nil repository scope, got %v", values)
		}
	})

	t.Run("ReturnsRepositoryWithoutSecretName", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		secret := "flux-system"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.userBlueprintLoader = &mockLoaderImpl{
			getSourceNameFunc: func() string { return "user" },
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{
					Repository: blueprintv1alpha1.Repository{
						Url: "https://github.com/acme/repo.git",
						Ref: blueprintv1alpha1.Reference{
							Branch: "develop",
							Tag:    "v1.2.3",
							Commit: "abc123",
						},
						SecretName: &secret,
					},
				}
			},
		}

		values := handler.getRepositoryScopeValues()

		repository, ok := values["repository"].(map[string]any)
		if !ok {
			t.Fatal("Expected repository map")
		}
		if repository["url"] != "https://github.com/acme/repo.git" {
			t.Errorf("Expected repository.url to be set, got %v", repository["url"])
		}
		if _, hasSecret := repository["secretName"]; hasSecret {
			t.Error("Expected repository.secretName to be excluded")
		}
		ref, ok := repository["ref"].(map[string]any)
		if !ok {
			t.Fatal("Expected repository.ref map")
		}
		if ref["branch"] != "develop" {
			t.Errorf("Expected branch develop, got %v", ref["branch"])
		}
		if ref["tag"] != "v1.2.3" {
			t.Errorf("Expected tag v1.2.3, got %v", ref["tag"])
		}
		if ref["commit"] != "abc123" {
			t.Errorf("Expected commit abc123, got %v", ref["commit"])
		}
	})
}

func TestHandler_setRepositoryDefaults(t *testing.T) {
	t.Run("SetsDevRepositoryURLInDevMode", func(t *testing.T) {
		// Given a handler in dev mode with domain and project configured, no user blueprint
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return key == "dev"
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.Runtime.ProjectRoot = "/path/to/myproject"

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should be set with local git URL
		if handler.composedBlueprint.Repository.Url != "http://git.test/git/myproject" {
			t.Errorf("Expected URL 'http://git.test/git/myproject', got '%s'", handler.composedBlueprint.Repository.Url)
		}
		if handler.composedBlueprint.Repository.Ref.Branch != "main" {
			t.Errorf("Expected branch 'main', got '%s'", handler.composedBlueprint.Repository.Ref.Branch)
		}
		if handler.composedBlueprint.Repository.SecretName == nil || *handler.composedBlueprint.Repository.SecretName != "flux-system" {
			t.Errorf("Expected secretName 'flux-system'")
		}
	})

	t.Run("FallsBackToGitRemoteWhenNotDevMode", func(t *testing.T) {
		// Given a handler not in dev mode with no user blueprint
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "https://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = &Shims{}
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should be set with normalized host/path URL
		if handler.composedBlueprint.Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("NormalizesSSHGitURL", func(t *testing.T) {
		// Given a handler with SSH-style git URL and no user blueprint
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "git@github.com:test/repo.git", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = NewShims()
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should be normalized to host/path URL
		if handler.composedBlueprint.Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("PreservesGitSchemeURL", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "git://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = NewShims()
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		handler.setRepositoryDefaults()

		if handler.composedBlueprint.Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("PreservesFileSchemeURL", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "file:///tmp/repo", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = NewShims()
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		handler.setRepositoryDefaults()

		if handler.composedBlueprint.Repository.Url != "file:///tmp/repo" {
			t.Errorf("Expected URL 'file:///tmp/repo', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("AddsHTTPSForInvalidSchemePrefix", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "1invalid://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = NewShims()
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		handler.setRepositoryDefaults()

		if handler.composedBlueprint.Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("PreservesExtendedValidSchemeURL", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return false
		}
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "remote.origin.url" {
				return "git+ssh://github.com/test/repo.git", nil
			}
			return "", nil
		}

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.shims = NewShims()
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = nil

		handler.setRepositoryDefaults()

		if handler.composedBlueprint.Repository.Url != "github.com/test/repo" {
			t.Errorf("Expected URL 'github.com/test/repo', got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("SetsDefaultsWhenRepositoryEmptyEvenWithUserBlueprint", func(t *testing.T) {
		// Given a handler with user blueprint but composed Repository.Url empty (e.g. user file has no repository)
		mocks := setupHandlerMocks(t)
		mocks.ConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
			return key == "dev"
		}
		mocks.ConfigHandler.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "dns.domain" {
				return "test"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		mocks.Runtime.ProjectRoot = "/path/to/myproject"

		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		handler.userBlueprintLoader = &mockLoaderImpl{
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint { return &blueprintv1alpha1.Blueprint{} },
		}

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then repository should still be set from dev defaults so rendered blueprint has repository section
		if handler.composedBlueprint.Repository.Url != "http://git.test/git/myproject" {
			t.Errorf("Expected URL 'http://git.test/git/myproject' when Repository empty, got '%s'", handler.composedBlueprint.Repository.Url)
		}
	})

	t.Run("HandlesNilBlueprint", func(t *testing.T) {
		// Given a handler with nil composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = nil

		// When setting repository defaults
		handler.setRepositoryDefaults()

		// Then no panic should occur
	})
}

func TestHandler_clearLocalTemplateSource(t *testing.T) {
	t.Run("ClearsTemplateSourceOnComponentsAndKustomizationsWhenTemplateIsLocal", func(t *testing.T) {
		// Given a blueprint with local template source and components/kustomizations referencing template
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{{Name: "template", Url: ""}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{
				{Source: "template", Path: "vpc"},
				{Source: "other", Path: "rds"},
			},
			Kustomizations: []blueprintv1alpha1.Kustomization{
				{Name: "k1", Source: "template", Path: "app"},
				{Name: "k2", Source: "other", Path: "base"},
			},
		}

		// When clearing local template source
		handler.clearLocalTemplateSource(blueprint)

		// Then template-referencing components and kustomizations have Source cleared
		if blueprint.TerraformComponents[0].Source != "" {
			t.Errorf("Expected TerraformComponent Source cleared, got '%s'", blueprint.TerraformComponents[0].Source)
		}
		if blueprint.TerraformComponents[1].Source != "other" {
			t.Errorf("Expected non-template Source unchanged, got '%s'", blueprint.TerraformComponents[1].Source)
		}
		if blueprint.Kustomizations[0].Source != "" {
			t.Errorf("Expected Kustomization Source cleared, got '%s'", blueprint.Kustomizations[0].Source)
		}
		if blueprint.Kustomizations[1].Source != "other" {
			t.Errorf("Expected non-template Kustomization Source unchanged, got '%s'", blueprint.Kustomizations[1].Source)
		}
	})

	t.Run("NoOpWhenTemplateIsRemote", func(t *testing.T) {
		// Given a blueprint with remote template source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources:             []blueprintv1alpha1.Source{{Name: "template", Url: "https://github.com/example/repo.git"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Source: "template", Path: "vpc"}},
			Kustomizations:      []blueprintv1alpha1.Kustomization{{Name: "k1", Source: "template", Path: "app"}},
		}

		// When clearing local template source
		handler.clearLocalTemplateSource(blueprint)

		if blueprint.TerraformComponents[0].Source != "template" {
			t.Errorf("Expected Source unchanged when template is remote, got '%s'", blueprint.TerraformComponents[0].Source)
		}
		// Then Source remains unchanged
		if blueprint.Kustomizations[0].Source != "template" {
			t.Errorf("Expected Kustomization Source unchanged when template is remote, got '%s'", blueprint.Kustomizations[0].Source)
		}
	})

	t.Run("ClearsUserSourceSoComponentsResolveToProjectPath", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		blueprint := &blueprintv1alpha1.Blueprint{
			Sources:             []blueprintv1alpha1.Source{{Name: "template", Url: "https://example.com/oci"}},
			TerraformComponents: []blueprintv1alpha1.TerraformComponent{{Source: "user", Path: "cluster"}},
			Kustomizations:      []blueprintv1alpha1.Kustomization{{Name: "app", Source: "user", Path: "app"}},
		}

		handler.clearLocalTemplateSource(blueprint)

		if blueprint.TerraformComponents[0].Source != "" {
			t.Errorf("Expected user Source cleared so FullPath uses project path, got '%s'", blueprint.TerraformComponents[0].Source)
		}
		if blueprint.Kustomizations[0].Source != "" {
			t.Errorf("Expected user Kustomization Source cleared, got '%s'", blueprint.Kustomizations[0].Source)
		}
	})

	t.Run("NoOpWhenBlueprintNil", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.clearLocalTemplateSource(nil)
	})
}

func TestHandler_resolveComponentSource(t *testing.T) {
	t.Run("ResolvesOCISourceWithTag", func(t *testing.T) {
		// Given a handler with a composed blueprint containing an OCI source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "core", Url: "oci://ghcr.io/windsorcli/core", Ref: blueprintv1alpha1.Reference{Tag: "v1.0.0"}},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "core", Path: "cluster/talos"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should be expanded to full OCI URL
		expected := "oci://ghcr.io/windsorcli/core:v1.0.0//terraform/cluster/talos"
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("ResolvesGitSourceWithBranch", func(t *testing.T) {
		// Given a handler with a composed blueprint containing a Git source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "infra", Url: "https://github.com/org/infra", Ref: blueprintv1alpha1.Reference{Branch: "develop"}},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "infra", Path: "vpc"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should be expanded to Git URL format
		expected := "https://github.com/org/infra//terraform/vpc?ref=develop"
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("UsesPathPrefixFromSource", func(t *testing.T) {
		// Given a source with a custom path prefix
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "modules", Url: "oci://ghcr.io/org/modules", PathPrefix: "modules/tf", Ref: blueprintv1alpha1.Reference{Tag: "latest"}},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "modules", Path: "network"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should use the custom path prefix
		expected := "oci://ghcr.io/org/modules:latest//modules/tf/network"
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("UsesEmptyRefWhenNoRefSpecified", func(t *testing.T) {
		// Given a source with no ref specified
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{
				{Name: "lib", Url: "https://github.com/org/lib"},
			},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "lib", Path: "utils"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should use empty ref
		expected := "https://github.com/org/lib//terraform/utils?ref="
		if component.Source != expected {
			t.Errorf("Expected '%s', got '%s'", expected, component.Source)
		}
	})

	t.Run("LeavesUnmatchedSourceUnchanged", func(t *testing.T) {
		// Given a component referencing a non-existent source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{
			Sources: []blueprintv1alpha1.Source{},
		}
		component := &blueprintv1alpha1.TerraformComponent{Source: "missing", Path: "vpc"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should remain unchanged
		if component.Source != "missing" {
			t.Errorf("Expected 'missing', got '%s'", component.Source)
		}
	})

	t.Run("SkipsEmptySource", func(t *testing.T) {
		// Given a component with no source
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		handler.composedBlueprint = &blueprintv1alpha1.Blueprint{}
		component := &blueprintv1alpha1.TerraformComponent{Path: "local"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should remain empty
		if component.Source != "" {
			t.Errorf("Expected empty source, got '%s'", component.Source)
		}
	})

	t.Run("SkipsWhenNoBlueprintComposed", func(t *testing.T) {
		// Given a handler with no composed blueprint
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{Source: "core", Path: "vpc"}

		// When resolving component source
		handler.resolveComponentSource(component)

		// Then source should remain unchanged
		if component.Source != "core" {
			t.Errorf("Expected 'core', got '%s'", component.Source)
		}
	})
}

func TestHandler_getSourceRef(t *testing.T) {
	t.Run("ReturnsCommitWhenPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Commit: "abc123", SemVer: "1.0.0", Tag: "v1.0.0", Branch: "main"}}

		ref := handler.getSourceRef(source)

		if ref != "abc123" {
			t.Errorf("Expected 'abc123', got '%s'", ref)
		}
	})

	t.Run("ReturnsSemVerWhenCommitNotPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{SemVer: "1.0.0", Tag: "v1.0.0", Branch: "main"}}

		ref := handler.getSourceRef(source)

		if ref != "1.0.0" {
			t.Errorf("Expected '1.0.0', got '%s'", ref)
		}
	})

	t.Run("ReturnsTagWhenCommitAndSemVerNotPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Tag: "v1.0.0", Branch: "main"}}

		ref := handler.getSourceRef(source)

		if ref != "v1.0.0" {
			t.Errorf("Expected 'v1.0.0', got '%s'", ref)
		}
	})

	t.Run("ReturnsBranchWhenOnlyBranchPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{Ref: blueprintv1alpha1.Reference{Branch: "develop"}}

		ref := handler.getSourceRef(source)

		if ref != "develop" {
			t.Errorf("Expected 'develop', got '%s'", ref)
		}
	})

	t.Run("ReturnsEmptyStringWhenNoRefFieldsPresent", func(t *testing.T) {
		mocks := setupHandlerMocks(t)
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		source := blueprintv1alpha1.Source{}

		ref := handler.getSourceRef(source)

		if ref != "" {
			t.Errorf("Expected empty string, got '%s'", ref)
		}
	})
}

func TestHandler_resolveComponentFullPath(t *testing.T) {
	t.Run("UsesNameForNamedComponents", func(t *testing.T) {
		// Given a handler with a named component
		mocks := setupHandlerMocks(t)
		mocks.Runtime.WindsorScratchPath = "/scratch"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Name: "my-component",
			Path: "some/path",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use Name in WindsorScratchPath
		expected := "/scratch/terraform/my-component"
		actual := filepath.ToSlash(component.FullPath)
		if actual != expected {
			t.Errorf("Expected '%s', got '%s'", expected, actual)
		}
	})

	t.Run("UsesSourcePathForSourceComponents", func(t *testing.T) {
		// Given a handler with a component having a source
		mocks := setupHandlerMocks(t)
		mocks.Runtime.WindsorScratchPath = "/scratch"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Source: "shared-modules",
			Path:   "vpc",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use WindsorScratchPath with Path
		expected := "/scratch/terraform/vpc"
		actual := filepath.ToSlash(component.FullPath)
		if actual != expected {
			t.Errorf("Expected '%s', got '%s'", expected, actual)
		}
	})

	t.Run("UsesProjectRootForLocalComponents", func(t *testing.T) {
		// Given a handler with a local component (no name, no source)
		mocks := setupHandlerMocks(t)
		mocks.Runtime.ProjectRoot = "/project"
		handler := NewBlueprintHandler(mocks.Runtime, mocks.ArtifactBuilder)
		component := &blueprintv1alpha1.TerraformComponent{
			Path: "network/vpc",
		}

		// When resolving full path
		handler.resolveComponentFullPath(component)

		// Then should use ProjectRoot
		expected := "/project/terraform/network/vpc"
		actual := filepath.ToSlash(component.FullPath)
		if actual != expected {
			t.Errorf("Expected '%s', got '%s'", expected, actual)
		}
	})
}
