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

// PrintEnvVars prints the provided environment variables in a sorted order.
// If the value of an environment variable is an empty string, it will print an unset command.
func (s *DefaultShell) PrintEnvVars(envVars map[string]string) {
	// Create a slice to hold the keys of the envVars map
	keys := make([]string, 0, len(envVars))

	// Append each key from the envVars map to the keys slice
	for k := range envVars {
		keys = append(keys, k)
	}

	// Sort the keys slice to ensure the environment variables are printed in order
	sort.Strings(keys)

	// Iterate over the sorted keys and print the corresponding environment variable
	for _, k := range keys {
		if envVars[k] == "" {
			// Print unset command if the value is an empty string
			fmt.Printf("unset %s\n", k)
		} else {
			// Print export command with the key and value
			fmt.Printf("export %s=\"%s\"\n", k, envVars[k])
		}
	}
}

// PrintAlias prints the aliases for the shell.
func (s *DefaultShell) PrintAlias(aliases map[string]string) {
	// Create a slice to hold the keys of the aliases map
	keys := make([]string, 0, len(aliases))

	// Append each key from the aliases map to the keys slice
	for k := range aliases {
		keys = append(keys, k)
	}

	// Sort the keys slice to ensure the aliases are printed in order
	sort.Strings(keys)

	// Iterate over the sorted keys and print the corresponding alias
	for _, k := range keys {
		if aliases[k] == "" {
			// Print unset command if the value is an empty string
			fmt.Printf("unalias %s\n", k)
		} else {
			// Print alias command with the key and value
			fmt.Printf("alias %s=\"%s\"\n", k, aliases[k])
		}
	}
}

// UnsetEnvs generates a command to unset multiple environment variables.
// For Unix shells, this produces a single 'unset' command with all variables in one line.
func (s *DefaultShell) UnsetEnvs(envVars []string) {
	if len(envVars) == 0 {
		return
	}

	// Create a single unset command with all environment variables
	fmt.Printf("unset %s\n", strings.Join(envVars, " "))
}

// UnsetAlias generates commands to unset multiple aliases.
// For Unix shells, this produces a separate 'unalias' command for each alias.
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) == 0 {
		return
	}

	// Print individual unalias commands for each alias
	for _, alias := range aliases {
		fmt.Printf("unalias %s\n", alias)
	}
}
