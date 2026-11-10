// The module reader finds the module blocks of a Terraform file.
// It provides the source of each module block, and it evaluates no Terraform.

package terraform

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// =============================================================================
// Constants
// =============================================================================

// moduleBlockSchema selects the module blocks of a Terraform file.
var moduleBlockSchema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "module", LabelNames: []string{"name"}}}}

// moduleSourceSchema selects the source attribute of a module block.
var moduleSourceSchema = &hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "source"}}}

// =============================================================================
// Types
// =============================================================================

// ModuleSource is the source of one module block. Literal is false when the source is not a literal
// string, and then Source is empty.
type ModuleSource struct {
	Name    string
	Source  string
	Literal bool
}

// =============================================================================
// Public Methods
// =============================================================================

// ParseModuleSources returns the source of each module block in the content of a Terraform file. A
// module block with no source gives no entry. It returns an error for content that does not parse.
func ParseModuleSources(filename string, content []byte) ([]ModuleSource, error) {
	parsed, diags := hclsyntax.ParseConfig(content, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}
	body, _, _ := parsed.Body.PartialContent(moduleBlockSchema)
	var sources []ModuleSource
	for _, block := range body.Blocks {
		moduleBody, _, _ := block.Body.PartialContent(moduleSourceSchema)
		attr, ok := moduleBody.Attributes["source"]
		if !ok {
			continue
		}
		module := ModuleSource{Name: block.Labels[0]}
		value, valueDiags := attr.Expr.Value(nil)
		if !valueDiags.HasErrors() && value.IsKnown() && !value.IsNull() && value.Type().FriendlyName() == "string" {
			module.Source = value.AsString()
			module.Literal = true
		}
		sources = append(sources, module)
	}
	return sources, nil
}
