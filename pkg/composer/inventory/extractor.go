// The Extractor is a built-in reader for an object whose operator picks images at runtime.
// It provides the objects that the operator would create from the object's spec.
// The Lister reads images from those objects as it reads them from a render.
// The FluxInstanceExtractor runs flux-operator build instance, which resolves versions as the operator does.

package inventory

import (
	"fmt"
	"path/filepath"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Types
// =============================================================================

// FluxInstanceExtractor derives the Flux controller objects of a FluxInstance.
type FluxInstanceExtractor struct {
	runtime *runtime.Runtime
	shims   *Shims
}

// =============================================================================
// Interfaces
// =============================================================================

// Extractor derives the objects that an operator creates from one kind of rendered object.
type Extractor interface {
	Matches(obj map[string]any) bool
	Extract(obj map[string]any) ([]map[string]any, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewFluxInstanceExtractor creates a FluxInstanceExtractor that runs the flux-operator CLI through rt.Shell.
func NewFluxInstanceExtractor(rt *runtime.Runtime, shims *Shims) *FluxInstanceExtractor {
	return &FluxInstanceExtractor{runtime: rt, shims: shims}
}

// =============================================================================
// Public Methods
// =============================================================================

// Matches reports whether obj is a FluxInstance of the fluxcd.controlplane.io API group.
func (e *FluxInstanceExtractor) Matches(obj map[string]any) bool {
	kind, _ := obj["kind"].(string)
	apiVersion, _ := obj["apiVersion"].(string)
	return kind == "FluxInstance" && strings.HasPrefix(apiVersion, "fluxcd.controlplane.io/")
}

// Extract writes obj to a temporary file, runs flux-operator build instance on it, and returns
// the objects it prints. The flux-operator CLI pulls the distribution artifact, so it needs
// network access. A missing flux-operator CLI returns an error with a remediation hint.
func (e *FluxInstanceExtractor) Extract(obj map[string]any) ([]map[string]any, error) {
	if _, err := e.shims.LookPath("flux-operator"); err != nil {
		return nil, werror.New("the flux-operator CLI is required to list the images of a FluxInstance",
			"Install the flux-operator CLI from https://fluxoperator.dev/docs/guides/install/.", nil)
	}

	data, err := sigsyaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal FluxInstance: %w", err)
	}
	tmpDir, err := e.shims.MkdirTemp("", "windsor-fluxinstance-")
	if err != nil {
		return nil, werror.Wrap(err, "failed to create a temporary directory")
	}
	defer e.shims.RemoveAll(tmpDir)
	file := filepath.Join(tmpDir, "flux-instance.yaml")
	if err := e.shims.WriteFile(file, data, 0600); err != nil {
		return nil, werror.Wrap(err, "failed to write FluxInstance")
	}

	out, err := e.runtime.Shell.ExecCaptureWithEnv("flux-operator", map[string]string{"NO_COLOR": "1"}, "build", "instance", "-f", file)
	if err != nil {
		return nil, werror.Wrap(err, "failed to build FluxInstance")
	}
	return decodeObjects(out)
}
