package ssh

import "os"

// These are the functions that we will use to replace os.Stat and os.ReadFile
var stat = os.Stat

// readFile is a function that we will use to replace os.ReadFile
var readFile = os.ReadFile
