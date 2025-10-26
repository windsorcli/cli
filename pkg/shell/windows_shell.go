//go:build windows
// +build windows

package shell

import (
	"fmt"
	"sort"
	"strings"
)

// The WindowsShell is a platform-specific implementation of shell operations for Windows systems.
// It provides Windows PowerShell-specific implementations of environment variable and alias management.
// It handles the differences between Windows PowerShell and Unix shells.
// Key features include PowerShell-specific command generation for environment variables and aliases.

// =============================================================================
// Public Methods
// =============================================================================

// UnsetEnvs generates commands to unset multiple environment variables.
// For Windows PowerShell, this produces a Remove-Item command for each environment variable.
func (s *DefaultShell) UnsetEnvs(envVars []string) {
	if len(envVars) == 0 {
		return
	}

	// Print Remove-Item commands for each environment variable
	for _, env := range envVars {
		fmt.Printf("Remove-Item Env:%s\n", env)
	}
}

// UnsetAlias generates commands to unset multiple aliases.
// For Windows PowerShell, this produces a Remove-Item command for each alias.
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) == 0 {
		return
	}

	// Print Remove-Item commands for each alias
	for _, alias := range aliases {
		fmt.Printf("Remove-Item Alias:%s\n", alias)
	}
}

// RenderAliases returns the rendered aliases as a string using PowerShell syntax
func (s *DefaultShell) RenderAliases(aliases map[string]string) string {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if aliases[k] == "" {
			result.WriteString(fmt.Sprintf("Remove-Item Alias:%s\n", k))
		} else {
			result.WriteString(fmt.Sprintf("Set-Alias %s '%s'\n", k, aliases[k]))
		}
	}
	return result.String()
}

// =============================================================================
// Private Methods
// =============================================================================

// renderEnvVarsWithExport returns environment variables with PowerShell commands as a string. Empty values trigger a removal command.
func (s *DefaultShell) renderEnvVarsWithExport(envVars map[string]string) string {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if envVars[k] == "" {
			result.WriteString(fmt.Sprintf("Remove-Item Env:%s\n", k))
		} else {
			result.WriteString(fmt.Sprintf("$env:%s='%s'\n", k, envVars[k]))
		}
	}
	return result.String()
}
