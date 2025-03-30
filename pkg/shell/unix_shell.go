//go:build darwin || linux
// +build darwin linux

package shell

import (
	"bytes"
	"fmt"
	"sort"
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
			// Check if the alias is already set before unaliasing
			cmd := execCommand("alias", k)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			if err := cmdRun(cmd); err != nil {
				return fmt.Errorf("failed to check alias for %s: %w", k, err)
			}
			if buf.Len() > 0 {
				fmt.Printf("unalias %s\n", k)
			}
		} else {
			// Print alias command with the key and value
			fmt.Printf("alias %s=\"%s\"\n", k, aliases[k])
		}
	}
	return nil
}
