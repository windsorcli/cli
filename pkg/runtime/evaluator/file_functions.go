// The file functions are the expression helpers that load content from files and template data.
// It provides jsonnet(), file(), yaml(), and the file form of yamlString().
// Each function looks in the in-memory template data first, then reads the file system.
// Relative paths resolve from the facet file directory, then from the project root.

package evaluator

import (
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// filePathRemediation is the hint for a file helper that cannot find or read its file.
const filePathRemediation = "Make sure that the file exists. A relative path resolves from the directory of the file that has the expression."

// =============================================================================
// Private Methods
// =============================================================================

// evaluateJsonnetFunction loads and evaluates a Jsonnet file at the specified path.
// It first attempts to load the file from template data if available, falling back to
// the filesystem if not found. The config is enriched with context information and
// passed to the Jsonnet VM as external code. Helper functions and import paths are
// configured, then the Jsonnet is evaluated and the JSON output is unmarshaled.
// Returns the evaluated value or an error if loading, evaluation, or parsing fails.
func (e *expressionEvaluator) evaluateJsonnetFunction(pathArg string, config map[string]any, facetPath string) (any, error) {
	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, facetPath)
	}

	var path string
	if content == nil {
		path = e.resolvePath(pathArg, facetPath)
		content, err = e.Shims.ReadFile(path)
		if err != nil {
			return nil, werror.New(fmt.Sprintf("The jsonnet() helper cannot read the file %s.", path), filePathRemediation, err)
		}
	} else {
		if e.templateRoot != "" && facetPath != "" {
			facetAbsPath := facetPath
			if !filepath.IsAbs(facetPath) {
				facetAbsPath = filepath.Join(e.templateRoot, facetPath)
			}
			facetDir := filepath.Dir(facetAbsPath)
			resolvedAbsPath := filepath.Clean(filepath.Join(facetDir, pathArg))
			if relPath, err := filepath.Rel(e.templateRoot, resolvedAbsPath); err == nil && !strings.HasPrefix(relPath, "..") {
				path = filepath.Join(e.templateRoot, relPath)
			} else {
				path = resolvedAbsPath
			}
		} else {
			path = e.resolvePath(pathArg, facetPath)
		}
	}

	configJSON, err := e.Shims.JsonMarshal(config)
	if err != nil {
		return nil, werror.Wrap(err, "failed to marshal config to JSON")
	}

	vm := e.Shims.NewJsonnetVM()

	helpersLibrary := e.buildHelperLibrary()
	vm.ExtCode("helpers", helpersLibrary)
	vm.ExtCode("context", string(configJSON))

	if dir := filepath.Dir(path); dir != "" {
		vm.Importer(&jsonnet.FileImporter{
			JPaths: []string{dir},
		})
	}

	result, err := vm.EvaluateAnonymousSnippet(filepath.Base(path), string(content))
	if err != nil {
		return nil, werror.New(fmt.Sprintf("The jsonnet() helper cannot evaluate the file %s: %v", path, err),
			"Correct the Jsonnet error at the location that the message shows.", err)
	}

	var value any
	if err := e.Shims.JsonUnmarshal([]byte(result), &value); err != nil {
		return nil, werror.New(fmt.Sprintf("The Jsonnet file %s does not output valid JSON: %v", path, err),
			"Make sure that the Jsonnet file evaluates to an object, an array, or a scalar value.", err)
	}

	return value, nil
}

// buildHelperLibrary returns a Jsonnet library string containing helper functions.
// The library provides safe accessors for nested object paths (get, getString, getInt, etc.),
// type checking and conversion utilities, and data manipulation functions like baseUrl
// and removeEmptyKeys. These helpers enable safer and more expressive Jsonnet code
// when working with configuration data and context objects.
func (e *expressionEvaluator) buildHelperLibrary() string {
	return `{
  get(obj, path, default=null):
    if std.findSubstr(".", path) == [] then
      if std.type(obj) == "object" && path in obj then obj[path] else default
    else
      local parts = std.split(path, ".");
      local getValue(o, pathParts) =
        if std.length(pathParts) == 0 then o
        else if std.type(o) != "object" then null
        else if !(pathParts[0] in o) then null
        else getValue(o[pathParts[0]], pathParts[1:]);
      local result = getValue(obj, parts);
      if result == null then default else result,

  getString(obj, path, default=""):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "string" then val
    else error "Expected string for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getInt(obj, path, default=0):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "number" then std.floor(val)
    else error "Expected number for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getNumber(obj, path, default=0):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "number" then val
    else error "Expected number for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getBool(obj, path, default=false):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "boolean" then val
    else error "Expected boolean for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getObject(obj, path, default={}):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "object" then val
    else error "Expected object for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  getArray(obj, path, default=[]):
    local val = self.get(obj, path, null);
    if val == null then default
    else if std.type(val) == "array" then val
    else error "Expected array for '" + path + "' but got " + std.type(val) + ": " + std.toString(val),

  has(obj, path):
    self.get(obj, path, null) != null,

  baseUrl(endpoint):
    if endpoint == "" then 
      ""
    else
      local withoutProtocol = if std.startsWith(endpoint, "https://") then
        std.substr(endpoint, 8, std.length(endpoint) - 8)
      else if std.startsWith(endpoint, "http://") then
        std.substr(endpoint, 7, std.length(endpoint) - 7)
      else
        endpoint;
      local colonPos = std.findSubstr(":", withoutProtocol);
      if std.length(colonPos) > 0 then
        std.substr(withoutProtocol, 0, colonPos[0])
      else
        withoutProtocol,

  removeEmptyKeys(obj):
    local _removeEmptyKeys(obj) =
      if std.type(obj) == "object" then
        local filteredFields = std.filter(
          function(key)
            local value = obj[key];
            if std.type(value) == "object" || std.type(value) == "array" then
              local cleaned = _removeEmptyKeys(value);
              if std.type(cleaned) == "object" then
                std.length(std.objectFields(cleaned)) > 0
              else
                std.length(cleaned) > 0
            else
              value != null && (std.type(value) != "string" || value != "")
          ,
          std.objectFields(obj)
        );
        {
          [key]: if std.type(obj[key]) == "object" || std.type(obj[key]) == "array" then _removeEmptyKeys(obj[key]) else obj[key]
          for key in filteredFields
        }
      else if std.type(obj) == "array" then
        local filteredElements = std.filter(
          function(element)
            if std.type(element) == "object" || std.type(element) == "array" then
              local cleaned = _removeEmptyKeys(element);
              if std.type(cleaned) == "object" then
                std.length(std.objectFields(cleaned)) > 0
              else
                std.length(cleaned) > 0
            else
              element != null && (std.type(element) != "string" || element != "")
          ,
          obj
        );
        [
          if std.type(element) == "object" || std.type(element) == "array" then _removeEmptyKeys(element) else element
          for element in filteredElements
        ]
      else
        obj;
    _removeEmptyKeys(obj),
}`
}

// evaluateFileFunction loads raw file content from the specified path.
// It first attempts to find the file in template data using lookupInTemplateData, then
// tries a fallback lookup relative to the template root if available. If not found in
// template data, it reads the file from the filesystem. The path is resolved relative
// to the facetPath or project root as appropriate. Returns the file content as a
// string or an error if the file cannot be found or read.
func (e *expressionEvaluator) evaluateFileFunction(pathArg string, facetPath string) (any, error) {
	var content []byte
	var err error

	if e.templateData != nil {
		content = e.lookupInTemplateData(pathArg, facetPath)
	}

	if content == nil {
		path := e.resolvePath(pathArg, facetPath)
		content, err = e.Shims.ReadFile(path)
		if err != nil {
			return nil, werror.New(fmt.Sprintf("The file() helper cannot read the file %s.", path), filePathRemediation, err)
		}
	}

	return string(content), nil
}

// evaluateYamlStringFromFile loads the file at pathArg, optionally templates it with input
// (scope = config + {"input": input}), and returns the result as a string without parsing YAML.
// Used by yamlString(path, input) for Terraform variables that expect a YAML string.
// Template expressions must use the input prefix, e.g. ${input.certSANs}, ${input.registryMirrors}.
func (e *expressionEvaluator) evaluateYamlStringFromFile(pathArg string, facetPath string, input any) (string, error) {
	var raw []byte
	if e.templateData != nil {
		raw = e.lookupInTemplateData(pathArg, facetPath)
	}
	if raw == nil {
		path := e.resolvePath(pathArg, facetPath)
		content, err := e.Shims.ReadFile(path)
		if err == nil {
			raw = content
		}
	}
	if raw == nil {
		raw = []byte(pathArg)
	}
	content := string(raw)
	if input != nil {
		config := e.getConfig()
		enrichedConfig := e.enrichConfig(config)
		scope := make(map[string]any)
		maps.Copy(scope, enrichedConfig)
		scope["input"] = input
		interpolated, err := e.evaluate(content, facetPath, scope, true)
		if err != nil {
			return "", werror.Wrap(err, "yamlString() failed to template")
		}
		str, ok := interpolated.(string)
		if !ok {
			return "", fmt.Errorf("yamlString(path, input) expects template to evaluate to a string, got %T", interpolated)
		}
		content = str
	}
	return content, nil
}

// evaluateYamlFunction parses YAML from either a file path or an inline YAML string.
// If the argument contains a newline it is treated as inline YAML only (no file read), avoiding
// a failed filesystem call. Otherwise template data and the filesystem are tried first; if no
// file is found, the argument is unmarshaled as YAML. When input is non-nil, the raw content
// is templated with scope = context + {"input": input} so ${input.xyz} and context vars (e.g.
// provider) resolve; the result is then parsed as YAML. Returns the parsed value (map or slice)
// or an error if the file cannot be read or the YAML is invalid.
func (e *expressionEvaluator) evaluateYamlFunction(arg string, facetPath string, input any) (any, error) {
	var raw []byte
	if strings.Contains(arg, "\n") {
		raw = []byte(arg)
	} else {
		if e.templateData != nil {
			raw = e.lookupInTemplateData(arg, facetPath)
			if raw == nil && facetPath == "" && (strings.HasPrefix(arg, "../") || strings.HasPrefix(arg, "./")) {
				key := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(arg, "../"), "./"))
				raw = e.templateDataLookup(key)
			}
		}
		if raw == nil {
			path := e.resolvePath(arg, facetPath)
			if facetPath == "" && e.templateData != nil && e.templateRoot != "" && strings.HasPrefix(path, e.templateRoot) {
				if relPath, err := filepath.Rel(e.templateRoot, path); err == nil {
					raw = e.templateDataLookup(filepath.ToSlash(relPath))
				}
			}
			if raw == nil {
				content, err := e.Shims.ReadFile(path)
				if err == nil {
					raw = content
				}
			}
		}
		if raw == nil {
			if input == nil {
				var fallback any
				if err := e.Shims.YamlUnmarshal([]byte(arg), &fallback); err != nil {
					return nil, yamlFileNotFoundError(arg)
				}
				if _, ok := fallback.(string); ok {
					return nil, yamlFileNotFoundError(arg)
				}
				normalized := normalizeYamlResult(fallback)
				unwrapped := unwrapSingleElementList(normalized)
				if unwrapped != nil {
					return unwrapped, nil
				}
				return normalized, nil
			}
			return nil, yamlFileNotFoundError(arg)
		}
	}
	content := string(raw)
	if input != nil {
		config := e.getConfig()
		enrichedConfig := e.enrichConfig(config)
		scope := make(map[string]any)
		maps.Copy(scope, enrichedConfig)
		scope["input"] = input
		interpolated, err := e.evaluate(content, facetPath, scope, true)
		if err != nil {
			return nil, werror.Wrap(err, "yaml() failed to template")
		}
		str, ok := interpolated.(string)
		if !ok {
			return nil, fmt.Errorf("yaml() with input expects template to evaluate to a string, got %T", interpolated)
		}
		content = str
	}
	var value any
	if err := e.Shims.YamlUnmarshal([]byte(content), &value); err != nil {
		source := "its inline input"
		if !strings.Contains(arg, "\n") {
			source = arg
		}
		return nil, werror.New(fmt.Sprintf("The yaml() helper cannot parse the YAML in %s: %v", source, err),
			"Correct the YAML syntax at the line that the message shows.", err)
	}
	normalized := normalizeYamlResult(value)
	unwrapped := unwrapSingleElementList(normalized)
	if unwrapped != nil {
		return unwrapped, nil
	}
	return normalized, nil
}

// templateDataLookup returns file content for the given key from templateData, trying both
// the key as-is and with a "_template/" prefix. The loader uses unprefixed keys (relative to
// template root); artifact bundles and tests may use "_template/"-prefixed keys.
func (e *expressionEvaluator) templateDataLookup(key string) []byte {
	if e.templateData == nil || key == "" {
		return nil
	}
	if data := e.templateData[key]; data != nil {
		return data
	}
	return e.templateData["_template/"+key]
}

// lookupInTemplateData attempts to find file content in the template data map.
// It resolves the path argument relative to the facet file's directory, converting
// the facet path to a relative path from the template root if available. For paths
// that go up directories (../), it ensures the resolved path stays within the template
// root. Returns the file content if found, or nil if not present.
func (e *expressionEvaluator) lookupInTemplateData(pathArg string, facetPath string) []byte {
	if e.templateData == nil {
		return nil
	}

	pathArg = strings.TrimSpace(pathArg)
	if filepath.IsAbs(pathArg) {
		return nil
	}

	if facetPath == "" {
		return nil
	}

	if e.templateRoot == "" {
		return nil
	}

	facetAbsPath := facetPath
	if !filepath.IsAbs(facetPath) {
		facetAbsPath = filepath.Join(e.templateRoot, facetPath)
	}

	facetDir := filepath.Dir(facetAbsPath)
	resolvedAbsPath := filepath.Clean(filepath.Join(facetDir, pathArg))

	if relPath, err := filepath.Rel(e.templateRoot, resolvedAbsPath); err == nil && !strings.HasPrefix(relPath, "..") {
		resolvedRelPath := strings.ReplaceAll(relPath, "\\", "/")
		if resolvedRelPath == "." {
			resolvedRelPath = ""
		}
		return e.templateDataLookup(resolvedRelPath)
	}

	return nil
}

// resolvePath resolves a file path to an absolute, cleaned path using the path to the
// file being processed (sourceFilePath) when provided. If path is already absolute
// it is cleaned and returned. For relative paths: when sourceFilePath is non-empty
// it is the path to the facet or blueprint file containing the expression; the
// relative path is resolved from that file's directory. If sourceFilePath is relative
// it is resolved against templateRoot. If the result is still relative it is resolved
// against projectRoot. When sourceFilePath is empty (e.g. GenerateTfvars) relative
// paths fall back to projectRoot. The result is always an absolute path when
// projectRoot or templateRoot are set.
func (e *expressionEvaluator) resolvePath(path string, sourceFilePath string) string {
	path = strings.TrimSpace(path)

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if sourceFilePath != "" {
		baseDir := sourceFilePath
		if !filepath.IsAbs(baseDir) && e.templateRoot != "" {
			baseDir = filepath.Join(e.templateRoot, baseDir)
		}
		baseDir = filepath.Dir(baseDir)
		result := filepath.Clean(filepath.Join(baseDir, path))
		if !filepath.IsAbs(result) && e.projectRoot != "" {
			result = filepath.Clean(filepath.Join(e.projectRoot, result))
		}
		return result
	}

	var result string
	if e.projectRoot != "" && (strings.HasPrefix(path, "..") || strings.HasPrefix(filepath.Clean(path), "..")) {
		trimmed := strings.TrimPrefix(path, "../")
		trimmed = strings.TrimPrefix(trimmed, "./")
		result = filepath.Clean(filepath.Join(e.projectRoot, trimmed))
	} else if e.projectRoot != "" {
		result = filepath.Clean(filepath.Join(e.projectRoot, path))
	} else if e.templateRoot != "" {
		result = filepath.Clean(filepath.Join(e.templateRoot, path))
	} else {
		result = filepath.Clean(path)
	}
	return result
}

// =============================================================================
// Helpers
// =============================================================================

// yamlFileNotFoundError reports a yaml() argument that names no file and is not YAML content.
func yamlFileNotFoundError(arg string) error {
	return werror.New(fmt.Sprintf("The yaml() helper cannot find the file %s.", arg), filePathRemediation, nil)
}

// normalizeYamlResult converts YAML-unmarshaled values to map[string]any and []any so expr's
// Fetch uses map lookup for string keys instead of ToInt (slice index). Handles
// map[interface{}]interface{} and []interface{} from goccy/go-yaml.
func normalizeYamlResult(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		out := make(map[string]any)
		iter := rv.MapRange()
		for iter.Next() {
			k := iter.Key()
			var key string
			switch k.Kind() {
			case reflect.Interface:
				if k.IsNil() {
					continue
				}
				key = fmt.Sprint(k.Elem().Interface())
			default:
				key = fmt.Sprint(k.Interface())
			}
			out[key] = normalizeYamlResult(iter.Value().Interface())
		}
		return out
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.String {
			return v
		}
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i)
			if elem.Kind() == reflect.Interface && !elem.IsNil() {
				elem = elem.Elem()
			}
			out = append(out, normalizeYamlResult(elem.Interface()))
		}
		return out
	default:
		return v
	}
}

// unwrapSingleElementList returns the single element when v is a list of one map-like value.
// expr's Fetch treats slice.key as slice[ToInt(key)], so .controlplane_labels on a list causes
// int(string) panic. Unwrapping allows yaml(path).key to work when the file has a single-document list at root.
// Uses reflection so any slice/map types from the YAML library (e.g. []interface{}, map[interface{}]interface{}) are accepted.
func unwrapSingleElementList(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	if rv.Len() != 1 {
		return nil
	}
	first := rv.Index(0)
	if first.Kind() == reflect.Interface && !first.IsNil() {
		first = first.Elem()
	}
	if first.Kind() != reflect.Map {
		return nil
	}
	return first.Interface()
}
