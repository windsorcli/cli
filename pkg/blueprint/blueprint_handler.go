package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "embed"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/kubernetes"
	"github.com/windsorcli/cli/pkg/shell"

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
	LoadConfig(reset ...bool) error
	Install() error
	GetMetadata() blueprintv1alpha1.Metadata
	GetSources() []blueprintv1alpha1.Source
	GetRepository() blueprintv1alpha1.Repository
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	WaitForKustomizations(message string, names ...string) error
	ProcessContextTemplates(contextName string, reset ...bool) error
	GetDefaultTemplateData(contextName string) (map[string][]byte, error)
	GetLocalTemplateData() (map[string][]byte, error)
	Down() error
}

//go:embed templates/default.jsonnet
var defaultJsonnetTemplate string

//go:embed templates/local.jsonnet
var localJsonnetTemplate string

//go:embed templates/metal.jsonnet
var metalJsonnetTemplate string

//go:embed templates/aws.jsonnet
var awsJsonnetTemplate string

//go:embed templates/azure.jsonnet
var azureJsonnetTemplate string

type BaseBlueprintHandler struct {
	BlueprintHandler
	injector          di.Injector
	configHandler     config.ConfigHandler
	shell             shell.Shell
	kubernetesManager kubernetes.KubernetesManager
	localBlueprint    blueprintv1alpha1.Blueprint
	blueprint         blueprintv1alpha1.Blueprint
	projectRoot       string
	shims             *Shims
}

// NewBlueprintHandler creates a new instance of BaseBlueprintHandler.
// It initializes the handler with the provided dependency injector.
func NewBlueprintHandler(injector di.Injector) *BaseBlueprintHandler {
	return &BaseBlueprintHandler{
		injector: injector,
		shims:    NewShims(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the BaseBlueprintHandler by resolving and assigning its dependencies,
// including the configHandler, contextHandler, and shell, from the provided dependency injector.
// It also determines the project root directory using the shell and sets the project name
// in the configuration. If any of these steps fail, it returns an error.
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

	return nil
}

// LoadConfig reads blueprint configuration from blueprint.yaml file.
// Only loads existing blueprint.yaml files - no templating or generation.
// All template processing happens in ProcessContextTemplates during init.
func (b *BaseBlueprintHandler) LoadConfig(reset ...bool) error {
	configRoot, err := b.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	yamlPath := filepath.Join(configRoot, "blueprint.yaml")
	if _, err := b.shims.Stat(yamlPath); err != nil {
		// No blueprint.yaml exists - use default blueprint
		context := b.configHandler.GetContext()
		b.blueprint = *DefaultBlueprint.DeepCopy()
		b.blueprint.Metadata.Name = context
		b.blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", context)
		return nil
	}

	yamlData, err := b.shims.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	return b.processBlueprintData(yamlData, &b.blueprint)
}

// WaitForKustomizations waits for the specified kustomizations to be ready.
// It polls the status of the kustomizations until they are all ready or a timeout occurs.
func (b *BaseBlueprintHandler) WaitForKustomizations(message string, names ...string) error {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	timeout := b.shims.TimeAfter(b.calculateMaxWaitTime())
	ticker := b.shims.NewTicker(constants.DEFAULT_KUSTOMIZATION_WAIT_POLL_INTERVAL)
	defer b.shims.TickerStop(ticker)

	var kustomizationNames []string
	if len(names) > 0 && len(names[0]) > 0 {
		kustomizationNames = names
	} else {
		kustomizations := b.getKustomizations()
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
				if consecutiveFailures >= constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES {
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

// Install applies the blueprint's Kubernetes resources to the cluster. It handles GitRepositories
// for the main repository and sources, Kustomizations for deployments, and a ConfigMap containing
// context-specific configuration. Uses environment KUBECONFIG or falls back to in-cluster config.
func (b *BaseBlueprintHandler) Install() error {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " 📐 Installing blueprint resources"
	spin.Start()
	defer spin.Stop()

	// Ensure namespace exists
	if err := b.createManagedNamespace(constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Apply blueprint repository
	if b.blueprint.Repository.Url != "" {
		source := blueprintv1alpha1.Source{
			Name:       b.blueprint.Metadata.Name,
			Url:        b.blueprint.Repository.Url,
			Ref:        b.blueprint.Repository.Ref,
			SecretName: b.blueprint.Repository.SecretName,
		}
		if err := b.applySourceRepository(source, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply blueprint repository: %w", err)
		}
	}

	// Apply other sources
	for _, source := range b.blueprint.Sources {
		if err := b.applySourceRepository(source, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply source %s: %w", source.Name, err)
		}
	}

	// Apply ConfigMap
	if err := b.applyConfigMap(); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
		return fmt.Errorf("failed to apply configmap: %w", err)
	}

	// Apply Kustomizations
	kustomizations := b.getKustomizations()
	kustomizationNames := make([]string, len(kustomizations))
	for i, k := range kustomizations {
		if err := b.kubernetesManager.ApplyKustomization(b.toKubernetesKustomization(k, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)); err != nil {
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

// GetDefaultTemplateData generates default template data based on the platform configuration.
// It uses the embedded platform templates to create a map of template files that can be
// used by the init pipeline for generating context-specific configurations.
func (b *BaseBlueprintHandler) GetDefaultTemplateData(contextName string) (map[string][]byte, error) {
	platform := b.configHandler.GetString("platform")
	if platform == "" {
		platform = b.configHandler.GetString("cluster.platform")
	}

	templateData, err := b.loadPlatformTemplate(platform)
	if err != nil || len(templateData) == 0 {
		templateData, err = b.loadPlatformTemplate("default")
		if err != nil {
			return nil, fmt.Errorf("error loading default template: %w", err)
		}
	}

	if len(templateData) == 0 {
		return map[string][]byte{}, nil
	}

	return map[string][]byte{
		"blueprint.jsonnet": templateData,
	}, nil
}

// GetLocalTemplateData collects template data from the local contexts/_template directory.
// It recursively walks through the template directory and collects only .jsonnet files,
// maintaining the relative path structure from the template directory root.
func (b *BaseBlueprintHandler) GetLocalTemplateData() (map[string][]byte, error) {
	projectRoot, err := b.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}

	templateDir := filepath.Join(projectRoot, "contexts", "_template")
	if _, err := b.shims.Stat(templateDir); err != nil {
		// Template directory doesn't exist, return empty map
		return make(map[string][]byte), nil
	}

	templateData := make(map[string][]byte)
	if err := b.walkAndCollectTemplates(templateDir, templateDir, templateData); err != nil {
		return nil, fmt.Errorf("failed to collect local templates: %w", err)
	}

	return templateData, nil
}

// walkAndCollectTemplates recursively walks through the template directory and collects only .jsonnet files.
// It maintains the relative path structure from the template directory root.
func (b *BaseBlueprintHandler) walkAndCollectTemplates(templateDir, templateRoot string, templateData map[string][]byte) error {
	entries, err := b.shims.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(templateDir, entry.Name())

		if entry.IsDir() {
			if err := b.walkAndCollectTemplates(entryPath, templateRoot, templateData); err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".jsonnet") {
			content, err := b.shims.ReadFile(filepath.Clean(entryPath))
			if err != nil {
				return fmt.Errorf("failed to read template file %s: %w", entryPath, err)
			}

			relPath, err := filepath.Rel(templateRoot, entryPath)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}

			templateData[filepath.ToSlash(relPath)] = content
		}
	}

	return nil
}

// Down orchestrates the controlled teardown of all kustomizations and their associated resources.
// It follows a specific sequence to ensure safe deletion:
// 1. Suspends all kustomizations and their associated helmreleases to prevent reconciliation
// 2. Applies cleanup kustomizations if defined, which handle resource cleanup tasks
// 3. Waits for cleanup kustomizations to complete their operations
// 4. Deletes main kustomizations in reverse dependency order
// 5. Deletes cleanup kustomizations and their namespace
//
// The function handles dependency resolution through topological sorting to ensure
// resources are deleted in the correct order. It also manages a dedicated cleanup
// namespace for cleanup kustomizations when needed.
func (b *BaseBlueprintHandler) Down() error {
	kustomizations := b.getKustomizations()
	if len(kustomizations) == 0 {
		return nil
	}

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " 🗑️  Tearing down blueprint resources"
	spin.Start()
	defer spin.Stop()

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

	needsCleanupNamespace := false
	for _, k := range kustomizations {
		if len(k.Cleanup) > 0 {
			needsCleanupNamespace = true
			break
		}
	}

	if needsCleanupNamespace {
		if err := b.createManagedNamespace("system-cleanup"); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to create system-cleanup namespace: %w", err)
		}
	}

	for _, name := range sorted {
		k := nameToK[name]

		if err := b.kubernetesManager.SuspendKustomization(k.Name, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to suspend kustomization %s: %w", k.Name, err)
		}

		helmReleases, err := b.kubernetesManager.GetHelmReleasesForKustomization(k.Name, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)
		if err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to get helmreleases for kustomization %s: %w", k.Name, err)
		}

		for _, hr := range helmReleases {
			if err := b.kubernetesManager.SuspendHelmRelease(hr.Name, hr.Namespace); err != nil {
				spin.Stop()
				fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
				return fmt.Errorf("failed to suspend helmrelease %s in namespace %s: %w", hr.Name, hr.Namespace, err)
			}
		}
	}

	var cleanupNames []string
	for _, name := range sorted {
		k := nameToK[name]
		if len(k.Cleanup) > 0 {
			cleanupKustomization := &blueprintv1alpha1.Kustomization{
				Name:          k.Name + "-cleanup",
				Path:          filepath.Join(k.Path, "cleanup"),
				Source:        k.Source,
				Components:    k.Cleanup,
				Timeout:       &metav1.Duration{Duration: 30 * time.Minute},
				Interval:      &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_INTERVAL},
				RetryInterval: &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL},
				Wait:          func() *bool { b := true; return &b }(),
				Force:         func() *bool { b := true; return &b }(),
				PostBuild: &blueprintv1alpha1.PostBuild{
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{},
				},
			}
			if err := b.kubernetesManager.ApplyKustomization(b.toKubernetesKustomization(*cleanupKustomization, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)); err != nil {
				spin.Stop()
				fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
				return fmt.Errorf("failed to apply cleanup kustomization for %s: %w", k.Name, err)
			}
			cleanupNames = append(cleanupNames, cleanupKustomization.Name)
		}
	}

	for _, name := range sorted {
		k := nameToK[name]
		if err := b.kubernetesManager.DeleteKustomization(k.Name, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to delete kustomization %s: %w", k.Name, err)
		}
	}

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m%s - \033[32mDone\033[0m\n", spin.Suffix)

	if err := b.kubernetesManager.WaitForKustomizationsDeleted("⌛️ Waiting for kustomizations to be deleted", sorted...); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
		return fmt.Errorf("failed waiting for kustomizations to be deleted: %w", err)
	}

	if len(cleanupNames) > 0 {
		for _, cname := range cleanupNames {
			if err := b.kubernetesManager.DeleteKustomization(cname, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
				spin.Stop()
				fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
				return fmt.Errorf("failed to delete cleanup kustomization %s: %w", cname, err)
			}
		}

		if err := b.kubernetesManager.WaitForKustomizationsDeleted("⌛️ Waiting for cleanup kustomizations to be deleted", cleanupNames...); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed waiting for cleanup kustomizations to be deleted: %w", err)
		}

		if err := b.deleteNamespace("system-cleanup"); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to delete system-cleanup namespace: %w", err)
		}
	}

	return nil
}

// ProcessContextTemplates processes jsonnet templates from the contexts/_template directory
// and generates corresponding blueprint files in the specified context directory. The function handles
// three scenarios:
//  1. Blueprint Flag Processing: If --blueprint flag is specified, uses embedded platform templates
//     and ignores any _template directory to ensure consistent behavior
//  2. Template Processing: If no --blueprint flag and contexts/_template exists, processes blueprint.jsonnet files,
//     evaluating them with context data and writing blueprint.yaml files
//  3. Default Blueprint Generation: Falls back to generating platform-specific blueprints
//
// Other template types (terraform, kustomize, etc.) are handled by their respective generators
// during the WriteConfigurationFiles phase.
func (b *BaseBlueprintHandler) ProcessContextTemplates(contextName string, reset ...bool) error {
	resetMode := len(reset) > 0 && reset[0]

	projectRoot, err := b.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error getting project root: %w", err)
	}

	contextDir := filepath.Join(projectRoot, "contexts", contextName)
	if err := b.shims.MkdirAll(contextDir, 0755); err != nil {
		return fmt.Errorf("error creating context directory: %w", err)
	}

	blueprintValue := b.configHandler.GetString("blueprint")
	if blueprintValue != "" {
		return b.generateDefaultBlueprint(contextDir, contextName, resetMode)
	}

	templateDir := filepath.Join(projectRoot, "contexts", "_template")
	if _, err := b.shims.Stat(templateDir); err == nil {
		return b.processTemplateDirectory(templateDir, contextDir, contextName, resetMode)
	}

	// Fall back to platform/default templates (lowest priority)
	return b.generateDefaultBlueprint(contextDir, contextName, resetMode)
}

// processTemplateDirectory processes blueprint templates from the _template directory
func (b *BaseBlueprintHandler) processTemplateDirectory(templateDir, contextDir, contextName string, resetMode bool) error {
	entries, err := b.shims.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("error reading template directory: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == "blueprint.jsonnet" {
			templateFile := filepath.Join(templateDir, entry.Name())
			return b.processJsonnetTemplate(templateFile, contextDir, contextName, resetMode)
		}
	}

	return b.generateDefaultBlueprint(contextDir, contextName, resetMode)
}

// processJsonnetTemplate reads and evaluates a jsonnet template file to generate blueprint configuration.
// It loads the template file, marshals the current context configuration to JSON for use as template data,
// evaluates the jsonnet template with the context data injected via ExtCode, and processes the resulting
// blueprint content through the standard blueprint template processing pipeline.
func (b *BaseBlueprintHandler) processJsonnetTemplate(templateFile, contextDir, contextName string, resetMode bool) error {
	jsonnetData, err := b.shims.ReadFile(templateFile)
	if err != nil {
		return fmt.Errorf("error reading template file: %w", err)
	}

	config := b.configHandler.GetConfig()
	contextYAML, err := b.configHandler.YamlMarshalWithDefinedPaths(config)
	if err != nil {
		return fmt.Errorf("error marshalling context to YAML: %w", err)
	}

	var contextMap map[string]any = make(map[string]any)
	if err := b.shims.YamlUnmarshal(contextYAML, &contextMap); err != nil {
		return fmt.Errorf("error unmarshalling context YAML: %w", err)
	}

	contextMap["name"] = contextName
	contextMap["projectName"] = filepath.Base(b.projectRoot)
	contextJSON, err := b.shims.JsonMarshal(contextMap)
	if err != nil {
		return fmt.Errorf("error marshalling context map to JSON: %w", err)
	}

	vm := b.shims.NewJsonnetVM()
	vm.ExtCode("context", string(contextJSON))
	evaluatedContent, err := vm.EvaluateAnonymousSnippet(templateFile, string(jsonnetData))
	if err != nil {
		return fmt.Errorf("error evaluating jsonnet template: %w", err)
	}

	outputPath := filepath.Join(contextDir, "blueprint.yaml")
	return b.processBlueprintTemplate(outputPath, evaluatedContent, contextName, resetMode)
}

// generateDefaultBlueprint generates a default blueprint when no templates exist
func (b *BaseBlueprintHandler) generateDefaultBlueprint(contextDir, contextName string, resetMode bool) error {
	blueprintPath := filepath.Join(contextDir, "blueprint.yaml")
	if _, err := b.shims.Stat(blueprintPath); err != nil || resetMode {
		// === Platform Template Loading ===
		// --platform flag determines which template file to use
		platform := b.configHandler.GetString("platform")
		if platform == "" {
			platform = b.configHandler.GetString("cluster.platform")
		}
		templateData, err := b.loadPlatformTemplate(platform)
		if err != nil || len(templateData) == 0 {
			templateData, err = b.loadPlatformTemplate("default")
			if err != nil {
				return fmt.Errorf("error loading default template: %w", err)
			}
		}

		// === Blueprint Data Generation ===
		if len(templateData) > 0 {
			config := b.configHandler.GetConfig()
			contextYAML, err := b.configHandler.YamlMarshalWithDefinedPaths(config)
			if err != nil {
				return fmt.Errorf("error marshalling context to YAML: %w", err)
			}
			var contextMap map[string]any = make(map[string]any)
			if err := b.shims.YamlUnmarshal(contextYAML, &contextMap); err != nil {
				return fmt.Errorf("error unmarshalling context YAML: %w", err)
			}
			contextMap["name"] = contextName
			contextMap["projectName"] = filepath.Base(b.projectRoot)

			// --blueprint flag controls the context.blueprint field value
			// Only set if explicitly provided via --blueprint flag
			blueprintValue := b.configHandler.GetString("blueprint")
			if blueprintValue != "" {
				contextMap["blueprint"] = blueprintValue
			}

			contextJSON, err := b.shims.JsonMarshal(contextMap)
			if err != nil {
				return fmt.Errorf("error marshalling context map to JSON: %w", err)
			}

			// Use ExtCode to make context available via std.extVar("context")
			// Templates must include: local context = std.extVar("context");
			// This follows standard jsonnet patterns and is explicit and debuggable
			vm := b.shims.NewJsonnetVM()
			vm.ExtCode("context", string(contextJSON))
			evaluatedContent, err := vm.EvaluateAnonymousSnippet("blueprint.jsonnet", string(templateData))
			if err != nil {
				return fmt.Errorf("error generating blueprint from jsonnet: %w", err)
			}
			if evaluatedContent != "" {
				// Process through standard pipeline (validates, converts to YAML, applies metadata)
				return b.processBlueprintTemplate(blueprintPath, evaluatedContent, contextName, resetMode)
			}
		}

		// === Fallback Blueprint Creation ===
		blueprint := *DefaultBlueprint.DeepCopy()
		blueprint.Metadata.Name = contextName
		blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", contextName)
		blueprintData, err := b.shims.YamlMarshal(blueprint)
		if err != nil {
			return fmt.Errorf("error marshalling default blueprint: %w", err)
		}

		// === Blueprint File Write ===
		if err := b.shims.WriteFile(blueprintPath, blueprintData, 0644); err != nil {
			return fmt.Errorf("error writing blueprint file: %w", err)
		}
	}

	return nil
}

// processBlueprintTemplate validates blueprint template output against the Blueprint schema,
// applies context-specific metadata overrides, and writes the result as a properly formatted
// blueprint.yaml file. It ensures template content conforms to the Blueprint schema before
// persisting to disk and automatically sets the blueprint name and description based on context.
func (b *BaseBlueprintHandler) processBlueprintTemplate(outputPath, content, contextName string, resetMode bool) error {
	if !resetMode {
		if _, err := b.shims.Stat(outputPath); err == nil {
			return nil
		}
	}

	var testBlueprint blueprintv1alpha1.Blueprint
	if err := b.processBlueprintData([]byte(content), &testBlueprint); err != nil {
		return fmt.Errorf("error validating blueprint template: %w", err)
	}

	testBlueprint.Metadata.Name = contextName
	testBlueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", contextName)

	yamlData, err := b.shims.YamlMarshal(testBlueprint)
	if err != nil {
		return fmt.Errorf("error converting blueprint to YAML: %w", err)
	}

	if err := b.shims.WriteFile(outputPath, yamlData, 0644); err != nil {
		return fmt.Errorf("error writing blueprint file: %w", err)
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

// processBlueprintData unmarshals and validates blueprint configuration data, ensuring required
// fields are present and converting any raw kustomization data into strongly typed objects.
func (b *BaseBlueprintHandler) processBlueprintData(data []byte, blueprint *blueprintv1alpha1.Blueprint) error {
	newBlueprint := &blueprintv1alpha1.PartialBlueprint{}
	if err := b.shims.YamlUnmarshal(data, newBlueprint); err != nil {
		return fmt.Errorf("error unmarshalling blueprint data: %w", err)
	}

	var kustomizations []blueprintv1alpha1.Kustomization

	for _, kMap := range newBlueprint.Kustomizations {
		kustomizationYAML, err := b.shims.YamlMarshalNonNull(kMap)
		if err != nil {
			return fmt.Errorf("error marshalling kustomization map: %w", err)
		}

		var kustomization blueprintv1alpha1.Kustomization
		err = b.shims.K8sYamlUnmarshal(kustomizationYAML, &kustomization)
		if err != nil {
			return fmt.Errorf("error unmarshalling kustomization YAML: %w", err)
		}

		kustomizations = append(kustomizations, kustomization)
	}

	completeBlueprint := &blueprintv1alpha1.Blueprint{
		Kind:                newBlueprint.Kind,
		ApiVersion:          newBlueprint.ApiVersion,
		Metadata:            newBlueprint.Metadata,
		Sources:             newBlueprint.Sources,
		TerraformComponents: newBlueprint.TerraformComponents,
		Kustomizations:      kustomizations,
		Repository:          newBlueprint.Repository,
	}

	blueprint.Merge(completeBlueprint)
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

// getKustomizations retrieves and normalizes the blueprint's Kustomization configurations.
// It provides default values for intervals, timeouts, and paths while ensuring consistent
// configuration across all kustomizations. The function also adds standard PostBuild
// configurations for variable substitution from the blueprint ConfigMap.
func (b *BaseBlueprintHandler) getKustomizations() []blueprintv1alpha1.Kustomization {
	if b.blueprint.Kustomizations == nil {
		return nil
	}

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
			kustomizations[i].Interval = &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_INTERVAL}
		}
		if kustomizations[i].RetryInterval == nil || kustomizations[i].RetryInterval.Duration == 0 {
			kustomizations[i].RetryInterval = &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_RETRY_INTERVAL}
		}
		if kustomizations[i].Timeout == nil || kustomizations[i].Timeout.Duration == 0 {
			kustomizations[i].Timeout = &metav1.Duration{Duration: constants.DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT}
		}
		if kustomizations[i].Wait == nil {
			defaultWait := constants.DEFAULT_FLUX_KUSTOMIZATION_WAIT
			kustomizations[i].Wait = &defaultWait
		}
		if kustomizations[i].Force == nil {
			defaultForce := constants.DEFAULT_FLUX_KUSTOMIZATION_FORCE
			kustomizations[i].Force = &defaultForce
		}

		kustomizations[i].PostBuild = &blueprintv1alpha1.PostBuild{
			SubstituteFrom: []blueprintv1alpha1.SubstituteReference{
				{
					Kind:     "ConfigMap",
					Name:     "blueprint",
					Optional: false,
				},
			},
		}
	}

	return kustomizations
}

// loadPlatformTemplate loads a platform-specific template or the default template
func (b *BaseBlueprintHandler) loadPlatformTemplate(platform string) ([]byte, error) {
	switch platform {
	case "local":
		return []byte(localJsonnetTemplate), nil
	case "metal":
		return []byte(metalJsonnetTemplate), nil
	case "aws":
		return []byte(awsJsonnetTemplate), nil
	case "azure":
		return []byte(azureJsonnetTemplate), nil
	case "default":
		return []byte(defaultJsonnetTemplate), nil
	default:
		if platform == "" {
			return []byte(defaultJsonnetTemplate), nil
		}
		return nil, nil
	}
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
				Duration: constants.DEFAULT_FLUX_SOURCE_INTERVAL,
			},
			Timeout: &metav1.Duration{
				Duration: constants.DEFAULT_FLUX_SOURCE_TIMEOUT,
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
				Duration: constants.DEFAULT_FLUX_SOURCE_INTERVAL,
			},
			Timeout: &metav1.Duration{
				Duration: constants.DEFAULT_FLUX_SOURCE_TIMEOUT,
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

// applyConfigMap creates or updates a ConfigMap in the cluster containing context-specific
// configuration values used by the blueprint's resources, such as domain names, IP ranges,
// and volume paths.
func (b *BaseBlueprintHandler) applyConfigMap() error {
	domain := b.configHandler.GetString("dns.domain")
	context := b.configHandler.GetContext()
	lbStart := b.configHandler.GetString("network.loadbalancer_ips.start")
	lbEnd := b.configHandler.GetString("network.loadbalancer_ips.end")
	registryURL := b.configHandler.GetString("docker.registry_url")
	localVolumePaths := b.configHandler.GetStringSlice("cluster.workers.volumes")

	loadBalancerIPRange := fmt.Sprintf("%s-%s", lbStart, lbEnd)

	var localVolumePath string
	if len(localVolumePaths) > 0 {
		localVolumePath = strings.Split(localVolumePaths[0], ":")[1]
	} else {
		localVolumePath = ""
	}

	data := map[string]string{
		"DOMAIN":                domain,
		"CONTEXT":               context,
		"CONTEXT_ID":            b.configHandler.GetString("id"),
		"LOADBALANCER_IP_RANGE": loadBalancerIPRange,
		"LOADBALANCER_IP_START": lbStart,
		"LOADBALANCER_IP_END":   lbEnd,
		"REGISTRY_URL":          registryURL,
		"LOCAL_VOLUME_PATH":     localVolumePath,
	}

	return b.kubernetesManager.ApplyConfigMap("blueprint", constants.DEFAULT_FLUX_SYSTEM_NAMESPACE, data)
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
	kustomizations := b.getKustomizations()
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
			timeouts[k.Name] = constants.DEFAULT_FLUX_KUSTOMIZATION_TIMEOUT
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

func (b *BaseBlueprintHandler) createManagedNamespace(name string) error {
	return b.kubernetesManager.CreateNamespace(name)
}

func (b *BaseBlueprintHandler) deleteNamespace(name string) error {
	return b.kubernetesManager.DeleteNamespace(name)
}

// toKubernetesKustomization converts a blueprint kustomization to a Flux kustomization
// It handles conversion of dependsOn, patches, and postBuild configurations
// It maps blueprint fields to their Flux kustomization equivalents
// It maintains namespace context and preserves all configuration options
// It automatically detects OCI sources and sets the appropriate SourceRef kind
func (b *BaseBlueprintHandler) toKubernetesKustomization(k blueprintv1alpha1.Kustomization, namespace string) kustomizev1.Kustomization {
	dependsOn := make([]meta.NamespacedObjectReference, len(k.DependsOn))
	for i, dep := range k.DependsOn {
		dependsOn[i] = meta.NamespacedObjectReference{
			Name:      dep,
			Namespace: namespace,
		}
	}

	patches := make([]kustomize.Patch, len(k.Patches))
	for i, p := range k.Patches {
		patches[i] = kustomize.Patch{
			Patch: p.Patch,
			Target: &kustomize.Selector{
				Kind: p.Target.Kind,
				Name: p.Target.Name,
			},
		}
	}

	var postBuild *kustomizev1.PostBuild
	if k.PostBuild != nil {
		substituteFrom := make([]kustomizev1.SubstituteReference, len(k.PostBuild.SubstituteFrom))
		for i, ref := range k.PostBuild.SubstituteFrom {
			substituteFrom[i] = kustomizev1.SubstituteReference{
				Kind:     ref.Kind,
				Name:     ref.Name,
				Optional: ref.Optional,
			}
		}
		postBuild = &kustomizev1.PostBuild{
			Substitute:     k.PostBuild.Substitute,
			SubstituteFrom: substituteFrom,
		}
	}

	interval := metav1.Duration{Duration: k.Interval.Duration}
	retryInterval := metav1.Duration{Duration: k.RetryInterval.Duration}
	timeout := metav1.Duration{Duration: k.Timeout.Duration}

	prune := true
	if k.Prune != nil {
		prune = *k.Prune
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
			Path:          k.Path,
			DependsOn:     dependsOn,
			Interval:      interval,
			RetryInterval: &retryInterval,
			Timeout:       &timeout,
			Patches:       patches,
			Force:         *k.Force,
			PostBuild:     postBuild,
			Components:    k.Components,
			Wait:          *k.Wait,
			Prune:         prune,
		},
	}
}

// isOCISource determines whether a given source name or resolved URL corresponds to an OCI repository
// source by examining the URL prefix of the blueprint's main repository and any additional sources,
// or by checking if the input is already a resolved OCI URL.
func (b *BaseBlueprintHandler) isOCISource(sourceNameOrURL string) bool {
	// Check if it's already a resolved OCI URL
	if strings.HasPrefix(sourceNameOrURL, "oci://") {
		return true
	}

	// Check if it's a source name that maps to an OCI URL
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
