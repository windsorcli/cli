// The tool checks are the BaseToolsManager methods that verify one external tool each.
// It provides the docker, colima, 1Password, sops, and kubelogin checks, and the CPU check for Talos.
// Each check looks up the binary on PATH and compares its version with the minimum version.
// The version helpers parse and compare the dotted version strings that the tools print.

package tools

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/constants"
	"golang.org/x/sys/cpu"
)

// =============================================================================
// Constants
// =============================================================================

// checkCPUMicroarchitecture reports an error when the host CPU lacks the x86-64-v2 instruction
// baseline that Talos requires. Talos's own container image already refuses to start with "This
// program can only be run on AMD64 processors with v2 microarchitecture support," but that
// failure surfaces deep inside the docker driver's startup path; catching it here (via
// golang.org/x/sys/cpu — no shelling out) fails fast during `windsor check` instead. A no-op on
// non-amd64 hosts: Apple Silicon's Docker Desktop runs an ARM Linux VM, where this constraint
// doesn't apply. See cli#1268.
//
// Declared as a variable, not a func, so CheckRequirements' wiring to it — that it fires when
// docker.enabled/workstation.runtime needs Docker AND cluster.driver is "talos" — is verifiable
// in tests without depending on the test runner's actual CPU features; golang.org/x/sys/cpu.X86
// has no injection point of its own.
var checkCPUMicroarchitecture = func() error {
	if runtime.GOARCH != "amd64" {
		return nil
	}
	return checkX86V2Baseline(x86V2Features{
		HasCX16:   cpu.X86.HasCX16,
		HasPOPCNT: cpu.X86.HasPOPCNT,
		HasSSE3:   cpu.X86.HasSSE3,
		HasSSSE3:  cpu.X86.HasSSSE3,
		HasSSE41:  cpu.X86.HasSSE41,
		HasSSE42:  cpu.X86.HasSSE42,
	})
}

// =============================================================================
// Types
// =============================================================================

// x86V2Features is the subset of golang.org/x/sys/cpu.X86Info that defines the x86-64-v2
// microarchitecture baseline (CMPXCHG16B, POPCNT, SSE3, SSSE3, SSE4.1, SSE4.2), taken as a
// parameter so checkX86V2Baseline is testable without needing to run on hardware that actually
// lacks these features.
type x86V2Features struct {
	HasCX16, HasPOPCNT, HasSSE3, HasSSSE3, HasSSE41, HasSSE42 bool
}

// =============================================================================
// Private Methods
// =============================================================================

// checkDocker ensures Docker is available in the system's PATH. When not in Colima mode it also
// verifies the docker CLI version meets the minimum. Docker Compose is not required.
func (t *BaseToolsManager) checkDocker() error {
	if _, err := t.shims.LookPath("docker"); err != nil {
		return missingToolError("docker")
	}

	workstationRuntime := t.configHandler.GetString("workstation.runtime")
	isColimaMode := workstationRuntime == "colima"

	if !isColimaMode {
		output, err := t.shell.ExecSilentWithTimeout("docker", []string{"--version"}, 5*time.Second)
		if err != nil {
			return werror.Wrap(err, "docker --version failed")
		}
		dockerVersion := extractVersion(output)
		if dockerVersion != "" && compareVersion(dockerVersion, constants.MinimumVersionDocker) < 0 {
			return outdatedToolError("docker", dockerVersion)
		}
	}

	return nil
}

// checkColima ensures Colima and Limactl are available in the system's PATH using LookPath.
// It checks for 'colima' and 'limactl' in the system's PATH and verifies their versions.
// Returns nil if both are found and meet the minimum version requirements, else an error indicating either is not available or outdated.
func (t *BaseToolsManager) checkColima() error {
	if _, err := t.shims.LookPath("colima"); err != nil {
		return missingToolError("colima")
	}
	output, err := t.shell.ExecSilentWithTimeout("colima", []string{"version"}, 5*time.Second)
	if err != nil {
		return werror.Wrap(err, "colima version check failed")
	}
	colimaVersion := extractVersion(output)
	if colimaVersion == "" {
		return fmt.Errorf("failed to extract colima version")
	}
	if compareVersion(colimaVersion, constants.MinimumVersionColima) < 0 {
		return outdatedToolError("colima", colimaVersion)
	}

	if _, err := t.shims.LookPath("limactl"); err != nil {
		return missingToolError("limactl")
	}
	output, err = t.shell.ExecSilentWithTimeout("limactl", []string{"--version"}, 5*time.Second)
	if err != nil {
		return werror.Wrap(err, "limactl version check failed")
	}
	limactlVersion := extractVersion(output)
	if limactlVersion == "" {
		return fmt.Errorf("failed to extract limactl version")
	}
	minLima := constants.MinimumVersionLima
	if t.configHandler.GetString("platform") == "incus" {
		minLima = constants.MinimumVersionLimaIncus
	}
	if compareVersion(limactlVersion, minLima) < 0 {
		return outdatedToolError("limactl", limactlVersion)
	}

	return nil
}

// checkOnePassword ensures 1Password CLI is available in the system's PATH using LookPath.
// It checks for 'op' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkOnePassword() error {
	if _, err := t.shims.LookPath("op"); err != nil {
		return missingToolError("op")
	}

	out, err := t.shell.ExecSilentWithTimeout("op", []string{"--version"}, 5*time.Second)
	if err != nil {
		return werror.Wrap(err, "1Password CLI --version failed")
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract 1Password CLI version")
	}

	if compareVersion(version, constants.MinimumVersion1Password) < 0 {
		return outdatedToolError("op", version)
	}

	return nil
}

// checkSops ensures SOPS CLI is available in the system's PATH using LookPath.
// It checks for 'sops' in the system's PATH and verifies its version.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkSops() error {
	if _, err := t.shims.LookPath("sops"); err != nil {
		return missingToolError("sops")
	}

	out, err := t.shell.ExecSilentWithTimeout("sops", []string{"--version"}, 5*time.Second)
	if err != nil {
		return werror.Wrap(err, "sops --version failed")
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract SOPS CLI version")
	}

	if compareVersion(version, constants.MinimumVersionSOPS) < 0 {
		return outdatedToolError("sops", version)
	}

	return nil
}

// checkKubelogin ensures kubelogin is available in the system's PATH using LookPath.
// It checks for 'kubelogin' in the system's PATH, verifies its version, and validates
// required environment variables for SPN authentication if AZURE_CLIENT_SECRET is set.
// Returns nil if found and meets the minimum version requirement, else an error indicating it is not available or outdated.
func (t *BaseToolsManager) checkKubelogin() error {
	if _, err := t.shims.LookPath("kubelogin"); err != nil {
		return missingToolError("kubelogin")
	}

	out, err := t.shell.ExecSilentWithTimeout("kubelogin", []string{"--version"}, 5*time.Second)
	if err != nil {
		return werror.Wrap(err, "kubelogin --version failed")
	}

	version := extractVersion(out)
	if version == "" {
		return fmt.Errorf("failed to extract kubelogin version")
	}

	if compareVersion(version, constants.MinimumVersionKubelogin) < 0 {
		return outdatedToolError("kubelogin", version)
	}

	validationRules := []struct {
		triggerVar string
		authMethod string
	}{
		{"AZURE_FEDERATED_TOKEN_FILE", "Workload Identity"},
		{"AZURE_CLIENT_SECRET", "SPN"},
	}

	for _, rule := range validationRules {
		if os.Getenv(rule.triggerVar) != "" {
			azureClientID := os.Getenv("AZURE_CLIENT_ID")
			azureTenantID := os.Getenv("AZURE_TENANT_ID")

			if azureClientID == "" {
				return werror.New(fmt.Sprintf("%s is set but AZURE_CLIENT_ID is missing. %s authentication needs both.", rule.triggerVar, rule.authMethod),
					fmt.Sprintf("Set AZURE_CLIENT_ID to the client ID of the Azure identity, or unset %s.", rule.triggerVar), nil)
			}
			if azureTenantID == "" {
				return werror.New(fmt.Sprintf("%s is set but AZURE_TENANT_ID is missing. %s authentication needs both.", rule.triggerVar, rule.authMethod),
					fmt.Sprintf("Set AZURE_TENANT_ID to the tenant ID of the Azure identity, or unset %s.", rule.triggerVar), nil)
			}
		}
	}

	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// checkX86V2Baseline returns an error naming every x86-64-v2 feature f is missing, or nil if f
// satisfies the full baseline.
func checkX86V2Baseline(f x86V2Features) error {
	var missing []string
	if !f.HasCX16 {
		missing = append(missing, "CMPXCHG16B")
	}
	if !f.HasPOPCNT {
		missing = append(missing, "POPCNT")
	}
	if !f.HasSSE3 {
		missing = append(missing, "SSE3")
	}
	if !f.HasSSSE3 {
		missing = append(missing, "SSSE3")
	}
	if !f.HasSSE41 {
		missing = append(missing, "SSE4.1")
	}
	if !f.HasSSE42 {
		missing = append(missing, "SSE4.2")
	}
	if len(missing) > 0 {
		return werror.New(fmt.Sprintf("The CPU lacks x86-64-v2 microarchitecture support (missing: %s). Talos in Docker needs a v2-capable host CPU.", strings.Join(missing, ", ")),
			"Run the Talos cluster in Docker on a host with an x86-64-v2 CPU.", nil)
	}
	return nil
}

// compareVersion returns -1, 0, or 1 when version1 is lower than, equal to, or higher than version2.
// It compares the dotted main parts as numbers, then the count of parts, then the pre-release text.
// A version without a pre-release part is higher than the same version with one.
func compareVersion(version1, version2 string) int {
	splitVersion := func(version string) (main, preRelease string) {
		parts := strings.SplitN(version, "-", 2)
		main = parts[0]
		if len(parts) > 1 {
			preRelease = parts[1]
		}
		return
	}

	main1, pre1 := splitVersion(version1)
	main2, pre2 := splitVersion(version2)

	v1 := strings.Split(main1, ".")
	v2 := strings.Split(main2, ".")
	length := len(v1)
	length = max(length, len(v2))

	for i := range make([]int, length) {
		var comp1, comp2 int

		if i < len(v1) {
			comp1, _ = strconv.Atoi(v1[i])
		}
		if i < len(v2) {
			comp2, _ = strconv.Atoi(v2[i])
		}

		if comp1 < comp2 {
			return -1
		}
		if comp1 > comp2 {
			return 1
		}
	}

	if len(v1) < len(v2) {
		return -1
	}
	if len(v1) > len(v2) {
		return 1
	}

	if pre1 == "" && pre2 == "" {
		return 0
	}
	if pre1 == "" {
		return 1
	}
	if pre2 == "" {
		return -1
	}
	return strings.Compare(pre1, pre2)
}

// extractVersion uses a regex to extract the first version component from a string.
func extractVersion(output string) string {
	re := regexp.MustCompile(`\d+\.\d+\.\d+`)
	match := re.FindString(output)
	return match
}
