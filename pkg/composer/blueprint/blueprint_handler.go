package blueprint

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	_ "embed"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/runtime"

	"github.com/fluxcd/pkg/apis/kustomize"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The BlueprintHandler is a core component that manages infrastructure and application configurations
// through a declarative, GitOps-based approach. It handles the lifecycle of infrastructure blueprints,
// which are composed of Terraform components, Kubernetes Kustomizations, and associated metadata.
// The handler facilitates the resolution of component sources, manages repository configurations,
// and processes blueprint data for use by the provisioner. It supports both local and remote
// infrastructure definitions, enabling consistent and reproducible infrastructure deployments.

type BlueprintHandler interface {
	LoadBlueprint(blueprintURL ...string) error
	Write(overwrite ...bool) error
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	GetLocalTemplateData() (map[string][]byte, error)
	Generate() *blueprintv1alpha1.Blueprint
}

type BaseBlueprintHandler struct {
	BlueprintHandler
	runtime              *runtime.Runtime
	artifactBuilder      artifact.Artifact
	blueprint            blueprintv1alpha1.Blueprint
	featureEvaluator     *FeatureEvaluator
	shims                *Shims
	kustomizeData        map[string]any
	featureSubstitutions map[string]map[string]string
	commonSubstitutions  map[string]string
	configLoaded         bool
}

// NewBlueprintHandler creates a new instance of BaseBlueprintHandler with the provided dependencies.
// If overrides are provided, any non-nil component in the override BaseBlueprintHandler will be used instead of creating a default.
func NewBlueprintHandler(rt *runtime.Runtime, artifactBuilder artifact.Artifact, opts ...*BaseBlueprintHandler) (*BaseBlueprintHandler, error) {
	handler := &BaseBlueprintHandler{
		runtime:              rt,
		artifactBuilder:      artifactBuilder,
		featureEvaluator:     NewFeatureEvaluator(rt),
		shims:                NewShims(),
		kustomizeData:        make(map[string]any),
		featureSubstitutions: make(map[string]map[string]string),
		commonSubstitutions:  make(map[string]string),
	}

	if len(opts) > 0 && opts[0] != nil {
		overrides := opts[0]
		if overrides.featureEvaluator != nil {
			handler.featureEvaluator = overrides.featureEvaluator
		}
	}

	return handler, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// LoadBlueprint loads all blueprint data into memory, establishing defaults from either templates
// or OCI artifacts, then applies any local blueprint.yaml overrides to ensure the correct precedence.
// All sources are processed and merged into the in-memory runtime state.
// The optional blueprintURL parameter specifies the blueprint artifact to load (OCI URL).
// If not provided, falls back to the default blueprint URL from constants.
// Returns an error if any required paths are inaccessible or any loading operation fails.
func (b *BaseBlueprintHandler) LoadBlueprint(blueprintURL ...string) error {
	b.blueprint = *DefaultBlueprint.DeepCopy()

	contextName := b.runtime.ContextName
	if contextName == "" {
		contextName = b.runtime.ConfigHandler.GetContext()
	}
	if contextName != "" {
		b.blueprint.Metadata.Name = contextName
		b.blueprint.Metadata.Description = fmt.Sprintf("Blueprint for %s context", contextName)
	}

	configRoot := b.runtime.ConfigRoot
	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	hasContextBlueprint := false
	if _, err := b.shims.Stat(blueprintPath); err == nil {
		hasContextBlueprint = true
		if err := b.loadConfig(); err != nil {
			return fmt.Errorf("failed to load blueprint config: %w", err)
		}
	}

	hasBlueprintURL := len(blueprintURL) > 0 && blueprintURL[0] != ""
	hasLocalTemplate := false

	if _, err := b.shims.Stat(b.runtime.TemplateRoot); err == nil && !hasBlueprintURL {
		if err := b.loadBlueprintFromLocalTemplate(hasContextBlueprint); err != nil {
			return err
		}
		hasLocalTemplate = true
	}

	if !hasLocalTemplate {
		blueprintRef, err := b.resolveBlueprintReference(blueprintURL...)
		if err != nil {
			return err
		}

		if b.artifactBuilder == nil {
			return fmt.Errorf("blueprint.yaml not found at %s and artifact builder not available", blueprintPath)
		}

		artifacts, err := b.artifactBuilder.Pull([]string{blueprintRef})
		if err != nil {
			return fmt.Errorf("blueprint.yaml not found at %s and failed to pull blueprint: %w", blueprintPath, err)
		}

		registry, repository, tag, err := b.artifactBuilder.ParseOCIRef(blueprintRef)
		if err != nil {
			return fmt.Errorf("failed to parse OCI reference %s: %w", blueprintRef, err)
		}

		cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
		cacheDir, exists := artifacts[cacheKey]
		if !exists {
			return fmt.Errorf("failed to retrieve cache directory for %s", blueprintRef)
		}

		templateData, err := b.readTemplateDataFromCache(cacheDir)
		if err != nil {
			return fmt.Errorf("blueprint.yaml not found at %s and failed to read template data from cache: %w", blueprintPath, err)
		}

		if err := b.processOCIArtifact(templateData, blueprintRef); err != nil {
			return err
		}
	}

	if hasContextBlueprint {
		if err := b.processOCISources(); err != nil {
			return fmt.Errorf("failed to process OCI sources from blueprint: %w", err)
		}
		return nil
	}

	if err := b.processOCISources(); err != nil {
		return fmt.Errorf("failed to process OCI sources: %w", err)
	}

	if err := b.loadBlueprintConfigOverrides(); err != nil {
		return err
	}

	contextName = b.runtime.ContextName
	if contextName == "" {
		contextName = b.runtime.ConfigHandler.GetContext()
	}
	if contextName != "" {
		b.blueprint.Metadata.Name = contextName
		b.blueprint.Metadata.Description = fmt.Sprintf("Blueprint for %s context", contextName)
	}

	return nil
}

// Write persists the current blueprint state to blueprint.yaml in the configuration root directory.
// If overwrite is true, the file is overwritten regardless of existence. If overwrite is false or omitted,
// the file is only written if it does not already exist. The method ensures the target directory exists,
// marshals the blueprint to YAML, and writes the file using the configured shims.
// Terraform inputs and kustomization substitutions are manually cleared to prevent them from appearing in the final blueprint.yaml.
// Patches from _template, OCI artifacts, and contexts/ are processed in memory only and not written to disk.
func (b *BaseBlueprintHandler) Write(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	configRoot := b.runtime.ConfigRoot
	if configRoot == "" {
		return fmt.Errorf("error getting config root: config root is empty")
	}

	yamlPath := filepath.Join(configRoot, "blueprint.yaml")

	if err := b.shims.MkdirAll(filepath.Dir(yamlPath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	if err := b.setRepositoryDefaults(); err != nil {
		return fmt.Errorf("error setting repository defaults: %w", err)
	}

	if !shouldOverwrite {
		if _, err := b.shims.Stat(yamlPath); err == nil {
			return nil
		}
	}

	cleanedBlueprint := b.blueprint.DeepCopy()
	for i := range cleanedBlueprint.TerraformComponents {
		cleanedBlueprint.TerraformComponents[i].Inputs = map[string]any{}
	}
	for i := range cleanedBlueprint.Kustomizations {
		cleanedBlueprint.Kustomizations[i].Patches = nil
	}

	data, err := b.shims.YamlMarshal(cleanedBlueprint)
	if err != nil {
		return fmt.Errorf("error marshalling blueprint data: %w", err)
	}

	if err := b.shims.WriteFile(yamlPath, data, 0644); err != nil {
		return fmt.Errorf("error writing blueprint.yaml: %w", err)
	}

	return nil
}

// GetTerraformComponents retrieves the blueprint's Terraform components after resolving
// their sources and paths to full URLs and filesystem paths respectively.
func (b *BaseBlueprintHandler) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	resolvedBlueprint := b.blueprint

	b.resolveComponentSources(&resolvedBlueprint)
	b.resolveComponentPaths(&resolvedBlueprint)

	return resolvedBlueprint.TerraformComponents
}

// Generate returns a fully processed blueprint with all defaults resolved, paths updated,
// and generation logic applied. The returned blueprint is ready for deployment and reflects
// the complete, concrete state including kustomization and terraform component details,
// restored feature substitutions, and merged common or legacy variables as ConfigMaps.
// This function performs logic equivalent to getKustomizations() but applies it to the entire blueprint.
func (b *BaseBlueprintHandler) Generate() *blueprintv1alpha1.Blueprint {
	generated := b.blueprint.DeepCopy()

	for i := range generated.Kustomizations {
		if generated.Kustomizations[i].Source == "" {
			generated.Kustomizations[i].Source = generated.Metadata.Name
		}
		if generated.Kustomizations[i].Path == "" {
			generated.Kustomizations[i].Path = "kustomize"
		} else {
			generated.Kustomizations[i].Path = "kustomize/" + strings.ReplaceAll(generated.Kustomizations[i].Path, "\\", "/")
		}
		if generated.Kustomizations[i].Interval == nil || generated.Kustomizations[i].Interval.Duration == 0 {
			generated.Kustomizations[i].Interval = &metav1.Duration{Duration: constants.DefaultFluxKustomizationInterval}
		}
		if generated.Kustomizations[i].RetryInterval == nil || generated.Kustomizations[i].RetryInterval.Duration == 0 {
			generated.Kustomizations[i].RetryInterval = &metav1.Duration{Duration: constants.DefaultFluxKustomizationRetryInterval}
		}
		if generated.Kustomizations[i].Timeout == nil || generated.Kustomizations[i].Timeout.Duration == 0 {
			generated.Kustomizations[i].Timeout = &metav1.Duration{Duration: constants.DefaultFluxKustomizationTimeout}
		}
		if generated.Kustomizations[i].Wait == nil {
			defaultWait := constants.DefaultFluxKustomizationWait
			generated.Kustomizations[i].Wait = &defaultWait
		}
		if generated.Kustomizations[i].Force == nil {
			defaultForce := constants.DefaultFluxKustomizationForce
			generated.Kustomizations[i].Force = &defaultForce
		}
		if generated.Kustomizations[i].Destroy == nil {
			defaultDestroy := true
			generated.Kustomizations[i].Destroy = &defaultDestroy
		}
	}

	b.resolveComponentSources(generated)
	b.resolveComponentPaths(generated)

	for i := range generated.Kustomizations {
		if subs, exists := b.featureSubstitutions[generated.Kustomizations[i].Name]; exists {
			generated.Kustomizations[i].Substitutions = maps.Clone(subs)
		}

		configRoot := b.runtime.ConfigRoot
		if configRoot != "" {
			patchesDir := filepath.Join(configRoot, "patches", generated.Kustomizations[i].Name)
			if _, err := b.shims.Stat(patchesDir); err == nil {
				var discoveredPatches []blueprintv1alpha1.BlueprintPatch
				err := b.shims.Walk(patchesDir, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if info.IsDir() {
						return nil
					}
					if !strings.HasSuffix(strings.ToLower(info.Name()), ".yaml") && !strings.HasSuffix(strings.ToLower(info.Name()), ".yml") {
						return nil
					}
					data, err := b.shims.ReadFile(path)
					if err != nil {
						return nil
					}
					patchContent := string(data)
					relPath, err := filepath.Rel(patchesDir, path)
					if err != nil {
						return nil
					}
					defaultNamespace := b.runtime.ConfigHandler.GetContext()
					isJSON6902, target := func() (bool, *kustomize.Selector) {
						decoder := yaml.NewDecoder(strings.NewReader(patchContent))
						for {
							var doc map[string]any
							if err := decoder.Decode(&doc); err != nil {
								if err == io.EOF {
									break
								}
								continue
							}
							if doc == nil {
								continue
							}
							hasAPIVersion := false
							hasKind := false
							hasMetadata := false
							if _, ok := doc["apiVersion"]; ok {
								hasAPIVersion = true
							}
							if _, ok := doc["kind"]; ok {
								hasKind = true
							}
							if _, ok := doc["metadata"]; ok {
								hasMetadata = true
							}
							if hasAPIVersion && hasKind && hasMetadata {
								for key, v := range doc {
									if key == "patch" || key == "patches" {
										if arr, ok := v.([]any); ok {
											if len(arr) > 0 {
												if firstItem, ok := arr[0].(map[string]any); ok {
													if _, ok := firstItem["op"].(string); ok {
														if _, hasPath := firstItem["path"]; hasPath {
															return true, b.extractTargetFromPatchData(doc, defaultNamespace)
														}
													}
												}
											}
										}
									}
								}
								return false, nil
							}
						}
						return false, nil
					}()
					patch := blueprintv1alpha1.BlueprintPatch{
						Path:  relPath,
						Patch: patchContent,
					}
					if isJSON6902 && target != nil {
						patch.Target = target
					}
					discoveredPatches = append(discoveredPatches, patch)
					return nil
				})
				if err == nil && len(discoveredPatches) > 0 {
					for j := range discoveredPatches {
						discoveredPatches[j].Path = ""
					}
					generated.Kustomizations[i].Patches = append(generated.Kustomizations[i].Patches, discoveredPatches...)
				}
			}
		}
	}

	mergedCommonValues := make(map[string]string)
	if b.commonSubstitutions != nil {
		maps.Copy(mergedCommonValues, b.commonSubstitutions)
	}

	b.mergeLegacySpecialVariables(mergedCommonValues)

	if len(mergedCommonValues) > 0 {
		if generated.ConfigMaps == nil {
			generated.ConfigMaps = make(map[string]map[string]string)
		}
		generated.ConfigMaps["values-common"] = mergedCommonValues
	}

	return generated
}

// GetLocalTemplateData loads template files from contexts/_template, merging values.yaml from both
// the _template and context directories. It collects all files recursively under the template root,
// preserving their relative paths. If values from an OCI artifact are present, they are merged with
// local values, and local values take precedence. The returned map has relative file paths as keys
// and file contents as values. Returns nil if no templates exist. Returns an error if processing fails.
func (b *BaseBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	if _, err := b.shims.Stat(b.runtime.TemplateRoot); os.IsNotExist(err) {
		return nil, nil
	}

	templateData, err := b.collectTemplateDataFromDirectory(b.runtime.TemplateRoot)
	if err != nil {
		return nil, err
	}

	metadataPath := filepath.Join(b.runtime.TemplateRoot, "metadata.yaml")
	if _, err := b.shims.Stat(metadataPath); err == nil {
		metadataContent, err := b.shims.ReadFile(metadataPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read metadata.yaml: %w", err)
		}
		var metadata artifact.BlueprintMetadataInput
		if err := b.shims.YamlUnmarshal(metadataContent, &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
		}
		if err := artifact.ValidateCliVersion(constants.Version, metadata.CliVersion); err != nil {
			return nil, err
		}
	}

	if schemaData, exists := templateData["schema"]; exists {
		if err := b.runtime.ConfigHandler.LoadSchemaFromBytes(schemaData); err != nil {
			return nil, fmt.Errorf("failed to load schema: %w", err)
		}
	} else if schemaData, exists := templateData["_template/schema.yaml"]; exists {
		if err := b.runtime.ConfigHandler.LoadSchemaFromBytes(schemaData); err != nil {
			return nil, fmt.Errorf("failed to load schema: %w", err)
		}
	}

	contextValues, err := b.runtime.ConfigHandler.GetContextValues()
	if err != nil {
		return nil, fmt.Errorf("failed to load context values: %w", err)
	}

	config := make(map[string]any)
	for k, v := range contextValues {
		if k != "substitutions" {
			config[k] = v
		}
	}

	configRoot := b.runtime.ConfigRoot
	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	modifyOnly := false
	if _, err := b.shims.Stat(blueprintPath); err == nil {
		modifyOnly = true
	}

	if err := b.processFeatures(templateData, config, modifyOnly); err != nil {
		return nil, fmt.Errorf("failed to process features: %w", err)
	}

	contextName := b.runtime.ConfigHandler.GetContext()
	if contextName != "" {
		b.blueprint.Metadata.Name = contextName
		b.blueprint.Metadata.Description = fmt.Sprintf("Blueprint for %s context", contextName)
	}

	if len(b.blueprint.TerraformComponents) > 0 || len(b.blueprint.Kustomizations) > 0 || b.blueprint.Metadata.Name != "" {
		composedBlueprintYAML, err := b.shims.YamlMarshal(b.blueprint)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal composed blueprint: %w", err)
		}
		templateData["blueprint"] = composedBlueprintYAML
	}

	var substitutionValues map[string]any
	if contextValues != nil {
		if contextSubs, ok := contextValues["substitutions"].(map[string]any); ok && len(contextSubs) > 0 {
			substitutionValues = contextSubs
		}
	}

	if len(substitutionValues) > 0 {
		substitutionYAML, err := b.shims.YamlMarshal(substitutionValues)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal substitution values: %w", err)
		}
		templateData["substitutions"] = substitutionYAML

		if commonSubs, ok := substitutionValues["common"].(map[string]any); ok && len(commonSubs) > 0 {
			b.commonSubstitutions = make(map[string]string)
			for k, v := range commonSubs {
				b.commonSubstitutions[k] = fmt.Sprintf("%v", v)
			}
		}

		for key, value := range substitutionValues {
			if key == "common" {
				continue
			}
			if kustomizationSubs, ok := value.(map[string]any); ok && len(kustomizationSubs) > 0 {
				if b.featureSubstitutions[key] == nil {
					b.featureSubstitutions[key] = make(map[string]string)
				}
				for k, v := range kustomizationSubs {
					b.featureSubstitutions[key][k] = fmt.Sprintf("%v", v)
				}
			}
		}
	}

	return templateData, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// loadBlueprintFromLocalTemplate loads the blueprint from the local template directory.
// If modifyOnly is true, features will only modify existing components in the blueprint.
func (b *BaseBlueprintHandler) loadBlueprintFromLocalTemplate(modifyOnly bool) error {
	templateData, err := b.GetLocalTemplateData()
	if err != nil {
		return fmt.Errorf("failed to get local template data: %w", err)
	}
	if len(templateData) > 0 {
		if err := b.processArtifactTemplateData(templateData, modifyOnly); err != nil {
			return fmt.Errorf("failed to process local template data: %w", err)
		}

		contextName := b.runtime.ContextName
		if contextName != "" {
			b.blueprint.Metadata.Name = contextName
			b.blueprint.Metadata.Description = fmt.Sprintf("Blueprint for %s context", contextName)
		}
		return nil
	}

	configRoot := b.runtime.ConfigRoot
	if configRoot == "" {
		return fmt.Errorf("blueprint.yaml not found at %s", filepath.Join(configRoot, "blueprint.yaml"))
	}
	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	if _, err := b.shims.Stat(blueprintPath); err != nil {
		return fmt.Errorf("blueprint.yaml not found at %s", blueprintPath)
	}
	return nil
}

// resolveBlueprintReference determines the effective blueprint URL and parses it into an OCI reference.
// Returns the blueprint OCI reference and any error.
func (b *BaseBlueprintHandler) resolveBlueprintReference(blueprintURL ...string) (blueprintRef string, err error) {
	var effectiveBlueprintURL string
	if len(blueprintURL) > 0 && blueprintURL[0] != "" {
		effectiveBlueprintURL = blueprintURL[0]
	} else {
		effectiveBlueprintURL = constants.GetEffectiveBlueprintURL()
	}

	ociInfo, err := artifact.ParseOCIReference(effectiveBlueprintURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse blueprint reference: %w", err)
	}
	if ociInfo == nil {
		return "", fmt.Errorf("invalid blueprint reference: %s", effectiveBlueprintURL)
	}

	return ociInfo.URL, nil
}

// processOCIArtifact processes blueprint data from an OCI artifact.
// It loads the schema, gets context values, processes features, and sets the OCI source on components.
func (b *BaseBlueprintHandler) processOCIArtifact(templateData map[string][]byte, blueprintRef string) error {
	modifyOnly := b.configLoaded
	if err := b.processArtifactTemplateData(templateData, modifyOnly); err != nil {
		return err
	}

	contextName := b.runtime.ContextName
	if contextName != "" {
		b.blueprint.Metadata.Name = contextName
		b.blueprint.Metadata.Description = fmt.Sprintf("Blueprint for %s context", contextName)
	}

	ociInfo, _ := artifact.ParseOCIReference(blueprintRef)
	if ociInfo != nil {
		b.setOCISource(ociInfo)
	}

	return nil
}

// processArtifactTemplateData processes template data from an artifact by loading schema, building feature template data,
// setting it on the feature evaluator, and processing features. This common functionality is shared by processOCIArtifact
// and processOCISources.
// If modifyOnly is true, features will only modify existing components in the blueprint.
// If sourceName is provided and non-empty, it sets the Source on components and kustomizations from Features that don't have a Source set.
func (b *BaseBlueprintHandler) processArtifactTemplateData(templateData map[string][]byte, modifyOnly bool, sourceName ...string) error {
	if schemaData, exists := templateData["_template/schema.yaml"]; exists {
		if err := b.runtime.ConfigHandler.LoadSchemaFromBytes(schemaData); err != nil {
			return fmt.Errorf("failed to load schema from artifact: %w", err)
		}
	}

	config, err := b.runtime.ConfigHandler.GetContextValues()
	if err != nil {
		return fmt.Errorf("failed to load context values: %w", err)
	}

	featureTemplateData := make(map[string][]byte)
	for k, v := range templateData {
		if featureKey, ok := strings.CutPrefix(k, "_template/"); ok {
			if featureKey == "blueprint.yaml" {
				featureTemplateData["blueprint"] = v
			} else {
				featureTemplateData[featureKey] = v
			}
		}
	}
	if blueprintData, exists := templateData["blueprint"]; exists {
		featureTemplateData["blueprint"] = blueprintData
	}

	b.featureEvaluator.SetTemplateData(templateData)

	if err := b.processFeatures(featureTemplateData, config, modifyOnly, sourceName...); err != nil {
		return fmt.Errorf("failed to process features: %w", err)
	}

	return nil
}

// setOCISource adds or updates the OCI source in the blueprint and sets it on components that don't have a source.
func (b *BaseBlueprintHandler) setOCISource(ociInfo *artifact.OCIArtifactInfo) {
	ociSource := blueprintv1alpha1.Source{
		Name: ociInfo.Name,
		Url:  ociInfo.URL,
	}

	sourceExists := false
	for i, source := range b.blueprint.Sources {
		if source.Name == ociInfo.Name {
			b.blueprint.Sources[i] = ociSource
			sourceExists = true
			break
		}
	}

	if !sourceExists {
		b.blueprint.Sources = append(b.blueprint.Sources, ociSource)
	}

	for i := range b.blueprint.TerraformComponents {
		if b.blueprint.TerraformComponents[i].Source == "" {
			b.blueprint.TerraformComponents[i].Source = ociInfo.Name
		}
	}

	for i := range b.blueprint.Kustomizations {
		if b.blueprint.Kustomizations[i].Source == "" {
			b.blueprint.Kustomizations[i].Source = ociInfo.Name
		}
	}
}

// pullOCISources pulls all OCI sources referenced in the blueprint.
func (b *BaseBlueprintHandler) pullOCISources() error {
	sources := b.getSources()
	if len(sources) == 0 {
		return nil
	}

	if b.artifactBuilder == nil {
		return nil
	}

	var ociURLs []string
	for _, source := range sources {
		if strings.HasPrefix(source.Url, "oci://") {
			ociURLs = append(ociURLs, source.Url)
		}
	}

	if len(ociURLs) > 0 {
		if _, err := b.artifactBuilder.Pull(ociURLs); err != nil {
			return fmt.Errorf("failed to load OCI sources: %w", err)
		}
	}

	return nil
}

// processOCISources processes all OCI sources listed in the blueprint's Sources section.
// It extracts Features from each OCI artifact and merges them into the blueprint.
// If a blueprint.yaml already exists, only Inputs are applied to existing components (modifyOnly=true).
// If no blueprint.yaml exists, components are added from Features (modifyOnly=false).
func (b *BaseBlueprintHandler) processOCISources() error {
	sources := b.getSources()
	if len(sources) == 0 {
		return nil
	}

	if b.artifactBuilder == nil {
		return nil
	}

	var ociURLs []string
	for _, source := range sources {
		if strings.HasPrefix(source.Url, "oci://") {
			ociURL := b.buildOCIURLWithRef(source)
			ociURLs = append(ociURLs, ociURL)
		}
	}

	if len(ociURLs) > 0 {
		artifacts, err := b.artifactBuilder.Pull(ociURLs)
		if err != nil {
			return fmt.Errorf("failed to pull OCI sources: %w", err)
		}

		for _, source := range sources {
			if !strings.HasPrefix(source.Url, "oci://") {
				continue
			}

			ociURL := b.buildOCIURLWithRef(source)

			registry, repository, tag, err := b.artifactBuilder.ParseOCIRef(ociURL)
			if err != nil {
				return fmt.Errorf("failed to parse OCI reference %s: %w", ociURL, err)
			}

			cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
			cacheDir, exists := artifacts[cacheKey]
			if !exists {
				return fmt.Errorf("failed to retrieve cache directory for %s", ociURL)
			}

			templateData, err := b.readTemplateDataFromCache(cacheDir)
			if err != nil {
				return fmt.Errorf("failed to read template data from OCI source %s: %w", ociURL, err)
			}

			if schemaData, exists := templateData["_template/schema.yaml"]; exists {
				if err := b.runtime.ConfigHandler.LoadSchemaFromBytes(schemaData); err != nil {
					return fmt.Errorf("failed to load schema from artifact: %w", err)
				}
			}

			config, err := b.runtime.ConfigHandler.GetContextValues()
			if err != nil {
				return fmt.Errorf("failed to load context values: %w", err)
			}

			featureTemplateData := make(map[string][]byte)
			for k, v := range templateData {
				if featureKey, ok := strings.CutPrefix(k, "_template/"); ok {
					if featureKey == "blueprint.yaml" {
						featureTemplateData["blueprint"] = v
					} else {
						featureTemplateData[featureKey] = v
					}
				}
			}

			b.featureEvaluator.SetTemplateData(templateData)

			configRoot := b.runtime.ConfigRoot
			blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
			modifyOnly := false
			if _, err := b.shims.Stat(blueprintPath); err == nil {
				modifyOnly = true
			}

			if err := b.processFeatures(featureTemplateData, config, modifyOnly, source.Name); err != nil {
				return fmt.Errorf("failed to process OCI source %s: %w", source.Url, err)
			}
		}
	}

	return nil
}

// getSourceRef extracts the reference (commit, semver, tag, or branch) from a source in priority order.
func (b *BaseBlueprintHandler) getSourceRef(source blueprintv1alpha1.Source) string {
	ref := source.Ref.Commit
	if ref == "" {
		ref = source.Ref.SemVer
	}
	if ref == "" {
		ref = source.Ref.Tag
	}
	if ref == "" {
		ref = source.Ref.Branch
	}
	return ref
}

// buildOCIURLWithRef constructs a full OCI URL from a source, appending the ref as a tag if present and not already in the URL.
func (b *BaseBlueprintHandler) buildOCIURLWithRef(source blueprintv1alpha1.Source) string {
	ociURL := source.Url
	ref := b.getSourceRef(source)
	if ref != "" {
		ociPrefix := "oci://"
		if strings.HasPrefix(ociURL, ociPrefix) {
			urlWithoutProtocol := ociURL[len(ociPrefix):]
			if !strings.Contains(urlWithoutProtocol, ":") {
				ociURL = ociURL + ":" + ref
			}
		} else if !strings.Contains(ociURL, ":") {
			ociURL = ociURL + ":" + ref
		}
	}
	return ociURL
}

// loadBlueprintConfigOverrides loads blueprint configuration overrides from the config root if they exist.
func (b *BaseBlueprintHandler) loadBlueprintConfigOverrides() error {
	configRoot := b.runtime.ConfigRoot
	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	if _, err := b.shims.Stat(blueprintPath); err == nil {
		if err := b.loadConfig(); err != nil {
			return fmt.Errorf("failed to load blueprint config overrides: %w", err)
		}
	}
	return nil
}

// loadConfig reads blueprint configuration from blueprint.yaml file.
// Returns an error if blueprint.yaml does not exist.
// Template processing is now handled by the pkg/template package.
func (b *BaseBlueprintHandler) loadConfig() error {
	configRoot := b.runtime.ConfigRoot
	if configRoot == "" {
		return fmt.Errorf("error getting config root: config root is empty")
	}

	yamlPath := filepath.Join(configRoot, "blueprint.yaml")
	if _, err := b.shims.Stat(yamlPath); err != nil {
		return fmt.Errorf("blueprint.yaml not found at %s", yamlPath)
	}

	yamlData, err := b.shims.ReadFile(yamlPath)
	if err != nil {
		return err
	}

	newBlueprint := &blueprintv1alpha1.Blueprint{}
	if err := b.shims.YamlUnmarshal(yamlData, newBlueprint); err != nil {
		return fmt.Errorf("error unmarshalling blueprint data: %w", err)
	}

	if newBlueprint.Kind == "" {
		newBlueprint.Kind = "Blueprint"
	}
	if newBlueprint.ApiVersion == "" {
		newBlueprint.ApiVersion = "blueprints.windsorcli.dev/v1alpha1"
	}

	b.blueprint = *newBlueprint

	b.configLoaded = true
	return nil
}

// getMetadata retrieves the current blueprint's metadata.
func (b *BaseBlueprintHandler) getMetadata() blueprintv1alpha1.Metadata {
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Metadata
}

// getRepository retrieves the current blueprint's repository configuration, ensuring
// default values are set for empty fields.
func (b *BaseBlueprintHandler) getRepository() blueprintv1alpha1.Repository {
	resolvedBlueprint := b.blueprint
	repository := resolvedBlueprint.Repository

	if repository.Url == "" {
		repository.Url = ""
	}
	if repository.Ref == (blueprintv1alpha1.Reference{}) {
		repository.Ref = blueprintv1alpha1.Reference{Branch: "main"}
	}

	return repository
}

// getSources retrieves the current blueprint's source configurations.
func (b *BaseBlueprintHandler) getSources() []blueprintv1alpha1.Source {
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Sources
}

// getKustomizations returns the current blueprint's kustomization configurations with all default values resolved.
// It copies the kustomizations from the blueprint, sets default values for Source, Path, Interval, RetryInterval,
// Timeout, Wait, Force, and Destroy fields if unset, discovers and appends patches, and sets the PostBuild configuration.
// This method ensures all kustomization fields are fully populated for downstream processing.
func (b *BaseBlueprintHandler) getKustomizations() []blueprintv1alpha1.Kustomization {
	resolvedBlueprint := b.blueprint
	kustomizations := make([]blueprintv1alpha1.Kustomization, len(resolvedBlueprint.Kustomizations))
	copy(kustomizations, resolvedBlueprint.Kustomizations)

	for i := range kustomizations {
		if kustomizations[i].Source == "" {
			kustomizations[i].Source = b.blueprint.Metadata.Name
		}

		if kustomizations[i].Path == "" {
			kustomizations[i].Path = "kustomize"
		} else {
			kustomizations[i].Path = "kustomize/" + strings.ReplaceAll(kustomizations[i].Path, "\\", "/")
		}

		if kustomizations[i].Interval == nil || kustomizations[i].Interval.Duration == 0 {
			kustomizations[i].Interval = &metav1.Duration{Duration: constants.DefaultFluxKustomizationInterval}
		}
		if kustomizations[i].RetryInterval == nil || kustomizations[i].RetryInterval.Duration == 0 {
			kustomizations[i].RetryInterval = &metav1.Duration{Duration: constants.DefaultFluxKustomizationRetryInterval}
		}
		if kustomizations[i].Timeout == nil || kustomizations[i].Timeout.Duration == 0 {
			kustomizations[i].Timeout = &metav1.Duration{Duration: constants.DefaultFluxKustomizationTimeout}
		}
		if kustomizations[i].Wait == nil {
			defaultWait := constants.DefaultFluxKustomizationWait
			kustomizations[i].Wait = &defaultWait
		}
		if kustomizations[i].Force == nil {
			defaultForce := constants.DefaultFluxKustomizationForce
			kustomizations[i].Force = &defaultForce
		}
		if kustomizations[i].Destroy == nil {
			defaultDestroy := true
			kustomizations[i].Destroy = &defaultDestroy
		}

	}

	return kustomizations
}

// walkAndCollectTemplates recursively traverses the specified template directory and collects all files into the
// templateData map. It adds the contents of each file by a normalized relative path key prefixed with "_template/".
// baseDir is used as the root for calculating relative paths. Directory entries are processed recursively.
// Any file or directory traversal errors are returned.
func (b *BaseBlueprintHandler) walkAndCollectTemplates(baseDir, templateDir string, templateData map[string][]byte) error {
	entries, err := b.shims.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(templateDir, entry.Name())

		if entry.IsDir() {
			if err := b.walkAndCollectTemplates(baseDir, entryPath, templateData); err != nil {
				return err
			}
		} else {
			content, err := b.shims.ReadFile(filepath.Clean(entryPath))
			if err != nil {
				return fmt.Errorf("failed to read template file %s: %w", entryPath, err)
			}

			relPath, err := filepath.Rel(baseDir, entryPath)
			if err != nil {
				return fmt.Errorf("failed to calculate relative path for %s: %w", entryPath, err)
			}

			relPath = strings.ReplaceAll(relPath, "\\", "/")
			key := "_template/" + relPath

			templateData[key] = content
		}
	}

	return nil
}

// processFeatures evaluates and applies blueprint features by processing conditional logic and merging matching feature content into the target blueprint.
// The method loads the base blueprint from provided template data (supporting both canonical and alternate keys), unmarshals it, and initially merges it with the handler's blueprint.
// It then loads all feature files from the template data, sorting them deterministically by feature name to ensure consistent merge order.
// For each feature whose conditional `When` expression evaluates to true, the function processes its Terraform components and Kustomizations, which may themselves have additional conditional logic.
// Inputs, substitutions, and patches for each component and kustomization are evaluated and applied according to a merge or replace strategy as specified.
// If sourceName is provided and non-empty, it sets the Source field on new components and Kustomizations if not already set from the feature definition.
// If modifyOnly is true, features will only modify existing components in the target blueprint; new components are not added and the template blueprint is not merged.
// Merges and substitutions are performed in accordance with each merge strategy, ensuring correct accumulation of substitutions for later use.
// Returns an error if conditional logic fails, unmarshalling fails, or a merge operation encounters an error.
func (b *BaseBlueprintHandler) processFeatures(templateData map[string][]byte, config map[string]any, modifyOnly bool, sourceName ...string) error {
	blueprintData, _ := templateData["_template/blueprint.yaml"]
	if blueprintData == nil {
		blueprintData, _ = templateData["blueprint"]
	}

	if blueprintData != nil && !modifyOnly {
		newBlueprint := &blueprintv1alpha1.Blueprint{}
		if err := b.shims.YamlUnmarshal(blueprintData, newBlueprint); err != nil {
			return fmt.Errorf("error unmarshalling blueprint data: %w", err)
		}

		completeBlueprint := &blueprintv1alpha1.Blueprint{
			Kind:                newBlueprint.Kind,
			ApiVersion:          newBlueprint.ApiVersion,
			Metadata:            newBlueprint.Metadata,
			Sources:             newBlueprint.Sources,
			TerraformComponents: newBlueprint.TerraformComponents,
			Kustomizations:      newBlueprint.Kustomizations,
			Repository:          newBlueprint.Repository,
		}

		if err := b.blueprint.StrategicMerge(completeBlueprint); err != nil {
			return fmt.Errorf("failed to strategic merge blueprint: %w", err)
		}
	}

	features, err := b.loadFeatures(templateData)
	if err != nil {
		return fmt.Errorf("failed to load features: %w", err)
	}

	if len(features) == 0 {
		return nil
	}

	evaluator := b.featureEvaluator

	sort.Slice(features, func(i, j int) bool {
		return features[i].Metadata.Name < features[j].Metadata.Name
	})

	var featureBlueprint *blueprintv1alpha1.Blueprint
	featureSubstitutionsByKustomization := make(map[string]map[string]string)

	targetBlueprint := &b.blueprint
	if modifyOnly {
		featureBlueprint = &blueprintv1alpha1.Blueprint{}
		targetBlueprint = featureBlueprint
	}

	var deferredRemovalsTerraform []blueprintv1alpha1.TerraformComponent
	var deferredRemovalsKustomize []blueprintv1alpha1.Kustomization

	for _, feature := range features {
		if feature.When != "" {
			matches, err := evaluator.EvaluateExpression(feature.When, config, feature.Path)
			if err != nil {
				return fmt.Errorf("failed to evaluate feature condition '%s': %w", feature.When, err)
			}
			if !matches {
				continue
			}
		}

		for _, terraformComponent := range feature.TerraformComponents {
			if terraformComponent.When != "" {
				matches, err := evaluator.EvaluateExpression(terraformComponent.When, config, feature.Path)
				if err != nil {
					return fmt.Errorf("failed to evaluate terraform component condition '%s': %w", terraformComponent.When, err)
				}
				if !matches {
					continue
				}
			}

			component := terraformComponent.TerraformComponent

			if len(sourceName) > 0 && sourceName[0] != "" && component.Source == "" {
				component.Source = sourceName[0]
			}

			if len(terraformComponent.Inputs) > 0 {
				evaluatedInputs, err := evaluator.EvaluateDefaults(terraformComponent.Inputs, config, feature.Path)
				if err != nil {
					return fmt.Errorf("failed to evaluate inputs for component '%s': %w", component.Path, err)
				}

				filteredInputs := make(map[string]any)
				for k, v := range evaluatedInputs {
					if v != nil {
						filteredInputs[k] = v
					}
				}

				if len(filteredInputs) > 0 {
					component.Inputs = filteredInputs
				}
			} else {
				component.Inputs = nil
			}

			if modifyOnly {
				if !b.componentExists(component, sourceName...) {
					continue
				}
			}

			strategy := terraformComponent.Strategy
			if strategy == "" {
				strategy = "merge"
			}

			switch strategy {
			case "remove":
				deferredRemovalsTerraform = append(deferredRemovalsTerraform, component)
			case "replace":
				if err := targetBlueprint.ReplaceTerraformComponent(component); err != nil {
					return fmt.Errorf("failed to replace terraform component: %w", err)
				}
			default:
				tempBlueprint := &blueprintv1alpha1.Blueprint{
					TerraformComponents: []blueprintv1alpha1.TerraformComponent{component},
				}
				if err := targetBlueprint.StrategicMerge(tempBlueprint); err != nil {
					return fmt.Errorf("failed to merge terraform component: %w", err)
				}
			}
		}

		for _, kustomization := range feature.Kustomizations {
			if kustomization.When != "" {
				matches, err := evaluator.EvaluateExpression(kustomization.When, config, feature.Path)
				if err != nil {
					return fmt.Errorf("failed to evaluate kustomization condition '%s': %w", kustomization.When, err)
				}
				if !matches {
					continue
				}
			}

			kustomizationCopy := kustomization.Kustomization

			if len(sourceName) > 0 && sourceName[0] != "" && kustomizationCopy.Source == "" {
				kustomizationCopy.Source = sourceName[0]
			}

			if modifyOnly {
				exists := false
				for _, existing := range b.blueprint.Kustomizations {
					if existing.Name == kustomizationCopy.Name {
						exists = true
						break
					}
				}
				if !exists {
					continue
				}
			}

			strategy := kustomization.Strategy
			if strategy == "" {
				strategy = "merge"
			}

			if strategy == "remove" {
				if len(kustomization.Substitutions) > 0 {
					evaluatedSubstitutions, err := b.evaluateSubstitutions(kustomization.Substitutions, config, feature.Path)
					if err != nil {
						return fmt.Errorf("failed to evaluate substitutions for kustomization '%s': %w", kustomizationCopy.Name, err)
					}
					kustomizationCopy.Substitutions = evaluatedSubstitutions
				}
				deferredRemovalsKustomize = append(deferredRemovalsKustomize, kustomizationCopy)
			} else {
				if len(kustomization.Substitutions) > 0 {
					evaluatedSubstitutions, err := b.evaluateSubstitutions(kustomization.Substitutions, config, feature.Path)
					if err != nil {
						return fmt.Errorf("failed to evaluate substitutions for kustomization '%s': %w", kustomizationCopy.Name, err)
					}

					if existingSubs, exists := featureSubstitutionsByKustomization[kustomizationCopy.Name]; exists {
						maps.Copy(existingSubs, evaluatedSubstitutions)
					} else {
						featureSubstitutionsByKustomization[kustomizationCopy.Name] = evaluatedSubstitutions
					}
				}

				for j := range kustomizationCopy.Patches {
					if kustomizationCopy.Patches[j].Patch != "" {
						evaluated, err := b.featureEvaluator.InterpolateString(kustomizationCopy.Patches[j].Patch, config, feature.Path)
						if err != nil {
							return fmt.Errorf("failed to evaluate patch for kustomization '%s': %w", kustomizationCopy.Name, err)
						}
						kustomizationCopy.Patches[j].Patch = evaluated
					}
				}

				kustomizationCopy.Substitutions = nil

				if strategy == "replace" {
					if err := targetBlueprint.ReplaceKustomization(kustomizationCopy); err != nil {
						return fmt.Errorf("failed to replace kustomization: %w", err)
					}
				} else {
					tempBlueprint := &blueprintv1alpha1.Blueprint{
						Kustomizations: []blueprintv1alpha1.Kustomization{kustomizationCopy},
					}
					if err := targetBlueprint.StrategicMerge(tempBlueprint); err != nil {
						return fmt.Errorf("failed to merge kustomization: %w", err)
					}
				}
			}
		}
	}

	for _, removal := range deferredRemovalsTerraform {
		if err := targetBlueprint.RemoveTerraformComponent(removal); err != nil {
			return fmt.Errorf("failed to remove terraform component: %w", err)
		}
	}

	for _, removal := range deferredRemovalsKustomize {
		if err := targetBlueprint.RemoveKustomization(removal); err != nil {
			return fmt.Errorf("failed to remove kustomization: %w", err)
		}
	}

	if modifyOnly {
		featureComponentsByPath := make(map[string]blueprintv1alpha1.TerraformComponent)
		for _, c := range featureBlueprint.TerraformComponents {
			key := b.componentKey(c, sourceName...)
			featureComponentsByPath[key] = c
		}

		for i := range b.blueprint.TerraformComponents {
			component := &b.blueprint.TerraformComponents[i]
			key := b.componentKey(*component, sourceName...)

			if featureComp, exists := featureComponentsByPath[key]; exists {
				if component.Inputs == nil {
					component.Inputs = make(map[string]any)
				}
				component.Inputs = b.deepMergeMaps(featureComp.Inputs, component.Inputs)

				if component.Source == "" && len(sourceName) > 0 && sourceName[0] != "" {
					component.Source = sourceName[0]
				}
			}
		}

		featureKustomizationsByName := make(map[string]blueprintv1alpha1.Kustomization)
		for _, k := range featureBlueprint.Kustomizations {
			featureKustomizationsByName[k.Name] = k
		}

		for i := range b.blueprint.Kustomizations {
			kustomization := &b.blueprint.Kustomizations[i]
			if featureKustom, exists := featureKustomizationsByName[kustomization.Name]; exists {
				if len(featureKustom.Patches) > 0 {
					kustomization.Patches = append(featureKustom.Patches, kustomization.Patches...)
				}
				if substitutions, exists := featureSubstitutionsByKustomization[kustomization.Name]; exists {
					if b.featureSubstitutions[kustomization.Name] == nil {
						b.featureSubstitutions[kustomization.Name] = make(map[string]string)
					}
					maps.Copy(b.featureSubstitutions[kustomization.Name], substitutions)
				}
				if kustomization.Source == "" && len(sourceName) > 0 && sourceName[0] != "" {
					kustomization.Source = sourceName[0]
				}
			}
		}

	} else {
		// Normal Mode: Apply all collected substitutions (components were already merged into b.blueprint via targetBlueprint)
		for name, substitutions := range featureSubstitutionsByKustomization {
			if b.featureSubstitutions[name] == nil {
				b.featureSubstitutions[name] = make(map[string]string)
			}
			maps.Copy(b.featureSubstitutions[name], substitutions)
		}
	}

	return nil
}

// loadFeatures extracts and parses feature files from template data.
// It looks for files with paths starting with "features/" or "_template/features/" and ending with ".yaml",
// parses them as Feature objects, and returns a slice of all valid features.
// Returns an error if any feature file cannot be parsed.
func (b *BaseBlueprintHandler) loadFeatures(templateData map[string][]byte) ([]blueprintv1alpha1.Feature, error) {
	var features []blueprintv1alpha1.Feature

	for path, content := range templateData {
		isFeature := (strings.HasPrefix(path, "features/") || strings.HasPrefix(path, "_template/features/")) && strings.HasSuffix(path, ".yaml")
		if isFeature {
			feature, err := b.parseFeature(content)
			if err != nil {
				return nil, fmt.Errorf("failed to parse feature %s: %w", path, err)
			}
			featurePath := strings.TrimPrefix(path, "_template/")
			feature.Path = filepath.Join(b.runtime.TemplateRoot, featurePath)
			features = append(features, *feature)
		}
	}

	return features, nil
}

// parseFeature parses YAML content into a Feature object.
// It validates that the feature has the correct kind and apiVersion,
// and ensures required fields are present.
func (b *BaseBlueprintHandler) parseFeature(content []byte) (*blueprintv1alpha1.Feature, error) {
	var feature blueprintv1alpha1.Feature

	if err := b.shims.YamlUnmarshal(content, &feature); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if feature.Kind != "Feature" {
		return nil, fmt.Errorf("expected kind 'Feature', got '%s'", feature.Kind)
	}

	if feature.ApiVersion == "" {
		return nil, fmt.Errorf("apiVersion is required")
	}

	if feature.Metadata.Name == "" {
		return nil, fmt.Errorf("metadata.name is required")
	}

	return &feature, nil
}

// resolveComponentSources transforms component source names into fully qualified URLs
// with path prefix and reference information based on the associated source configuration.
// It processes both OCI and Git sources, constructing appropriate URL formats for each type.
// For OCI sources, it creates URLs in the format oci://registry/repo:tag//path/to/module.
// For Git sources, it uses the format url//path/to/module?ref=reference.
func (b *BaseBlueprintHandler) resolveComponentSources(blueprint *blueprintv1alpha1.Blueprint) {
	resolvedComponents := make([]blueprintv1alpha1.TerraformComponent, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		for _, source := range blueprint.Sources {
			if component.Source == source.Name {
				pathPrefix := source.PathPrefix
				if pathPrefix == "" {
					pathPrefix = "terraform"
				}

				ref := b.getSourceRef(source)

				if strings.HasPrefix(source.Url, "oci://") {
					baseURL := source.Url
					if ref != "" {
						ociPrefix := "oci://"
						urlWithoutProtocol := baseURL[len(ociPrefix):]
						if !strings.Contains(urlWithoutProtocol, ":") {
							baseURL = baseURL + ":" + ref
						}
					}
					resolvedComponents[i].Source = baseURL + "//" + pathPrefix + "/" + component.Path
				} else {
					resolvedComponents[i].Source = source.Url + "//" + pathPrefix + "/" + component.Path + "?ref=" + ref
				}
				break
			}
		}
	}

	blueprint.TerraformComponents = resolvedComponents
}

// resolveComponentPaths determines the full filesystem path for each Terraform component,
// using either the module cache location for remote sources or the project's terraform directory
// for local modules. It processes all components in the blueprint, checking source types to
// determine appropriate path resolution strategies and updating component paths accordingly.
func (b *BaseBlueprintHandler) resolveComponentPaths(blueprint *blueprintv1alpha1.Blueprint) {
	projectRoot := b.runtime.ProjectRoot

	resolvedComponents := make([]blueprintv1alpha1.TerraformComponent, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		componentCopy := component

		var dirName string
		if componentCopy.Name != "" {
			dirName = componentCopy.Name
		} else {
			dirName = componentCopy.Path
		}

		if componentCopy.Name != "" ||
			b.isValidTerraformRemoteSource(componentCopy.Source) ||
			b.isOCISource(componentCopy.Source) ||
			strings.HasPrefix(componentCopy.Source, "file://") {
			componentCopy.FullPath = filepath.Join(projectRoot, ".windsor", "contexts", b.runtime.ContextName, "terraform", dirName)
		} else {
			componentCopy.FullPath = filepath.Join(projectRoot, "terraform", dirName)
		}

		componentCopy.FullPath = filepath.FromSlash(componentCopy.FullPath)

		resolvedComponents[i] = componentCopy
	}

	blueprint.TerraformComponents = resolvedComponents
}

// processBlueprintData parses blueprint YAML data into a Blueprint object.
// Parses and validates required fields, converts kustomization maps to typed objects, and merges results into
// the target blueprint. If ociInfo is provided, injects the OCI source into the sources list, updates Terraform
// components and kustomizations lacking a source to use the OCI source, and ensures the OCI source is present
// or updated in the sources slice. If skipSources is true, sources from the blueprint data are not merged.
func (b *BaseBlueprintHandler) processBlueprintData(data []byte, blueprint *blueprintv1alpha1.Blueprint, ociInfo ...*artifact.OCIArtifactInfo) error {
	newBlueprint := &blueprintv1alpha1.Blueprint{}
	if err := b.shims.YamlUnmarshal(data, newBlueprint); err != nil {
		return fmt.Errorf("error unmarshalling blueprint data: %w", err)
	}

	kustomizations := newBlueprint.Kustomizations
	sources := newBlueprint.Sources
	terraformComponents := newBlueprint.TerraformComponents

	if len(ociInfo) > 0 && ociInfo[0] != nil {
		oci := ociInfo[0]
		ociSource := blueprintv1alpha1.Source{
			Name: oci.Name,
			Url:  oci.URL,
		}

		sourceExists := false
		for i, source := range sources {
			if source.Name == oci.Name {
				sources[i] = ociSource
				sourceExists = true
				break
			}
		}

		if !sourceExists {
			sources = append(sources, ociSource)
		}

		for i, component := range terraformComponents {
			if component.Source == "" {
				terraformComponents[i].Source = oci.Name
			}
		}

		for i, kustomization := range kustomizations {
			if kustomization.Source == "" {
				kustomizations[i].Source = oci.Name
			}
		}
	}

	completeBlueprint := &blueprintv1alpha1.Blueprint{
		Kind:                newBlueprint.Kind,
		ApiVersion:          newBlueprint.ApiVersion,
		Metadata:            newBlueprint.Metadata,
		Sources:             sources,
		TerraformComponents: terraformComponents,
		Kustomizations:      kustomizations,
		Repository:          newBlueprint.Repository,
	}

	if err := blueprint.StrategicMerge(completeBlueprint); err != nil {
		return fmt.Errorf("failed to strategic merge blueprint: %w", err)
	}

	return nil
}

// collectTemplateDataFromDirectory walks a template directory and collects all files.
// Returns a map with file paths as keys (prefixed with "_template/") and file contents as values.
// If the directory does not exist (os.IsNotExist), returns an empty map with no error.
// If Stat fails for any other reason (e.g., permission denied), returns the error.
func (b *BaseBlueprintHandler) collectTemplateDataFromDirectory(templateDir string) (map[string][]byte, error) {
	templateData := make(map[string][]byte)

	_, err := b.shims.Stat(templateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return templateData, nil
		}
		return nil, fmt.Errorf("failed to stat template directory: %w", err)
	}

	if err := b.walkAndCollectTemplates(templateDir, templateDir, templateData); err != nil {
		return nil, fmt.Errorf("failed to collect templates: %w", err)
	}

	return templateData, nil
}

// readTemplateDataFromCache reads template data from a cache directory that has already been extracted.
// It walks the _template directory and reads metadata.yaml, returning a map with file paths as keys
// and file contents as values. The metadata name is stored with the key "_metadata_name".
// Returns an error if the cache directory is invalid or files cannot be read.
func (b *BaseBlueprintHandler) readTemplateDataFromCache(cacheDir string) (map[string][]byte, error) {
	templateDir := filepath.Join(cacheDir, "_template")
	templateData, err := b.collectTemplateDataFromDirectory(templateDir)
	if err != nil {
		return nil, err
	}

	metadataPath := filepath.Join(cacheDir, "metadata.yaml")
	if _, err := b.shims.Stat(metadataPath); err == nil {
		metadataContent, err := b.shims.ReadFile(metadataPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read metadata.yaml: %w", err)
		}
		var metadata artifact.BlueprintMetadata
		if err := b.shims.YamlUnmarshal(metadataContent, &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
		}
		if err := artifact.ValidateCliVersion(constants.Version, metadata.CliVersion); err != nil {
			return nil, err
		}

		if metadata.Name != "" {
			templateData["_metadata_name"] = []byte(metadata.Name)
		}
		if metadata.Version != "" {
			templateData["_metadata_version"] = []byte(metadata.Version)
		}
		if metadata.Description != "" {
			templateData["_metadata_description"] = []byte(metadata.Description)
		}
		if metadata.Author != "" {
			templateData["_metadata_author"] = []byte(metadata.Author)
		}
	} else {
		return nil, fmt.Errorf("OCI artifact missing required metadata.yaml file")
	}

	return templateData, nil
}

// isValidTerraformRemoteSource checks if the source is a valid Terraform module reference.
// It uses regular expressions to match the source string against known patterns for remote Terraform modules.
func (b *BaseBlueprintHandler) isValidTerraformRemoteSource(source string) bool {
	patterns := []string{
		`^git::https://[^/]+/.*\.git(?:@.*)?$`,
		`^git@[^:]+:.*\.git(?:@.*)?$`,
		`^https?://[^/]+/.*\.git(?:@.*)?$`,
		`^https?://[^/]+/.*\.zip(?:@.*)?$`,
		`^https?://[^/]+/.*//.*(?:@.*)?$`,
		`^registry\.terraform\.io/.*`,
		`^[^/]+\.com/.*`,
	}

	for _, pattern := range patterns {
		matched, err := b.shims.RegexpMatchString(pattern, source)
		if err != nil {
			return false
		}
		if matched {
			return true
		}
	}

	return false
}

// resolvePatchFromPath yields patch content as YAML string and the target selector for a given patch path.
// Combines template data with user-defined files; user files take precedence. If a user file exists and cannot be merged as YAML, it overrides template data entirely.
// patchPath: relative path to the patch file within the kustomize directory
// defaultNamespace: namespace to use if not specified in patch metadata
// Output: patch content (YAML), extracted target selector or nil if not found
func (b *BaseBlueprintHandler) resolvePatchFromPath(patchPath, defaultNamespace string) (string, *kustomize.Selector) {
	patchKey := "kustomize/patches/" + strings.TrimPrefix(patchPath, "kustomize/patches/")
	if strings.HasSuffix(patchKey, ".yaml") || strings.HasSuffix(patchKey, ".yml") {
		patchKey = strings.TrimSuffix(patchKey, filepath.Ext(patchKey))
	}

	var basePatchData map[string]any
	var target *kustomize.Selector

	if renderedPatch, exists := b.kustomizeData[patchKey]; exists {
		if patchMap, ok := renderedPatch.(map[string]any); ok {
			basePatchData = make(map[string]any)
			for k, v := range patchMap {
				basePatchData[k] = v
			}
		}
	}

	configRoot := b.runtime.ConfigRoot
	if configRoot != "" {
		patchFilePath := filepath.Join(configRoot, "kustomize", patchPath)
		if data, err := b.shims.ReadFile(patchFilePath); err == nil {
			if basePatchData == nil {
				target = b.extractTargetFromPatchContent(string(data), defaultNamespace)
				return string(data), target
			}

			var userPatchData map[string]any
			if err := b.shims.YamlUnmarshal(data, &userPatchData); err == nil {
				basePatchData = b.deepMergeMaps(basePatchData, userPatchData)
			} else {
				target = b.extractTargetFromPatchContent(string(data), defaultNamespace)
				return string(data), target
			}
		}
	}

	if basePatchData == nil {
		return "", nil
	}

	patchYAML, err := b.shims.YamlMarshal(basePatchData)
	if err != nil {
		return "", nil
	}

	target = b.extractTargetFromPatchData(basePatchData, defaultNamespace)
	return string(patchYAML), target
}

// extractTargetFromPatchData extracts target selector information from patch data map.
// Returns nil if the required metadata fields are not found or invalid.
func (b *BaseBlueprintHandler) extractTargetFromPatchData(patchData map[string]any, defaultNamespace string) *kustomize.Selector {
	kind, ok := patchData["kind"].(string)
	if !ok {
		return nil
	}

	metadata, ok := patchData["metadata"].(map[string]any)
	if !ok {
		return nil
	}

	name, ok := metadata["name"].(string)
	if !ok {
		return nil
	}

	namespace := defaultNamespace
	if ns, ok := metadata["namespace"].(string); ok {
		namespace = ns
	}

	return &kustomize.Selector{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}
}

// extractTargetFromPatchContent extracts target selector information from patch YAML content.
// Parses the YAML and returns the first valid target found, or nil if none found.
func (b *BaseBlueprintHandler) extractTargetFromPatchContent(patchContent, defaultNamespace string) *kustomize.Selector {
	decoder := yaml.NewDecoder(strings.NewReader(patchContent))
	for {
		var patchData map[string]any
		if err := decoder.Decode(&patchData); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		if target := b.extractTargetFromPatchData(patchData, defaultNamespace); target != nil {
			return target
		}
	}
	return nil
}

// isOCISource returns true if the provided sourceNameOrURL is an OCI repository reference.
// It checks if the input is a resolved OCI URL, matches the blueprint's main repository with an OCI URL,
// or matches any additional source with an OCI URL.
func (b *BaseBlueprintHandler) isOCISource(sourceNameOrURL string) bool {
	if strings.HasPrefix(sourceNameOrURL, "oci://") {
		return true
	}
	if sourceNameOrURL == b.blueprint.Metadata.Name && strings.HasPrefix(b.blueprint.Repository.Url, "oci://") {
		return true
	}
	for _, source := range b.blueprint.Sources {
		if source.Name == sourceNameOrURL && strings.HasPrefix(source.Url, "oci://") {
			return true
		}
	}
	return false
}

// categorizePatches categorizes patches into strategic merge patches to write and inline patches to keep in-memory.
// Returns strategic merge patches to write and inline patches (JSON 6902 or OCI patches).
func (b *BaseBlueprintHandler) categorizePatches(kustomization blueprintv1alpha1.Kustomization) ([]blueprintv1alpha1.BlueprintPatch, []blueprintv1alpha1.BlueprintPatch) {
	strategicMergePatchesToWrite := make([]blueprintv1alpha1.BlueprintPatch, 0)
	inlinePatches := make([]blueprintv1alpha1.BlueprintPatch, 0)

	for _, patch := range kustomization.Patches {
		isLocalTemplatePatch := false
		if patch.Path != "" && !strings.HasPrefix(patch.Path, "patches/") {
			patchFilePath := filepath.Join(b.runtime.TemplateRoot, patch.Path)
			if _, err := b.shims.Stat(patchFilePath); err == nil {
				isLocalTemplatePatch = true
			}
		}

		if patch.Target != nil {
			if isLocalTemplatePatch && patch.Patch == "" && patch.Path != "" {
				patchFilePath := filepath.Join(b.runtime.TemplateRoot, patch.Path)
				data, err := b.shims.ReadFile(patchFilePath)
				if err == nil {
					patch.Patch = string(data)
					patch.Path = ""
				}
			}
			inlinePatches = append(inlinePatches, patch)
		} else if patch.Patch != "" {
			strategicMergePatchesToWrite = append(strategicMergePatchesToWrite, patch)
		} else if isLocalTemplatePatch {
			strategicMergePatchesToWrite = append(strategicMergePatchesToWrite, patch)
		} else {
			inlinePatches = append(inlinePatches, patch)
		}
	}

	return strategicMergePatchesToWrite, inlinePatches
}

// mergeLegacySpecialVariables merges legacy special variables into the common values map for backward compatibility.
// These variables were previously extracted from config and kustomize/values.yaml and are now merged from the config handler.
// This method can be removed when legacy variable support is no longer needed.
func (b *BaseBlueprintHandler) mergeLegacySpecialVariables(mergedCommonValues map[string]string) {
	if b.runtime == nil || b.runtime.ConfigHandler == nil {
		return
	}

	domain := b.runtime.ConfigHandler.GetString("dns.domain")
	context := b.runtime.ConfigHandler.GetContext()
	contextID := b.runtime.ConfigHandler.GetString("id")
	lbStart := b.runtime.ConfigHandler.GetString("network.loadbalancer_ips.start")
	lbEnd := b.runtime.ConfigHandler.GetString("network.loadbalancer_ips.end")
	registryURL := b.runtime.ConfigHandler.GetString("docker.registry_url")
	localVolumePaths := b.runtime.ConfigHandler.GetStringSlice("cluster.workers.volumes")

	loadBalancerIPRange := fmt.Sprintf("%s-%s", lbStart, lbEnd)

	var localVolumePath string
	if len(localVolumePaths) > 0 {
		parts := strings.Split(localVolumePaths[0], ":")
		if len(parts) > 1 {
			localVolumePath = parts[1]
		}
	}

	if domain != "" {
		mergedCommonValues["DOMAIN"] = domain
	}
	if context != "" {
		mergedCommonValues["CONTEXT"] = context
	}
	if contextID != "" {
		mergedCommonValues["CONTEXT_ID"] = contextID
	}
	if loadBalancerIPRange != "-" {
		mergedCommonValues["LOADBALANCER_IP_RANGE"] = loadBalancerIPRange
	}
	if lbStart != "" {
		mergedCommonValues["LOADBALANCER_IP_START"] = lbStart
	}
	if lbEnd != "" {
		mergedCommonValues["LOADBALANCER_IP_END"] = lbEnd
	}
	if registryURL != "" {
		mergedCommonValues["REGISTRY_URL"] = registryURL
	}
	if localVolumePath != "" {
		mergedCommonValues["LOCAL_VOLUME_PATH"] = localVolumePath
	}

	buildID, err := b.runtime.GetBuildID()
	if err == nil && buildID != "" {
		mergedCommonValues["BUILD_ID"] = buildID
	}
}

// validateValuesForSubstitution validates that the given values map contains only types supported for Flux post-build variable substitution.
// Permitted types are string, numeric, and boolean scalars. A single level of map[string]any is allowed if all nested values are scalar.
// Slices and nested complex types are not allowed. Returns an error describing the first unsupported value encountered,
// or nil if all values are supported.
func (b *BaseBlueprintHandler) validateValuesForSubstitution(values map[string]any) error {
	var validate func(map[string]any, string, int) error
	validate = func(values map[string]any, parentKey string, depth int) error {
		for key, value := range values {
			currentKey := key
			if parentKey != "" {
				currentKey = parentKey + "." + key
			}
			if value == nil {
				return fmt.Errorf("values for post-build substitution cannot contain nil values, key '%s'", currentKey)
			}
			if reflect.TypeOf(value).Kind() == reflect.Slice {
				return fmt.Errorf("values for post-build substitution cannot contain slices, key '%s' has type %T", currentKey, value)
			}
			switch v := value.(type) {
			case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
				continue
			case map[string]any:
				if depth >= 1 {
					return fmt.Errorf("values for post-build substitution cannot contain nested maps, key '%s' has type %T", currentKey, v)
				}
				for nestedKey, nestedValue := range v {
					nestedCurrentKey := currentKey + "." + nestedKey
					switch nestedValue.(type) {
					case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
						continue
					case nil:
						return fmt.Errorf("values for post-build substitution cannot contain nil values, key '%s'", nestedCurrentKey)
					default:
						if reflect.TypeOf(nestedValue).Kind() == reflect.Slice {
							return fmt.Errorf("values for post-build substitution cannot contain slices, key '%s' has type %T", nestedCurrentKey, nestedValue)
						}
						return fmt.Errorf("values for post-build substitution can only contain scalar values in maps, key '%s' has unsupported type %T", nestedCurrentKey, nestedValue)
					}
				}
			default:
				return fmt.Errorf("values for post-build substitution can only contain strings, numbers, booleans, or maps of scalar types, key '%s' has unsupported type %T", currentKey, v)
			}
		}
		return nil
	}
	return validate(values, "", 0)
}

// deepMergeMaps returns a new map from a deep merge of base and overlay maps.
// Overlay values take precedence; nested maps merge recursively. Non-map overlay values replace base values.
func (b *BaseBlueprintHandler) deepMergeMaps(base, overlay map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range base {
		result[k] = v
	}
	for k, overlayValue := range overlay {
		if baseValue, exists := result[k]; exists {
			if baseMap, baseIsMap := baseValue.(map[string]any); baseIsMap {
				if overlayMap, overlayIsMap := overlayValue.(map[string]any); overlayIsMap {
					result[k] = b.deepMergeMaps(baseMap, overlayMap)
					continue
				}
			}
		}
		result[k] = overlayValue
	}
	return result
}

// setRepositoryDefaults sets or overrides the blueprint repository URL based on development mode and git configuration.
// If development mode is enabled, the development URL is always used. Otherwise, the git remote origin URL is used if the URL is unset.
// If a URL is set and the repository reference is empty, the branch is set to "main".
// In dev mode, the secretName is set to "flux-system" if not already set.
func (b *BaseBlueprintHandler) setRepositoryDefaults() error {
	devMode := b.runtime.ConfigHandler.GetBool("dev")

	if devMode {
		url := b.getDevelopmentRepositoryURL()
		if url != "" {
			b.blueprint.Repository.Url = url
		}
	}
	if b.blueprint.Repository.Url == "" {
		gitURL, err := b.runtime.Shell.ExecSilent("git", "config", "--get", "remote.origin.url")
		if err == nil && gitURL != "" {
			b.blueprint.Repository.Url = b.normalizeGitURL(strings.TrimSpace(gitURL))
		}
	}
	if b.blueprint.Repository.Url != "" && b.blueprint.Repository.Ref == (blueprintv1alpha1.Reference{}) {
		b.blueprint.Repository.Ref = blueprintv1alpha1.Reference{Branch: "main"}
	}
	if devMode && b.blueprint.Repository.Url != "" && b.blueprint.Repository.SecretName == nil {
		secretName := "flux-system"
		b.blueprint.Repository.SecretName = &secretName
	}
	return nil
}

// evaluateSubstitutions evaluates expressions in substitution values and converts all results to strings.
// Values can use ${} syntax for expressions (e.g., "${dns.domain}") or be literals.
// All evaluated values are converted to strings as required by Flux postBuild substitution.
func (b *BaseBlueprintHandler) evaluateSubstitutions(substitutions map[string]string, config map[string]any, featurePath string) (map[string]string, error) {
	result := make(map[string]string)
	evaluator := b.featureEvaluator

	for key, value := range substitutions {
		if strings.Contains(value, "${") {
			anyMap := map[string]any{key: value}
			evaluated, err := evaluator.EvaluateDefaults(anyMap, config, featurePath)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate substitution for key '%s': %w", key, err)
			}

			evaluatedValue := evaluated[key]
			if evaluatedValue == nil {
				result[key] = ""
			} else {
				result[key] = fmt.Sprintf("%v", evaluatedValue)
			}
		} else {
			result[key] = value
		}
	}

	return result, nil
}

// normalizeGitURL normalizes git repository URLs by prepending https:// when needed.
// Preserves SSH URLs (git@...), http://, and https:// URLs as-is.
func (b *BaseBlueprintHandler) normalizeGitURL(url string) string {
	if strings.HasPrefix(url, "git@") ||
		strings.HasPrefix(url, "http://") ||
		strings.HasPrefix(url, "https://") {
		return url
	}
	return "https://" + url
}

// getDevelopmentRepositoryURL generates a development repository URL from configuration.
// Returns URL in format: http://git.<domain>/git/<folder>
func (b *BaseBlueprintHandler) getDevelopmentRepositoryURL() string {
	domain := b.runtime.ConfigHandler.GetString("dns.domain", "test")
	if domain == "" {
		return ""
	}

	if b.runtime.ProjectRoot == "" {
		return ""
	}

	folder := b.shims.FilepathBase(b.runtime.ProjectRoot)
	if folder == "" || folder == "." {
		return ""
	}

	return fmt.Sprintf("http://git.%s/git/%s", domain, folder)
}

// componentKey generates a unique key for a terraform component based on its Path and Source.
func (b *BaseBlueprintHandler) componentKey(component blueprintv1alpha1.TerraformComponent, sourceName ...string) string {
	key := component.Path
	if component.Source != "" {
		key = component.Source + ":" + component.Path
	} else if len(sourceName) > 0 && sourceName[0] != "" {
		key = sourceName[0] + ":" + component.Path
	}
	return key
}

// componentExists checks if a terraform component exists in the blueprint.
func (b *BaseBlueprintHandler) componentExists(component blueprintv1alpha1.TerraformComponent, sourceName ...string) bool {
	componentKey := b.componentKey(component, sourceName...)
	for _, existing := range b.blueprint.TerraformComponents {
		existingKey := b.componentKey(existing, sourceName...)
		if existingKey == componentKey {
			return true
		}
	}
	return false
}
