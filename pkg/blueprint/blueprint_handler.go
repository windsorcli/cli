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
	Initialize() error
	LoadConfig(path ...string) error
	GetMetadata() MetadataV1Alpha1
	GetSources() []SourceV1Alpha1
	GetTerraformComponents() []TerraformComponentV1Alpha1
	SetMetadata(metadata MetadataV1Alpha1) error
	SetSources(sources []SourceV1Alpha1) error
	SetTerraformComponents(terraformComponents []TerraformComponentV1Alpha1) error
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
	configRoot, err := b.contextHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("error getting config root: %w", err)
	}

	basePath := filepath.Join(configRoot, "blueprint")
	if len(path) > 0 && path[0] != "" {
		basePath = path[0]
	}

	jsonnetData, jsonnetErr := loadFileData(basePath + ".jsonnet")
	yamlData, yamlErr := loadFileData(basePath + ".yaml")
	if jsonnetErr != nil {
		return jsonnetErr
	}
	if yamlErr != nil && !os.IsNotExist(yamlErr) {
		return yamlErr
	}

	config := b.configHandler.GetConfig()
	contextYAML, err := yamlMarshalWithDefinedPaths(config)
	if err != nil {
		return fmt.Errorf("error marshalling context to YAML: %w", err)
	}

	var contextMap map[string]interface{}
	if err := yamlUnmarshal(contextYAML, &contextMap); err != nil {
		return fmt.Errorf("error unmarshalling context YAML to map: %w", err)
	}

	contextJSON, err := jsonMarshal(contextMap)
	if err != nil {
		return fmt.Errorf("error marshalling context map to JSON: %w", err)
	}

	var evaluatedJsonnet string
	context := b.contextHandler.GetContext()

	if len(jsonnetData) > 0 {
		vm := jsonnetMakeVM()
		vm.ExtCode("context", string(contextJSON))

		evaluatedJsonnet, err = vm.EvaluateAnonymousSnippet("blueprint.jsonnet", string(jsonnetData))
		if err != nil {
			return fmt.Errorf("error generating blueprint from jsonnet: %w", err)
		}
	} else if strings.HasPrefix(context, "local") {
		vm := jsonnetMakeVM()
		vm.ExtCode("context", string(contextJSON))

		evaluatedJsonnet, err = vm.EvaluateAnonymousSnippet("local.jsonnet", localJsonnetTemplate)
		if err != nil {
			return fmt.Errorf("error generating blueprint from local jsonnet: %w", err)
		}
	}

	if evaluatedJsonnet == "" {
		b.blueprint = *DefaultBlueprint.Copy()
		b.blueprint.Metadata.Name = context
		b.blueprint.Metadata.Description = fmt.Sprintf("This blueprint outlines resources in the %s context", context)
	} else {
		newBlueprint := &BlueprintV1Alpha1{}
		if err := yamlUnmarshal([]byte(evaluatedJsonnet), newBlueprint); err != nil {
			return fmt.Errorf("error unmarshalling jsonnet data: %w", err)
		}
		b.blueprint.Merge(newBlueprint)
	}

	if len(yamlData) > 0 {
		newLocalBlueprint := &BlueprintV1Alpha1{}
		if err := yamlUnmarshal(yamlData, newLocalBlueprint); err != nil {
			return fmt.Errorf("error unmarshalling yaml data: %w", err)
		}
		b.localBlueprint.Merge(newLocalBlueprint)
	}

	b.blueprint.Merge(&b.localBlueprint)

	return nil
}

// WriteConfig saves the current blueprint to a specified file path
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

	fullBlueprint := b.blueprint.Copy()

	for i := range fullBlueprint.TerraformComponents {
		fullBlueprint.TerraformComponents[i].Variables = nil
		fullBlueprint.TerraformComponents[i].Values = nil
	}

	fullBlueprint.Merge(&b.localBlueprint)

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
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Metadata
}

// GetSources retrieves the sources for the blueprint
func (b *BaseBlueprintHandler) GetSources() []SourceV1Alpha1 {
	resolvedBlueprint := b.blueprint
	return resolvedBlueprint.Sources
}

// GetTerraformComponents retrieves the Terraform components for the blueprint
func (b *BaseBlueprintHandler) GetTerraformComponents() []TerraformComponentV1Alpha1 {
	resolvedBlueprint := b.blueprint

	b.resolveComponentSources(&resolvedBlueprint)
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

	blueprint.TerraformComponents = resolvedComponents
}

// resolveComponentPaths resolves the path for each Terraform component
func (b *BaseBlueprintHandler) resolveComponentPaths(blueprint *BlueprintV1Alpha1) {
	projectRoot := b.projectRoot

	resolvedComponents := make([]TerraformComponentV1Alpha1, len(blueprint.TerraformComponents))
	copy(resolvedComponents, blueprint.TerraformComponents)

	for i, component := range resolvedComponents {
		componentCopy := component

		if isValidTerraformRemoteSource(componentCopy.Source) {
			componentCopy.FullPath = filepath.Join(projectRoot, ".tf_modules", componentCopy.Path)
		} else {
			componentCopy.FullPath = filepath.Join(projectRoot, "terraform", componentCopy.Path)
		}

		componentCopy.FullPath = filepath.FromSlash(componentCopy.FullPath)

		resolvedComponents[i] = componentCopy
	}

	blueprint.TerraformComponents = resolvedComponents
}

// Ensure that BaseBlueprintHandler implements the BlueprintHandler interface
var _ BlueprintHandler = &BaseBlueprintHandler{}

// isValidTerraformRemoteSource checks if the source is a valid Terraform module reference
var isValidTerraformRemoteSource = func(source string) bool {
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
	yamlBytes, err := yamlMarshal(contextConfig)
	if err != nil {
		return "", err
	}
	jsonBytes, err := yamlToJson(yamlBytes)
	if err != nil {
		return "", err
	}

	snippetWithContext := fmt.Sprintf(`
local context = %s;
%s
`, string(jsonBytes), jsonnetTemplate)

	vm := jsonnetMakeVM()
	evaluatedJsonnet, err := vm.EvaluateAnonymousSnippet("blueprint", snippetWithContext)
	if err != nil {
		return "", err
	}

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
