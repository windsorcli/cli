package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	WriteConfig(overwrite ...bool) error
	Install() error
	GetMetadata() blueprintv1alpha1.Metadata
	GetSources() []blueprintv1alpha1.Source
	GetRepository() blueprintv1alpha1.Repository
	GetTerraformComponents() []blueprintv1alpha1.TerraformComponent
	SetMetadata(metadata blueprintv1alpha1.Metadata) error
	SetSources(sources []blueprintv1alpha1.Source) error
	SetRepository(repository blueprintv1alpha1.Repository) error
	SetTerraformComponents(terraformComponents []blueprintv1alpha1.TerraformComponent) error
	SetKustomizations(kustomizations []blueprintv1alpha1.Kustomization) error
	WaitForKustomizations(message string, names ...string) error
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

	if err := b.configHandler.SetContextValue("projectName", filepath.Base(projectRoot)); err != nil {
		return fmt.Errorf("error setting project name in config: %w", err)
	}

	return nil
}

// LoadConfig reads blueprint configuration from specified path or default location.
// Priority: blueprint.yaml (if !reset), blueprint.jsonnet, platform template, default.
// Processes Jsonnet templates with context data injection for dynamic configuration.
// Falls back to embedded defaults if no configuration files exist.
func (b *BaseBlueprintHandler) LoadConfig(reset ...bool) error {
	shouldReset := false
	if len(reset) > 0 {
		shouldReset = reset[0]
	}

	configRoot, err := b.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	basePath := filepath.Join(configRoot, "blueprint")
	yamlPath := basePath + ".yaml"
	jsonnetPath := basePath + ".jsonnet"

	if !shouldReset {
		// 1. blueprint.yaml
		if _, err := b.shims.Stat(yamlPath); err == nil {
			yamlData, err := b.shims.ReadFile(yamlPath)
			if err != nil {
				return err
			}
			if err := b.processBlueprintData(yamlData, &b.blueprint); err != nil {
				return err
			}
			return nil
		}
	}

	// 2. blueprint.jsonnet
	if _, err := b.shims.Stat(jsonnetPath); err == nil {
		jsonnetData, err := b.shims.ReadFile(jsonnetPath)
		if err != nil {
			return err
		}
		config := b.configHandler.GetConfig()
		contextYAML, err := b.yamlMarshalWithDefinedPaths(config)
		if err != nil {
			return fmt.Errorf("error marshalling context to YAML: %w", err)
		}
		var contextMap map[string]any = make(map[string]any)
		if err := b.shims.YamlUnmarshal(contextYAML, &contextMap); err != nil {
			return fmt.Errorf("error unmarshalling context YAML: %w", err)
		}
		context := b.configHandler.GetContext()
		contextMap["name"] = context
		contextJSON, err := b.shims.JsonMarshal(contextMap)
		if err != nil {
			return fmt.Errorf("error marshalling context map to JSON: %w", err)
		}
		vm := b.shims.NewJsonnetVM()
		vm.ExtCode("context", string(contextJSON))
		evaluatedJsonnet, err := vm.EvaluateAnonymousSnippet("blueprint.jsonnet", string(jsonnetData))
		if err != nil {
			return fmt.Errorf("error generating blueprint from jsonnet: %w", err)
		}
		if err := b.processBlueprintData([]byte(evaluatedJsonnet), &b.blueprint); err != nil {
			return err
		}
		return nil
	}

	// 3. internal default (platform-specific if available, else global default)
	platform := ""
	if b.configHandler.GetConfig().Cluster != nil && b.configHandler.GetConfig().Cluster.Platform != nil {
		platform = *b.configHandler.GetConfig().Cluster.Platform
	}
	var platformData []byte
	if platform != "" {
		platformData, err = b.loadPlatformTemplate(platform)
		if err != nil {
			return fmt.Errorf("error loading platform template: %w", err)
		}
	}
	var evaluatedJsonnet string
	config := b.configHandler.GetConfig()
	contextYAML, err := b.yamlMarshalWithDefinedPaths(config)
	if err != nil {
		return fmt.Errorf("error marshalling context to YAML: %w", err)
	}
	var contextMap map[string]any = make(map[string]any)
	if err := b.shims.YamlUnmarshal(contextYAML, &contextMap); err != nil {
		return fmt.Errorf("error unmarshalling context YAML: %w", err)
	}
	context := b.configHandler.GetContext()
	contextMap["name"] = context
	contextJSON, err := b.shims.JsonMarshal(contextMap)
	if err != nil {
		return fmt.Errorf("error marshalling context map to JSON: %w", err)
	}
	vm := b.shims.NewJsonnetVM()
	vm.ExtCode("context", string(contextJSON))
	if len(platformData) > 0 {
		evaluatedJsonnet, err = vm.EvaluateAnonymousSnippet("blueprint.jsonnet", string(platformData))
		if err != nil {
			return fmt.Errorf("error generating blueprint from jsonnet: %w", err)
		}
	} else {
		evaluatedJsonnet, err = vm.EvaluateAnonymousSnippet("default.jsonnet", defaultJsonnetTemplate)
		if err != nil {
			return fmt.Errorf("error generating blueprint from default jsonnet: %w", err)
		}
	}
	if evaluatedJsonnet == "" {
		b.blueprint = *DefaultBlueprint.DeepCopy()
		b.blueprint.Metadata.Name = context
		b.blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", context)
	} else {
		if err := b.processBlueprintData([]byte(evaluatedJsonnet), &b.blueprint); err != nil {
			return err
		}
	}
	return nil
}

// WriteConfig persists the current blueprint configuration to disk. It handles path resolution,
// directory creation, and writes the blueprint in YAML format. The function cleans sensitive or
// redundant data before writing, such as Terraform component variables/values and empty PostBuild configs.
func (b *BaseBlueprintHandler) WriteConfig(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}

	configRoot, err := b.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	finalPath := filepath.Join(configRoot, "blueprint.yaml")
	dir := filepath.Dir(finalPath)
	if err := b.shims.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	if !shouldOverwrite {
		if _, err := b.shims.Stat(finalPath); err == nil {
			return nil
		}
	}

	fullBlueprint := b.blueprint.DeepCopy()
	for i := range fullBlueprint.TerraformComponents {
		fullBlueprint.TerraformComponents[i].Values = nil
	}
	for i := range fullBlueprint.Kustomizations {
		postBuild := fullBlueprint.Kustomizations[i].PostBuild
		if postBuild != nil && len(postBuild.Substitute) == 0 && len(postBuild.SubstituteFrom) == 0 {
			fullBlueprint.Kustomizations[i].PostBuild = nil
		}
	}
	fullBlueprint.Merge(&b.localBlueprint)
	data, err := b.shims.YamlMarshalNonNull(fullBlueprint)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}
	if err := b.shims.WriteFile(finalPath, data, 0644); err != nil {
		return fmt.Errorf("error writing blueprint file: %w", err)
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

	timeout := time.After(b.calculateMaxWaitTime())
	ticker := time.NewTicker(constants.DEFAULT_KUSTOMIZATION_WAIT_POLL_INTERVAL)
	defer ticker.Stop()

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

	consecutiveFailures := 0
	for {
		select {
		case <-timeout:
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("timeout waiting for kustomizations")
		case <-ticker.C:
			if err := b.kubernetesManager.CheckGitRepositoryStatus(); err != nil {
				consecutiveFailures++
				if consecutiveFailures >= constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES {
					spin.Stop()
					fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
					return fmt.Errorf("git repository error after %d consecutive failures: %w", consecutiveFailures, err)
				}
				continue
			}
			status, err := b.kubernetesManager.GetKustomizationStatus(kustomizationNames)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures >= constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES {
					spin.Stop()
					fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
					return fmt.Errorf("kustomization error after %d consecutive failures: %w", consecutiveFailures, err)
				}
				continue
			}

			allReady := true
			for _, ready := range status {
				if !ready {
					allReady = false
					break
				}
			}

			if allReady {
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

	// Apply GitRepository for the main repository
	if b.blueprint.Repository.Url != "" {
		source := blueprintv1alpha1.Source{
			Name:       b.configHandler.GetContext(),
			Url:        b.blueprint.Repository.Url,
			Ref:        b.blueprint.Repository.Ref,
			SecretName: b.blueprint.Repository.SecretName,
		}
		if err := b.applyGitRepository(source, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply main repository: %w", err)
		}
	}

	// Apply GitRepositories for sources
	for _, source := range b.blueprint.Sources {
		if err := b.applyGitRepository(source, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply source repository %s: %w", source.Name, err)
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
		if err := b.kubernetesManager.ApplyKustomization(b.ToKubernetesKustomization(k, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
			return fmt.Errorf("failed to apply kustomization %s: %w", k.Name, err)
		}
		kustomizationNames[i] = k.Name
	}

	// Wait for kustomizations to be ready
	if err := b.WaitForKustomizations("⌛️ Waiting for kustomizations to be ready", kustomizationNames...); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "✗%s - \033[31mFailed\033[0m\n", spin.Suffix)
		return fmt.Errorf("failed waiting for kustomizations: %w", err)
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

// getKustomizations retrieves the blueprint's Kustomization configurations, ensuring default values
// are set for intervals, timeouts, and adding standard PostBuild configurations for variable substitution.
func (b *BaseBlueprintHandler) getKustomizations() []blueprintv1alpha1.Kustomization {
	if b.blueprint.Kustomizations == nil {
		return nil
	}

	resolvedBlueprint := b.blueprint
	kustomizations := make([]blueprintv1alpha1.Kustomization, len(resolvedBlueprint.Kustomizations))
	copy(kustomizations, resolvedBlueprint.Kustomizations)

	for i := range kustomizations {
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

		// Add the substituteFrom configuration
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

// SetMetadata updates the metadata for the current blueprint.
// It replaces the existing metadata with the provided metadata information.
func (b *BaseBlueprintHandler) SetMetadata(metadata blueprintv1alpha1.Metadata) error {
	b.blueprint.Metadata = metadata
	return nil
}

// SetRepository updates the repository for the current blueprint.
// It replaces the existing repository with the provided repository information.
func (b *BaseBlueprintHandler) SetRepository(repository blueprintv1alpha1.Repository) error {
	b.blueprint.Repository = repository
	return nil
}

// SetSources updates the source configurations for the current blueprint.
// It replaces the existing sources with the provided list of sources.
func (b *BaseBlueprintHandler) SetSources(sources []blueprintv1alpha1.Source) error {
	b.blueprint.Sources = sources
	return nil
}

// SetTerraformComponents updates the Terraform components for the current blueprint.
// It replaces the existing components with the provided list of Terraform components.
func (b *BaseBlueprintHandler) SetTerraformComponents(terraformComponents []blueprintv1alpha1.TerraformComponent) error {
	b.blueprint.TerraformComponents = terraformComponents
	return nil
}

// SetKustomizations updates the Kustomizations for the current blueprint.
// It replaces the existing Kustomizations with the provided list of Kustomizations.
// If the provided list is nil, it clears the existing Kustomizations.
func (b *BaseBlueprintHandler) SetKustomizations(kustomizations []blueprintv1alpha1.Kustomization) error {
	b.blueprint.Kustomizations = kustomizations
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
			if err := b.kubernetesManager.ApplyKustomization(b.ToKubernetesKustomization(*cleanupKustomization, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE)); err != nil {
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

// =============================================================================
// Private Methods
// =============================================================================

// resolveComponentSources processes each Terraform component's source field, expanding it into a full
// URL with path prefix and reference information based on the associated source configuration.
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

				resolvedComponents[i].Source = source.Url + "//" + pathPrefix + "/" + component.Path + "?ref=" + ref
				break
			}
		}
	}

	blueprint.TerraformComponents = resolvedComponents
}

// resolveComponentPaths determines the full filesystem path for each Terraform component,
// using either the module cache location for remote sources or the project's terraform directory
// for local modules.
func (b *BaseBlueprintHandler) resolveComponentPaths(blueprint *blueprintv1alpha1.Blueprint) {
	projectRoot := b.projectRoot

	resolvedComponents := make([]blueprintv1alpha1.TerraformComponent, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		componentCopy := component

		if b.isValidTerraformRemoteSource(componentCopy.Source) {
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

// loadPlatformTemplate loads a platform-specific template if one exists
func (b *BaseBlueprintHandler) loadPlatformTemplate(platform string) ([]byte, error) {
	if platform == "" {
		return nil, nil
	}

	switch platform {
	case "local":
		return []byte(localJsonnetTemplate), nil
	case "metal":
		return []byte(metalJsonnetTemplate), nil
	case "aws":
		return []byte(awsJsonnetTemplate), nil
	case "azure":
		return []byte(azureJsonnetTemplate), nil
	default:
		return nil, nil
	}
}

// yamlMarshalWithDefinedPaths marshals data to YAML format while ensuring all parent paths are defined.
// It handles various Go types including structs, maps, slices, and primitive types, preserving YAML
// tags and properly representing nil values.
func (b *BaseBlueprintHandler) yamlMarshalWithDefinedPaths(v any) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("invalid input: nil value")
	}

	var convert func(reflect.Value) (any, error)
	convert = func(val reflect.Value) (any, error) {
		switch val.Kind() {
		case reflect.Ptr, reflect.Interface:
			if val.IsNil() {
				if val.Kind() == reflect.Interface || (val.Kind() == reflect.Ptr && val.Type().Elem().Kind() == reflect.Struct) {
					return make(map[string]any), nil
				}
				return nil, nil
			}
			return convert(val.Elem())
		case reflect.Struct:
			result := make(map[string]any)
			typ := val.Type()
			for i := range make([]int, val.NumField()) {
				fieldValue := val.Field(i)
				fieldType := typ.Field(i)

				if fieldType.PkgPath != "" {
					continue
				}

				yamlTag := strings.Split(fieldType.Tag.Get("yaml"), ",")[0]
				if yamlTag == "-" {
					continue
				}
				if yamlTag == "" {
					yamlTag = fieldType.Name
				}

				fieldInterface, err := convert(fieldValue)
				if err != nil {
					return nil, fmt.Errorf("error converting field %s: %w", fieldType.Name, err)
				}
				if fieldInterface != nil || fieldType.Type.Kind() == reflect.Interface || fieldType.Type.Kind() == reflect.Slice || fieldType.Type.Kind() == reflect.Map || fieldType.Type.Kind() == reflect.Struct {
					result[yamlTag] = fieldInterface
				}
			}
			return result, nil
		case reflect.Slice, reflect.Array:
			if val.Len() == 0 {
				return []any{}, nil
			}
			slice := make([]any, val.Len())
			for i := 0; i < val.Len(); i++ {
				elemVal := val.Index(i)
				if elemVal.Kind() == reflect.Ptr || elemVal.Kind() == reflect.Interface {
					if elemVal.IsNil() {
						slice[i] = nil
						continue
					}
				}
				elemInterface, err := convert(elemVal)
				if err != nil {
					return nil, fmt.Errorf("error converting slice element at index %d: %w", i, err)
				}
				slice[i] = elemInterface
			}
			return slice, nil
		case reflect.Map:
			result := make(map[string]any)
			for _, key := range val.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				elemVal := val.MapIndex(key)
				if elemVal.Kind() == reflect.Interface && elemVal.IsNil() {
					result[keyStr] = nil
					continue
				}
				elemInterface, err := convert(elemVal)
				if err != nil {
					return nil, fmt.Errorf("error converting map value for key %s: %w", keyStr, err)
				}
				if elemInterface != nil || elemVal.Kind() == reflect.Interface || elemVal.Kind() == reflect.Slice || elemVal.Kind() == reflect.Map || elemVal.Kind() == reflect.Struct {
					result[keyStr] = elemInterface
				}
			}
			return result, nil
		case reflect.String:
			return val.String(), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return val.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return val.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return val.Float(), nil
		case reflect.Bool:
			return val.Bool(), nil
		default:
			return nil, fmt.Errorf("unsupported value type %s", val.Kind())
		}
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Func {
		return nil, fmt.Errorf("unsupported value type func")
	}

	processed, err := convert(val)
	if err != nil {
		return nil, err
	}

	yamlData, err := b.shims.YamlMarshal(processed)
	if err != nil {
		return nil, fmt.Errorf("error marshalling yaml: %w", err)
	}

	return yamlData, nil
}

func (b *BaseBlueprintHandler) createManagedNamespace(name string) error {
	return b.kubernetesManager.CreateNamespace(name)
}

func (b *BaseBlueprintHandler) deleteNamespace(name string) error {
	return b.kubernetesManager.DeleteNamespace(name)
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

// calculateMaxWaitTime calculates the maximum wait time needed based on kustomization dependencies.
// It builds a dependency graph and uses DFS to find the longest path through it, accumulating
// timeouts for each kustomization in the path. Returns the total time needed for the longest path.
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
				// For circular dependencies, we still want to consider the path
				// but we don't want to recurse further
				if currentTime+timeouts[dep] > maxPathTime {
					maxPathTime = currentTime + timeouts[dep]
				}
			}
		}

		visited[name] = false
		path = path[:len(path)-1]
	}

	// Start DFS from each root node (nodes with no incoming dependencies)
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
		// No roots found (cycle or all nodes have dependencies), start from every node
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

// ToKubernetesKustomization converts a blueprint kustomization to a Flux kustomization
// It handles conversion of dependsOn, patches, and postBuild configurations
// It maps blueprint fields to their Flux kustomization equivalents
// It maintains namespace context and preserves all configuration options
func (b *BaseBlueprintHandler) ToKubernetesKustomization(k blueprintv1alpha1.Kustomization, namespace string) kustomizev1.Kustomization {
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
				Kind: "GitRepository",
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
		},
	}
}
