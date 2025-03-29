//go:build darwin || linux
// +build darwin linux

package shell

import (
	"fmt"
	"sort"
	"strings"
)

// PrintEnvVars prints the provided environment variables in a sorted order.
// If the value of an environment variable is an empty string, it will print an unset command.
func (s *DefaultShell) PrintEnvVars(envVars map[string]string) error {
	// Create a slice to hold the keys of the envVars map
	keys := make([]string, 0, len(envVars))

	// Append each key from the envVars map to the keys slice
	for k := range envVars {
		keys = append(keys, k)
	}

	// Sort the keys slice to ensure the environment variables are printed in order
	sort.Strings(keys)

	// Create a slice to hold the keys that need to be unset
	var unsetKeys []string

	// Iterate over the sorted keys and print the corresponding environment variable
	for _, k := range keys {
		if envVars[k] == "" {
			// Add to unsetKeys if the value is an empty string
			unsetKeys = append(unsetKeys, k)
		} else {
			// Print export command with the key and value
			fmt.Printf("export %s=\"%s\"\n", k, envVars[k])
		}
	}

	// Call UnsetEnvVars to print unset commands for all keys with empty values
	if len(unsetKeys) > 0 {
		s.UnsetEnvVars(unsetKeys)
	}

	return nil
}

// UnsetEnvVars takes an array of variables and outputs "unset ..." on one line.
func (s *DefaultShell) UnsetEnvVars(vars []string) {
	if len(vars) > 0 {
		fmt.Printf("unset %s\n", strings.Join(vars, " "))
	}
}

// PrintAlias prints the aliases for the shell.
func (s *DefaultShell) PrintAlias(aliases map[string]string) error {
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
			// Check if the alias is already set before unaliasing
			if _, err := execCommandOutput("alias", k); err == nil {
				fmt.Printf("unalias %s\n", k)
			}
		} else {
			// Print alias command with the key and value
			fmt.Printf("alias %s=\"%s\"\n", k, aliases[k])
		}
	}
	return nil
}

// UnsetAlias unsets the provided aliases
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) > 0 {
		for _, alias := range aliases {
			// Check if the alias is already set before unaliasing
			if _, err := execCommandOutput("alias", alias); err == nil {
				fmt.Printf("unalias %s\n", alias)
			}
		}
	}
}
