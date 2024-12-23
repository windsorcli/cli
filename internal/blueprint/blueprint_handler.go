package blueprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	_ "embed"

	"github.com/windsorcli/cli/internal/config"
	"github.com/windsorcli/cli/internal/context"
	"github.com/windsorcli/cli/internal/di"
	"github.com/windsorcli/cli/internal/shell"
)

// BlueprintHandler defines the interface for handling blueprint operations
type BlueprintHandler interface {
	// Initialize initializes the blueprint handler
	Initialize() error

	// LoadConfig loads the blueprint from the specified path
	LoadConfig(path ...string) error

	// GetMetadata retrieves the metadata for the blueprint
	GetMetadata() MetadataV1Alpha1

	// GetSources retrieves the sources for the blueprint
	GetSources() []SourceV1Alpha1

	// GetTerraformComponents retrieves the Terraform components for the blueprint
	GetTerraformComponents() []TerraformComponentV1Alpha1

	// SetMetadata sets the metadata for the blueprint
	SetMetadata(metadata MetadataV1Alpha1) error

	// SetSources sets the sources for the blueprint
	SetSources(sources []SourceV1Alpha1) error

	// SetTerraformComponents sets the Terraform components for the blueprint
	SetTerraformComponents(terraformComponents []TerraformComponentV1Alpha1) error

	// WriteConfig writes the current blueprint to the specified path
	WriteConfig(path ...string) error
}

// BaseBlueprintHandler is a base implementation of the BlueprintHandler interface
type BaseBlueprintHandler struct {
	BlueprintHandler
	injector       di.Injector
	contextHandler context.ContextHandler
	configHandler  config.ConfigHandler
	shell          shell.Shell
	localBlueprint BlueprintV1Alpha1
	blueprint      BlueprintV1Alpha1
	projectRoot    string
}

// Create a new blueprint handler
func NewBlueprintHandler(injector di.Injector) *BaseBlueprintHandler {
	return &BaseBlueprintHandler{injector: injector}
}

// Initialize initializes the blueprint handler
func (b *BaseBlueprintHandler) Initialize() error {
	// Resolve the config handler
	configHandler, ok := b.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	b.configHandler = configHandler

	// Resolve the context handler
	contextHandler, ok := b.injector.Resolve("contextHandler").(context.ContextHandler)
	if !ok {
		return fmt.Errorf("error resolving contextHandler")
	}
	b.contextHandler = contextHandler

	// Resolve the shell
	shell, ok := b.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	b.shell = shell

	// Get the project root
	projectRoot, err := b.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("error getting project root: %w", err)
	}
	b.projectRoot = projectRoot

	return nil
}

//go:embed templates/local.jsonnet
var localJsonnetTemplate string

// LoadConfig Loads the blueprint from the specified path
func (b *BaseBlueprintHandler) LoadConfig(path ...string) error {
	// Get the config root
	configRoot, err := b.contextHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	// Get the blueprint path
	basePath := filepath.Join(configRoot, "blueprint")
	if len(path) > 0 && path[0] != "" {
		basePath = path[0]
	}

	// Get the jsonnet and yaml paths
	jsonnetPath, yamlPath := basePath+".jsonnet", basePath+".yaml"

	// Attempt to load the Jsonnet file first
	var jsonnetData []byte
	if _, err := osStat(jsonnetPath); err == nil {
		jsonnetData, err = osReadFile(jsonnetPath)
		if err != nil {
			return fmt.Errorf("error reading file %s: %w", jsonnetPath, err)
		}
	}

	// Attempt to load the YAML file
	var yamlData []byte
	if _, err := osStat(yamlPath); err == nil {
		yamlData, err = osReadFile(yamlPath)
		if err != nil {
			return fmt.Errorf("error reading file %s: %w", yamlPath, err)
		}
	}

	// Determine which data to use for generating the blueprint
	var evaluatedJsonnet string
	template := ""
	if len(jsonnetData) > 0 {
		template = string(jsonnetData)
	} else if b.blueprint.Metadata.Name == "" && strings.HasPrefix(b.contextHandler.GetContext(), "local") {
		template = localJsonnetTemplate
	}

	// Generate the blueprint from the template
	if template != "" {
		evaluatedJsonnet, err = generateBlueprintFromJsonnet(b.configHandler.GetConfig(), template)
		if err != nil {
			return fmt.Errorf("error generating blueprint from jsonnet: %w", err)
		}
	}

	if len(evaluatedJsonnet) > 0 {
		if err := yamlUnmarshal([]byte(evaluatedJsonnet), &b.blueprint); err != nil {
			return fmt.Errorf("error unmarshalling jsonnet data: %w", err)
		}
	} else if len(yamlData) > 0 {
		if err := yamlUnmarshal(yamlData, &b.localBlueprint); err != nil {
			return fmt.Errorf("error unmarshalling yaml data: %w", err)
		}
	}

	mergeBlueprints(&b.blueprint, &b.localBlueprint)

	if b.blueprint.Metadata.Name == "" {
		b.blueprint = DefaultBlueprint
		context := b.contextHandler.GetContext()
		b.blueprint.Metadata.Name = context
		b.blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", context)
	}

	return nil
}

// WriteConfig writes the current blueprint to a specified path or a default location
func (b *BaseBlueprintHandler) WriteConfig(path ...string) error {
	finalPath := ""
	// Determine the final path to save the blueprint
	if len(path) > 0 && path[0] != "" {
		finalPath = path[0]
	} else {
		configRoot, err := b.contextHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error getting config root: %w", err)
		}
		finalPath = filepath.Join(configRoot, "blueprint.yaml")
	}

	// Ensure the parent directory exists
	dir := filepath.Dir(finalPath)
	if err := osMkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	// Create a copy of the blueprint to avoid modifying the original
	fullBlueprint := b.blueprint.deepCopy()

	// Remove "variables" and "values" sections from all terraform components in the full blueprint
	for i := range fullBlueprint.TerraformComponents {
		fullBlueprint.TerraformComponents[i].Variables = nil
		fullBlueprint.TerraformComponents[i].Values = nil
	}

	// Merge the local blueprint into the full blueprint, giving precedence to the local blueprint
	mergeBlueprints(fullBlueprint, &b.localBlueprint)

	// Convert the merged blueprint struct into YAML format, omitting null values
	data, err := yamlMarshalNonNull(fullBlueprint)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

	// Write the YAML data to the determined path with appropriate permissions
	if err := osWriteFile(finalPath, data, 0644); err != nil {
		return fmt.Errorf("error writing blueprint file: %w", err)
	}
	return nil
}

// GetMetadata retrieves the metadata for the blueprint
func (b *BaseBlueprintHandler) GetMetadata() MetadataV1Alpha1 {
	// Create a copy of the blueprint to avoid modifying the original
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Metadata
}

// GetSources retrieves the sources for the blueprint
func (b *BaseBlueprintHandler) GetSources() []SourceV1Alpha1 {
	// Create a copy of the blueprint to avoid modifying the original
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Sources
}

// GetTerraformComponents retrieves the Terraform components for the blueprint
func (b *BaseBlueprintHandler) GetTerraformComponents() []TerraformComponentV1Alpha1 {
	// Create a copy of the blueprint to avoid modifying the original
	resolvedBlueprint := b.blueprint

	// Resolve the component sources
	b.resolveComponentSources(&resolvedBlueprint)

	// Resolve the component paths
	b.resolveComponentPaths(&resolvedBlueprint)

	return resolvedBlueprint.TerraformComponents
}

// SetMetadata sets the metadata for the blueprint
func (b *BaseBlueprintHandler) SetMetadata(metadata MetadataV1Alpha1) error {
	b.blueprint.Metadata = metadata
	return nil
}

// SetSources sets the sources for the blueprint
func (b *BaseBlueprintHandler) SetSources(sources []SourceV1Alpha1) error {
	b.blueprint.Sources = sources
	return nil
}

// SetTerraformComponents sets the Terraform components for the blueprint
func (b *BaseBlueprintHandler) SetTerraformComponents(terraformComponents []TerraformComponentV1Alpha1) error {
	b.blueprint.TerraformComponents = terraformComponents
	return nil
}

// resolveComponentSources resolves the source for each Terraform component
func (b *BaseBlueprintHandler) resolveComponentSources(blueprint *BlueprintV1Alpha1) {
	// Create a copy of the TerraformComponents to avoid modifying the original components
	resolvedComponents := make([]TerraformComponentV1Alpha1, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		for _, source := range blueprint.Sources {
			if component.Source == source.Name {
				pathPrefix := source.PathPrefix
				if pathPrefix == "" {
					pathPrefix = "terraform"
				}
				resolvedComponents[i].Source = source.Url + "//" + pathPrefix + "/" + component.Path + "?ref=" + source.Ref
				break
			}
		}
	}

	// Replace the original components with the resolved ones
	blueprint.TerraformComponents = resolvedComponents
}

// resolveComponentPaths resolves the path for each Terraform component
func (b *BaseBlueprintHandler) resolveComponentPaths(blueprint *BlueprintV1Alpha1) {
	projectRoot := b.projectRoot

	// Create a copy of the TerraformComponents to avoid modifying the original components
	resolvedComponents := make([]TerraformComponentV1Alpha1, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		// Create a copy of the component to avoid modifying the original component
		componentCopy := component

		if isValidTerraformRemoteSource(componentCopy.Source) {
			componentCopy.FullPath = filepath.Join(projectRoot, ".tf_modules", componentCopy.Path)
		} else {
			componentCopy.FullPath = filepath.Join(projectRoot, "terraform", componentCopy.Path)
		}

		// Normalize FullPath
		componentCopy.FullPath = filepath.FromSlash(componentCopy.FullPath)

		// Update the resolved component in the slice
		resolvedComponents[i] = componentCopy
	}

	// Replace the original components with the resolved ones
	blueprint.TerraformComponents = resolvedComponents
}

// deepCopy creates a deep copy of the Blueprint
func (b *BlueprintV1Alpha1) deepCopy() *BlueprintV1Alpha1 {
	// Create a new Blueprint instance
	copy := *b

	// Use reflection to copy each slice field generically
	val := reflect.ValueOf(b).Elem()
	copyVal := reflect.ValueOf(&copy).Elem()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		copyField := copyVal.Field(i)

		if field.Kind() == reflect.Slice && !field.IsNil() {
			copyField.Set(reflect.MakeSlice(field.Type(), field.Len(), field.Cap()))
			reflect.Copy(copyField, field)
		}
	}

	return &copy
}

// Ensure that BaseBlueprintHandler implements the BlueprintHandler interface
var _ BlueprintHandler = &BaseBlueprintHandler{}

// isValidTerraformRemoteSource checks if the source is a valid Terraform module reference
var isValidTerraformRemoteSource = func(source string) bool {
	// Define patterns for different valid source types
	patterns := []string{
		`^git::https://[^/]+/.*\.git(?:@.*)?$`, // Generic Git URL with .git suffix
		`^git@[^:]+:.*\.git(?:@.*)?$`,          // Generic SSH Git URL with .git suffix
		`^https?://[^/]+/.*\.git(?:@.*)?$`,     // HTTP URL with .git suffix
		`^https?://[^/]+/.*\.zip(?:@.*)?$`,     // HTTP URL pointing to a .zip archive
		`^https?://[^/]+/.*//.*(?:@.*)?$`,      // HTTP URL with double slashes and optional ref
		`^registry\.terraform\.io/.*`,          // Terraform Registry
		`^[^/]+\.com/.*`,                       // Generic domain reference
	}

	// Check if the source matches any of the valid patterns
	for _, pattern := range patterns {
		matched, err := regexpMatchString(pattern, source)
		if err != nil {
			return false
		}
		if matched {
			return true
		}
	}

	return false
}

// generateBlueprintFromJsonnet generates a blueprint from a jsonnet template
var generateBlueprintFromJsonnet = func(contextConfig *config.Context, jsonnetTemplate string) (string, error) {
	// Convert contextConfig to JSON
	yamlBytes, err := yamlMarshal(contextConfig)
	if err != nil {
		return "", err
	}
	jsonBytes, err := yamlToJson(yamlBytes)
	if err != nil {
		return "", err
	}

	// Build the snippet to define a local context object
	snippetWithContext := fmt.Sprintf(`
local context = %s;
%s
`, string(jsonBytes), jsonnetTemplate)

	// Evaluate the snippet with the Jsonnet VM
	vm := jsonnetMakeVM()
	evaluatedJsonnet, err := vm.EvaluateAnonymousSnippet("blueprint", snippetWithContext)
	if err != nil {
		return "", err
	}

	// Convert JSON to YAML
	yamlOutput, err := yamlJSONToYAML([]byte(evaluatedJsonnet))
	if err != nil {
		return "", err
	}

	return string(yamlOutput), nil
}

// Convert YAML (as []byte) to JSON (as []byte)
var yamlToJson = func(yamlBytes []byte) ([]byte, error) {
	var data interface{}
	if err := yamlUnmarshal(yamlBytes, &data); err != nil {
		return nil, err
	}
	return json.Marshal(data)
}

// mergeBlueprints merges fields from src into dst, giving precedence to src.
//
// This helps ensure map fields (like Variables and Values) and other struct fields
// are handled more reliably without relying on reflection or intermediate map conversions.
var mergeBlueprints = func(dst, src *BlueprintV1Alpha1) {
	if src == nil {
		return
	}

	// Merge top-level fields
	if src.Kind != "" {
		dst.Kind = src.Kind
	}
	if src.ApiVersion != "" {
		dst.ApiVersion = src.ApiVersion
	}

	// Merge Metadata
	if src.Metadata.Name != "" {
		dst.Metadata.Name = src.Metadata.Name
	}
	if src.Metadata.Description != "" {
		dst.Metadata.Description = src.Metadata.Description
	}
	if len(src.Metadata.Authors) > 0 {
		dst.Metadata.Authors = src.Metadata.Authors
	}

	// Merge Sources
	if len(src.Sources) > 0 {
		dst.Sources = src.Sources
	}

	// Merge TerraformComponents
	if len(src.TerraformComponents) > 0 {
		for _, srcComp := range src.TerraformComponents {
			found := false
			for i, dstComp := range dst.TerraformComponents {
				// Identify matching components by Source+Path
				if dstComp.Source == srcComp.Source && dstComp.Path == srcComp.Path {
					// Merge variables
					for k, v := range srcComp.Variables {
						dstComp.Variables[k] = v
					}
					// Merge values
					if dstComp.Values == nil {
						dstComp.Values = make(map[string]interface{})
					}
					for k, v := range srcComp.Values {
						dstComp.Values[k] = v
					}
					// Update other fields if they are non-zero in src
					if srcComp.FullPath != "" {
						dstComp.FullPath = srcComp.FullPath
					}
					dst.TerraformComponents[i] = dstComp
					found = true
					break
				}
			}
			// If there's no matching component, append it
			if !found {
				dst.TerraformComponents = append(dst.TerraformComponents, srcComp)
			}
		}
	}
}
