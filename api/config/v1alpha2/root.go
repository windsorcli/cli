// The Root is the closed schema of the v1alpha2 root windsor.yaml.
// It provides the three keys of the root file as typed fields, so a strict decode rejects any other key.
// The root file holds project-wide settings and never per-context values.
// Path rules and the path resolver live outside this package.

package v1alpha2

// =============================================================================
// Constants
// =============================================================================

// Version is the version value of a v1alpha2 root file.
const Version = "v1alpha2"

// =============================================================================
// Types
// =============================================================================

// Root holds the keys of the v1alpha2 root file.
type Root struct {
	Version     string            `yaml:"version"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Paths       *Paths            `yaml:"paths,omitempty"`
}

// Paths holds the project directories. Base is relative to the project root. The other keys are relative to Base.
type Paths struct {
	Base      string `yaml:"base,omitempty"`
	Terraform string `yaml:"terraform,omitempty"`
	Kustomize string `yaml:"kustomize,omitempty"`
	Contexts  string `yaml:"contexts,omitempty"`
}
