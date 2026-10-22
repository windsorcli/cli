// The GCP auth check is the BaseToolsManager check for the gcloud CLI and the GCP credentials.
// It provides the gcloud binary check, the credential check, and the login hint.
// It runs gcloud with the context CLOUDSDK_CONFIG when the env does not already point at it.
// With GOOGLE_APPLICATION_CREDENTIALS set, it does not need the gcloud CLI.

package tools

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Private Methods
// =============================================================================

// gcpEnabled reports whether the current context exercises GCP.
func (t *BaseToolsManager) gcpEnabled() bool {
	platform := t.configHandler.GetString("platform")
	if platform == "gcp" {
		return true
	}
	cfg := t.configHandler.GetHeader()
	return cfg != nil && cfg.GCP != nil
}

// checkGCPAuth verifies the gcloud CLI is present at the minimum version and that
// Application Default Credentials resolve. Probes `gcloud auth application-default
// print-access-token` — a live token mint, not a cached-account listing — so a revoked
// or expired credential is caught here rather than at terraform apply time. gcloud is
// required regardless of ambient credentials: GKE auth (via gke-gcloud-auth-plugin) and
// terraform teardown cleanup both shell out to it directly.
func (t *BaseToolsManager) checkGCPAuth() error {
	if err := t.checkGCPBinary(); err != nil {
		return err
	}
	env, err := t.gcpContextEnv()
	if err != nil {
		return werror.Wrap(err, "cannot resolve context-scoped GCP env for credential check")
	}
	if _, err := t.shell.ExecSilentWithEnvAndTimeout("gcloud", env, []string{"auth", "application-default", "print-access-token"}, 10*time.Second); err != nil {
		return t.gcpAuthError()
	}
	return nil
}

// gcpContextEnv returns env vars pointing the gcloud CLI at the context's config dir.
// In project mode CLOUDSDK_CONFIG points at the context's .gcp/gcloud/ so credentials stay
// scoped to the context; returns nil in global mode (defer to ~/.config/gcloud) or when
// ambient credentials are present (don't mask the native chain).
func (t *BaseToolsManager) gcpContextEnv() (map[string]string, error) {
	if t.hasAmbientGCPCredentials() {
		return nil, nil
	}
	if t.shell.IsGlobal() {
		return nil, nil
	}
	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil {
		return nil, err
	}
	gcloudConfigDir := filepath.Join(configRoot, ".gcp", "gcloud")
	return map[string]string{
		"CLOUDSDK_CONFIG": filepath.ToSlash(gcloudConfigDir),
	}, nil
}

// checkGCPBinary verifies the gcloud CLI is available in PATH and meets the minimum version.
func (t *BaseToolsManager) checkGCPBinary() error {
	if _, err := t.shims.LookPath("gcloud"); err != nil {
		return missingToolError("gcloud")
	}

	out, err := t.shell.ExecSilentWithTimeout("gcloud", []string{"--version"}, 10*time.Second)
	if err != nil {
		return werror.Wrap(err, "gcloud --version failed")
	}
	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract gcloud CLI version")
	}
	if compareVersion(version, constants.MinimumVersionGCP) < 0 {
		return outdatedToolError("gcloud", version)
	}

	return nil
}

// gcpAuthError returns the error for a GCP credential failure, with the gcloud login as the hint.
// In project mode, the command gets a CLOUDSDK_CONFIG prefix when the env does not already point
// at the context directory.
func (t *BaseToolsManager) gcpAuthError() error {
	if t.shell.IsGlobal() {
		return werror.New("The GCP application-default credentials have likely expired or are not set up.", "Run 'gcloud auth application-default login'.", nil)
	}
	configRoot, err := t.configHandler.GetConfigRoot()
	if err != nil || configRoot == "" {
		return werror.New("The GCP application-default credentials have likely expired or are not set up.", "Run 'gcloud auth application-default login'.", nil)
	}
	gcloudConfigDir := filepath.Join(configRoot, ".gcp", "gcloud")
	prefix := ""
	if !t.gcpEnvPointsAtContext(gcloudConfigDir) {
		prefix = fmt.Sprintf("CLOUDSDK_CONFIG=%q ", filepath.ToSlash(gcloudConfigDir))
	}
	return werror.New(fmt.Sprintf("The GCP application-default credentials for context %q have likely expired or are not set up.", t.configHandler.GetContext()),
		fmt.Sprintf("Run '%sgcloud auth application-default login'.", prefix), nil)
}

// hasAmbientGCPCredentials reports whether the parent env already carries GCP Application
// Default Credentials (a service account key or Workload Identity Federation config file).
// Workload Identity via the metadata server has no env signal and is intentionally not
// detected here — those runners fall through to the gcloud CLI check.
func (t *BaseToolsManager) hasAmbientGCPCredentials() bool {
	return t.shims.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != ""
}

// gcpEnvPointsAtContext reports whether CLOUDSDK_CONFIG already resolves to configDir.
func (t *BaseToolsManager) gcpEnvPointsAtContext(configDir string) bool {
	cd := t.shims.Getenv("CLOUDSDK_CONFIG")
	if cd == "" {
		return false
	}
	return filepath.ToSlash(cd) == filepath.ToSlash(configDir)
}
