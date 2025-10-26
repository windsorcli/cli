//go:build darwin || linux
// +build darwin linux

package shell

import (
	"fmt"
	"sort"
	"strings"
)

// The UnixShell is a platform-specific implementation of shell operations for Unix-like systems.
// It provides Unix-specific implementations of environment variable and alias management.
// It handles the differences between Unix shells and Windows PowerShell.
// Key features include Unix-specific command generation for environment variables and aliases.

// =============================================================================
// Public Methods
// =============================================================================

// UnsetEnvs generates a single unset command for multiple environment variables in Unix shells.
// It prints a single 'unset' command with all provided variable names separated by spaces.
// If the input slice is empty, no output is produced.
func (s *DefaultShell) UnsetEnvs(envVars []string) {
	if len(envVars) == 0 {
		return
	}
	fmt.Printf("unset %s\n", strings.Join(envVars, " "))
}

// UnsetAlias generates individual unalias commands for each alias in Unix shells.
// It prints a separate 'unalias' command for each alias name provided.
// If the input slice is empty, no output is produced.
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) == 0 {
		return
	}
	for _, alias := range aliases {
		fmt.Printf("unalias %s\n", alias)
	}
}

// RenderAliases returns the rendered aliases as a string using Unix shell syntax
func (s *DefaultShell) RenderAliases(aliases map[string]string) string {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if aliases[k] == "" {
			result.WriteString(fmt.Sprintf("unalias %s\n", k))
		} else {
			result.WriteString(fmt.Sprintf("alias %s=\"%s\"\n", k, aliases[k]))
		}
	}
	return result.String()
}

// =============================================================================
// Private Methods
// =============================================================================

// renderEnvVarsWithExport returns environment variables in sorted order using export commands as a string.
// If a variable's value is empty, it returns an unset command instead.
func (s *DefaultShell) renderEnvVarsWithExport(envVars map[string]string) string {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	for _, k := range keys {
		if envVars[k] == "" {
			result.WriteString(fmt.Sprintf("unset %s\n", k))
		} else {
			result.WriteString(fmt.Sprintf("export %s=\"%s\"\n", k, envVars[k]))
		}
	}
	return result.String()
}
