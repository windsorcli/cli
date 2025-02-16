package secrets

import (
	"os"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/goccy/go-yaml"
)

// stat is a shim for os.Stat to allow for easier testing and mocking.
var stat = os.Stat

// yamlUnmarshal is a shim for yaml.Unmarshal to allow for easier testing and mocking.
var yamlUnmarshal = yaml.Unmarshal

// decryptFileFunc is a shim for decrypt.File to allow for easier testing and mocking.
var decryptFileFunc = decrypt.File
