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
	return nil
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
			// Print unset command if the value is an empty string
			fmt.Printf("unalias %s\n", k)
		} else {
			// Print alias command with the key and value
			fmt.Printf("alias %s=\"%s\"\n", k, aliases[k])
		}
	}
	return nil
}

// UnsetEnv prints commands to unset the provided environment variables.
// It groups variables together in a single 'unset' command for efficiency.
func (s *DefaultShell) UnsetEnv(vars []string) error {
	if len(vars) == 0 {
		return nil
	}

	// Sort variables for consistent output
	sortedVars := make([]string, len(vars))
	copy(sortedVars, vars)
	sort.Strings(sortedVars)

	// Join all variables with spaces and print a single unset command
	fmt.Printf("unset %s\n", strings.Join(sortedVars, " "))

	return nil
}

// UnsetAlias prints commands to unset the provided aliases.
// It checks if each alias exists before unsetting it to avoid errors.
func (s *DefaultShell) UnsetAlias(aliases []string) error {
	if len(aliases) == 0 {
		return nil
	}

	// Sort aliases for consistent output
	sortedAliases := make([]string, len(aliases))
	copy(sortedAliases, aliases)
	sort.Strings(sortedAliases)

	// Print unalias command for each alias with a check if it exists
	for _, alias := range sortedAliases {
		fmt.Printf("alias %s >/dev/null 2>&1 && unalias %s\n", alias, alias)
	}

	return nil
}
