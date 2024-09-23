//go:build windows
// +build windows

package shell

import (
	"fmt"
	"sort"
)

// PrintEnvVars prints the provided environment variables in a sorted order.
// If the value of an environment variable is an empty string, it will print a command to remove the variable.
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
			// Print command to remove the environment variable if the value is an empty string
			fmt.Printf("Remove-Item Env:%s\n", k)
		} else {
			// Print command to set the environment variable with the key and value
			fmt.Printf("$env:%s=\"%s\"\n", k, envVars[k])
		}
	}
}
