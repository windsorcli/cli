package v1alpha2

import (
	"embed"
)

//go:embed providers/schema.yaml
//go:embed secrets/schema.yaml
//go:embed terraform/schema.yaml
//go:embed workstation/schema.yaml
var SchemasFS embed.FS

