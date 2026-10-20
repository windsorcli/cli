// The tfvars functions of the BaseModuleResolver write the tfvars file of a component.
// It provides the variable parser of a module and the HCL writers for the tfvars values.
// The tfvars header names the module source and says that Windsor regenerates the file on each run.
// The writers change Go values to cty values, and write multi-line strings as heredocs.

package terraform

import (
	"fmt"
	"math/big"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/zclconf/go-cty/cty"
)

// =============================================================================
// Types
// =============================================================================

// VariableInfo holds metadata for a single Terraform variable
type VariableInfo struct {
	Name        string
	Description string
	Default     any
	Sensitive   bool
}

// =============================================================================
// Private Methods
// =============================================================================

// parseVariablesFromModule returns the variables in the .tf files of a module directory.
// Each variable has its name, description, default value, and sensitive flag. It does not return protected values.
// It skips a .tf file that it cannot read or parse, and returns an empty slice if it finds no variables.
// It returns an error only if the .tf file search fails.
func (h *BaseModuleResolver) parseVariablesFromModule(modulePath string, protectedValues map[string]bool) ([]VariableInfo, error) {
	pattern := filepath.Join(modulePath, "*.tf")
	matches, err := h.shims.Glob(pattern)
	if err != nil {
		return nil, werror.Wrap(err, "failed to find .tf files in module")
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

// generateTfvarsFile writes a tfvars file at tfvarsFilePath from the module variables and the component values.
// It does not write protected values that a component sets. It always replaces the file so that it shows the current values.
// It creates the parent directory if necessary.
func (h *BaseModuleResolver) generateTfvarsFile(tfvarsFilePath, modulePath string, componentValues map[string]any, source string) error {
	protectedValues := map[string]bool{
		"context_path": true,
		"os_type":      true,
		"context_id":   true,
	}

	variables, err := h.parseVariablesFromModule(modulePath, protectedValues)
	if err != nil {
		return werror.Wrap(err, "failed to parse variables from module")
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
		return werror.Wrap(err, "failed to create directory")
	}

	if err := h.shims.WriteFile(tfvarsFilePath, mergedFile.Bytes(), 0644); err != nil {
		return werror.Wrap(err, "failed to write tfvars file")
	}

	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// addTfvarsHeader adds a Windsor CLI management header to the tfvars file body.
// It includes a module source comment if provided, ensuring users are aware of CLI management and module provenance.
// The header states plainly that the file is fully regenerated (matching generateTfvarsFile's
// actual behavior) and names the supported override path, rather than inviting hand edits that
// the next run silently discards — see cli#3150.
func addTfvarsHeader(body *hclwrite.Body, source string) {
	windsorHeaderToken := "Managed by Windsor CLI:"
	headerComment := fmt.Sprintf("# %s this file is regenerated on every run from the blueprint's component inputs. Edits here are discarded without warning — set values via this component's `inputs:` in blueprint.yaml instead.", windsorHeaderToken)
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
