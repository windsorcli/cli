package config

import (
	"os"

	"github.com/goccy/go-yaml"
)

// osReadFile is a variable to allow mocking os.ReadFile in tests
var osReadFile = os.ReadFile

// osWriteFile is a variable to allow mocking os.WriteFile in tests
var osWriteFile = os.WriteFile

// osRemoveAll is a variable to allow mocking os.RemoveAll in tests
var osRemoveAll = os.RemoveAll

// osGetenv is a variable to allow mocking os.Getenv in tests
var osGetenv = os.Getenv

// Override variable for yamlMarshal
var yamlMarshal = yaml.Marshal

// Override variable for yamlUnmarshal
var yamlUnmarshal = yaml.Unmarshal

// osStat is a variable to allow mocking os.Stat in tests
var osStat = os.Stat

// osMkdirAll is a variable to allow mocking os.MkdirAll in tests
var osMkdirAll = os.MkdirAll

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
