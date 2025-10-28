package types

import (
	"github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/secrets"
	"github.com/windsorcli/cli/pkg/shell"
)

// ExecutionContext holds common execution values and core dependencies used across the Windsor CLI.
// These fields are set during various initialization steps rather than computed on-demand.
// Includes secret providers for Sops and 1Password, enabling access to secrets across all contexts.
type ExecutionContext struct {
	// ContextName is the current context name
	ContextName string

	// ProjectRoot is the project root directory path
	ProjectRoot string

	// ConfigRoot is the config root directory (<projectRoot>/contexts/<contextName>)
	ConfigRoot string

	// TemplateRoot is the template directory (<projectRoot>/contexts/_template)
	TemplateRoot string

	// Injector is the dependency injector
	Injector di.Injector

	// Core dependencies
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell

	// Blueprint contains the generated blueprint data from the resources package
	Blueprint *v1alpha1.Blueprint

	// SecretsProviders contains providers for Sops and 1Password secrets management
	SecretsProviders struct {
		Sops        secrets.SecretsProvider
		Onepassword secrets.SecretsProvider
	}
}
