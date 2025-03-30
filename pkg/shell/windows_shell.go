//go:build windows
// +build windows

package shell

import (
	"fmt"
	"sort"
	"strings"
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
		} else {
			fmt.Printf("$env:%s='%s'\n", k, envVars[k])
		}
	}
	return nil
}

// PrintAlias sorts and prints shell aliases. Empty values trigger a removal command.
func (s *DefaultShell) PrintAlias(aliases map[string]string) error {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if aliases[k] == "" {
			if _, err := execCommandOutput("Get-Alias", k); err == nil {
				fmt.Printf("Remove-Item Alias:%s\n", k)
			}
		} else {
			fmt.Printf("Set-Alias -Name %s -Value \"%s\"\n", k, aliases[k])
		}
	}
	return nil
}

// UnsetEnvVars takes an array of variables and outputs "Remove-Item Env:..." for each.
func (s *DefaultShell) UnsetEnvVars(envVars []string) {
	if len(envVars) > 0 {
		fmt.Printf("Remove-Item Env:%s\n", strings.Join(envVars, " Env:"))
	}
}

// UnsetAlias removes each alias in the provided slice if it exists, using "Remove-Item Alias:<name>".
func (s *DefaultShell) UnsetAlias(aliases []string) {
	if len(aliases) > 0 {
		for _, alias := range aliases {
			if _, err := execCommandOutput("Get-Alias", alias); err == nil {
				fmt.Printf("Remove-Item Alias:%s\n", alias)
			}
		}
	}
}
