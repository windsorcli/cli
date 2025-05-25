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

	"github.com/briandowns/spinner"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"

	ctx "context"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
	GetKustomizations() []blueprintv1alpha1.Kustomization
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
	injector       di.Injector
	configHandler  config.ConfigHandler
	shell          shell.Shell
	localBlueprint blueprintv1alpha1.Blueprint
	blueprint      blueprintv1alpha1.Blueprint
	projectRoot    string
	shims          *Shims

	kustomizationWaitPollInterval time.Duration
}

// NewBlueprintHandler creates a new instance of BaseBlueprintHandler.
// It initializes the handler with the provided dependency injector.
func NewBlueprintHandler(injector di.Injector) *BaseBlueprintHandler {
	return &BaseBlueprintHandler{
		injector:                      injector,
		shims:                         NewShims(),
		kustomizationWaitPollInterval: constants.DEFAULT_KUSTOMIZATION_WAIT_POLL_INTERVAL,
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

// WaitForKustomizations polls for readiness of all kustomizations with a maximum timeout.
// It uses a spinner to show progress and checks both GitRepository and Kustomization status.
// The timeout is calculated based on the longest dependency path through the kustomizations.
func (b *BaseBlueprintHandler) WaitForKustomizations(message string, names ...string) error {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	timeout := time.After(b.calculateMaxWaitTime())
	ticker := time.NewTicker(b.kustomizationWaitPollInterval)
	defer ticker.Stop()

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

	consecutiveFailures := 0
	for {
		select {
		case <-timeout:
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - \033[31mFailed\033[0m\n", message)
			return fmt.Errorf("timeout waiting for kustomizations")
		case <-ticker.C:
			kubeconfig := os.Getenv("KUBECONFIG")
			if err := checkGitRepositoryStatus(kubeconfig); err != nil {
				consecutiveFailures++
				if consecutiveFailures >= constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES {
					spin.Stop()
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s - \033[31mFailed\033[0m\n", message)
					return fmt.Errorf("git repository error after %d consecutive failures: %w", consecutiveFailures, err)
				}
				continue
			}
			status, err := checkKustomizationStatus(kubeconfig, kustomizationNames)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures >= constants.DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES {
					spin.Stop()
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s - \033[31mFailed\033[0m\n", message)
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
				fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
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
	context := b.configHandler.GetContext()

	message := "📐 Installing blueprint components"
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	repository := b.GetRepository()
	if repository.Url != "" {
		source := blueprintv1alpha1.Source{
			Name:       context,
			Url:        repository.Url,
			Ref:        repository.Ref,
			SecretName: repository.SecretName,
		}
		if err := b.applyGitRepository(source); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("failed to apply GitRepository: %w", err)
		}
	}

	for _, source := range b.GetSources() {
		if source.Url == "" {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("source URL cannot be empty")
		}
		if err := b.applyGitRepository(source); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("failed to apply GitRepository: %w", err)
		}
	}

	for _, kustomization := range b.GetKustomizations() {
		if err := b.applyKustomization(kustomization, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
			return fmt.Errorf("failed to apply Kustomization: %w", err)
		}
	}

	if err := b.applyConfigMap(); err != nil {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", message)
		return fmt.Errorf("failed to apply ConfigMap: %w", err)
	}

	spin.Stop()
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
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

// GetKustomizations retrieves the blueprint's Kustomization configurations, ensuring default values
// are set for intervals, timeouts, and adding standard PostBuild configurations for variable substitution.
func (b *BaseBlueprintHandler) GetKustomizations() []blueprintv1alpha1.Kustomization {
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

// Down tears down all kustomizations in the correct order, running cleanup kustomizations if defined.
func (b *BaseBlueprintHandler) Down() error {
	kustomizations := b.GetKustomizations()
	if len(kustomizations) == 0 {
		return nil
	}

	// Build dependency graph
	deps := make(map[string][]string)
	for _, k := range kustomizations {
		deps[k.Name] = k.DependsOn
	}

	// Topological sort (reverse order)
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
	// Reverse for teardown order
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	nameToK := make(map[string]blueprintv1alpha1.Kustomization)
	for _, k := range kustomizations {
		nameToK[k.Name] = k
	}

	// Check if we need cleanup namespace
	needsCleanupNamespace := false
	for _, k := range kustomizations {
		if len(k.Cleanup) > 0 {
			needsCleanupNamespace = true
			break
		}
	}

	// Create cleanup namespace if needed
	if needsCleanupNamespace {
		if err := b.createManagedNamespace("system-cleanup"); err != nil {
			return fmt.Errorf("failed to create system-cleanup namespace: %w", err)
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
				PostBuild: &blueprintv1alpha1.PostBuild{
					SubstituteFrom: []blueprintv1alpha1.SubstituteReference{},
				},
			}
			if err := b.applyKustomization(*cleanupKustomization, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
				return fmt.Errorf("failed to apply cleanup kustomization for %s: %w", k.Name, err)
			}
			cleanupNames = append(cleanupNames, cleanupKustomization.Name)
		}
		// Delete the main kustomization
		if err := b.deleteKustomization(k.Name, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
			return fmt.Errorf("failed to delete kustomization %s: %w", k.Name, err)
		}
	}

	if len(cleanupNames) > 0 {
		if err := b.WaitForKustomizations("📐 Deploying cleanup kustomizations", cleanupNames...); err != nil {
			return fmt.Errorf("failed waiting for cleanup kustomizations: %w", err)
		}

		// Delete cleanup kustomizations
		for _, cname := range cleanupNames {
			if err := b.deleteKustomization(cname, constants.DEFAULT_FLUX_SYSTEM_NAMESPACE); err != nil {
				return fmt.Errorf("failed to delete cleanup kustomization %s: %w", cname, err)
			}
		}

		// Delete the cleanup namespace
		if err := b.deleteNamespace("system-cleanup"); err != nil {
			return fmt.Errorf("failed to delete system-cleanup namespace: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// applyKustomization creates or updates a Kustomization resource in the cluster.
func (b *BaseBlueprintHandler) applyKustomization(kustomization blueprintv1alpha1.Kustomization, namespace string) error {
	if kustomization.Source == "" {
		context := b.configHandler.GetContext()
		kustomization.Source = context
	}

	kustomizeObj := &kustomizev1.Kustomization{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kustomize.toolkit.fluxcd.io/v1",
			Kind:       "Kustomization",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kustomization.Name,
			Namespace: namespace,
		},
		Spec: kustomizev1.KustomizationSpec{
			Interval:      *kustomization.Interval,
			Timeout:       kustomization.Timeout,
			RetryInterval: kustomization.RetryInterval,
			Path:          kustomization.Path,
			Prune:         constants.DEFAULT_FLUX_KUSTOMIZATION_PRUNE,
			Wait:          constants.DEFAULT_FLUX_KUSTOMIZATION_WAIT,
			DependsOn: func() []meta.NamespacedObjectReference {
				dependsOn := make([]meta.NamespacedObjectReference, len(kustomization.DependsOn))
				for i, dep := range kustomization.DependsOn {
					dependsOn[i] = meta.NamespacedObjectReference{Name: dep}
				}
				return dependsOn
			}(),
			SourceRef: kustomizev1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      kustomization.Source,
				Namespace: constants.DEFAULT_FLUX_SYSTEM_NAMESPACE,
			},
			Patches:    kustomization.Patches,
			Components: kustomization.Components,
			PostBuild: &kustomizev1.PostBuild{
				SubstituteFrom: func() []kustomizev1.SubstituteReference {
					substituteFrom := make([]kustomizev1.SubstituteReference, len(kustomization.PostBuild.SubstituteFrom))
					for i, sub := range kustomization.PostBuild.SubstituteFrom {
						substituteFrom[i] = kustomizev1.SubstituteReference{
							Kind: sub.Kind,
							Name: sub.Name,
						}
					}
					return substituteFrom
				}(),
			},
		},
	}

	// Ensure the status field is not included in the request body, it breaks the request
	kustomizeObj.Status = kustomizev1.KustomizationStatus{}

	config := ResourceOperationConfig{
		ApiPath:              "/apis/kustomize.toolkit.fluxcd.io/v1",
		Namespace:            namespace,
		ResourceName:         "kustomizations",
		ResourceInstanceName: kustomizeObj.Name,
		ResourceObject:       kustomizeObj,
		ResourceType:         func() runtime.Object { return &kustomizev1.Kustomization{} },
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	return kubeClientResourceOperation(kubeconfig, config)
}

// deleteKustomization deletes a Kustomization resource from the cluster.
func (b *BaseBlueprintHandler) deleteKustomization(name string, namespace string) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	config := ResourceOperationConfig{
		ApiPath:              "/apis/kustomize.toolkit.fluxcd.io/v1",
		Namespace:            namespace,
		ResourceName:         "kustomizations",
		ResourceInstanceName: name,
		ResourceObject:       nil,
		ResourceType:         func() runtime.Object { return &kustomizev1.Kustomization{} },
	}
	return b.deleteResource(kubeconfig, config)
}

// deleteResource deletes a resource from the cluster using the REST client.
func (b *BaseBlueprintHandler) deleteResource(kubeconfigPath string, config ResourceOperationConfig) error {
	return kubeClient(kubeconfigPath, KubeRequestConfig{
		Method:    "DELETE",
		ApiPath:   config.ApiPath,
		Namespace: config.Namespace,
		Resource:  config.ResourceName,
		Name:      config.ResourceInstanceName,
	})
}

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

// =============================================================================
// Helper Functions
// =============================================================================

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
	kubeconfig := os.Getenv("KUBECONFIG")
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "windsor-cli",
			},
		},
	}
	return kubeClient(kubeconfig, KubeRequestConfig{
		Method:   "POST",
		ApiPath:  "/api/v1",
		Resource: "namespaces",
		Body:     ns,
	})
}

func (b *BaseBlueprintHandler) deleteNamespace(name string) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	return kubeClient(kubeconfig, KubeRequestConfig{
		Method:   "DELETE",
		ApiPath:  "/api/v1",
		Resource: "namespaces",
		Name:     name,
	})
}

// applyGitRepository creates or updates a GitRepository resource in the cluster. It normalizes
// the repository URL format, configures standard intervals and timeouts, and handles secret
// references for private repositories.
func (b *BaseBlueprintHandler) applyGitRepository(source blueprintv1alpha1.Source) error {
	sourceUrl := source.Url
	if !strings.HasPrefix(sourceUrl, "http://") && !strings.HasPrefix(sourceUrl, "https://") {
		sourceUrl = "https://" + sourceUrl
	}
	if !strings.HasSuffix(sourceUrl, ".git") {
		sourceUrl = sourceUrl + ".git"
	}

	gitRepository := &sourcev1.GitRepository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "source.toolkit.fluxcd.io/v1",
			Kind:       "GitRepository",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: constants.DEFAULT_FLUX_SYSTEM_NAMESPACE,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: sourceUrl,
			Reference: &sourcev1.GitRepositoryRef{
				Commit: source.Ref.Commit,
				Name:   source.Ref.Name,
				SemVer: source.Ref.SemVer,
				Tag:    source.Ref.Tag,
				Branch: source.Ref.Branch,
			},
			Interval: metav1Duration{Duration: constants.DEFAULT_FLUX_SOURCE_INTERVAL},
			Timeout:  &metav1Duration{Duration: constants.DEFAULT_FLUX_SOURCE_TIMEOUT},
		},
	}

	if source.SecretName != "" {
		gitRepository.Spec.SecretRef = &meta.LocalObjectReference{
			Name: source.SecretName,
		}
	}

	// Ensure the status field is not included in the request body
	gitRepository.Status = sourcev1.GitRepositoryStatus{}

	config := ResourceOperationConfig{
		ApiPath:              "/apis/source.toolkit.fluxcd.io/v1",
		Namespace:            gitRepository.Namespace,
		ResourceName:         "gitrepositories",
		ResourceInstanceName: gitRepository.Name,
		ResourceObject:       gitRepository,
		ResourceType:         func() runtime.Object { return &sourcev1.GitRepository{} },
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	return kubeClientResourceOperation(kubeconfig, config)
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

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blueprint",
			Namespace: constants.DEFAULT_FLUX_SYSTEM_NAMESPACE,
		},
		Data: map[string]string{
			"DOMAIN":                domain,
			"CONTEXT":               context,
			"LOADBALANCER_IP_RANGE": loadBalancerIPRange,
			"LOADBALANCER_IP_START": lbStart,
			"LOADBALANCER_IP_END":   lbEnd,
			"REGISTRY_URL":          registryURL,
			"LOCAL_VOLUME_PATH":     localVolumePath,
		},
	}

	config := ResourceOperationConfig{
		ApiPath:              "/api/v1",
		Namespace:            configMap.Namespace,
		ResourceName:         "configmaps",
		ResourceInstanceName: configMap.Name,
		ResourceObject:       configMap,
		ResourceType: func() runtime.Object {
			return &corev1.ConfigMap{}
		},
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	return kubeClientResourceOperation(kubeconfig, config)
}

// calculateMaxWaitTime calculates the maximum wait time needed based on kustomization dependencies.
// It builds a dependency graph and uses DFS to find the longest path through it, accumulating
// timeouts for each kustomization in the path. Returns the total time needed for the longest path.
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

// =============================================================================
// Kubernetes Client Operations
// =============================================================================

type ResourceOperationConfig struct {
	ApiPath              string
	Namespace            string
	ResourceName         string
	ResourceInstanceName string
	ResourceObject       runtime.Object
	ResourceType         func() runtime.Object
}

type KubeRequestConfig struct {
	Method    string
	ApiPath   string
	Namespace string
	Resource  string
	Name      string
	Body      interface{}
}

var kubeClient = func(kubeconfigPath string, config KubeRequestConfig) error {
	var kubeConfig *rest.Config
	var err error

	if kubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	restClient := clientset.CoreV1().RESTClient()
	backgroundCtx := ctx.Background()

	req := restClient.Verb(config.Method).
		AbsPath(config.ApiPath).
		Resource(config.Resource)

	if config.Namespace != "" {
		req = req.Namespace(config.Namespace)
	}

	if config.Name != "" {
		req = req.Name(config.Name)
	}

	if config.Body != nil {
		req = req.Body(config.Body)
	}

	return req.Do(backgroundCtx).Error()
}

var kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
	// First try to get the resource
	err := kubeClient(kubeconfigPath, KubeRequestConfig{
		Method:    "GET",
		ApiPath:   config.ApiPath,
		Namespace: config.Namespace,
		Resource:  config.ResourceName,
		Name:      config.ResourceInstanceName,
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create if not found
			return kubeClient(kubeconfigPath, KubeRequestConfig{
				Method:    "POST",
				ApiPath:   config.ApiPath,
				Namespace: config.Namespace,
				Resource:  config.ResourceName,
				Body:      config.ResourceObject,
			})
		}
		return fmt.Errorf("failed to get resource: %w", err)
	}

	// Update if found
	return kubeClient(kubeconfigPath, KubeRequestConfig{
		Method:    "PUT",
		ApiPath:   config.ApiPath,
		Namespace: config.Namespace,
		Resource:  config.ResourceName,
		Name:      config.ResourceInstanceName,
		Body:      config.ResourceObject,
	})
}

// NOTE: This is a temporary solution until we've integrated the kube client into our DI system.
// As such, this function is not internally covered by our tests.
//
// checkKustomizationStatus checks the status of all kustomizations in the cluster by name.
// It returns a map of kustomization names to their readiness (true if ready, false otherwise).
// If any kustomization is missing or has failed, it returns an error. The function queries all
// kustomizations in the default namespace, converts them to typed objects, and inspects their
// status conditions for readiness or failure. It ensures all requested kustomizations are present.
var checkKustomizationStatus = func(kubeconfigPath string, names []string) (map[string]bool, error) {
	var kubeConfig *rest.Config
	var err error

	if kubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	objList, err := dynamicClient.Resource(gvr).Namespace(constants.DEFAULT_FLUX_SYSTEM_NAMESPACE).
		List(ctx.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list kustomizations: %w", err)
	}

	status := make(map[string]bool)
	found := make(map[string]bool)

	for _, obj := range objList.Items {
		var kustomizeObj kustomizev1.Kustomization
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &kustomizeObj); err != nil {
			return nil, fmt.Errorf("failed to convert kustomization %s: %w", kustomizeObj.Name, err)
		}

		found[kustomizeObj.Name] = true
		ready := false
		for _, condition := range kustomizeObj.Status.Conditions {
			if condition.Type == "Ready" {
				if condition.Status == "True" {
					ready = true
				} else if condition.Status == "False" && condition.Reason == "ReconciliationFailed" {
					return nil, fmt.Errorf("kustomization %s failed: %s", kustomizeObj.Name, condition.Message)
				}
				break
			}
		}
		status[kustomizeObj.Name] = ready
	}

	for _, name := range names {
		if !found[name] {
			status[name] = false
			continue
		}
	}

	return status, nil
}

// NOTE: This is a temporary solution until we've integrated the kube client into our DI system.
// As such, this function is not internally covered by our tests.
//
// checkGitRepositoryStatus checks the status of all GitRepository resources in the cluster.
// It returns an error if any repository is not ready or has failed. The function queries all
// GitRepository resources in the default namespace, converts them to typed objects, and inspects
// their status conditions for readiness or failure. If any repository is not ready, it returns an error
// with the repository name and failure message.
var checkGitRepositoryStatus = func(kubeconfigPath string) error {
	var kubeConfig *rest.Config
	var err error

	if kubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	objList, err := dynamicClient.Resource(gvr).Namespace(constants.DEFAULT_FLUX_SYSTEM_NAMESPACE).
		List(ctx.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list git repositories: %w", err)
	}

	for _, obj := range objList.Items {
		var gitRepo sourcev1.GitRepository
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &gitRepo); err != nil {
			return fmt.Errorf("failed to convert git repository %s: %w", gitRepo.Name, err)
		}

		for _, condition := range gitRepo.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "False" {
				return fmt.Errorf("%s: %s", gitRepo.Name, condition.Message)
			}
		}
	}

	return nil
}
