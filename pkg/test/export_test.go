package test

import (
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Helpers
// =============================================================================

// ProjectRoot returns the project root of the runner.
func (r *TestRunner) ProjectRoot() string {
	return r.projectRoot
}

// BaseProjectRoot returns the project root that each test case runtime starts from.
func (r *TestRunner) BaseProjectRoot() string {
	return r.baseProjectRoot
}

// BaseShell returns the shell that each test case runtime starts from.
func (r *TestRunner) BaseShell() shell.Shell {
	return r.baseShell
}

// ArtifactBuilder returns the artifact builder of the runner.
func (r *TestRunner) ArtifactBuilder() artifact.Artifact {
	return r.artifactBuilder
}
