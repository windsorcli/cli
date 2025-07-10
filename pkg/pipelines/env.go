package pipelines

import (
	"context"
	"fmt"
	"os"

	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/env"
	"github.com/windsorcli/cli/pkg/secrets"
)

// The EnvPipeline is a specialized component that manages environment variable printing functionality.
// It provides environment-specific command execution including environment variable collection,
// secrets decryption, and environment injection for the Windsor CLI env command.
// The EnvPipeline handles environment variable management with proper initialization and validation.

// =============================================================================
// Types
// =============================================================================

// EnvPipeline provides environment variable printing functionality
type EnvPipeline struct {
	BasePipeline
	envPrinters      []env.EnvPrinter
	secretsProviders []secrets.SecretsProvider
}

// =============================================================================
// Constructor
// =============================================================================

// NewEnvPipeline creates a new EnvPipeline instance
func NewEnvPipeline() *EnvPipeline {
	return &EnvPipeline{
		BasePipeline: *NewBasePipeline(),
	}
}

// NewDefaultEnvPipeline creates a new EnvPipeline with all default constructors
func NewDefaultEnvPipeline() *EnvPipeline {
	return NewEnvPipeline()
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize creates and registers all required components for the env pipeline including
// secrets providers and environment printers. It validates dependencies and ensures
// proper initialization of all components required for environment variable management.
func (p *EnvPipeline) Initialize(injector di.Injector, ctx context.Context) error {
	if err := p.BasePipeline.Initialize(injector, ctx); err != nil {
		return err
	}

	secretsProviders, err := p.withSecretsProviders()
	if err != nil {
		return fmt.Errorf("failed to create secrets providers: %w", err)
	}
	p.secretsProviders = secretsProviders

	for i, secretsProvider := range p.secretsProviders {
		providerKey := fmt.Sprintf("secretsProvider_%d", i)
		p.injector.Register(providerKey, secretsProvider)
	}

	for _, secretsProvider := range p.secretsProviders {
		if err := secretsProvider.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize secrets provider: %w", err)
		}
	}

	envPrinters, err := p.withEnvPrinters()
	if err != nil {
		return fmt.Errorf("failed to create env printers: %w", err)
	}
	p.envPrinters = envPrinters

	for _, envPrinter := range p.envPrinters {
		if err := envPrinter.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize env printer: %w", err)
		}
	}

	return nil
}

// Execute runs the environment variable logic by checking directory trust status,
// handling session reset, loading secrets if requested, collecting and injecting
// environment variables into the process, and printing them unless in quiet mode.
func (p *EnvPipeline) Execute(ctx context.Context) error {
	isTrusted := p.shell.CheckTrustedDirectory() == nil
	hook, _ := ctx.Value("hook").(bool)
	quiet, _ := ctx.Value("quiet").(bool)

	if !isTrusted {
		p.shell.Reset(quiet)
		if !hook {
			fmt.Fprintf(os.Stderr, "\033[33mWarning: You are not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve.\033[0m\n")
		}
		return nil
	}

	if err := p.handleSessionReset(); err != nil {
		return fmt.Errorf("failed to handle session reset: %w", err)
	}

	if decrypt, ok := ctx.Value("decrypt").(bool); ok && decrypt && len(p.secretsProviders) > 0 {
		for _, secretsProvider := range p.secretsProviders {
			if err := secretsProvider.LoadSecrets(); err != nil {
				verbose, _ := ctx.Value("verbose").(bool)
				if verbose {
					return fmt.Errorf("failed to load secrets: %w", err)
				}
				return nil
			}
		}
	}

	allEnvVars := make(map[string]string)
	for _, envPrinter := range p.envPrinters {
		envVars, err := envPrinter.GetEnvVars()
		if err != nil {
			return fmt.Errorf("error getting environment variables: %w", err)
		}

		for key, value := range envVars {
			allEnvVars[key] = value
		}
	}

	for key, value := range allEnvVars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("error setting environment variable %s: %w", key, err)
		}
	}

	if !quiet {
		p.shell.PrintEnvVars(allEnvVars)

		var firstError error
		for _, envPrinter := range p.envPrinters {
			if err := envPrinter.PostEnvHook(); err != nil && firstError == nil {
				firstError = fmt.Errorf("failed to execute post env hook: %w", err)
			}
		}

		verbose, _ := ctx.Value("verbose").(bool)
		if verbose {
			return firstError
		}
	}

	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// =============================================================================
// Interface Compliance
// =============================================================================

var _ Pipeline = (*EnvPipeline)(nil)
