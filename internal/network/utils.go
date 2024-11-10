package network

import "os"

// stat is a wrapper around os.Stat
var stat = os.Stat

// writeFile is a wrapper around os.WriteFile
var writeFile = os.WriteFile
