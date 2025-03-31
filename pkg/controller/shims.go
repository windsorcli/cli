package controller

import "os"

// osUserHomeDir retrieves the user's home directory
var osUserHomeDir = os.UserHomeDir

// osStat retrieves the file info for a given path
var osStat = os.Stat

// osSetenv sets an environment variable
var osSetenv = os.Setenv
