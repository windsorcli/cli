//go:build darwin || linux
// +build darwin linux

package shell

import (
	"fmt"
	"sort"
)

func (d *DefaultShell) PrintEnvVars(envVars map[string]string) {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("export %s=\"%s\"\n", k, envVars[k])
	}
}
