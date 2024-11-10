package env

import (
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// stat is a variable that holds the os.Stat function for mocking
var stat = os.Stat

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// Wrapper function for yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
	return &s
}

// boolPtr returns a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
}
