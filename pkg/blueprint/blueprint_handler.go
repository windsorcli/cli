package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	LoadConfig(path ...string) error
	WriteConfig(path ...string) error
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
}

//go:embed templates/default.jsonnet
var defaultJsonnetTemplate string

type BaseBlueprintHandler struct {
	BlueprintHandler
	injector       di.Injector
	configHandler  config.ConfigHandler
	shell          shell.Shell
	localBlueprint blueprintv1alpha1.Blueprint
	blueprint      blueprintv1alpha1.Blueprint
	projectRoot    string
	shims          *Shims
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

// LoadConfig reads and processes blueprint configuration from either a specified path or the default location.
// It supports both Jsonnet and YAML formats, evaluates any Jsonnet templates with the current context,
// and merges local blueprint data. The function handles default blueprints when no config exists.
func (b *BaseBlueprintHandler) LoadConfig(path ...string) error {
	configRoot, err := b.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	basePath := filepath.Join(configRoot, "blueprint")
	if len(path) > 0 && path[0] != "" {
		basePath = path[0]
	}

	jsonnetData, jsonnetErr := b.loadFileData(basePath + ".jsonnet")
	yamlData, yamlErr := b.loadFileData(basePath + ".yaml")
	if jsonnetErr != nil {
		return jsonnetErr
	}
	if yamlErr != nil && !os.IsNotExist(yamlErr) {
		return yamlErr
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

	// Add "name" to the context map
	context := b.configHandler.GetContext()
	contextMap["name"] = context

	contextJSON, err := b.shims.JsonMarshal(contextMap)
	if err != nil {
		return fmt.Errorf("error marshalling context map to JSON: %w", err)
	}

	var evaluatedJsonnet string

	vm := b.shims.NewJsonnetVM()
	vm.ExtCode("context", string(contextJSON))

	if len(jsonnetData) > 0 {
		evaluatedJsonnet, err = vm.EvaluateAnonymousSnippet("blueprint.jsonnet", string(jsonnetData))
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

	if len(yamlData) > 0 {
		if err := b.processBlueprintData(yamlData, &b.localBlueprint); err != nil {
			return err
		}
	}

	b.blueprint.Merge(&b.localBlueprint)

	return nil
}

// WriteConfig persists the current blueprint configuration to disk. It handles path resolution,
// directory creation, and writes the blueprint in YAML format. The function cleans sensitive or
// redundant data before writing, such as Terraform component variables/values and empty PostBuild configs.
func (b *BaseBlueprintHandler) WriteConfig(path ...string) error {
	finalPath := ""
	if len(path) > 0 && path[0] != "" {
		finalPath = path[0]
	} else {
		configRoot, err := b.configHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error getting config root: %w", err)
		}
		finalPath = filepath.Join(configRoot, "blueprint.yaml")
	}

	dir := filepath.Dir(finalPath)
	if err := b.shims.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
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
		if err := b.applyKustomization(kustomization); err != nil {
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
			kustomizations[i].Path = filepath.Join("kustomize", kustomizations[i].Path)
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

// loadFileData loads the file data from the specified path.
// It checks if the file exists and reads its content, returning the data as a byte slice.
func (b *BaseBlueprintHandler) loadFileData(path string) ([]byte, error) {
	if _, err := b.shims.Stat(path); err == nil {
		return b.shims.ReadFile(path)
	}
	return nil, nil
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

// NOTE: This is a temporary solution until we've integrated the kube client into our DI system.
// As such, this function is not internally covered by our tests.
//
// kubeClientResourceOperation is a comprehensive function that handles the entire lifecycle of creating a Kubernetes client
// and performing a sequence of operations (Get, Post, Put) on Kubernetes resources. It takes a kubeconfig path and a
// configuration object that specifies the parameters for the operations.
var kubeClientResourceOperation = func(kubeconfigPath string, config ResourceOperationConfig) error {
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

	existingResource := config.ResourceType().(runtime.Object)
	err = restClient.Get().
		AbsPath(config.ApiPath).
		Namespace(config.Namespace).
		Resource(config.ResourceName).
		Name(config.ResourceInstanceName).
		Do(backgroundCtx).
		Into(existingResource)

	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := restClient.Post().
				AbsPath(config.ApiPath).
				Namespace(config.Namespace).
				Resource(config.ResourceName).
				Body(config.ResourceObject).
				Do(backgroundCtx).
				Error(); err != nil {
				return fmt.Errorf("failed to create resource: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get resource: %w", err)
		}
	} else {
		// Ensure the resourceVersion is set for the update
		config.ResourceObject.(metav1.Object).SetResourceVersion(existingResource.(metav1.Object).GetResourceVersion())

		if err := restClient.Put().
			AbsPath(config.ApiPath).
			Namespace(config.Namespace).
			Resource(config.ResourceName).
			Name(config.ResourceInstanceName).
			Body(config.ResourceObject).
			Do(backgroundCtx).
			Error(); err != nil {
			return fmt.Errorf("failed to update resource: %w", err)
		}
	}

	return nil
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

// applyKustomization creates or updates a Kustomization resource in the cluster. It configures
// dependencies, source references, and PostBuild substitutions while applying standard defaults
// for intervals and operational parameters.
func (b *BaseBlueprintHandler) applyKustomization(kustomization blueprintv1alpha1.Kustomization) error {
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
			Namespace: constants.DEFAULT_FLUX_SYSTEM_NAMESPACE,
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
		Namespace:            kustomizeObj.Namespace,
		ResourceName:         "kustomizations",
		ResourceInstanceName: kustomizeObj.Name,
		ResourceObject:       kustomizeObj,
		ResourceType:         func() runtime.Object { return &kustomizev1.Kustomization{} },
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
