//go:build darwin || linux
// +build darwin linux

package shell

import (
	"fmt"
	"sort"
)

// PrintEnvVars prints the provided environment variables in a sorted order.
// If the value of an environment variable is an empty string, it will print an unset command.
func (d *DefaultShell) PrintEnvVars(envVars map[string]string) {
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