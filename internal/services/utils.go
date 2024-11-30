package services

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// Define a variable for os.Getwd() for easier testing
var getwd = os.Getwd

// Define a variable for filepath.Glob for easier testing
var glob = filepath.Glob

// Wrapper function for os.WriteFile
var writeFile = os.WriteFile

// Override variable for os.Stat
var stat = os.Stat

// Override variable for os.Mkdir
var mkdir = os.Mkdir

// Override variable for os.MkdirAll
var mkdirAll = os.MkdirAll

// Wrapper function for os.Rename
var rename = os.Rename

// Override variable for yaml.Marshal
var yamlMarshal = yaml.Marshal

// Override variable for yaml.Unmarshal
var yamlUnmarshal = yaml.Unmarshal

// Override variable for json.Unmarshal
var jsonUnmarshal = json.Unmarshal

// Mockable function for os.UserHomeDir
var userHomeDir = os.UserHomeDir

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}
