package blueprint

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	_ "embed"

	"github.com/goccy/go-yaml"
	contextpkg "github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/context/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/context/shell"

	"github.com/briandowns/spinner"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	kustomize "github.com/fluxcd/pkg/apis/kustomize"
	meta "github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The BlueprintHandler is a core component that manages infrastructure and application configurations
// through a declarative, GitOps-based approach. It handles the lifecycle of infrastructure blueprints,
// which are composed of Terraform components, Kubernetes Kustomizations, and associated metadata.
// The handler facilitates the resolution of component sources, manages repository configurations,
// and orchestrates the deployment of infrastructure components across different environments.
// It integrates with Kubernetes for resource management and supports both local and remote
// infrastructure definitions, enabling consistent and reproducible infrastructure deployments.

type BlueprintHandler interface {
	Initialize() error
	LoadBlueprint() error
	LoadConfig() error
	LoadData(data map[string]any, ociInfo ...*artifact.OCIArtifactInfo) error
	Write(overwrite ...bool) error
	Install() error
	SetRenderedKustomizeData(data map[string]any)
	GetMetadata() blueprintv1alpha1.Metadata
	GetSources() []blueprintv1alpha1.Source
	GetRepository() blueprintv1alpha1.Repository
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	GetKustomizations() []blueprintv1alpha1.Kustomization
	WaitForKustomizations(message string, names ...string) error
	GetDefaultTemplateData(contextName string) (map[string][]byte, error)
	GetLocalTemplateData() (map[string][]byte, error)
	Generate() *blueprintv1alpha1.Blueprint
	Down() error
}

//go:embed templates/default.jsonnet
var defaultJsonnetTemplate string

type BaseBlueprintHandler struct {
	BlueprintHandler
	injector             di.Injector
	configHandler        config.ConfigHandler
	shell                shell.Shell
	kubernetesManager    kubernetes.KubernetesManager
	blueprint            blueprintv1alpha1.Blueprint
	projectRoot          string
	templateRoot         string
	featureEvaluator     *FeatureEvaluator
	shims                *Shims
	kustomizeData        map[string]any
	featureSubstitutions map[string]map[string]string
	configLoaded         bool
}

// NewBlueprintHandler creates a new instance of BaseBlueprintHandler.
// It initializes the handler with the provided dependency injector.
func NewBlueprintHandler(injector di.Injector) *BaseBlueprintHandler {
	return &BaseBlueprintHandler{
		injector:             injector,
		featureEvaluator:     NewFeatureEvaluator(injector),
		shims:                NewShims(),
		kustomizeData:        make(map[string]any),
		featureSubstitutions: make(map[string]map[string]string),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize resolves and assigns dependencies for BaseBlueprintHandler using the provided dependency injector.
// It sets configHandler, shell, and kubernetesManager, determines the project root directory.
// Returns an error if any dependency resolution or initialization step fails.
func (b *BaseBlueprintHandler) Initialize() error {
	configHandler, ok := b.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	b.configHandler = configHandler

	shell, ok := b.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	b.shell = shell

	kubernetesManager, ok := b.injector.Resolve("kubernetesManager").(kubernetes.KubernetesManager)
	if !ok {
		return fmt.Errorf("error resolving kubernetesManager")
	}
	b.kubernetesManager = kubernetesManager

	projectRoot, err := b.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error getting project root: %w", err)
	}
	b.projectRoot = projectRoot
	b.templateRoot = filepath.Join(projectRoot, "contexts", "_template")

	if err := b.featureEvaluator.Initialize(); err != nil {
		return fmt.Errorf("error initializing feature evaluator: %w", err)
	}

	return nil
}

// LoadBlueprint loads all blueprint data into memory, establishing defaults from either templates
// or OCI artifacts, then applies any local blueprint.yaml overrides to ensure the correct precedence.
// All sources are processed and merged into the in-memory runtime state.
// Returns an error if any required paths are inaccessible or any loading operation fails.
func (b *BaseBlueprintHandler) LoadBlueprint() error {
	if _, err := b.shims.Stat(b.templateRoot); err == nil {
		if _, err := b.GetLocalTemplateData(); err != nil {
			return fmt.Errorf("failed to get local template data: %w", err)
		}
	} else {
		effectiveBlueprintURL := constants.GetEffectiveBlueprintURL()
		ociInfo, err := artifact.ParseOCIReference(effectiveBlueprintURL)
		if err != nil {
			return fmt.Errorf("failed to parse default blueprint reference: %w", err)
		}
		if ociInfo == nil {
			return fmt.Errorf("invalid default blueprint reference: %s", effectiveBlueprintURL)
		}
		artifactBuilder := b.injector.Resolve("artifactBuilder")
		if artifactBuilder == nil {
			return fmt.Errorf("artifact builder not available")
		}
		ab, ok := artifactBuilder.(artifact.Artifact)
		if !ok {
			return fmt.Errorf("artifact builder has wrong type")
		}
		templateData, err := ab.GetTemplateData(ociInfo.URL)
		if err != nil {
			return fmt.Errorf("failed to get template data from default blueprint: %w", err)
		}
		blueprintData := make(map[string]any)
		for key, value := range templateData {
			blueprintData[key] = string(value)
		}
		if err := b.LoadData(blueprintData, ociInfo); err != nil {
			return fmt.Errorf("failed to load default blueprint data: %w", err)
		}
	}

	sources := b.GetSources()
	if len(sources) > 0 {
		artifactBuilder := b.injector.Resolve("artifactBuilder")
		if artifactBuilder != nil {
			if ab, ok := artifactBuilder.(artifact.Artifact); ok {
				var ociURLs []string
				for _, source := range sources {
					if strings.HasPrefix(source.Url, "oci://") {
						ociURLs = append(ociURLs, source.Url)
					}
				}
				if len(ociURLs) > 0 {
					_, err := ab.Pull(ociURLs)
					if err != nil {
						return fmt.Errorf("failed to load OCI sources: %w", err)
					}
				}
			}
		}
	}

	configRoot, err := b.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	if _, err := b.shims.Stat(blueprintPath); err == nil {
		if err := b.LoadConfig(); err != nil {
			return fmt.Errorf("failed to load blueprint config overrides: %w", err)
		}
	}

	return nil
}

// LoadConfig reads blueprint configuration from blueprint.yaml file.
// Returns an error if blueprint.yaml does not exist.
// Template processing is now handled by the pkg/template package.
func (b *BaseBlueprintHandler) LoadConfig() error {
	configRoot, err := b.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
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

// LoadData loads blueprint configuration from a map containing blueprint data.
// It marshals the input map to YAML, processes it as a Blueprint object, and updates the handler's blueprint state.
// The ociInfo parameter optionally provides OCI artifact source information for source resolution and tracking.
// If config is already loaded from YAML, this is a no-op to preserve resolved state.
func (b *BaseBlueprintHandler) LoadData(data map[string]any, ociInfo ...*artifact.OCIArtifactInfo) error {
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

	configRoot, err := b.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
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

// WaitForKustomizations waits for the specified kustomizations to be ready.
// It polls the status of the kustomizations until they are all ready or a timeout occurs.
func (b *BaseBlueprintHandler) WaitForKustomizations(message string, names ...string) error {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	timeout := b.shims.TimeAfter(b.calculateMaxWaitTime())
	ticker := b.shims.NewTicker(constants.DefaultKustomizationWaitPollInterval)
	defer b.shims.TickerStop(ticker)

	var kustomizationNames []string
	if len(names) > 0 && len(names[0]) > 0 {
		kustomizationNames = names
	} else {
		kustomizations := b.GetKustomizations()
		kustomizationNames = make([]string, len(kustomizations))
		for i, k := range kustomizations {
			kustomizationNames[i] = k.Name
		}
	}

	// Check immediately before starting polling loop
	ready, err := b.checkKustomizationStatus(kustomizationNames)
	if err == nil && ready {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m%s - \033[32mDone\033[0m\n", spin.Suffix)
		return nil
	}

	consecutiveFailures := 0
	if err != nil {
		consecutiveFailures = 1
	}

	for {
		select {
		case <-timeout:
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("timeout waiting for kustomizations")
		case <-ticker.C:
			ready, err := b.checkKustomizationStatus(kustomizationNames)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures >= constants.DefaultKustomizationWaitMaxFailures {
					spin.Stop()
					fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
					return fmt.Errorf("%s after %d consecutive failures", err.Error(), consecutiveFailures)
				}
				continue
			}

			if ready {
				spin.Stop()
				fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m%s - \033[32mDone\033[0m\n", spin.Suffix)
				return nil
			}

			// Reset consecutive failures on successful check
			consecutiveFailures = 0
		}
	}
}

// Install applies all blueprint Kubernetes resources to the cluster, including the main
// repository, additional sources, Kustomizations, and the context ConfigMap. The method
// ensures the target namespace exists, applies the main and additional source repositories,
// creates the ConfigMap, and applies all Kustomizations. Uses the environment KUBECONFIG or
// in-cluster configuration for access. Returns an error if any resource application fails.
func (b *BaseBlueprintHandler) Install() error {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " 📐 Installing blueprint resources"
	spin.Start()
	defer spin.Stop()

	if err := b.kubernetesManager.CreateNamespace(constants.DefaultFluxSystemNamespace); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	if b.blueprint.Repository.Url != "" {
		source := blueprintv1alpha1.Source{
			Name:       b.blueprint.Metadata.Name,
			Url:        b.blueprint.Repository.Url,
			Ref:        b.blueprint.Repository.Ref,
			SecretName: b.blueprint.Repository.SecretName,
		}
		if err := b.applySourceRepository(source, constants.DefaultFluxSystemNamespace); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply blueprint repository: %w", err)
		}
	}

	for _, source := range b.blueprint.Sources {
		if err := b.applySourceRepository(source, constants.DefaultFluxSystemNamespace); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply source %s: %w", source.Name, err)
		}
	}

	if err := b.applyValuesConfigMaps(); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
		return fmt.Errorf("failed to apply values configmaps: %w", err)
	}

	kustomizations := b.GetKustomizations()
	kustomizationNames := make([]string, len(kustomizations))
	for i, k := range kustomizations {
		if err := b.kubernetesManager.ApplyKustomization(b.toFluxKustomization(k, constants.DefaultFluxSystemNamespace)); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply kustomization %s: %w", k.Name, err)
		}
		kustomizationNames[i] = k.Name
	}

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m%s - \033[32mDone\033[0m\n", spin.Suffix)

	return nil
}

// GetMetadata retrieves the current blueprint's metadata.
func (b *BaseBlueprintHandler) GetMetadata() blueprintv1alpha1.Metadata {
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Metadata
}

// GetRepository retrieves the current blueprint's repository configuration, ensuring
// default values are set for empty fields.
func (b *BaseBlueprintHandler) GetRepository() blueprintv1alpha1.Repository {
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

// GetSources retrieves the current blueprint's source configurations.
func (b *BaseBlueprintHandler) GetSources() []blueprintv1alpha1.Source {
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Sources
}

// GetTerraformComponents retrieves the blueprint's Terraform components after resolving
// their sources and paths to full URLs and filesystem paths respectively.
func (b *BaseBlueprintHandler) GetTerraformComponents() []blueprintv1alpha1.TerraformComponent {
	resolvedBlueprint := b.blueprint

	b.resolveComponentSources(&resolvedBlueprint)
	b.resolveComponentPaths(&resolvedBlueprint)

	return resolvedBlueprint.TerraformComponents
}

// GetKustomizations returns the current blueprint's kustomization configurations with all default values resolved.
// It copies the kustomizations from the blueprint, sets default values for Source, Path, Interval, RetryInterval,
// Timeout, Wait, Force, and Destroy fields if unset, discovers and appends patches, and sets the PostBuild configuration.
// This method ensures all kustomization fields are fully populated for downstream processing.
func (b *BaseBlueprintHandler) GetKustomizations() []blueprintv1alpha1.Kustomization {
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

// Generate returns the fully processed blueprint with all defaults resolved,
// paths processed, and generation logic applied - equivalent to what would be deployed.
// It applies the same processing logic as GetKustomizations() but for the entire blueprint structure.
func (b *BaseBlueprintHandler) Generate() *blueprintv1alpha1.Blueprint {
	generated := b.blueprint.DeepCopy()

	// Process kustomizations with the same logic as GetKustomizations()
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

// SetRenderedKustomizeData stores rendered kustomize data for use during install.
// This includes values and patches from template processing that should be composed with user-defined files.
func (b *BaseBlueprintHandler) SetRenderedKustomizeData(data map[string]any) {
	b.kustomizeData = data
}

// GetDefaultTemplateData generates default template data based on the provider configuration.
// It uses the embedded default template to create a map of template files that can be
// used by the init pipeline for generating context-specific configurations.
func (b *BaseBlueprintHandler) GetDefaultTemplateData(contextName string) (map[string][]byte, error) {
	return map[string][]byte{
		"blueprint.jsonnet": []byte(defaultJsonnetTemplate),
	}, nil
}

// GetLocalTemplateData returns template files from contexts/_template, merging values.yaml from
// both _template and context dirs. All .jsonnet files are collected recursively with relative
// paths preserved. If OCI artifact values exist, they are merged with local values, with local
// values taking precedence. Returns nil if no templates exist. Keys are relative file paths,
// values are file contents.
func (b *BaseBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	if _, err := b.shims.Stat(b.templateRoot); os.IsNotExist(err) {
		return nil, nil
	}

	templateData := make(map[string][]byte)
	if err := b.walkAndCollectTemplates(b.templateRoot, templateData); err != nil {
		return nil, fmt.Errorf("failed to collect templates: %w", err)
	}

	if schemaData, exists := templateData["schema"]; exists {
		if err := b.configHandler.LoadSchemaFromBytes(schemaData); err != nil {
			return nil, fmt.Errorf("failed to load schema: %w", err)
		}
	}

	contextValues, err := b.configHandler.GetContextValues()
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
		contextName := b.configHandler.GetContext()
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

// Down manages the teardown of kustomizations and related resources, ignoring "not found" errors.
// It suspends kustomizations and helmreleases, applies cleanup kustomizations, waits for completion,
// deletes main kustomizations in reverse dependency order, and removes cleanup kustomizations and namespaces.
// The function filters kustomizations for destruction, sorts them by dependencies, and performs cleanup if specified.
// Dependency resolution is achieved through topological sorting for correct deletion order.
func (b *BaseBlueprintHandler) Down() error {
	allKustomizations := b.GetKustomizations()
	if len(allKustomizations) == 0 {
		return nil
	}

	var kustomizations []blueprintv1alpha1.Kustomization
	for _, k := range allKustomizations {
		if k.Destroy == nil || *k.Destroy {
			kustomizations = append(kustomizations, k)
		}
	}

	if len(kustomizations) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\nReceived interrupt signal, cancelling operations...\n")
		cancel()
	}()

	if err := b.destroyKustomizations(ctx, kustomizations); err != nil {
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("operation cancelled by user: %w", err)
		}
		return err
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// destroyKustomizations removes kustomizations and performs cleanup tasks.
// It sorts kustomizations by dependencies, applies cleanup kustomizations if defined,
// ensures readiness, and deletes them, followed by the main kustomizations.
func (b *BaseBlueprintHandler) destroyKustomizations(ctx context.Context, kustomizations []blueprintv1alpha1.Kustomization) error {
	deps := make(map[string][]string)
	for _, k := range kustomizations {
		deps[k.Name] = k.DependsOn
	}

	var sorted []string
	visited := make(map[string]bool)
	var visit func(string)
	visit = func(n string) {
		if visited[n] {
			return
		}
		visited[n] = true
		for _, dep := range deps[n] {
			visit(dep)
		}
		sorted = append(sorted, n)
	}
	for _, k := range kustomizations {
		visit(k.Name)
	}
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	nameToK := make(map[string]blueprintv1alpha1.Kustomization)
	for _, k := range kustomizations {
		nameToK[k.Name] = k
	}

	for _, name := range sorted {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		k := nameToK[name]

		if len(k.Cleanup) > 0 {
			status, err := b.kubernetesManager.GetKustomizationStatus([]string{k.Name})
			if err != nil {
				return fmt.Errorf("failed to check if kustomization %s exists: %w", k.Name, err)
			}

			if !status[k.Name] {
				continue
			}

			cleanupSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
			cleanupSpin.Suffix = fmt.Sprintf(" 🧹 Applying cleanup kustomization for %s", k.Name)
			cleanupSpin.Start()

			cleanupKustomization := &blueprintv1alpha1.Kustomization{
				Name:          k.Name + "-cleanup",
				Path:          strings.ReplaceAll(filepath.Join(k.Path, "cleanup"), "\\", "/"),
				Source:        k.Source,
				Components:    k.Cleanup,
				Timeout:       &metav1.Duration{Duration: 30 * time.Minute},
				Interval:      &metav1.Duration{Duration: constants.DefaultFluxKustomizationInterval},
				RetryInterval: &metav1.Duration{Duration: constants.DefaultFluxKustomizationRetryInterval},
				Wait:          func() *bool { b := true; return &b }(),
				Force:         func() *bool { b := true; return &b }(),
			}

			if err := b.kubernetesManager.ApplyKustomization(b.toFluxKustomization(*cleanupKustomization, constants.DefaultFluxSystemNamespace)); err != nil {
				return fmt.Errorf("failed to apply cleanup kustomization for %s: %w", k.Name, err)
			}

			timeout := b.shims.TimeAfter(constants.DefaultFluxCleanupTimeout)
			ticker := b.shims.NewTicker(2 * time.Second)
			defer b.shims.TickerStop(ticker)

			cleanupReady := false

		cleanupLoop:
			for !cleanupReady {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-timeout:
					break cleanupLoop
				case <-ticker.C:
					ready, err := b.kubernetesManager.GetKustomizationStatus([]string{cleanupKustomization.Name})
					if err != nil {
						return fmt.Errorf("cleanup kustomization %s failed: %w", cleanupKustomization.Name, err)
					}
					if ready[cleanupKustomization.Name] {
						cleanupReady = true
					}
				}
			}

			cleanupSpin.Stop()

			if !cleanupReady {
				fmt.Fprintf(os.Stderr, "Warning: Cleanup kustomization %s did not become ready within %v, proceeding anyway\n", cleanupKustomization.Name, constants.DefaultFluxCleanupTimeout)
			}
			fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🧹 Applying cleanup kustomization for %s - \033[32mDone\033[0m\n", k.Name)

			cleanupDeleteSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
			cleanupDeleteSpin.Suffix = fmt.Sprintf(" 🗑️  Deleting cleanup kustomization %s", cleanupKustomization.Name)
			cleanupDeleteSpin.Start()
			if err := b.kubernetesManager.DeleteKustomization(cleanupKustomization.Name, constants.DefaultFluxSystemNamespace); err != nil {
				return fmt.Errorf("failed to delete cleanup kustomization %s: %w", cleanupKustomization.Name, err)
			}

			cleanupDeleteSpin.Stop()
			fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🗑️  Deleting cleanup kustomization %s - \033[32mDone\033[0m\n", cleanupKustomization.Name)
		}

		deleteSpin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
		deleteSpin.Suffix = fmt.Sprintf(" 🗑️  Deleting kustomization %s", k.Name)
		deleteSpin.Start()
		if err := b.kubernetesManager.DeleteKustomization(k.Name, constants.DefaultFluxSystemNamespace); err != nil {
			return fmt.Errorf("failed to delete kustomization %s: %w", k.Name, err)
		}

		deleteSpin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m 🗑️  Deleting kustomization %s - \033[32mDone\033[0m\n", k.Name)
	}

	return nil
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
			(strings.HasPrefix(filepath.Dir(entryPath), filepath.Join(b.templateRoot, "features")) && strings.HasSuffix(entry.Name(), ".yaml")) {
			content, err := b.shims.ReadFile(filepath.Clean(entryPath))
			if err != nil {
				return fmt.Errorf("failed to read template file %s: %w", entryPath, err)
			}

			if entry.Name() == "schema.yaml" {
				templateData["schema"] = content
			} else if entry.Name() == "blueprint.yaml" {
				templateData["blueprint"] = content
			} else {
				relPath, err := filepath.Rel(b.templateRoot, entryPath)
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

	evaluator := NewFeatureEvaluator(b.injector)

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
			feature.Path = filepath.Join(b.templateRoot, path)
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
	projectRoot := b.projectRoot

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

// applySourceRepository routes to the appropriate source handler based on URL type
func (b *BaseBlueprintHandler) applySourceRepository(source blueprintv1alpha1.Source, namespace string) error {
	if strings.HasPrefix(source.Url, "oci://") {
		return b.applyOCIRepository(source, namespace)
	}
	return b.applyGitRepository(source, namespace)
}

// applyGitRepository creates or updates a GitRepository resource in the cluster. It normalizes
// the repository URL format, configures standard intervals and timeouts, and handles secret
// references for private repositories.
func (b *BaseBlueprintHandler) applyGitRepository(source blueprintv1alpha1.Source, namespace string) error {
	sourceUrl := source.Url
	if !strings.HasPrefix(sourceUrl, "http://") && !strings.HasPrefix(sourceUrl, "https://") {
		sourceUrl = "https://" + sourceUrl
	}

	gitRepo := &sourcev1.GitRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitRepository",
			APIVersion: "source.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: namespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: sourceUrl,
			Interval: metav1.Duration{
				Duration: constants.DefaultFluxSourceInterval,
			},
			Timeout: &metav1.Duration{
				Duration: constants.DefaultFluxSourceTimeout,
			},
			Reference: &sourcev1.GitRepositoryRef{
				Branch: source.Ref.Branch,
				Tag:    source.Ref.Tag,
				SemVer: source.Ref.SemVer,
				Commit: source.Ref.Commit,
			},
		},
	}

	if source.SecretName != "" {
		gitRepo.Spec.SecretRef = &meta.LocalObjectReference{
			Name: source.SecretName,
		}
	}

	return b.kubernetesManager.ApplyGitRepository(gitRepo)
}

// applyOCIRepository creates or updates an OCIRepository resource in the cluster. It handles
// OCI URL parsing, configures standard intervals and timeouts, and handles secret references
// for private registries. The OCI URL should include the tag/version (e.g., oci://registry/repo:tag).
func (b *BaseBlueprintHandler) applyOCIRepository(source blueprintv1alpha1.Source, namespace string) error {
	ociURL := source.Url
	var ref *sourcev1.OCIRepositoryRef

	if lastColon := strings.LastIndex(ociURL, ":"); lastColon > len("oci://") {
		if tagPart := ociURL[lastColon+1:]; tagPart != "" && !strings.Contains(tagPart, "/") {
			ociURL = ociURL[:lastColon]
			ref = &sourcev1.OCIRepositoryRef{
				Tag: tagPart,
			}
		}
	}

	if ref == nil && (source.Ref.Tag != "" || source.Ref.SemVer != "" || source.Ref.Commit != "") {
		ref = &sourcev1.OCIRepositoryRef{
			Tag:    source.Ref.Tag,
			SemVer: source.Ref.SemVer,
			Digest: source.Ref.Commit,
		}
	}

	if ref == nil {
		ref = &sourcev1.OCIRepositoryRef{
			Tag: "latest",
		}
	}

	ociRepo := &sourcev1.OCIRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCIRepository",
			APIVersion: "source.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: namespace,
		},
		Spec: sourcev1.OCIRepositorySpec{
			URL: ociURL,
			Interval: metav1.Duration{
				Duration: constants.DefaultFluxSourceInterval,
			},
			Timeout: &metav1.Duration{
				Duration: constants.DefaultFluxSourceTimeout,
			},
			Reference: ref,
		},
	}

	if source.SecretName != "" {
		ociRepo.Spec.SecretRef = &meta.LocalObjectReference{
			Name: source.SecretName,
		}
	}

	return b.kubernetesManager.ApplyOCIRepository(ociRepo)
}

// checkKustomizationStatus verifies the readiness of specified kustomizations by first checking
// the git repository status and then polling each kustomization's status. Returns true if all
// kustomizations are ready, false otherwise, along with any errors encountered during the checks.
func (b *BaseBlueprintHandler) checkKustomizationStatus(kustomizationNames []string) (bool, error) {
	if err := b.kubernetesManager.CheckGitRepositoryStatus(); err != nil {
		return false, fmt.Errorf("git repository error: %w", err)
	}
	status, err := b.kubernetesManager.GetKustomizationStatus(kustomizationNames)
	if err != nil {
		return false, fmt.Errorf("kustomization error: %w", err)
	}

	allReady := true
	for _, ready := range status {
		if !ready {
			allReady = false
			break
		}
	}
	return allReady, nil
}

// calculateMaxWaitTime calculates the maximum wait time needed based on kustomization dependencies.
// It builds a dependency graph from all kustomizations, mapping each to its dependencies and timeouts.
// Using depth-first search (DFS), it explores all possible dependency paths to find the longest one,
// accumulating timeout values along each path. The function handles circular dependencies by tracking
// visited nodes and avoiding infinite recursion while still considering their timeout contributions.
// It identifies root nodes (those with no incoming dependencies) as starting points, or if no roots
// exist due to cycles, it starts DFS from every node to ensure complete coverage. Returns the total
// time needed for the longest dependency path through the kustomization graph.
func (b *BaseBlueprintHandler) calculateMaxWaitTime() time.Duration {
	kustomizations := b.GetKustomizations()
	if len(kustomizations) == 0 {
		return 0
	}

	deps := make(map[string][]string)
	timeouts := make(map[string]time.Duration)
	for _, k := range kustomizations {
		deps[k.Name] = k.DependsOn
		if k.Timeout != nil {
			timeouts[k.Name] = k.Timeout.Duration
		} else {
			timeouts[k.Name] = constants.DefaultFluxKustomizationTimeout
		}
	}

	var maxPathTime time.Duration
	visited := make(map[string]bool)
	path := make([]string, 0)

	var dfs func(name string, currentTime time.Duration)
	dfs = func(name string, currentTime time.Duration) {
		visited[name] = true
		path = append(path, name)
		currentTime += timeouts[name]

		if currentTime > maxPathTime {
			maxPathTime = currentTime
		}

		for _, dep := range deps[name] {
			if !visited[dep] {
				dfs(dep, currentTime)
			} else {
				if currentTime+timeouts[dep] > maxPathTime {
					maxPathTime = currentTime + timeouts[dep]
				}
			}
		}

		visited[name] = false
		path = path[:len(path)-1]
	}

	roots := []string{}
	for _, k := range kustomizations {
		isRoot := true
		for _, other := range kustomizations {
			if slices.Contains(other.DependsOn, k.Name) {
				isRoot = false
				break
			}
		}
		if isRoot {
			roots = append(roots, k.Name)
		}
	}
	if len(roots) == 0 {
		for _, k := range kustomizations {
			dfs(k.Name, 0)
		}
	} else {
		for _, root := range roots {
			dfs(root, 0)
		}
	}

	return maxPathTime
}

// toFluxKustomization constructs a Flux Kustomization resource from the given
// blueprintv1alpha1.Kustomization and namespace. Maps blueprint fields to Flux equivalents,
// resolves dependencies, processes patch definitions (including reading and decoding patch files
// to extract selectors), configures post-build variable substitution using ConfigMaps and Secrets,
// determines the source reference type (GitRepository or OCIRepository), and sets all required
// Flux Kustomization fields for cluster application.
func (b *BaseBlueprintHandler) toFluxKustomization(k blueprintv1alpha1.Kustomization, namespace string) kustomizev1.Kustomization {
	dependsOn := make([]kustomizev1.DependencyReference, len(k.DependsOn))
	for i, dep := range k.DependsOn {
		dependsOn[i] = kustomizev1.DependencyReference{
			Name:      dep,
			Namespace: namespace,
		}
	}

	patches := make([]kustomize.Patch, 0, len(k.Patches))
	for _, p := range k.Patches {
		var target *kustomize.Selector
		var patchContent string

		if p.Path != "" {
			patchContent, target = b.resolvePatchFromPath(p.Path, namespace)
		}

		if p.Patch != "" {
			patchContent = p.Patch
		}
		if p.Target != nil {
			target = &kustomize.Selector{
				Kind:      p.Target.Kind,
				Name:      p.Target.Name,
				Namespace: p.Target.Namespace,
			}
		}

		if patchContent != "" {
			patches = append(patches, kustomize.Patch{
				Patch:  patchContent,
				Target: target,
			})
		}
	}

	var postBuild *kustomizev1.PostBuild
	substituteFrom := make([]kustomizev1.SubstituteReference, 0)

	if substitutions, hasSubstitutions := b.featureSubstitutions[k.Name]; hasSubstitutions && len(substitutions) > 0 {
		configMapName := fmt.Sprintf("values-%s", k.Name)
		substituteFrom = append(substituteFrom, kustomizev1.SubstituteReference{
			Kind:     "ConfigMap",
			Name:     configMapName,
			Optional: false,
		})
	}

	postBuild = &kustomizev1.PostBuild{
		SubstituteFrom: substituteFrom,
	}

	interval := metav1.Duration{Duration: k.Interval.Duration}
	retryInterval := metav1.Duration{Duration: k.RetryInterval.Duration}
	timeout := metav1.Duration{Duration: k.Timeout.Duration}

	prune := true
	if k.Prune != nil {
		prune = *k.Prune
	}

	deletionPolicy := "MirrorPrune"
	if k.Destroy == nil || *k.Destroy {
		deletionPolicy = "WaitForTermination"
	}

	sourceKind := "GitRepository"
	if b.isOCISource(k.Source) {
		sourceKind = "OCIRepository"
	}

	return kustomizev1.Kustomization{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Kustomization",
			APIVersion: "kustomize.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.Name,
			Namespace: namespace,
		},
		Spec: kustomizev1.KustomizationSpec{
			SourceRef: kustomizev1.CrossNamespaceSourceReference{
				Kind: sourceKind,
				Name: k.Source,
			},
			Path:           k.Path,
			DependsOn:      dependsOn,
			Interval:       interval,
			RetryInterval:  &retryInterval,
			Timeout:        &timeout,
			Patches:        patches,
			Force:          *k.Force,
			PostBuild:      postBuild,
			Components:     k.Components,
			Wait:           *k.Wait,
			Prune:          prune,
			DeletionPolicy: deletionPolicy,
		},
	}
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

	configRoot, err := b.configHandler.GetConfigRoot()
	if err == nil {
		patchFilePath := filepath.Join(configRoot, "kustomize", patchPath)
		if data, err := b.shims.ReadFile(patchFilePath); err == nil {
			if basePatchData == nil {
				target = b.extractTargetFromPatchContent(string(data), defaultNamespace)
				return string(data), target
			}

			var userPatchData map[string]any
			if err := b.shims.YamlUnmarshal(data, &userPatchData); err == nil {
				maps.Copy(basePatchData, userPatchData)
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

// applyValuesConfigMaps generates ConfigMaps for Flux post-build variable substitution using rendered template values and context-specific values.yaml files.
// Merges rendered template values with context values, giving precedence to context values in case of conflict.
// Produces a ConfigMap for the "common" section and for each component section, with system values merged into "common".
// The resulting ConfigMaps are referenced in PostBuild.SubstituteFrom for variable substitution.
func (b *BaseBlueprintHandler) applyValuesConfigMaps() error {
	mergedCommonValues := make(map[string]any)

	domain := b.configHandler.GetString("dns.domain")
	context := b.configHandler.GetContext()
	lbStart := b.configHandler.GetString("network.loadbalancer_ips.start")
	lbEnd := b.configHandler.GetString("network.loadbalancer_ips.end")
	registryURL := b.configHandler.GetString("docker.registry_url")
	localVolumePaths := b.configHandler.GetStringSlice("cluster.workers.volumes")

	loadBalancerIPRange := fmt.Sprintf("%s-%s", lbStart, lbEnd)

	var localVolumePath string
	if len(localVolumePaths) > 0 {
		volumeParts := strings.Split(localVolumePaths[0], ":")
		if len(volumeParts) > 1 {
			localVolumePath = volumeParts[1]
		} else {
			localVolumePath = ""
		}
	} else {
		localVolumePath = ""
	}

	mergedCommonValues["DOMAIN"] = domain
	mergedCommonValues["CONTEXT"] = context
	mergedCommonValues["CONTEXT_ID"] = b.configHandler.GetString("id")
	mergedCommonValues["LOADBALANCER_IP_RANGE"] = loadBalancerIPRange
	mergedCommonValues["LOADBALANCER_IP_START"] = lbStart
	mergedCommonValues["LOADBALANCER_IP_END"] = lbEnd
	mergedCommonValues["REGISTRY_URL"] = registryURL
	mergedCommonValues["LOCAL_VOLUME_PATH"] = localVolumePath

	execCtx := &contextpkg.ExecutionContext{
		Shell:         b.shell,
		ConfigHandler: b.configHandler,
		Injector:      b.injector,
	}
	buildID, err := execCtx.GetBuildID()
	if err != nil {
		return fmt.Errorf("failed to get build ID: %w", err)
	}
	if buildID != "" {
		mergedCommonValues["BUILD_ID"] = buildID
	}

	allValues := make(map[string]any)

	for kustomizationName, substitutions := range b.featureSubstitutions {
		if len(substitutions) > 0 {
			if allValues[kustomizationName] == nil {
				allValues[kustomizationName] = make(map[string]any)
			}
			if componentMap, ok := allValues[kustomizationName].(map[string]any); ok {
				for k, v := range substitutions {
					componentMap[k] = v
				}
			}
		}
	}

	contextValues, err := b.configHandler.GetContextValues()
	if err != nil {
		return fmt.Errorf("failed to load context values: %w", err)
	}

	if contextValues != nil {
		if substitutionValues, ok := contextValues["substitutions"].(map[string]any); ok {
			allValues = b.deepMergeMaps(allValues, substitutionValues)
		}
	}

	if allValues["common"] == nil {
		allValues["common"] = make(map[string]any)
	}

	if commonMap, ok := allValues["common"].(map[string]any); ok {
		maps.Copy(commonMap, mergedCommonValues)
	}

	for componentName, componentValues := range allValues {
		if componentMap, ok := componentValues.(map[string]any); ok {
			configMapName := fmt.Sprintf("values-%s", componentName)
			if err := b.createConfigMap(componentMap, configMapName); err != nil {
				return fmt.Errorf("failed to create ConfigMap for component %s: %w", componentName, err)
			}
		}
	}

	return nil
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

// createConfigMap creates a ConfigMap named configMapName in the "flux-system" namespace for post-build variable substitution.
// Supports scalar values and one level of map nesting. The resulting ConfigMap data is a map of string keys to string values.
func (b *BaseBlueprintHandler) createConfigMap(values map[string]any, configMapName string) error {
	if err := b.validateValuesForSubstitution(values); err != nil {
		return fmt.Errorf("invalid values in %s: %w", configMapName, err)
	}

	stringValues := make(map[string]string)
	if err := b.flattenValuesToConfigMap(values, "", stringValues); err != nil {
		return fmt.Errorf("failed to flatten values for %s: %w", configMapName, err)
	}

	if err := b.kubernetesManager.ApplyConfigMap(configMapName, constants.DefaultFluxSystemNamespace, stringValues); err != nil {
		return fmt.Errorf("failed to apply ConfigMap %s: %w", configMapName, err)
	}

	return nil
}

// flattenValuesToConfigMap recursively flattens nested values into a flat map suitable for ConfigMap data.
// Nested maps are flattened using dot notation (e.g., "ingress.host").
func (b *BaseBlueprintHandler) flattenValuesToConfigMap(values map[string]any, prefix string, result map[string]string) error {
	for key, value := range values {
		currentKey := key
		if prefix != "" {
			currentKey = prefix + "." + key
		}

		switch v := value.(type) {
		case string:
			result[currentKey] = v
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			result[currentKey] = fmt.Sprintf("%v", v)
		case bool:
			result[currentKey] = fmt.Sprintf("%t", v)
		case map[string]any:
			err := b.flattenValuesToConfigMap(v, currentKey, result)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported value type for key %s: %T", key, v)
		}
	}
	return nil
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
	devMode := b.configHandler.GetBool("dev")

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

	gitURL, err := b.shell.ExecSilent("git", "config", "--get", "remote.origin.url")
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
	evaluator := NewFeatureEvaluator(b.injector)

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
	domain := b.configHandler.GetString("dns.domain", "test")
	if domain == "" {
		return ""
	}

	projectRoot, err := b.shell.GetProjectRoot()
	if err != nil {
		return ""
	}

	folder := b.shims.FilepathBase(projectRoot)
	if folder == "" {
		return ""
	}

	return fmt.Sprintf("http://git.%s/git/%s", domain, folder)
}
