package pipelines

import (
	"context"
	"fmt"

	bundler "github.com/windsorcli/cli/pkg/artifact"
	"github.com/windsorcli/cli/pkg/di"
)

// The ArtifactPipeline is a specialized component that manages artifact creation and distribution functionality.
// It provides unified artifact processing including bundling of files and final artifact creation or distribution.
// The ArtifactPipeline supports multiple execution modes (bundle, push) through context-based configuration,
// eliminating code duplication between bundle and push operations while maintaining flexibility for future extensions.

// =============================================================================
// Types
// =============================================================================

// ArtifactPipeline provides artifact creation and distribution functionality
type ArtifactPipeline struct {
	BasePipeline
	artifactBuilder bundler.Artifact
	bundlers        []bundler.Bundler
}

// =============================================================================
// Constructor
// =============================================================================

// NewArtifactPipeline creates a new ArtifactPipeline instance
func NewArtifactPipeline() *ArtifactPipeline {
	return &ArtifactPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the artifact pipeline components including artifact builder and bundlers.
// It initializes the components needed for artifact creation and distribution functionality.
func (p *ArtifactPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	p.artifactBuilder = p.withArtifactBuilder()

	bundlers, err := p.withBundlers()
	if err != nil {
		return fmt.Errorf("failed to create bundlers: %w", err)
	}
	p.bundlers = bundlers

	if p.artifactBuilder != nil {
		if err := p.artifactBuilder.Initialize(p.injector); err != nil {
			return fmt.Errorf("failed to initialize artifact builder: %w", err)
		}
	}

	for _, bundler := range p.bundlers {
		if err := bundler.Initialize(p.injector); err != nil {
			return fmt.Errorf("failed to initialize bundler: %w", err)
		}
	}

	return nil
}

// Execute runs the artifact pipeline, performing bundling operations and final artifact creation or distribution.
// The execution mode is determined by context values:
// - "artifactMode": "bundle" for file creation, "push" for registry distribution
// - "outputPath": output path for bundle mode
// - "tag": tag for both modes
// - "registryBase": registry base URL for push mode
// - "repoName": repository name for push mode
func (p *ArtifactPipeline) Execute(ctx context.Context) error {
	if p.artifactBuilder == nil {
		return fmt.Errorf("artifact builder not available")
	}

	// Run all bundlers to collect files into the artifact
	for _, bundler := range p.bundlers {
		if err := bundler.Bundle(p.artifactBuilder); err != nil {
			return fmt.Errorf("bundling failed: %w", err)
		}
	}

	// Determine execution mode from context
	mode, ok := ctx.Value("artifactMode").(string)
	if !ok {
		return fmt.Errorf("artifact mode not specified in context")
	}

	switch mode {
	case "bundle":
		return p.executeBundleMode(ctx)
	case "push":
		return p.executePushMode(ctx)
	default:
		return fmt.Errorf("unknown artifact mode: %s", mode)
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// executeBundleMode creates a tar.gz artifact file on disk using the provided context parameters.
// It retrieves the output path and optional tag from the context, invokes the artifact builder's Create method,
// and prints a confirmation message with the resulting artifact path. Returns an error if required context values
// are missing or if artifact creation fails.
func (p *ArtifactPipeline) executeBundleMode(ctx context.Context) error {
	outputPath, ok := ctx.Value("outputPath").(string)
	if !ok {
		return fmt.Errorf("output path not specified in context for bundle mode")
	}

	tag, ok := ctx.Value("tag").(string)
	if !ok {
		tag = ""
	}

	actualOutputPath, err := p.artifactBuilder.Create(outputPath, tag)
	if err != nil {
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	fmt.Printf("Blueprint bundled successfully: %s\n", actualOutputPath)
	return nil
}

// executePushMode uploads the artifact to an OCI registry using the provided context parameters.
// It retrieves the registry base, repository name, and optional tag from the context, then invokes
// the artifact builder's Push method. On success, it prints a confirmation message indicating the
// destination. Returns an error if required context values are missing or if the push operation fails.
func (p *ArtifactPipeline) executePushMode(ctx context.Context) error {
	registryBase, ok := ctx.Value("registryBase").(string)
	if !ok {
		return fmt.Errorf("registry base not specified in context for push mode")
	}

	repoName, ok := ctx.Value("repoName").(string)
	if !ok {
		return fmt.Errorf("repository name not specified in context for push mode")
	}

	tag, ok := ctx.Value("tag").(string)
	if !ok {
		tag = ""
	}

	if err := p.artifactBuilder.Push(registryBase, repoName, tag); err != nil {
		return fmt.Errorf("failed to push artifact: %w", err)
	}

	if tag != "" {
		fmt.Printf("Blueprint pushed successfully to %s/%s:%s\n", registryBase, repoName, tag)
	} else {
		fmt.Printf("Blueprint pushed successfully to %s/%s\n", registryBase, repoName)
	}
	return nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*ArtifactPipeline)(nil)
