// The shim functions of the BaseModuleResolver write the shim module of a component.
// It provides the writers for the main.tf, variables.tf, and outputs.tf files of the shim module.
// The shim module calls the real module source and passes its variables and outputs through.
// The standard and OCI resolvers clear the old .tf files of a shim directory before they write it.

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// =============================================================================
// Private Methods
// =============================================================================

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
	_, err := h.shims.Stat(outputsPath)
	if err == nil {
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
	} else if os.IsNotExist(err) {
		if err := h.shims.WriteFile(shimOutputsPath, nil, 0644); err != nil {
			return fmt.Errorf("failed to write shim outputs.tf: %w", err)
		}
	} else {
		return fmt.Errorf("failed to stat source outputs.tf: %w", err)
	}
	return nil
}
