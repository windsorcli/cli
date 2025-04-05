//go:build windows
// +build windows

package shell

import (
	"fmt"
	"sort"
)

// PrintEnvVars sorts and prints environment variables. Empty values trigger a removal command.
func (s *DefaultShell) PrintEnvVars(envVars map[string]string) error {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if envVars[k] == "" {
			fmt.Printf("Remove-Item Env:%s\n", k)
			if err := osUnsetenv(k); err != nil {
				return fmt.Errorf("failed to unset environment variable %s: %w", k, err)
			}
		} else {
			fmt.Printf("$env:%s='%s'\n", k, envVars[k])
			if err := osSetenv(k, envVars[k]); err != nil {
				return fmt.Errorf("failed to set environment variable %s: %w", k, err)
			}
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
			// Print command to remove the alias if the value is an empty string
			fmt.Printf("Remove-Item Alias:%s\n", k)
		} else {
			// Print command to set the alias with the key and value
			fmt.Printf("Set-Alias -Name %s -Value \"%s\"\n", k, aliases[k])
		}
	}
	return nil
}

// UnsetEnv prints commands to unset the provided environment variables for Windows.
// It uses PowerShell's Remove-Item for each environment variable and also unsets them using osUnsetenv.
func (s *DefaultShell) UnsetEnv(vars []string) error {
	if len(vars) == 0 {
		return nil
	}

	// Sort variables for consistent output
	sortedVars := make([]string, len(vars))
	copy(sortedVars, vars)
	sort.Strings(sortedVars)

	// Print Remove-Item for each environment variable and unset it using osUnsetenv
	for _, v := range sortedVars {
		fmt.Printf("Remove-Item Env:%s -ErrorAction SilentlyContinue\n", v)
		if err := osUnsetenv(v); err != nil {
			return fmt.Errorf("failed to unset environment variable %s: %w", v, err)
		}
	}

	return nil
}

// UnsetAlias prints commands to unset the provided aliases for Windows.
// It uses PowerShell's Remove-Item with error suppression for each alias.
func (s *DefaultShell) UnsetAlias(aliases []string) error {
	if len(aliases) == 0 {
		return nil
	}

	// Sort aliases for consistent output
	sortedAliases := make([]string, len(aliases))
	copy(sortedAliases, aliases)
	sort.Strings(sortedAliases)

	// Print Remove-Item for each alias with error handling
	for _, alias := range sortedAliases {
		fmt.Printf("if (Test-Path Alias:%s) { Remove-Item Alias:%s }\n", alias, alias)
	}

	return nil
}
