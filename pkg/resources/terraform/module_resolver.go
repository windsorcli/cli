package terraform

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/resources/blueprint"
	"github.com/windsorcli/cli/pkg/shell"
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
	Initialize() error
	ProcessModules() error
}

// =============================================================================
// Types
// =============================================================================

// BaseModuleResolver provides common functionality for all module resolvers
type BaseModuleResolver struct {
	shims            *Shims
	injector         di.Injector
	shell            shell.Shell
	blueprintHandler blueprint.BlueprintHandler
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseModuleResolver creates a new base module resolver
func NewBaseModuleResolver(injector di.Injector) *BaseModuleResolver {
	return &BaseModuleResolver{
		shims:    NewShims(),
		injector: injector,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the base module resolver
func (h *BaseModuleResolver) Initialize() error {
	shellInterface := h.injector.Resolve("shell")
	var ok bool
	h.shell, ok = shellInterface.(shell.Shell)
	if !ok {
		return fmt.Errorf("failed to resolve shell")
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

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

// writeShimVariablesTf creates the variables.tf file for the shim module by reading the original
// module's variables.tf file and generating corresponding variable blocks and module arguments.
// It parses variable definitions from the source module, creates shim variable blocks that preserve
// all attributes (description, type, default, sensitive), and configures the main module block
// to pass through all variables using var.variable_name references.
func (h *BaseModuleResolver) writeShimVariablesTf(moduleDir, modulePath, source string) error {
	shimMainContent := hclwrite.NewEmptyFile()
	shimBlock := shimMainContent.Body().AppendNewBlock("module", []string{"main"})
	shimBody := shimBlock.Body()
	shimBody.SetAttributeRaw("source", hclwrite.TokensForValue(cty.StringVal(source)))

	variablesPath := filepath.Join(modulePath, "variables.tf")
	if _, err := h.shims.Stat(variablesPath); err != nil {
		if err := h.shims.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to write empty variables.tf: %w", err)
		}
		if err := h.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim main.tf: %w", err)
		}
		return nil
	}

	variablesContent, err := h.shims.ReadFile(variablesPath)
	if err != nil {
		return fmt.Errorf("failed to read variables.tf: %w", err)
	}

	variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("failed to parse variables.tf: %w", diags)
	}

	shimVariablesContent := hclwrite.NewEmptyFile()
	shimVariablesBody := shimVariablesContent.Body()

	for _, block := range variablesFile.Body().Blocks() {
		if block.Type() == "variable" {
			labels := block.Labels()
			if len(labels) > 0 {
				variableName := labels[0]

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
// module.main.output_name traversal pattern.
func (h *BaseModuleResolver) writeShimOutputsTf(moduleDir, modulePath string) error {
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

		if err := h.shims.WriteFile(filepath.Join(moduleDir, "outputs.tf"), shimOutputsContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim outputs.tf: %w", err)
		}
	}
	return nil
}
