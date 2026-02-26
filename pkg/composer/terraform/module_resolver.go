package terraform

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/zclconf/go-cty/cty"
)

// The ModuleResolver is a terraform module source resolution and configuration system.
// It provides functionality to process different types of terraform module sources including standard git/local sources and OCI artifacts.
// The ModuleResolver acts as the central orchestrator for terraform module management in Windsor projects,
// coordinating module extraction, shim generation, and configuration file creation for proper terraform initialization.

// =============================================================================
// Interfaces
// =============================================================================

// ModuleResolver processes terraform module sources and generates appropriate module configurations
type ModuleResolver interface {
	ProcessModules() error
	GenerateTfvars(overwrite bool) error
}

// =============================================================================
// Types
// =============================================================================

// BaseModuleResolver provides common functionality for all module resolvers
type BaseModuleResolver struct {
	shims            *Shims
	runtime          *runtime.Runtime
	blueprintHandler blueprint.BlueprintHandler
	evaluator        evaluator.ExpressionEvaluator
	reset            bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseModuleResolver creates a new base module resolver with the provided dependencies.
// If overrides are provided, any non-nil component in the override BaseModuleResolver will be used instead of creating a default.
// Panics if rt, blueprintHandler, or rt.Evaluator are nil.
func NewBaseModuleResolver(rt *runtime.Runtime, blueprintHandler blueprint.BlueprintHandler) *BaseModuleResolver {
	if rt == nil {
		panic("runtime is required")
	}
	if blueprintHandler == nil {
		panic("blueprint handler is required")
	}
	if rt.Evaluator == nil {
		panic("evaluator is required on runtime")
	}

	return &BaseModuleResolver{
		shims:            NewShims(),
		runtime:          rt,
		blueprintHandler: blueprintHandler,
		evaluator:        rt.Evaluator,
	}
}

// GenerateTfvars creates Terraform configuration files, including tfvars files, for all blueprint components.
// It processes template data keyed by "terraform/<module_path>", generating tfvars files at
// contexts/<context>/terraform/<module_path>.tfvars. For each entry in the input data, it skips keys
// not prefixed with "terraform/" and skips components not present in the blueprint. For all components
// in the blueprint, it ensures a tfvars file is generated if not already handled by the input data.
// The method uses the blueprint handler to retrieve TerraformComponents and parses variables from all
// .tf files in the module directory based on component source (remote or local). Module resolution is handled by pkg/terraform.
// Input expressions are evaluated against the current configuration before being written to tfvars.
func (h *BaseModuleResolver) GenerateTfvars(overwrite bool) error {
	h.reset = overwrite

	projectRoot := h.runtime.ProjectRoot

	components := h.blueprintHandler.GetTerraformComponents()

	for _, component := range components {
		componentValues := component.Inputs
		if componentValues == nil {
			componentValues = make(map[string]any)
		}

		// Use evaluateDeferred=false so terraform_output() and similar stay deferred; they are filtered
		// out below and not written to tfvars, and are evaluated later when terraform runs.
		evaluatedValues, err := h.evaluator.EvaluateMap(componentValues, "", nil, false)
		if err != nil {
			return fmt.Errorf("failed to evaluate inputs for component %s: %w", component.GetID(), err)
		}

		nonDeferredValues := make(map[string]any)
		for key, value := range evaluatedValues {
			if evaluator.ContainsExpression(value) {
				continue
			}
			if s, ok := value.(string); ok && strings.Contains(s, "${") {
				continue
			}
			nonDeferredValues[key] = value
		}

		if err := h.generateComponentTfvars(projectRoot, component, nonDeferredValues); err != nil {
			return fmt.Errorf("failed to generate tfvars for component %s: %w", component.Path, err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// validateAndSanitizePath sanitizes a file path for safe extraction by removing path traversal sequences
// and rejecting absolute paths. Returns the cleaned path if valid, or an error if the path is unsafe.
func (h *BaseModuleResolver) validateAndSanitizePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path contains directory traversal sequence: %s", path)
	}
	if strings.HasPrefix(cleanPath, string(filepath.Separator)) || (len(cleanPath) >= 2 && cleanPath[1] == ':' && (cleanPath[0] >= 'A' && cleanPath[0] <= 'Z' || cleanPath[0] >= 'a' && cleanPath[0] <= 'z')) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}
	return cleanPath, nil
}

// VariableInfo holds metadata for a single Terraform variable
type VariableInfo struct {
	Name        string
	Description string
	Default     any
	Sensitive   bool
}

// parseVariablesFromModule parses all .tf files in a module directory and returns metadata about the variables.
// It extracts variable names, descriptions, default values, and sensitivity flags from any .tf file in the module.
// Protected values are excluded from the returned metadata. Returns an empty slice if no variables are found,
// but returns an error if Glob fails (indicating a filesystem issue).
func (h *BaseModuleResolver) parseVariablesFromModule(modulePath string, protectedValues map[string]bool) ([]VariableInfo, error) {
	pattern := filepath.Join(modulePath, "*.tf")
	matches, err := h.shims.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find .tf files in module: %w", err)
	}

	if len(matches) == 0 {
		return []VariableInfo{}, nil
	}

	variableMap := make(map[string]*VariableInfo)

	for _, tfFile := range matches {
		content, err := h.shims.ReadFile(tfFile)
		if err != nil {
			continue
		}

		parsedFile, diags := hclwrite.ParseConfig(content, tfFile, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			continue
		}

		for _, block := range parsedFile.Body().Blocks() {
			if block.Type() == "variable" && len(block.Labels()) > 0 {
				variableName := block.Labels()[0]

				if protectedValues[variableName] {
					continue
				}

				if _, exists := variableMap[variableName]; exists {
					continue
				}

				info := &VariableInfo{
					Name: variableName,
				}

				if attr := block.Body().GetAttribute("description"); attr != nil {
					exprBytes := attr.Expr().BuildTokens(nil).Bytes()
					parsedExpr, diags := hclsyntax.ParseExpression(exprBytes, "description", hcl.Pos{Line: 1, Column: 1})
					if !diags.HasErrors() {
						val, diags := parsedExpr.Value(nil)
						if !diags.HasErrors() && val.Type() == cty.String {
							info.Description = val.AsString()
						}
					}
				}

				if attr := block.Body().GetAttribute("sensitive"); attr != nil {
					exprBytes := attr.Expr().BuildTokens(nil).Bytes()
					parsedExpr, diags := hclsyntax.ParseExpression(exprBytes, "sensitive", hcl.Pos{Line: 1, Column: 1})
					if !diags.HasErrors() {
						val, diags := parsedExpr.Value(nil)
						if !diags.HasErrors() && val.Type() == cty.Bool {
							info.Sensitive = val.True()
						}
					}
				}

				if attr := block.Body().GetAttribute("default"); attr != nil {
					exprBytes := attr.Expr().BuildTokens(nil).Bytes()
					parsedExpr, diags := hclsyntax.ParseExpression(exprBytes, "default", hcl.Pos{Line: 1, Column: 1})
					if !diags.HasErrors() {
						val, diags := parsedExpr.Value(nil)
						if !diags.HasErrors() {
							info.Default = convertFromCtyValue(val)
						}
					}
				}

				variableMap[variableName] = info
			}
		}
	}

	variables := make([]VariableInfo, 0, len(variableMap))
	for _, info := range variableMap {
		variables = append(variables, *info)
	}

	return variables, nil
}

// generateComponentTfvars generates tfvars files for a single Terraform component.
// All components write tfvars files to .windsor/contexts/<context>/terraform/<componentID>/terraform.tfvars,
// regardless of whether they have a Source (remote) or not (local). This unifies the behavior
// between local templates and OCI artifacts, preventing writes to the contexts folder.
// Returns an error if tfvars file generation fails.
func (h *BaseModuleResolver) generateComponentTfvars(projectRoot string, component blueprintv1alpha1.TerraformComponent, componentValues map[string]any) error {
	modulePath, err := h.findModulePathForComponent(projectRoot, component)
	if err != nil {
		return fmt.Errorf("failed to find module path for component %s: %w", component.GetID(), err)
	}

	componentID := component.GetID()
	moduleTfvarsPath := filepath.Join(projectRoot, ".windsor", "contexts", h.runtime.ContextName, "terraform", componentID, "terraform.tfvars")
	if err := h.removeTfvarsFiles(filepath.Dir(moduleTfvarsPath)); err != nil {
		return fmt.Errorf("failed cleaning existing .tfvars in module dir %s: %w", filepath.Dir(moduleTfvarsPath), err)
	}
	if err := h.generateTfvarsFile(moduleTfvarsPath, modulePath, componentValues, component.Source); err != nil {
		return fmt.Errorf("failed to generate module tfvars file: %w", err)
	}

	return nil
}

// findModulePathForComponent returns the path to the module directory for the specified Terraform component.
// For components with a name, the path is .windsor/contexts/<context>/terraform/<name> (where the shim is located).
// For components without a name but with a Source, the path is .windsor/contexts/<context>/terraform/<component.Path>.
// For local components without a name, the path is terraform/<component.Path> (the actual module location).
// For template sources (absolute paths containing contexts/_template), returns the actual template module path.
// Returns the module directory path if it exists, or an error if not found.
func (h *BaseModuleResolver) findModulePathForComponent(projectRoot string, component blueprintv1alpha1.TerraformComponent) (string, error) {
	componentID := component.GetID()

	if filepath.IsAbs(component.Source) && (strings.Contains(component.Source, filepath.Join("contexts", "_template")) || strings.HasPrefix(component.Source, filepath.Join(projectRoot, "terraform"))) {
		modulePath := component.Source

		if _, err := h.shims.Stat(modulePath); err != nil {
			if os.IsNotExist(err) {
				_, _ = h.shims.ReadDir(filepath.Dir(modulePath))
			}
			return "", fmt.Errorf("module directory not found for component %s at %s", component.GetID(), modulePath)
		}
		return modulePath, nil
	}

	useScratchPath := component.Name != "" || component.Source != ""
	var modulePath string
	if useScratchPath {
		modulePath = filepath.Join(projectRoot, ".windsor", "contexts", h.runtime.ContextName, "terraform", componentID)
	} else {
		modulePath = filepath.Join(projectRoot, "terraform", componentID)
	}

	if _, err := h.shims.Stat(modulePath); err != nil {
		return "", fmt.Errorf("module directory not found for component %s at %s", component.GetID(), modulePath)
	}

	return modulePath, nil
}

// generateTfvarsFile generates a tfvars file at the specified path using the provided module directory and component values.
// It parses all .tf files in the module directory to extract variable definitions, merges them with the given component values
// (excluding protected values), and writes a formatted tfvars file. Tfvars are always overwritten so scratch state reflects the current rendered output.
// The function ensures the parent directory exists and returns an error if any file or directory operation fails.
func (h *BaseModuleResolver) generateTfvarsFile(tfvarsFilePath, modulePath string, componentValues map[string]any, source string) error {
	protectedValues := map[string]bool{
		"context_path": true,
		"os_type":      true,
		"context_id":   true,
	}

	variables, err := h.parseVariablesFromModule(modulePath, protectedValues)
	if err != nil {
		return fmt.Errorf("failed to parse variables from module: %w", err)
	}

	mergedFile := hclwrite.NewEmptyFile()
	body := mergedFile.Body()

	addTfvarsHeader(body, source)

	if len(componentValues) > 0 {
		writeComponentValues(body, componentValues, protectedValues, variables)
	} else {
		writeComponentValues(body, componentValues, map[string]bool{}, variables)
	}

	parentDir := filepath.Dir(tfvarsFilePath)
	if err := h.shims.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := h.shims.WriteFile(tfvarsFilePath, mergedFile.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write tfvars file: %w", err)
	}

	return nil
}

// clearShimDirTfFiles removes all .tf files in the shim directory before writing shim files,
// so that the directory only contains the files written by writeShimMainTf, writeShimVariablesTf,
// and writeShimOutputsTf. Prevents residual files (e.g. outputs.tf from a previous run when
// the source module no longer has outputs) from persisting.
func (h *BaseModuleResolver) clearShimDirTfFiles(moduleDir string) error {
	if _, err := h.shims.Stat(moduleDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	entries, err := h.shims.ReadDir(moduleDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".tf") {
			fullPath := filepath.Join(moduleDir, name)
			if err := h.shims.RemoveAll(fullPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// removeTfvarsFiles removes any .tfvars files directly under the specified directory.
// This is used to ensure module directories do not retain stale tfvars prior to regeneration.
func (h *BaseModuleResolver) removeTfvarsFiles(dir string) error {
	if _, err := h.shims.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	entries, err := h.shims.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".tfvars") {
			fullPath := filepath.Join(dir, name)
			if err := h.shims.RemoveAll(fullPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// addTfvarsHeader adds a Windsor CLI management header to the tfvars file body.
// It includes a module source comment if provided, ensuring users are aware of CLI management and module provenance.
func addTfvarsHeader(body *hclwrite.Body, source string) {
	windsorHeaderToken := "Managed by Windsor CLI:"
	headerComment := fmt.Sprintf("# %s This file is partially managed by the windsor CLI. Your changes will not be overwritten.", windsorHeaderToken)
	body.AppendUnstructuredTokens(hclwrite.Tokens{
		{Type: hclsyntax.TokenComment, Bytes: []byte(headerComment + "\n")},
	})
	if source != "" {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# Module source: %s\n", source))},
		})
	}
}

// writeComponentValues writes all component-provided or default variable values to the tfvars file body.
// It comments out default values and descriptions for unset variables, and writes explicit values for set variables.
// Handles sensitive variables and writes variables in alphabetical order by name.
func writeComponentValues(body *hclwrite.Body, values map[string]any, protectedValues map[string]bool, variables []VariableInfo) {
	sortedVariables := make([]VariableInfo, len(variables))
	copy(sortedVariables, variables)
	sort.Slice(sortedVariables, func(i, j int) bool {
		return sortedVariables[i].Name < sortedVariables[j].Name
	})

	for _, info := range sortedVariables {
		if protectedValues[info.Name] {
			continue
		}

		val, exists := values[info.Name]
		if exists && val == nil {
			continue
		}

		body.AppendNewline()

		if info.Description != "" {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte("# " + info.Description)},
			})
			body.AppendNewline()
		}

		if exists {
			writeVariable(body, info.Name, val, []VariableInfo{})
			continue
		}

		if info.Sensitive {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# %s = \"(sensitive)\"", info.Name))},
			})
			body.AppendNewline()
			continue
		}

		if info.Default != nil {
			defaultVal := convertToCtyValue(info.Default)
			if !defaultVal.IsNull() {
				var rendered string
				if defaultVal.Type().IsObjectType() || defaultVal.Type().IsMapType() {
					var mapStr strings.Builder
					mapStr.WriteString(fmt.Sprintf("%s = %s", info.Name, formatValue(convertFromCtyValue(defaultVal))))
					rendered = mapStr.String()
				} else {
					rendered = fmt.Sprintf("%s = %s", info.Name, string(hclwrite.TokensForValue(defaultVal).Bytes()))
				}
				for _, line := range strings.Split(rendered, "\n") {
					body.AppendUnstructuredTokens(hclwrite.Tokens{
						{Type: hclsyntax.TokenComment, Bytes: []byte("# " + line)},
					})
					body.AppendNewline()
				}
				continue
			}
		}

		buf := make([]byte, 0, 32)
		buf = fmt.Appendf(buf, "# %s = null", info.Name)
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: buf},
		})
		body.AppendNewline()
	}
}

// writeHeredoc writes a multi-line string value as a heredoc assignment in the tfvars file body.
// Used for YAML or other multi-line string values to preserve formatting.
// Content that parses as YAML is re-serialized with clean formatting (unquoted keys) before writing.
func writeHeredoc(body *hclwrite.Body, name string, content string) {
	formatted := formatHeredocContent(content)
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOHeredoc, Bytes: []byte("<<EOF")},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		{Type: hclsyntax.TokenStringLit, Bytes: []byte(formatted)},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		{Type: hclsyntax.TokenCHeredoc, Bytes: []byte("EOF")},
	}
	body.SetAttributeRaw(name, tokens)
	body.AppendNewline()
}

// formatHeredocContent re-serializes multi-line content as YAML when valid, producing unquoted keys
// and consistent indentation. Returns the original string if content is not valid YAML.
func formatHeredocContent(content string) string {
	var decoded any
	if err := yaml.Unmarshal([]byte(content), &decoded); err != nil {
		return content
	}
	out, err := yaml.Marshal(decoded)
	if err != nil {
		return content
	}
	s := strings.TrimSuffix(string(out), "\n")
	if s == "" && content != "" {
		return content
	}
	return s
}

// writeVariable writes a single variable assignment to the tfvars file body.
// Handles descriptions, sensitive flags, multi-line strings, and object/map formatting.
// Ensures correct HCL syntax for all supported value types.
func writeVariable(body *hclwrite.Body, name string, value any, variables []VariableInfo) {
	var info *VariableInfo
	for _, v := range variables {
		if v.Name == name {
			info = &v
			break
		}
	}

	if info != nil && info.Description != "" {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte("# " + info.Description)},
		})
		body.AppendNewline()
	}

	if info != nil && info.Sensitive {
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# %s = \"(sensitive)\"", name))},
		})
		body.AppendNewline()
		return
	}

	switch v := value.(type) {
	case string:
		if strings.Contains(v, "\n") {
			writeHeredoc(body, name, v)
			return
		}
	case map[string]any, []any:
		ctyVal := convertToCtyValue(v)
		if ctyVal != cty.NilVal {
			body.SetAttributeValue(name, ctyVal)
			body.AppendNewline()
			return
		}
	}

	body.SetAttributeValue(name, convertToCtyValue(value))
}

// formatValue formats a Go value as a valid HCL literal string for tfvars output.
// Handles strings, lists, maps, nested objects, and nil values with proper indentation and quoting.
func formatValue(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []string:
		if len(v) == 0 {
			return "[]"
		}
		var items []string
		for _, item := range v {
			items = append(items, fmt.Sprintf("%q", item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		var items []string
		for _, item := range v {
			items = append(items, formatValue(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case map[string]any:
		if len(v) == 0 {
			return "{}"
		}
		var pairs []string
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := v[k]
			formattedVal := formatValue(val)
			if formattedVal == "{}" || formattedVal == "[]" {
				pairs = append(pairs, fmt.Sprintf("%s = %s", k, formattedVal))
			} else {
				if strings.HasPrefix(formattedVal, "{") {
					indented := strings.ReplaceAll(formattedVal, "\n", "\n  ")
					pairs = append(pairs, fmt.Sprintf("%s = %s", k, indented))
				} else {
					pairs = append(pairs, fmt.Sprintf("%s = %s", k, formattedVal))
				}
			}
		}
		return fmt.Sprintf("{\n  %s\n}", strings.Join(pairs, "\n  "))
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// convertToCtyValue converts a Go value to a cty.Value for HCL serialization.
// Supports strings, numbers, booleans, lists, and maps; returns NilVal for unsupported types.
// Callers must pass values already normalized to map[string]any and []any (e.g. from evaluator output).
func convertToCtyValue(value any) cty.Value {
	switch v := value.(type) {
	case string:
		return cty.StringVal(v)
	case int:
		return cty.NumberIntVal(int64(v))
	case int64:
		return cty.NumberIntVal(v)
	case int32:
		return cty.NumberIntVal(int64(v))
	case uint:
		return cty.NumberVal(new(big.Float).SetUint64(uint64(v)))
	case uint64:
		return cty.NumberVal(new(big.Float).SetUint64(v))
	case uint32:
		return cty.NumberIntVal(int64(v))
	case float64:
		return cty.NumberFloatVal(v)
	case float32:
		return cty.NumberFloatVal(float64(v))
	case bool:
		return cty.BoolVal(v)
	case []string:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.String)
		}
		var ctyList []cty.Value
		for _, item := range v {
			ctyList = append(ctyList, cty.StringVal(item))
		}
		return cty.ListVal(ctyList)
	case []any:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		var ctyList []cty.Value
		var elementType cty.Type
		hasElementType := false
		hasMixedTypes := false
		for _, item := range v {
			itemVal := convertToCtyValue(item)
			if itemVal == cty.NilVal {
				continue
			}
			ctyList = append(ctyList, itemVal)
			if !hasElementType {
				elementType = itemVal.Type()
				hasElementType = true
			} else if !elementType.Equals(itemVal.Type()) {
				hasMixedTypes = true
			}
		}
		if len(ctyList) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		if hasMixedTypes {
			return cty.TupleVal(ctyList)
		}
		return cty.ListVal(ctyList)
	case map[string]any:
		ctyMap := make(map[string]cty.Value)
		for key, val := range v {
			ctyMap[key] = convertToCtyValue(val)
		}
		return cty.ObjectVal(ctyMap)
	default:
		return cty.NilVal
	}
}

// convertFromCtyValue converts a cty.Value to its Go representation for use in tfvars generation.
// Handles all supported HCL types, including lists, maps, objects, and primitives.
func convertFromCtyValue(val cty.Value) any {
	if !val.IsKnown() || val.IsNull() {
		return nil
	}

	switch {
	case val.Type() == cty.String:
		return val.AsString()
	case val.Type() == cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInt() {
			i, _ := bf.Int64()
			return i
		}
		f, _ := bf.Float64()
		return f
	case val.Type() == cty.Bool:
		return val.True()
	case val.Type().IsListType() || val.Type().IsTupleType() || val.Type().IsSetType():
		var list []any
		it := val.ElementIterator()
		for it.Next() {
			_, v := it.Element()
			list = append(list, convertFromCtyValue(v))
		}
		return list
	case val.Type().IsMapType() || val.Type().IsObjectType():
		m := make(map[string]any)
		it := val.ElementIterator()
		for it.Next() {
			k, v := it.Element()
			m[k.AsString()] = convertFromCtyValue(v)
		}
		return m
	default:
		return nil
	}
}

// writeShimMainTf creates the main.tf file for the shim module by generating a module block
// that references the specified source. It creates an HCL configuration with a single module
// block named "main" that points to the provided source location, then writes the generated
// configuration to main.tf in the specified module directory.
func (h *BaseModuleResolver) writeShimMainTf(moduleDir, source string) error {
	mainContent := hclwrite.NewEmptyFile()
	block := mainContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()
	body.SetAttributeValue("source", cty.StringVal(source))

	if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), mainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}
	return nil
}

// writeShimVariablesTf creates the variables.tf file for the shim module by parsing all .tf files
// in the original module and generating corresponding variable blocks and module arguments.
// It parses variable definitions from any .tf file in the source module, creates shim variable blocks
// that preserve all attributes (description, type, default, sensitive), and configures the main
// module block to pass through all variables using var.variable_name references. Variables are
// generated in alphabetical order in both the variables.tf file and the module arguments.
func (h *BaseModuleResolver) writeShimVariablesTf(moduleDir, modulePath, source string) error {
	shimMainContent := hclwrite.NewEmptyFile()
	shimBlock := shimMainContent.Body().AppendNewBlock("module", []string{"main"})
	shimBody := shimBlock.Body()
	shimBody.SetAttributeRaw("source", hclwrite.TokensForValue(cty.StringVal(source)))

	pattern := filepath.Join(modulePath, "*.tf")
	matches, err := h.shims.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find .tf files in module: %w", err)
	}

	if len(matches) == 0 {
		if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim main.tf: %w", err)
		}
		return nil
	}

	variableMap := make(map[string]*hclwrite.Block)

	for _, tfFile := range matches {
		content, err := h.shims.ReadFile(tfFile)
		if err != nil {
			continue
		}

		parsedFile, diags := hclwrite.ParseConfig(content, tfFile, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			continue
		}

		for _, block := range parsedFile.Body().Blocks() {
			if block.Type() == "variable" {
				labels := block.Labels()
				if len(labels) > 0 {
					variableName := labels[0]
					if _, exists := variableMap[variableName]; !exists {
						variableMap[variableName] = block
					}
				}
			}
		}
	}

	if len(variableMap) == 0 {
		if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim main.tf: %w", err)
		}
		return nil
	}

	shimVariablesContent := hclwrite.NewEmptyFile()
	shimVariablesBody := shimVariablesContent.Body()

	variableNames := make([]string, 0, len(variableMap))
	for variableName := range variableMap {
		variableNames = append(variableNames, variableName)
	}
	sort.Strings(variableNames)

	for _, variableName := range variableNames {
		block := variableMap[variableName]
		shimBody.SetAttributeTraversal(variableName, hcl.Traversal{
			hcl.TraverseRoot{Name: "var"},
			hcl.TraverseAttr{Name: variableName},
		})

		shimBlock := shimVariablesBody.AppendNewBlock("variable", []string{variableName})
		shimBlockBody := shimBlock.Body()

		if attr := block.Body().GetAttribute("description"); attr != nil {
			shimBlockBody.SetAttributeRaw("description", attr.Expr().BuildTokens(nil))
		}

		if attr := block.Body().GetAttribute("type"); attr != nil {
			shimBlockBody.SetAttributeRaw("type", attr.Expr().BuildTokens(nil))
		}

		if attr := block.Body().GetAttribute("default"); attr != nil {
			shimBlockBody.SetAttributeRaw("default", attr.Expr().BuildTokens(nil))
		}

		if attr := block.Body().GetAttribute("sensitive"); attr != nil {
			shimBlockBody.SetAttributeRaw("sensitive", attr.Expr().BuildTokens(nil))
		}
	}

	if err := h.shims.WriteFile(filepath.Join(moduleDir, "variables.tf"), shimVariablesContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim variables.tf: %w", err)
	}

	if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim main.tf: %w", err)
	}

	return nil
}

// writeShimOutputsTf creates the outputs.tf file for the shim module by reading the original
// module's outputs.tf file and generating corresponding output blocks that reference the
// main module. It preserves output descriptions and creates value references using the
// module.main.output_name traversal pattern. When the source module has no outputs.tf,
// it writes an empty outputs.tf so that any previous shim outputs.tf is overwritten.
func (h *BaseModuleResolver) writeShimOutputsTf(moduleDir, modulePath string) error {
	shimOutputsPath := filepath.Join(moduleDir, "outputs.tf")
	outputsPath := filepath.Join(modulePath, "outputs.tf")
	if _, err := h.shims.Stat(outputsPath); err == nil {
		outputsContent, err := h.shims.ReadFile(outputsPath)
		if err != nil {
			return fmt.Errorf("failed to read outputs.tf: %w", err)
		}

		outputsFile, diags := hclwrite.ParseConfig(outputsContent, outputsPath, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return fmt.Errorf("failed to parse outputs.tf: %w", diags)
		}

		shimOutputsContent := hclwrite.NewEmptyFile()
		shimBody := shimOutputsContent.Body()

		for _, block := range outputsFile.Body().Blocks() {
			if block.Type() == "output" {
				labels := block.Labels()
				if len(labels) > 0 {
					outputName := labels[0]
					shimBlock := shimBody.AppendNewBlock("output", []string{outputName})
					shimBlockBody := shimBlock.Body()

					if attr := block.Body().GetAttribute("description"); attr != nil {
						shimBlockBody.SetAttributeRaw("description", attr.Expr().BuildTokens(nil))
					}

					if attr := block.Body().GetAttribute("sensitive"); attr != nil {
						shimBlockBody.SetAttributeRaw("sensitive", attr.Expr().BuildTokens(nil))
					}

					shimBlockBody.SetAttributeTraversal("value", hcl.Traversal{
						hcl.TraverseRoot{Name: "module"},
						hcl.TraverseAttr{Name: "main"},
						hcl.TraverseAttr{Name: outputName},
					})
				}
			}
		}

		if err := h.shims.WriteFile(shimOutputsPath, shimOutputsContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim outputs.tf: %w", err)
		}
	} else {
		if err := h.shims.WriteFile(shimOutputsPath, nil, 0644); err != nil {
			return fmt.Errorf("failed to write shim outputs.tf: %w", err)
		}
	}
	return nil
}
