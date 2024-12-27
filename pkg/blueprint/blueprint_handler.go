package blueprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	_ "embed"

	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/context"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
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

// Initialize sets up the blueprint handler by resolving dependencies
// and configuring the project environment. It resolves the config handler,
// context handler, and shell, assigning them to the blueprint handler.
// It also retrieves and sets the project root directory.
func (b *BaseBlueprintHandler) Initialize() error {
	configHandler, ok := b.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	b.configHandler = configHandler

	contextHandler, ok := b.injector.Resolve("contextHandler").(context.ContextHandler)
	if !ok {
		return fmt.Errorf("error resolving contextHandler")
	}
	b.contextHandler = contextHandler

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

	return nil
}

//go:embed templates/local.jsonnet
var localJsonnetTemplate string

// LoadConfig loads a blueprint from a path, using Jsonnet or YAML data.
func (b *BaseBlueprintHandler) LoadConfig(path ...string) error {
	// Retrieve the configuration root
	configRoot, err := b.contextHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	// Determine the blueprint path
	basePath := filepath.Join(configRoot, "blueprint")
	if len(path) > 0 && path[0] != "" {
		basePath = path[0]
	}

	// Load Jsonnet and YAML data
	jsonnetData, jsonnetErr := loadFileData(basePath + ".jsonnet")
	yamlData, yamlErr := loadFileData(basePath + ".yaml")
	if jsonnetErr != nil {
		return jsonnetErr
	}
	if yamlErr != nil && !os.IsNotExist(yamlErr) {
		return yamlErr
	}

	// Retrieve the configuration and marshal it to YAML
	config := b.configHandler.GetConfig()
	contextYAML, err := yamlMarshalWithDefinedPaths(config)
	if err != nil {
		return fmt.Errorf("error marshalling context to YAML: %w", err)
	}

	// Unmarshal the YAML back into a generic map
	var contextMap map[string]interface{}
	if err := yamlUnmarshal(contextYAML, &contextMap); err != nil {
		return fmt.Errorf("error unmarshalling context YAML to map: %w", err)
	}

	// Marshal the map to JSON
	contextJSON, err := jsonMarshal(contextMap)
	if err != nil {
		return fmt.Errorf("error marshalling context map to JSON: %w", err)
	}

	var evaluatedJsonnet string
	context := b.contextHandler.GetContext()

	// Process Jsonnet data if available
	if len(jsonnetData) > 0 {
		// Evaluate Jsonnet data
		vm := jsonnetMakeVM()
		vm.ExtCode("context", string(contextJSON))

		evaluatedJsonnet, err = vm.EvaluateAnonymousSnippet("blueprint.jsonnet", string(jsonnetData))
		if err != nil {
			return fmt.Errorf("error generating blueprint from jsonnet: %w", err)
		}
	} else if strings.HasPrefix(context, "local") {
		// Load local Jsonnet template
		vm := jsonnetMakeVM()
		vm.ExtCode("context", string(contextJSON))

		evaluatedJsonnet, err = vm.EvaluateAnonymousSnippet("local.jsonnet", localJsonnetTemplate)
		if err != nil {
			return fmt.Errorf("error generating blueprint from local jsonnet: %w", err)
		}
	}

	// Load default blueprint if no Jsonnet data was processed, else unmarshal evaluated Jsonnet data
	if evaluatedJsonnet == "" {
		b.blueprint = DefaultBlueprint
		b.blueprint.Metadata.Name = context
		b.blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", context)
	} else {
		if err := yamlUnmarshal([]byte(evaluatedJsonnet), &b.blueprint); err != nil {
			return fmt.Errorf("error unmarshalling jsonnet data: %w", err)
		}
	}

	// Unmarshal YAML data if present
	if len(yamlData) > 0 {
		if err := yamlUnmarshal(yamlData, &b.localBlueprint); err != nil {
			return fmt.Errorf("error unmarshalling yaml data: %w", err)
		}
	}

	// Merge local blueprint into the main blueprint
	mergeBlueprints(&b.blueprint, &b.localBlueprint)

	return nil
}

// WriteConfig saves the current blueprint to a specified file path or defaults to a standard location if no path is provided.
// It ensures the directory structure exists, creates a deep copy of the blueprint, removes variables and values,
// merges local blueprint data, and writes the final YAML representation to the file system.
func (b *BaseBlueprintHandler) WriteConfig(path ...string) error {
	finalPath := ""
	if len(path) > 0 && path[0] != "" {
		finalPath = path[0]
	} else {
		configRoot, err := b.contextHandler.GetConfigRoot()
		if err != nil {
			return fmt.Errorf("error getting config root: %w", err)
		}
		finalPath = filepath.Join(configRoot, "blueprint.yaml")
	}

	dir := filepath.Dir(finalPath)
	if err := osMkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	fullBlueprint := b.blueprint.deepCopy()

	for i := range fullBlueprint.TerraformComponents {
		fullBlueprint.TerraformComponents[i].Variables = nil
		fullBlueprint.TerraformComponents[i].Values = nil
	}

	mergeBlueprints(fullBlueprint, &b.localBlueprint)

	data, err := yamlMarshalNonNull(fullBlueprint)
	if err != nil {
		return fmt.Errorf("error marshalling yaml: %w", err)
	}

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

// loadFileData loads the file data from the specified path
var loadFileData = func(path string) ([]byte, error) {
	if _, err := osStat(path); err == nil {
		return osReadFile(path)
	}
	return nil, nil
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

// yamlMarshalWithDefinedPaths marshals YAML ensuring all parent paths are defined.
func yamlMarshalWithDefinedPaths(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("invalid input: nil value")
	}

	var convert func(reflect.Value) (interface{}, error)
	convert = func(val reflect.Value) (interface{}, error) {
		switch val.Kind() {
		case reflect.Ptr, reflect.Interface:
			if val.IsNil() {
				// Handle nil pointers to empty structs
				if val.Kind() == reflect.Interface || (val.Kind() == reflect.Ptr && val.Type().Elem().Kind() == reflect.Struct) {
					return make(map[string]interface{}), nil
				}
				return nil, nil
			}
			return convert(val.Elem())
		case reflect.Struct:
			result := make(map[string]interface{})
			typ := val.Type()
			for i := 0; i < val.NumField(); i++ {
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
				return []interface{}{}, nil
			}
			slice := make([]interface{}, val.Len())
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
			result := make(map[string]interface{})
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

	yamlData, err := yamlMarshal(processed)
	if err != nil {
		return nil, fmt.Errorf("error marshalling yaml: %w", err)
	}

	return yamlData, nil
}
