// The handler_repository file holds the repository and source-path steps of the BlueprintHandler.
// It provides default repository values for the composed blueprint and for expression scope.
// It resolves the source of each component into a URL and a reference.
// It sets the full filesystem path of each terraform component.

package blueprint

import (
	"fmt"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	runtimegit "github.com/windsorcli/cli/pkg/runtime/git"
)

// =============================================================================
// Private Methods
// =============================================================================

// getRepositoryScopeValues returns repository values for expression/config scopes, excluding secrets.
// The "name" field is the blueprint metadata name, which is also the name Windsor uses when creating
// the Flux GitRepository resource for this blueprint's repository.
func (h *BaseBlueprintHandler) getRepositoryScopeValues() map[string]any {
	repo := h.getEffectiveRepository()
	if repo.Url == "" {
		return nil
	}

	var blueprintName string
	if h.runtime != nil {
		blueprintName = h.runtime.ContextName
	}

	repositoryValue := map[string]any{
		"name": blueprintName,
		"url":  repo.Url,
	}
	refValue := make(map[string]any)
	if repo.Ref.Branch != "" {
		refValue["branch"] = repo.Ref.Branch
	}
	if repo.Ref.Tag != "" {
		refValue["tag"] = repo.Ref.Tag
	}
	if repo.Ref.SemVer != "" {
		refValue["semver"] = repo.Ref.SemVer
	}
	if repo.Ref.Name != "" {
		refValue["name"] = repo.Ref.Name
	}
	if repo.Ref.Commit != "" {
		refValue["commit"] = repo.Ref.Commit
	}
	if len(refValue) > 0 {
		repositoryValue["ref"] = refValue
	}

	return map[string]any{
		"repository": repositoryValue,
	}
}

// getEffectiveRepository resolves repository values from blueprint/runtime defaults for scope injection.
func (h *BaseBlueprintHandler) getEffectiveRepository() blueprintv1alpha1.Repository {
	var repo blueprintv1alpha1.Repository
	if h.composedBlueprint != nil && h.composedBlueprint.Repository.Url != "" {
		repo = h.composedBlueprint.Repository
	} else if h.userBlueprintLoader != nil && h.userBlueprintLoader.GetBlueprint() != nil {
		repo = h.userBlueprintLoader.GetBlueprint().Repository
	}
	return h.resolveRepositoryDefaults(repo, false)
}

// resolveRepositoryDefaults applies URL/ref/secret defaults to repository metadata.
// When includeSecret is false, SecretName is always removed from the returned value.
func (h *BaseBlueprintHandler) resolveRepositoryDefaults(repo blueprintv1alpha1.Repository, includeSecret bool) blueprintv1alpha1.Repository {
	devMode := h.runtime != nil && h.runtime.ConfigHandler != nil && h.runtime.ConfigHandler.GetBool("dev")
	if repo.Url == "" && devMode && h.runtime != nil && h.runtime.ConfigHandler != nil {
		domain := h.runtime.ConfigHandler.GetString("dns.domain", "test")
		folder := filepath.Base(h.runtime.ProjectRoot)
		if domain != "" && folder != "" && folder != "." {
			repo.Url = fmt.Sprintf("http://git.%s/git/%s", domain, folder)
		}
	}
	if repo.Url == "" && h.runtime != nil && h.runtime.Shell != nil {
		if gitURL, err := h.runtime.Shell.ExecSilent("git", "config", "--get", "remote.origin.url"); err == nil && gitURL != "" {
			repo.Url = runtimegit.NormalizeRepositoryURL(gitURL)
		}
	}
	if repo.Url != "" && repo.Ref == (blueprintv1alpha1.Reference{}) {
		repo.Ref = blueprintv1alpha1.Reference{Branch: "main"}
	}
	if repo.Url != "" && includeSecret && devMode && repo.SecretName == nil {
		secretName := "flux-system"
		repo.SecretName = &secretName
	}
	if !includeSecret {
		repo.SecretName = nil
	}
	return repo
}

// setRepositoryDefaults sets default repository values on the composed blueprint when
// Repository.Url is empty. In dev mode uses http://git.<dns.domain>/git/<project>;
// otherwise uses git remote origin URL. Also sets Ref.Branch to main and, in dev mode,
// SecretName to flux-system when not already set.
func (h *BaseBlueprintHandler) setRepositoryDefaults() {
	if h.composedBlueprint == nil || h.runtime == nil {
		return
	}
	h.composedBlueprint.Repository = h.resolveRepositoryDefaults(h.composedBlueprint.Repository, true)
}

// clearLocalTemplateSource clears Source on terraform components and kustomizations when Source
// is "template" and the template source has no URL (local template), or when Source is "user".
// The composed blueprint does not define a "user" source, so components tagged by user facet
// processing must be treated as local (project path) rather than sourced modules (scratch path).
func (h *BaseBlueprintHandler) clearLocalTemplateSource(blueprint *blueprintv1alpha1.Blueprint) {
	if blueprint == nil {
		return
	}
	var templateIsLocal bool
	for _, s := range blueprint.Sources {
		if blueprintv1alpha1.IsLocalTemplateSource(s) {
			templateIsLocal = true
			break
		}
	}
	for i := range blueprint.TerraformComponents {
		if templateIsLocal && blueprint.TerraformComponents[i].Source == "template" {
			blueprint.TerraformComponents[i].Source = ""
		}
		if blueprint.TerraformComponents[i].Source == "user" {
			blueprint.TerraformComponents[i].Source = ""
		}
	}
	for i := range blueprint.Kustomizations {
		if templateIsLocal && blueprint.Kustomizations[i].Source == "template" {
			blueprint.Kustomizations[i].Source = ""
		}
		if blueprint.Kustomizations[i].Source == "user" {
			blueprint.Kustomizations[i].Source = ""
		}
	}
}

// resolveComponentSource transforms a component's Source field from a source name (e.g., "local")
// into a fully qualified URL based on the matching entry in the blueprint's Sources array. For OCI
// sources, the URL is formatted as oci://registry/repo:tag//path/prefix/component.path. For Git
// sources, the URL is formatted as url//path/prefix/component.path?ref=reference. Components
// without a matching source or with an already-resolved URL are left unchanged.
func (h *BaseBlueprintHandler) resolveComponentSource(component *blueprintv1alpha1.TerraformComponent) {
	if h.composedBlueprint == nil || component.Source == "" {
		return
	}

	for _, source := range h.composedBlueprint.Sources {
		if component.Source != source.Name {
			continue
		}

		pathPrefix := source.PathPrefix
		if pathPrefix == "" {
			pathPrefix = "terraform"
		}

		if source.Name == "template" && source.Url == "" {
			component.Source = ""
			return
		}

		ref := h.getSourceRef(source)

		if strings.HasPrefix(source.Url, "oci://") {
			baseURL := source.Url
			if ref != "" {
				ociPrefix := "oci://"
				urlWithoutProtocol := baseURL[len(ociPrefix):]
				if !strings.Contains(urlWithoutProtocol, ":") {
					baseURL = baseURL + ":" + ref
				}
			}
			component.Source = baseURL + "//" + pathPrefix + "/" + component.Path
		} else {
			component.Source = source.Url + "//" + pathPrefix + "/" + component.Path + "?ref=" + ref
		}
		return
	}
}

// getSourceRef returns the reference (commit, semver, tag, or branch) for a source. It checks
// fields in priority order: Commit → SemVer → Tag → Branch, returning an empty string if none
// are specified. This matches the priority used by the terraform provisioner for consistency.
func (h *BaseBlueprintHandler) getSourceRef(source blueprintv1alpha1.Source) string {
	if source.Ref.Commit != "" {
		return source.Ref.Commit
	}
	if source.Ref.SemVer != "" {
		return source.Ref.SemVer
	}
	if source.Ref.Tag != "" {
		return source.Ref.Tag
	}
	if source.Ref.Branch != "" {
		return source.Ref.Branch
	}
	return ""
}

// resolveComponentFullPath computes and sets the absolute filesystem path where a terraform
// component's module will be located. Named components or those with a Source are placed in
// the Windsor scratch directory under contexts/<context>/terraform/<name|path>. Local components
// without a source are resolved to the project's terraform/<path> directory. This path is used
// by module resolvers and the terraform provisioner to locate module files.
func (h *BaseBlueprintHandler) resolveComponentFullPath(component *blueprintv1alpha1.TerraformComponent) {
	componentID := component.GetID()

	useScratchPath := component.Name != "" || component.Source != ""
	if useScratchPath {
		component.FullPath = filepath.Join(h.runtime.WindsorScratchPath, "terraform", componentID)
	} else {
		component.FullPath = filepath.Join(h.runtime.ProjectRoot, "terraform", componentID)
	}
}
