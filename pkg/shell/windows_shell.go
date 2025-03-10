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
			if _, err := execCommand("Get-Alias", k).Output(); err == nil {
				fmt.Printf("Remove-Item Alias:%s\n", k)
			}
		} else {
			fmt.Printf("Set-Alias -Name %s -Value \"%s\"\n", k, aliases[k])
		}
	}
	return nil
}
