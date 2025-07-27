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

// printEnvVarsWithExport prints environment variables in sorted order using export commands.
// If a variable's value is empty, it prints an unset command instead.
func (s *DefaultShell) printEnvVarsWithExport(envVars map[string]string) {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if envVars[k] == "" {
			fmt.Printf("unset %s\n", k)
		} else {
			fmt.Printf("export %s=\"%s\"\n", k, envVars[k])
		}
	}
}

// PrintAlias prints sorted aliases. Empty values print unalias; non-empty print alias with key and value.
func (s *DefaultShell) PrintAlias(aliases map[string]string) {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if aliases[k] == "" {
			fmt.Printf("unalias %s\n", k)
		} else {
			fmt.Printf("alias %s=\"%s\"\n", k, aliases[k])
		}
	}
}

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
