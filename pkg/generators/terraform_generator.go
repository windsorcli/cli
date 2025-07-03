package generators

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/google/go-jsonnet"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/zclconf/go-cty/cty"
)

// The TerraformGenerator is a specialized component that manages Terraform configuration files.
// It provides functionality to create and update Terraform modules, variables, and tfvars files.
// The TerraformGenerator ensures proper infrastructure-as-code configuration for Windsor projects,
// maintaining consistent Terraform structure across all contexts.

// =============================================================================
// Types
// =============================================================================

// TerraformGenerator is a generator that writes Terraform files
type TerraformGenerator struct {
	BaseGenerator
	reset bool
}

// VariableInfo holds metadata for a single Terraform variable
type VariableInfo struct {
	Name        string
	Description string
	Default     any
	Sensitive   bool
}

// TerraformInitOutput represents the JSON output from terraform init
type TerraformInitOutput struct {
	Level     string `json:"@level"`
	Message   string `json:"@message"`
	Module    string `json:"@module"`
	Timestamp string `json:"@timestamp"`
	Type      string `json:"type"`
}

// =============================================================================
// Constructor
// =============================================================================

// NewTerraformGenerator creates a new TerraformGenerator with the provided dependency injector.
// It initializes the base generator and prepares it for Terraform file generation.
func NewTerraformGenerator(injector di.Injector) *TerraformGenerator {
	return &TerraformGenerator{
		BaseGenerator: *NewGenerator(injector),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Write generates Terraform configuration files including module shims and tfvars files for all components.
// It processes jsonnet templates from contexts/_template/terraform directory to merge template values into
// blueprint terraform components, then handles components by generating module shims for remote sources and
// creating corresponding tfvars files. The function manages terraform state cleanup on reset.
func (g *TerraformGenerator) Write(overwrite ...bool) error {
	shouldOverwrite := false
	if len(overwrite) > 0 {
		shouldOverwrite = overwrite[0]
	}
	g.reset = shouldOverwrite

	// Preload all OCI artifacts before processing components
	ociArtifacts, err := g.preloadOCIArtifacts()
	if err != nil {
		return fmt.Errorf("failed to preload OCI artifacts: %w", err)
	}

	components := g.blueprintHandler.GetTerraformComponents()

	templateValues, err := g.processTemplates(shouldOverwrite)
	if err != nil {
		return fmt.Errorf("failed to process terraform templates: %w", err)
	}

	for i, component := range components {
		if values, exists := templateValues[component.Path]; exists {
			if component.Values == nil {
				component.Values = make(map[string]any)
			}
			maps.Copy(component.Values, values)
			components[i] = component
		}
	}

	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return fmt.Errorf("failed to get config root: %w", err)
	}

	if g.reset {
		terraformStateDir := filepath.Join(contextPath, ".terraform")
		if _, err := g.shims.Stat(terraformStateDir); err == nil {
			if err := g.shims.RemoveAll(terraformStateDir); err != nil {
				return fmt.Errorf("failed to remove .terraform directory: %w", err)
			}
		}
	}

	terraformFolderPath := filepath.Join(projectRoot, "terraform")
	if err := g.shims.MkdirAll(terraformFolderPath, 0755); err != nil {
		return fmt.Errorf("failed to create terraform directory: %w", err)
	}

	for _, component := range components {
		if component.Source != "" {
			if err := g.generateModuleShim(component, ociArtifacts); err != nil {
				return fmt.Errorf("failed to generate module shim: %w", err)
			}
		}

		if err := g.writeTfvarsFile(contextPath, component); err != nil {
			return fmt.Errorf("failed to write tfvars file: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// processTemplates discovers and processes jsonnet template files from the contexts/_template/terraform directory.
// It checks for template directory existence, retrieves the current context configuration, and recursively
// walks through template files to generate corresponding .tfvars files. The function handles template
// discovery, context resolution, and delegates actual processing to walkTemplateDirectory.
func (g *TerraformGenerator) processTemplates(reset bool) (map[string]map[string]any, error) {
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get project root: %w", err)
	}

	templateDir := filepath.Join(projectRoot, "contexts", "_template", "terraform")

	if _, err := g.shims.Stat(templateDir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to check template directory: %w", err)
	}

	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get config root: %w", err)
	}

	contextName := g.configHandler.GetString("context")
	if contextName == "" {
		contextName = os.Getenv("WINDSOR_CONTEXT")
	}

	templateValues := make(map[string]map[string]any)

	return templateValues, g.walkTemplateDirectory(templateDir, contextPath, contextName, reset, templateValues)
}

// walkTemplateDirectory recursively traverses the template directory structure and processes jsonnet files.
// It handles both files and subdirectories, maintaining the directory structure in the output location.
// For each .jsonnet file found, it delegates processing to processJsonnetTemplate to collect template
// values that will be merged into terraform components.
func (g *TerraformGenerator) walkTemplateDirectory(templateDir, contextPath, contextName string, reset bool, templateValues map[string]map[string]any) error {
	entries, err := g.shims.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(templateDir, entry.Name())

		if entry.IsDir() {
			if err := g.walkTemplateDirectory(entryPath, contextPath, contextName, reset, templateValues); err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".jsonnet") {
			if err := g.processJsonnetTemplate(entryPath, contextName, templateValues); err != nil {
				return err
			}
		}
	}

	return nil
}

// processJsonnetTemplate processes a jsonnet template file and collects generated values
// for merging into blueprint terraform components. It evaluates the template with context data
// made available via std.extVar("context"), then stores the result in templateValues using
// the relative path from the template directory as the key.
// Templates must include: local context = std.extVar("context"); to access context data.
func (g *TerraformGenerator) processJsonnetTemplate(templateFile, contextName string, templateValues map[string]map[string]any) error {
	templateContent, err := g.shims.ReadFile(templateFile)
	if err != nil {
		return fmt.Errorf("error reading template file %s: %w", templateFile, err)
	}

	config := g.configHandler.GetConfig()

	contextYAML, err := g.configHandler.YamlMarshalWithDefinedPaths(config)
	if err != nil {
		return fmt.Errorf("error marshalling context to YAML: %w", err)
	}

	var contextMap map[string]any = make(map[string]any)
	if err := g.shims.YamlUnmarshal(contextYAML, &contextMap); err != nil {
		return fmt.Errorf("error unmarshalling context YAML: %w", err)
	}

	contextMap["name"] = contextName
	contextJSON, err := g.shims.JsonMarshal(contextMap)
	if err != nil {
		return fmt.Errorf("error marshalling context map to JSON: %w", err)
	}

	vm := jsonnet.MakeVM()
	vm.ExtCode("context", string(contextJSON))
	result, err := vm.EvaluateAnonymousSnippet("template.jsonnet", string(templateContent))
	if err != nil {
		return fmt.Errorf("error evaluating jsonnet template %s: %w", templateFile, err)
	}

	var values map[string]any
	if err := g.shims.JsonUnmarshal([]byte(result), &values); err != nil {
		return fmt.Errorf("jsonnet template must output valid JSON: %w", err)
	}

	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	templateDir := filepath.Join(projectRoot, "contexts", "_template", "terraform")
	relPath, err := g.shims.FilepathRel(templateDir, templateFile)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path: %w", err)
	}

	componentPath := strings.TrimSuffix(relPath, ".jsonnet")
	componentPath = strings.ReplaceAll(componentPath, "\\", "/") // Windows fix
	templateValues[componentPath] = values

	return nil
}

// generateModuleShim creates a local reference to a remote Terraform module.
// It provides a shim layer that maintains module configuration while allowing Windsor to manage it.
// The function orchestrates the creation of main.tf, variables.tf, and outputs.tf files for module initialization,
// handling both OCI and standard source types with proper path resolution and state management.
func (g *TerraformGenerator) generateModuleShim(component blueprintv1alpha1.TerraformComponent, ociArtifacts map[string][]byte) error {
	moduleDir := component.FullPath
	if err := g.shims.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	var resolvedSource string
	var modulePath string
	var err error

	if g.isOCISource(component.Source) {
		extractedPath, err := g.extractOCIModule(component.Source, component.Path, ociArtifacts)
		if err != nil {
			return fmt.Errorf("failed to extract OCI module: %w", err)
		}

		relPath, err := g.shims.FilepathRel(moduleDir, extractedPath)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}

		resolvedSource = relPath
		modulePath = extractedPath
	} else {
		resolvedSource = component.Source
		modulePath = moduleDir
	}

	if err := g.writeShimMainTf(moduleDir, resolvedSource); err != nil {
		return err
	}

	if !g.isOCISource(component.Source) {
		if err := g.shims.Chdir(moduleDir); err != nil {
			return fmt.Errorf("failed to change to module directory: %w", err)
		}

		modulePath, err = g.initializeTerraformModule(component)
		if err != nil {
			return err
		}
	}

	if err := g.writeShimVariablesTf(moduleDir, modulePath, resolvedSource); err != nil {
		return err
	}

	if err := g.writeShimOutputsTf(moduleDir, modulePath); err != nil {
		return err
	}

	return nil
}

// isOCISource determines if a source reference points to an OCI artifact by checking for OCI URL patterns
// and resolving source names through the blueprint handler. It handles both direct OCI URLs and named
// source references that map to OCI repositories, while excluding already-resolved extraction paths.
func (g *TerraformGenerator) isOCISource(source string) bool {
	if strings.Contains(source, ".oci_extracted") {
		return false
	}

	if strings.HasPrefix(source, "oci://") {
		return true
	}

	sources := g.blueprintHandler.GetSources()
	for _, src := range sources {
		if src.Name == source && strings.HasPrefix(src.Url, "oci://") {
			return true
		}
	}

	return false
}

// extractOCIModule extracts a specific terraform module from an OCI artifact.
// It handles OCI URL resolution, artifact caching, and module path extraction.
// The function manages the complete lifecycle from source lookup to module deployment,
// ensuring efficient caching and proper directory structure for terraform modules.
func (g *TerraformGenerator) extractOCIModule(source, path string, ociArtifacts map[string][]byte) (string, error) {
	message := fmt.Sprintf("📥 Loading component %s", path)

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	defer func() {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	}()

	sources := g.blueprintHandler.GetSources()
	var ociURL string
	for _, src := range sources {
		if src.Name == source {
			ociURL = src.Url
			break
		}
	}

	if ociURL == "" {
		return "", fmt.Errorf("source %s not found", source)
	}

	registry, repository, tag, err := g.parseOCIRef(ociURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse OCI reference: %w", err)
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	extractionKey := fmt.Sprintf("%s-%s-%s", registry, repository, tag)
	fullModulePath := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey, "terraform", path)
	if _, err := g.shims.Stat(fullModulePath); err == nil {
		return fullModulePath, nil
	}

	var artifactData []byte
	if cachedData, exists := ociArtifacts[cacheKey]; exists {
		artifactData = cachedData
	} else {
		artifactData, err = g.downloadOCIArtifact(registry, repository, tag)
		if err != nil {
			return "", fmt.Errorf("failed to download OCI artifact: %w", err)
		}
		ociArtifacts[cacheKey] = artifactData
	}

	if err := g.extractModuleFromArtifact(artifactData, path, extractionKey); err != nil {
		return "", fmt.Errorf("failed to extract module from artifact: %w", err)
	}

	return fullModulePath, nil
}

// downloadOCIArtifact downloads an OCI artifact and returns the tar.gz data.
// It provides OCI registry communication, image retrieval, and layer extraction.
// The function handles OCI reference parsing, remote image access, and data streaming.
// It ensures proper resource cleanup and memory-efficient artifact data retrieval.
func (g *TerraformGenerator) downloadOCIArtifact(registry, repository, tag string) ([]byte, error) {
	ref := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

	parsedRef, err := g.shims.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference %s: %w", ref, err)
	}

	img, err := g.shims.RemoteImage(parsedRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	layers, err := g.shims.ImageLayers(img)
	if err != nil {
		return nil, fmt.Errorf("failed to get image layers: %w", err)
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no layers found in image")
	}

	layer := layers[0]
	reader, err := g.shims.LayerUncompressed(layer)
	if err != nil {
		return nil, fmt.Errorf("failed to get layer reader: %w", err)
	}
	defer reader.Close()

	data, err := g.shims.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer data: %w", err)
	}

	return data, nil
}

// extractModuleFromArtifact extracts a specific terraform module from cached artifact data directly to .oci_extracted.
// It provides selective tar stream processing, directory structure creation, and file permission management.
// The function handles OCI artifact data extraction, module file deployment, and executable script permissions.
// It ensures proper file system operations with error handling and maintains original tar header permissions.
func (g *TerraformGenerator) extractModuleFromArtifact(artifactData []byte, modulePath, extractionKey string) error {
	projectRoot, err := g.shell.GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	reader := g.shims.NewBytesReader(artifactData)
	tarReader := g.shims.NewTarReader(reader)
	targetPrefix := "terraform/" + modulePath

	extractionDir := filepath.Join(projectRoot, ".windsor", ".oci_extracted", extractionKey)

	for {
		header, err := tarReader.Next()
		if err == g.shims.EOFError() {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		if !strings.HasPrefix(header.Name, targetPrefix) {
			continue
		}

		relativePath := strings.TrimPrefix(header.Name, "terraform/")
		destPath := filepath.Join(extractionDir, "terraform", relativePath)

		if header.Typeflag == g.shims.TypeDir() {
			if err := g.shims.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			continue
		}

		if err := g.shims.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
		}

		file, err := g.shims.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", destPath, err)
		}

		_, err = g.shims.Copy(file, tarReader)
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("failed to close file %s: %w", destPath, closeErr)
		}
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		modeValue := header.Mode & 0777
		if modeValue < 0 || modeValue > 0777 {
			return fmt.Errorf("invalid file mode %o for %s", header.Mode, destPath)
		}
		fileMode := os.FileMode(uint32(modeValue))

		if strings.HasSuffix(destPath, ".sh") {
			fileMode |= 0111
		}

		if err := g.shims.Chmod(destPath, fileMode); err != nil {
			return fmt.Errorf("failed to set file permissions for %s: %w", destPath, err)
		}
	}

	return nil
}

// writeShimMainTf creates the main.tf file for the shim module.
// It provides the initial module configuration with source reference.
// The function ensures proper HCL syntax and maintains consistent module structure.
// It handles file writing with appropriate permissions and error handling.
func (g *TerraformGenerator) writeShimMainTf(moduleDir, source string) error {
	mainContent := hclwrite.NewEmptyFile()
	block := mainContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()
	body.SetAttributeValue("source", cty.StringVal(source))

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), mainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}
	return nil
}

// initializeTerraformModule initializes the Terraform module and returns its path.
// It provides module initialization, path resolution, and environment setup.
// The function handles terraform init execution and module path detection.
// It ensures proper state directory configuration and error handling.
func (g *TerraformGenerator) initializeTerraformModule(component blueprintv1alpha1.TerraformComponent) (string, error) {
	contextPath, err := g.configHandler.GetConfigRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get config root: %w", err)
	}

	tfDataDir := filepath.Join(contextPath, ".terraform", component.Path)
	if err := g.shims.Setenv("TF_DATA_DIR", tfDataDir); err != nil {
		return "", fmt.Errorf("failed to set TF_DATA_DIR: %w", err)
	}

	output, err := g.shell.ExecProgress(
		fmt.Sprintf("📥 Loading component %s", component.Path),
		"terraform",
		"init",
		"--backend=false",
		"-input=false",
		"-upgrade",
		"-json",
	)
	if err != nil {
		return "", fmt.Errorf("failed to initialize terraform: %w", err)
	}

	detectedPath := ""
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var initOutput TerraformInitOutput
		if err := g.shims.JsonUnmarshal([]byte(line), &initOutput); err != nil {
			continue
		}
		if initOutput.Type == "log" {
			msg := initOutput.Message
			startIdx := strings.Index(msg, "- main in")
			if startIdx == -1 {
				continue
			}

			pathStart := startIdx + len("- main in")
			if pathStart >= len(msg) {
				continue
			}

			path := strings.TrimSpace(msg[pathStart:])
			if path == "" {
				continue
			}

			if _, err := g.shims.Stat(path); err == nil {
				detectedPath = path
				break
			}
		}
	}

	modulePath := filepath.Join(contextPath, ".terraform", component.Path, "modules", "main", "terraform", component.Path)
	if detectedPath != "" {
		if detectedPath != modulePath {
			fmt.Printf("\033[33mWarning: Using detected module path %s instead of standard path %s\033[0m\n", detectedPath, modulePath)
		}
		modulePath = detectedPath
	}

	return modulePath, nil
}

// writeShimVariablesTf creates the variables.tf file for the shim module.
// It extracts variable definitions from the source module's variables.tf file and generates
// a shim module that references these variables. The function reads the source variables.tf,
// creates a main.tf file with the module source reference, and generates a variables.tf file
// that preserves all variable attributes (description, type, default, sensitive) from the
// original module. This creates a transparent wrapper around the source module.
func (g *TerraformGenerator) writeShimVariablesTf(moduleDir, modulePath, source string) error {
	shimMainContent := hclwrite.NewEmptyFile()
	shimBlock := shimMainContent.Body().AppendNewBlock("module", []string{"main"})
	shimBody := shimBlock.Body()
	shimBody.SetAttributeRaw("source", hclwrite.TokensForValue(cty.StringVal(source)))

	variablesPath := filepath.Join(modulePath, "variables.tf")
	variablesContent, err := g.shims.ReadFile(variablesPath)
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

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "variables.tf"), shimVariablesContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim variables.tf: %w", err)
	}

	if err := g.shims.WriteFile(filepath.Join(moduleDir, "main.tf"), shimMainContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write shim main.tf: %w", err)
	}

	return nil
}

// writeShimOutputsTf creates the outputs.tf file for the shim module by extracting output definitions from the source module.
// It provides output definition extraction and shim generation that preserves descriptions while creating references to module.main outputs.
// The function ensures proper HCL syntax and maintains consistent output structure for terraform modules.
// It handles file reading, parsing, and writing with comprehensive error handling for module compatibility.
func (g *TerraformGenerator) writeShimOutputsTf(moduleDir, modulePath string) error {
	outputsPath := filepath.Join(modulePath, "outputs.tf")
	if _, err := g.shims.Stat(outputsPath); err == nil {
		outputsContent, err := g.shims.ReadFile(outputsPath)
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

					shimBlockBody.SetAttributeTraversal("value", hcl.Traversal{
						hcl.TraverseRoot{Name: "module"},
						hcl.TraverseAttr{Name: "main"},
						hcl.TraverseAttr{Name: outputName},
					})
				}
			}
		}

		if err := g.shims.WriteFile(filepath.Join(moduleDir, "outputs.tf"), shimOutputsContent.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write shim outputs.tf: %w", err)
		}
	}
	return nil
}

// writeModuleFile creates a main.tf file that defines the Terraform module configuration.
// It sets up the module source and creates variable references for all defined variables.
// The function ensures proper HCL syntax and maintains consistent module structure.
func (g *TerraformGenerator) writeModuleFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	moduleContent := hclwrite.NewEmptyFile()

	block := moduleContent.Body().AppendNewBlock("module", []string{"main"})
	body := block.Body()

	body.SetAttributeValue("source", cty.StringVal(component.Source))

	variablesTfPath := filepath.Join(dirPath, "variables.tf")
	variablesContent, err := g.shims.ReadFile(variablesTfPath)
	if err != nil {
		return fmt.Errorf("failed to read variables.tf: %w", err)
	}

	variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesTfPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("failed to parse variables.tf: %w", diags)
	}

	var variableNames []string
	for _, block := range variablesFile.Body().Blocks() {
		if block.Type() == "variable" && len(block.Labels()) > 0 {
			variableNames = append(variableNames, block.Labels()[0])
		}
	}
	sort.Strings(variableNames)

	for _, variableName := range variableNames {
		body.SetAttributeTraversal(variableName, hcl.Traversal{
			hcl.TraverseRoot{Name: "var"},
			hcl.TraverseAttr{Name: variableName},
		})
	}

	filePath := filepath.Join(dirPath, "main.tf")

	if err := g.shims.WriteFile(filePath, moduleContent.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

// writeTfvarsFile creates or updates a .tfvars file with variable values for the Terraform module.
// It uses variables.tf as the basis for variable definitions and allows component.Values to override specific values.
// The function maintains a header indicating Windsor CLI management and handles module source comments.
// If the file already exists, it will not be overwritten.
func (g *TerraformGenerator) writeTfvarsFile(dirPath string, component blueprintv1alpha1.TerraformComponent) error {
	protectedValues := map[string]bool{
		"context_path": true,
		"os_type":      true,
		"context_id":   true,
	}

	componentPath := filepath.Join(dirPath, "terraform", component.Path)
	tfvarsFilePath := componentPath + ".tfvars"
	variablesTfPath := filepath.Join(component.FullPath, "variables.tf")

	if !g.reset {
		if err := g.checkExistingTfvarsFile(tfvarsFilePath); err != nil {
			if err == os.ErrExist {
				return nil
			}
			return err
		}
	}

	mergedFile := hclwrite.NewEmptyFile()
	body := mergedFile.Body()

	addTfvarsHeader(body, component.Source)

	variables, err := g.parseVariablesFile(variablesTfPath, protectedValues)
	if err != nil {
		return err
	}

	if len(component.Values) > 0 {
		writeComponentValues(body, component.Values, protectedValues, variables)
	} else {
		writeDefaultValues(body, variables, component.Values)
	}

	parentDir := filepath.Dir(tfvarsFilePath)
	if err := g.shims.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := g.shims.WriteFile(tfvarsFilePath, mergedFile.Bytes(), 0644); err != nil {
		return fmt.Errorf("error writing tfvars file: %w", err)
	}

	return nil
}

// checkExistingTfvarsFile checks if a tfvars file exists and is readable.
// Returns os.ErrExist if the file exists and is readable, or an error if the file exists but is not readable.
func (g *TerraformGenerator) checkExistingTfvarsFile(tfvarsFilePath string) error {
	_, err := g.shims.Stat(tfvarsFilePath)
	if err == nil {
		_, err := g.shims.ReadFile(tfvarsFilePath)
		if err != nil {
			return fmt.Errorf("failed to read existing tfvars file: %w", err)
		}
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error checking tfvars file: %w", err)
	}
	return nil
}

// parseVariablesFile parses variables.tf and returns metadata about the variables.
// It extracts variable names, descriptions, default values, and sensitivity flags.
// Protected values are excluded from the returned metadata.
func (g *TerraformGenerator) parseVariablesFile(variablesTfPath string, protectedValues map[string]bool) ([]VariableInfo, error) {
	variablesContent, err := g.shims.ReadFile(variablesTfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read variables.tf: %w", err)
	}

	variablesFile, diags := hclwrite.ParseConfig(variablesContent, variablesTfPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse variables.tf: %w", diags)
	}

	var variables []VariableInfo
	for _, block := range variablesFile.Body().Blocks() {
		if block.Type() == "variable" && len(block.Labels()) > 0 {
			variableName := block.Labels()[0]

			if protectedValues[variableName] {
				continue
			}

			info := VariableInfo{
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

			variables = append(variables, info)
		}
	}

	return variables, nil
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
// Handles sensitive variables and preserves variable order from variables.tf.
func writeComponentValues(body *hclwrite.Body, values map[string]any, protectedValues map[string]bool, variables []VariableInfo) {
	for _, info := range variables {
		if protectedValues[info.Name] {
			continue
		}

		body.AppendNewline()

		if info.Description != "" {
			body.AppendUnstructuredTokens(hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte("# " + info.Description)},
			})
			body.AppendNewline()
		}

		if val, exists := values[info.Name]; exists {
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

		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte(fmt.Sprintf("# %s = null", info.Name))},
		})
		body.AppendNewline()
	}
}

// writeDefaultValues writes only the default values from variables.tf to the tfvars file body.
// This is an alias for writeComponentValues with no explicit values, ensuring all defaults are commented.
func writeDefaultValues(body *hclwrite.Body, variables []VariableInfo, componentValues map[string]any) {
	writeComponentValues(body, componentValues, map[string]bool{}, variables)
}

// writeHeredoc writes a multi-line string value as a heredoc assignment in the tfvars file body.
// Used for YAML or other multi-line string values to preserve formatting.
func writeHeredoc(body *hclwrite.Body, name string, content string) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOHeredoc, Bytes: []byte("<<EOF")},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		{Type: hclsyntax.TokenStringLit, Bytes: []byte(content)},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		{Type: hclsyntax.TokenCHeredoc, Bytes: []byte("EOF")},
	}
	body.SetAttributeRaw(name, tokens)
	body.AppendNewline()
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
	case map[string]any:
		rendered := formatValue(v)
		assignment := fmt.Sprintf("%s = %s", name, rendered)
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(assignment)},
		})
		body.AppendNewline()
		return
	}

	body.SetAttributeValue(name, convertToCtyValue(value))
}

// formatValue formats a Go value as a valid HCL literal string for tfvars output.
// Handles strings, lists, maps, nested objects, and nil values with proper indentation and quoting.
func formatValue(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
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
func convertToCtyValue(value any) cty.Value {
	switch v := value.(type) {
	case string:
		return cty.StringVal(v)
	case int:
		return cty.NumberIntVal(int64(v))
	case float64:
		return cty.NumberFloatVal(v)
	case bool:
		return cty.BoolVal(v)
	case []any:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		var ctyList []cty.Value
		for _, item := range v {
			ctyList = append(ctyList, convertToCtyValue(item))
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

// parseOCIRef parses an OCI reference into registry, repository, and tag components.
// It validates the OCI reference format and extracts the individual components for artifact resolution.
func (g *TerraformGenerator) parseOCIRef(ociRef string) (registry, repository, tag string, err error) {
	if !strings.HasPrefix(ociRef, "oci://") {
		return "", "", "", fmt.Errorf("invalid OCI reference format: %s", ociRef)
	}

	ref := strings.TrimPrefix(ociRef, "oci://")

	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}

	repoWithRegistry := parts[0]
	tag = parts[1]

	repoParts := strings.SplitN(repoWithRegistry, "/", 2)
	if len(repoParts) != 2 {
		return "", "", "", fmt.Errorf("invalid OCI reference format, expected registry/repository:tag: %s", ociRef)
	}

	registry = repoParts[0]
	repository = repoParts[1]

	return registry, repository, tag, nil
}

// preloadOCIArtifacts analyzes all blueprint sources and downloads unique OCI artifacts upfront.
// It returns a map of cached artifacts keyed by their registry/repository:tag identifier.
func (g *TerraformGenerator) preloadOCIArtifacts() (map[string][]byte, error) {
	sources := g.blueprintHandler.GetSources()
	ociArtifacts := make(map[string][]byte)

	uniqueOCISources := make(map[string]bool)
	for _, source := range sources {
		if strings.HasPrefix(source.Url, "oci://") {
			uniqueOCISources[source.Url] = true
		}
	}

	if len(uniqueOCISources) == 0 {
		return ociArtifacts, nil
	}

	message := "📦 Loading OCI sources"
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	defer func() {
		spin.Stop()
		fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	}()

	for _, source := range sources {
		if !strings.HasPrefix(source.Url, "oci://") {
			continue
		}

		registry, repository, tag, err := g.parseOCIRef(source.Url)
		if err != nil {
			return nil, fmt.Errorf("failed to parse OCI reference for source %s: %w", source.Name, err)
		}

		cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)

		if _, exists := ociArtifacts[cacheKey]; exists {
			continue
		}

		artifactData, err := g.downloadOCIArtifact(registry, repository, tag)
		if err != nil {
			return nil, fmt.Errorf("failed to download OCI artifact for source %s: %w", source.Name, err)
		}

		ociArtifacts[cacheKey] = artifactData
	}

	return ociArtifacts, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure TerraformGenerator implements Generator
var _ Generator = (*TerraformGenerator)(nil)
