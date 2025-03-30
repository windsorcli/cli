package config

import (
	"os"

	"os/exec"

	"github.com/goccy/go-yaml"
)

// osReadFile is a variable to allow mocking os.ReadFile in tests
var osReadFile = os.ReadFile

// osWriteFile is a variable to allow mocking os.WriteFile in tests
var osWriteFile = os.WriteFile

// osRemoveAll is a variable to allow mocking os.RemoveAll in tests
var osRemoveAll = os.RemoveAll

// osRemove is a variable to allow mocking os.Remove in tests
var osRemove = os.Remove

// execCommand is a variable to allow mocking exec.Command in tests
var execCommand = exec.Command

// cmdOutput is a variable to allow mocking exec.Command.Output in tests
var cmdOutput = func(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

// osUserHomeDir is a variable to allow mocking os.UserHomeDir in tests
var osUserHomeDir = os.UserHomeDir

// Override variable for yamlMarshal
var yamlMarshal = yaml.Marshal

// Override variable for yamlUnmarshal
var yamlUnmarshal = yaml.Unmarshal

// osStat is a variable to allow mocking os.Stat in tests
var osStat = os.Stat

// osMkdirAll is a variable to allow mocking os.MkdirAll in tests
var osMkdirAll = os.MkdirAll

// osGetppid is a variable to allow mocking os.Getppid in tests
var osGetppid = os.Getppid

// osGetpid is a variable to allow mocking os.Getpid in tests
var osGetpid = os.Getpid

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

func ptrInt(i int) *int {
	return &i
}
