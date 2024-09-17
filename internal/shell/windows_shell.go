//go:build windows
// +build windows

package shell

import (
	"fmt"
	"sort"
)

type DefaultShell struct{}

func NewDefaultShell() *DefaultShell {
	return &DefaultShell{}
}

func (d *DefaultShell) DetermineShell() string {
	return "powershell"
}

func (d *DefaultShell) PrintEnvVars(envVars map[string]string) {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("$env:%s=\"%s\"\n", k, envVars[k])
	}
}
