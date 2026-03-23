// This file defines default configurations that supplement schema and facet defaults.
// Only values NOT already provided by schema.yaml defaults or facet config blocks belong here.

package config

import (
	"github.com/windsorcli/cli/api/v1alpha1"
)

// DefaultConfig is the base configuration for non-dev contexts (platform "none").
var DefaultConfig = v1alpha1.Context{
	Platform: ptrString("none"),
}

// DefaultConfig_Dev provides defaults for all dev contexts (docker-desktop, colima, incus).
var DefaultConfig_Dev = v1alpha1.Context{}
