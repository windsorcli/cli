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
	LoadBlueprint() error
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
// Returns an error if any required paths are inaccessible or any loading operation fails.
func (b *BaseBlueprintHandler) LoadBlueprint() error {
	if _, err := b.shims.Stat(b.runtime.TemplateRoot); err == nil {
		templateData, err := b.GetLocalTemplateData()
		if err != nil {
			return fmt.Errorf("failed to get local template data: %w", err)
		}
		if len(templateData) == 0 {
			configRoot := b.runtime.ConfigRoot
			if configRoot == "" {
				return fmt.Errorf("blueprint.yaml not found at %s", filepath.Join(configRoot, "blueprint.yaml"))
			}
			blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
			if _, err := b.shims.Stat(blueprintPath); err != nil {
				return fmt.Errorf("blueprint.yaml not found at %s", blueprintPath)
			}
		}
	} else {
		configRoot := b.runtime.ConfigRoot
		blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
		if _, err := b.shims.Stat(blueprintPath); err == nil {
			if err := b.loadConfig(); err != nil {
				return fmt.Errorf("failed to load blueprint config: %w", err)
			}
			return nil
		}
		effectiveBlueprintURL := constants.GetEffectiveBlueprintURL()
		ociInfo, err := artifact.ParseOCIReference(effectiveBlueprintURL)
		if err != nil {
			return fmt.Errorf("failed to parse default blueprint reference: %w", err)
		}
		if ociInfo == nil {
			return fmt.Errorf("invalid default blueprint reference: %s", effectiveBlueprintURL)
		}
		if b.artifactBuilder == nil {
			return fmt.Errorf("blueprint.yaml not found at %s and artifact builder not available", blueprintPath)
		}
		templateData, err := b.artifactBuilder.GetTemplateData(ociInfo.URL)
		if err != nil {
			return fmt.Errorf("blueprint.yaml not found at %s and failed to get template data from default blueprint: %w", blueprintPath, err)
		}
		blueprintData := make(map[string]any)
		for key, value := range templateData {
			blueprintData[key] = string(value)
		}
		if err := b.loadData(blueprintData, ociInfo); err != nil {
			return fmt.Errorf("failed to load default blueprint data: %w", err)
		}
	}

	sources := b.getSources()
	if len(sources) > 0 {
		if b.artifactBuilder != nil {
			var ociURLs []string
			for _, source := range sources {
				if strings.HasPrefix(source.Url, "oci://") {
					ociURLs = append(ociURLs, source.Url)
				}
			}
			if len(ociURLs) > 0 {
				_, err := b.artifactBuilder.Pull(ociURLs)
				if err != nil {
					return fmt.Errorf("failed to load OCI sources: %w", err)
				}
			}
		}
	}

	configRoot := b.runtime.ConfigRoot

	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	if _, err := b.shims.Stat(blueprintPath); err == nil {
		if err := b.loadConfig(); err != nil {
			return fmt.Errorf("failed to load blueprint config overrides: %w", err)
		}
	}

	return nil
}

// Write persists the current blueprint state to blueprint.yaml in the configuration root directory.
// If overwrite is true, the file is overwritten regardless of existence. If overwrite is false or omitted,
// the file is only written if it does not already exist. The method ensures the target directory exists,
// marshals the blueprint to YAML, and writes the file using the configured shims.
// Terraform inputs and kustomization substitutions are manually cleared to prevent them from appearing in the final blueprint.yaml.
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

	if !shouldOverwrite {
		if _, err := b.shims.Stat(yamlPath); err == nil {
			return nil
		}
	}

	if err := b.shims.MkdirAll(filepath.Dir(yamlPath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	if err := b.setRepositoryDefaults(); err != nil {
		return fmt.Errorf("error setting repository defaults: %w", err)
	}

	cleanedBlueprint := b.blueprint.DeepCopy()
	for i := range cleanedBlueprint.TerraformComponents {
		cleanedBlueprint.TerraformComponents[i].Inputs = map[string]any{}
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

// Generate returns the fully processed blueprint with all defaults resolved,
// paths processed, and generation logic applied - equivalent to what would be deployed.
// It applies the same processing logic as getKustomizations() but for the entire blueprint structure.
func (b *BaseBlueprintHandler) Generate() *blueprintv1alpha1.Blueprint {
	generated := b.blueprint.DeepCopy()

	// Process kustomizations with the same logic as getKustomizations()
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

	// Process terraform components with source resolution
	b.resolveComponentSources(generated)
	b.resolveComponentPaths(generated)

	return generated
}

// GetLocalTemplateData returns template files from contexts/_template, merging values.yaml from
// both _template and context dirs. All .jsonnet files are collected recursively with relative
// paths preserved. If OCI artifact values exist, they are merged with local values, with local
// values taking precedence. Returns nil if no templates exist. Keys are relative file paths,
// values are file contents.
func (b *BaseBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	if _, err := b.shims.Stat(b.runtime.TemplateRoot); os.IsNotExist(err) {
		return nil, nil
	}

	templateData := make(map[string][]byte)
	if err := b.walkAndCollectTemplates(b.runtime.TemplateRoot, templateData); err != nil {
		return nil, fmt.Errorf("failed to collect templates: %w", err)
	}

	if schemaData, exists := templateData["schema"]; exists {
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

	if err := b.processFeatures(templateData, config); err != nil {
		return nil, fmt.Errorf("failed to process features: %w", err)
	}

	if len(b.blueprint.TerraformComponents) > 0 || len(b.blueprint.Kustomizations) > 0 {
		contextName := b.runtime.ConfigHandler.GetContext()
		if contextName != "" {
			b.blueprint.Metadata.Name = contextName
			b.blueprint.Metadata.Description = fmt.Sprintf("Blueprint for %s context", contextName)
		}

		composedBlueprintYAML, err := b.shims.YamlMarshal(b.blueprint)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal composed blueprint: %w", err)
		}
		templateData["blueprint"] = composedBlueprintYAML
	}

	if contextValues != nil {
		if substitutionValues, ok := contextValues["substitutions"].(map[string]any); ok && len(substitutionValues) > 0 {
			if existingValues, exists := templateData["substitutions"]; exists {
				var ociSubstitutionValues map[string]any
				if err := b.shims.YamlUnmarshal(existingValues, &ociSubstitutionValues); err == nil {
					substitutionValues = b.deepMergeMaps(ociSubstitutionValues, substitutionValues)
				}
			}
			substitutionYAML, err := b.shims.YamlMarshal(substitutionValues)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal substitution values: %w", err)
			}
			templateData["substitutions"] = substitutionYAML
		}
	}

	return templateData, nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

	if err := b.processBlueprintData(yamlData, &b.blueprint); err != nil {
		return err
	}

	b.configLoaded = true
	return nil
}

// loadData loads blueprint configuration from a map containing blueprint data.
// It marshals the input map to YAML, processes it as a Blueprint object, and updates the handler's blueprint state.
// The ociInfo parameter optionally provides OCI artifact source information for source resolution and tracking.
// If config is already loaded from YAML, this is a no-op to preserve resolved state.
func (b *BaseBlueprintHandler) loadData(data map[string]any, ociInfo ...*artifact.OCIArtifactInfo) error {
	if b.configLoaded {
		return nil
	}

	yamlData, err := b.shims.YamlMarshal(data)
	if err != nil {
		return fmt.Errorf("error marshalling blueprint data to yaml: %w", err)
	}

	if err := b.processBlueprintData(yamlData, &b.blueprint, ociInfo...); err != nil {
		return err
	}

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

// walkAndCollectTemplates traverses template directories to gather .jsonnet files.
// It updates the provided templateData map with the relative paths and content of
// the .jsonnet files found. The function handles directory recursion and file reading
// errors, returning an error if any operation fails.
func (b *BaseBlueprintHandler) walkAndCollectTemplates(templateDir string, templateData map[string][]byte) error {
	entries, err := b.shims.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(templateDir, entry.Name())

		if entry.IsDir() {
			if err := b.walkAndCollectTemplates(entryPath, templateData); err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".jsonnet") ||
			entry.Name() == "schema.yaml" ||
			entry.Name() == "blueprint.yaml" ||
			(strings.HasPrefix(filepath.Dir(entryPath), filepath.Join(b.runtime.TemplateRoot, "features")) && strings.HasSuffix(entry.Name(), ".yaml")) {
			content, err := b.shims.ReadFile(filepath.Clean(entryPath))
			if err != nil {
				return fmt.Errorf("failed to read template file %s: %w", entryPath, err)
			}

			if entry.Name() == "schema.yaml" {
				templateData["schema"] = content
			} else if entry.Name() == "blueprint.yaml" {
				templateData["blueprint"] = content
			} else {
				relPath, err := filepath.Rel(b.runtime.TemplateRoot, entryPath)
				if err != nil {
					return fmt.Errorf("failed to calculate relative path for %s: %w", entryPath, err)
				}

				relPath = strings.ReplaceAll(relPath, "\\", "/")
				templateData[relPath] = content
			}
		}
	}

	return nil
}

// processFeatures loads the base blueprint and merges features that match evaluated conditions.
// It loads the base blueprint.yaml from templateData, loads features, evaluates their When expressions
// against the provided config, and merges matching features into the base blueprint. Features and their
// components are merged in deterministic order by feature name.
func (b *BaseBlueprintHandler) processFeatures(templateData map[string][]byte, config map[string]any) error {
	if blueprintData, exists := templateData["blueprint"]; exists {
		if err := b.processBlueprintData(blueprintData, &b.blueprint); err != nil {
			return fmt.Errorf("failed to load base blueprint.yaml: %w", err)
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
					if component.Inputs == nil {
						component.Inputs = make(map[string]any)
					}

					component.Inputs = b.deepMergeMaps(component.Inputs, filteredInputs)
				}
			}

			tempBlueprint := &blueprintv1alpha1.Blueprint{
				TerraformComponents: []blueprintv1alpha1.TerraformComponent{component},
			}
			if err := b.blueprint.StrategicMerge(tempBlueprint); err != nil {
				return fmt.Errorf("failed to merge terraform component: %w", err)
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

			if len(kustomization.Substitutions) > 0 {
				if b.featureSubstitutions[kustomizationCopy.Name] == nil {
					b.featureSubstitutions[kustomizationCopy.Name] = make(map[string]string)
				}

				evaluatedSubstitutions, err := b.evaluateSubstitutions(kustomization.Substitutions, config, feature.Path)
				if err != nil {
					return fmt.Errorf("failed to evaluate substitutions for kustomization '%s': %w", kustomizationCopy.Name, err)
				}

				maps.Copy(b.featureSubstitutions[kustomizationCopy.Name], evaluatedSubstitutions)
			}

			// Clear substitutions as they are used for ConfigMap generation and should not appear in the final blueprint
			kustomizationCopy.Substitutions = nil

			tempBlueprint := &blueprintv1alpha1.Blueprint{
				Kustomizations: []blueprintv1alpha1.Kustomization{kustomizationCopy},
			}
			if err := b.blueprint.StrategicMerge(tempBlueprint); err != nil {
				return fmt.Errorf("failed to merge kustomization: %w", err)
			}
		}
	}

	return nil
}

// loadFeatures extracts and parses feature files from template data.
// It looks for files with paths starting with "features/" and ending with ".yaml",
// parses them as Feature objects, and returns a slice of all valid features.
// Returns an error if any feature file cannot be parsed.
func (b *BaseBlueprintHandler) loadFeatures(templateData map[string][]byte) ([]blueprintv1alpha1.Feature, error) {
	var features []blueprintv1alpha1.Feature

	for path, content := range templateData {
		if strings.HasPrefix(path, "features/") && strings.HasSuffix(path, ".yaml") {
			feature, err := b.parseFeature(content)
			if err != nil {
				return nil, fmt.Errorf("failed to parse feature %s: %w", path, err)
			}
			feature.Path = filepath.Join(b.runtime.TemplateRoot, path)
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

				if strings.HasPrefix(source.Url, "oci://") {
					baseURL := source.Url
					if ref != "" && !strings.Contains(baseURL, ":") {
						baseURL = baseURL + ":" + ref
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

		if b.isValidTerraformRemoteSource(componentCopy.Source) || b.isOCISource(componentCopy.Source) {
			componentCopy.FullPath = filepath.Join(projectRoot, ".windsor", ".tf_modules", componentCopy.Path)
		} else {
			componentCopy.FullPath = filepath.Join(projectRoot, "terraform", componentCopy.Path)
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
// or updated in the sources slice.
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

// validateValuesForSubstitution checks that all values are valid for Flux post-build variable substitution.
// Permitted types are string, numeric, and boolean. Allows one level of map nesting if all nested values are scalar.
// Slices and nested complex types are not allowed. Returns an error if any value is not a supported type.
func (b *BaseBlueprintHandler) validateValuesForSubstitution(values map[string]any) error {
	var validate func(map[string]any, string, int) error
	validate = func(values map[string]any, parentKey string, depth int) error {
		for key, value := range values {
			currentKey := key
			if parentKey != "" {
				currentKey = parentKey + "." + key
			}

			// Handle nil values first to avoid panic in reflect.TypeOf
			if value == nil {
				return fmt.Errorf("values for post-build substitution cannot contain nil values, key '%s'", currentKey)
			}

			// Check if the value is a slice using reflection
			if reflect.TypeOf(value).Kind() == reflect.Slice {
				return fmt.Errorf("values for post-build substitution cannot contain slices, key '%s' has type %T", currentKey, value)
			}

			switch v := value.(type) {
			case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
				continue
			case map[string]any:
				// Post-build substitution should only allow flat key/value maps, no nesting at all
				if depth >= 1 {
					return fmt.Errorf("values for post-build substitution cannot contain nested maps, key '%s' has type %T", currentKey, v)
				}
				// Validate that the nested map only contains scalar values (no further nesting)
				for nestedKey, nestedValue := range v {
					nestedCurrentKey := currentKey + "." + nestedKey
					switch nestedValue.(type) {
					case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
						continue
					case nil:
						return fmt.Errorf("values for post-build substitution cannot contain nil values, key '%s'", nestedCurrentKey)
					default:
						// Check if it's a slice
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

// setRepositoryDefaults sets the blueprint repository URL if not already specified.
// Uses development URL if dev flag is enabled, otherwise falls back to git remote origin URL.
// In dev mode, always overrides the URL even if it's already set.
func (b *BaseBlueprintHandler) setRepositoryDefaults() error {
	devMode := b.runtime.ConfigHandler.GetBool("dev")

	if devMode {
		url := b.getDevelopmentRepositoryURL()
		if url != "" {
			b.blueprint.Repository.Url = url
			return nil
		}
	}

	// Only set from git remote if URL is not already set
	if b.blueprint.Repository.Url != "" {
		return nil
	}

	gitURL, err := b.runtime.Shell.ExecSilent("git", "config", "--get", "remote.origin.url")
	if err == nil && gitURL != "" {
		b.blueprint.Repository.Url = b.normalizeGitURL(strings.TrimSpace(gitURL))
		return nil
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
