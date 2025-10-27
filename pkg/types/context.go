package types

import (
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/shell"
)

// ExecutionContext holds common execution values and core dependencies used across the Windsor CLI.
// These fields are set during various initialization steps rather than computed on-demand.
type ExecutionContext struct {
	// ContextName is the current context name
	ContextName string

	// ProjectRoot is the project root directory path
	ProjectRoot string

	// ConfigRoot is the config root directory (<projectRoot>/contexts/<contextName>)
	ConfigRoot string

	// TemplateRoot is the template directory (<projectRoot>/contexts/_template)
	TemplateRoot string

	// Core dependencies
	ConfigHandler config.ConfigHandler
	Shell         shell.Shell
}
