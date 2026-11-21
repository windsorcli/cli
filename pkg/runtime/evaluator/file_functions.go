// The file functions are the expression helpers that load content from files and template data.
// It provides file(), yaml(), and the file form of yamlString().
// Each function looks in the in-memory template data first, then reads the file system.
// Relative paths resolve from the facet file directory, then from the project root.

package evaluator

import (
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"strings"

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

// evaluateFileFunction loads raw file content from the specified path.
// It first attempts to find the file in template data using lookupInTemplateData, then
// tries a fallback lookup relative to the template root if available. If not found in
// template data, it reads the file from the filesystem. The path is resolved relative
// to the facetPath or project root as appropriate. Returns the file content as a
// string or an error if the file cannot be found or read.
func (e *DefaultExpressionEvaluator) evaluateFileFunction(pathArg string, facetPath string) (any, error) {
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
func (e *DefaultExpressionEvaluator) evaluateYamlStringFromFile(pathArg string, facetPath string, input any) (string, error) {
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

// evaluateYamlFunction parses YAML from a file path or from an inline YAML string.
// An argument that contains a newline is inline YAML, and the function reads no file.
// Otherwise it tries the template data and the filesystem, then parses the argument as YAML.
// When input is not nil, it templates the content with the context plus {"input": input}.
// It returns the parsed map or slice, or an error when the file or the YAML is not valid.
func (e *DefaultExpressionEvaluator) evaluateYamlFunction(arg string, facetPath string, input any) (any, error) {
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
func (e *DefaultExpressionEvaluator) templateDataLookup(key string) []byte {
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
func (e *DefaultExpressionEvaluator) lookupInTemplateData(pathArg string, facetPath string) []byte {
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

// resolvePath returns path cleaned, and absolute when projectRoot or templateRoot is set.
// A relative path resolves from the directory of sourceFilePath, the file that holds the expression.
// A relative sourceFilePath resolves against templateRoot, and a result that is still relative
// resolves against projectRoot. When sourceFilePath is empty, path resolves against projectRoot,
// with a leading "../" or "./" removed, or else against templateRoot.
func (e *DefaultExpressionEvaluator) resolvePath(path string, sourceFilePath string) string {
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
