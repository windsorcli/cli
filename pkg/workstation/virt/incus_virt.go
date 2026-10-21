// The IncusVirt runs the workstation containers and VMs on the Incus runtime inside Colima.
// It provides Up and Down, and checks and prepares the Colima VM for Incus.
// It adds the Incus remote and waits for each instance to become ready.

package virt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime"
)

// =============================================================================
// Constants
// =============================================================================

const (
	defaultPollInterval       = 200 * time.Millisecond
	defaultMaxWaitTimeout     = 5 * time.Second
	maxIterationsForLaunch    = 75
	maxIterationsForDevice    = 50
	maxIterationsForStop      = 75
	instanceExistsWaitTimeout = 15 * time.Second
	deleteInstanceWaitTimeout = 10 * time.Second
	sshCommandTimeout         = 3 * time.Second
	recentOperationThreshold  = 3 * time.Second
	IncusNetworkName          = "incusbr0"
)

// instanceBusyHint is the remediation for an Incus instance that stays busy after the wait time.
const instanceBusyHint = "Run incus operation list in the VM to see the busy operation. Wait for it to finish, then try again."

// =============================================================================
// Types
// =============================================================================

// IncusVirt implements both the ContainerRuntime and VirtualMachine interfaces for Incus.
// It embeds ColimaVirt to inherit Colima VM functionality. Instance creation is handled by Terraform in the stack.
type IncusVirt struct {
	*ColimaVirt
	pollInterval   time.Duration
	maxWaitTimeout time.Duration
}

// IncusInstanceConfig represents the complete configuration for creating an Incus instance.
// It contains all necessary parameters including instance name, type, image, network settings,
// device configurations, profiles, and resource limits required for instance creation.
type IncusInstanceConfig struct {
	Name      string
	Type      string
	Image     string
	Config    map[string]string
	Devices   map[string]map[string]string
	Profiles  []string
	IPv4      string
	Network   string
	Resources map[string]string
}

// IncusInstance represents an Incus instance parsed from JSON output.
// It contains the instance name, status information, type, and expanded device configurations
// as returned by the Incus CLI when listing instances.
type IncusInstance struct {
	Name            string                 `json:"name"`
	Status          string                 `json:"status"`
	StatusCode      int                    `json:"status_code"`
	Type            string                 `json:"type"`
	ExpandedDevices map[string]interface{} `json:"expanded_devices"`
}

// IncusOperation represents an Incus operation parsed from JSON output.
// It contains operation metadata including ID, status, timestamps, resources affected,
// and error information as returned by the Incus CLI when listing operations.
type IncusOperation struct {
	ID         string         `json:"id"`
	Class      string         `json:"class"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
	Status     string         `json:"status"`
	StatusCode int            `json:"status_code"`
	Resources  map[string]any `json:"resources"`
	Metadata   map[string]any `json:"metadata"`
	MayCancel  bool           `json:"may_cancel"`
	Err        string         `json:"err"`
}

// IncusRemote represents an Incus remote repository parsed from JSON output.
// It contains the remote name, URL, protocol type, and public access flag
// as returned by the Incus CLI when listing configured remotes.
type IncusRemote struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Protocol string `json:"protocol"`
	Public   bool   `json:"public"`
}

// =============================================================================
// Constructor
// =============================================================================

// NewIncusVirt creates a new IncusVirt with the provided runtime and embeds ColimaVirt for VM lifecycle.
func NewIncusVirt(rt *runtime.Runtime) *IncusVirt {
	return &IncusVirt{
		ColimaVirt:     NewColimaVirt(rt),
		pollInterval:   defaultPollInterval,
		maxWaitTimeout: defaultMaxWaitTimeout,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Up performs startup for the IncusVirt environment according to its current role.
// When invoked as a VirtualMachine, it starts the Colima VM by delegating to ColimaVirt.Up()
// if the VM is not already running. When invoked as a ContainerRuntime, and the VM is already
// running, it creates Incus instances for all configured services. This method determines the
// operation to perform by checking the VM's running status. It performs Incus-specific
// initialization when running as a container runtime, such as remote repository setup and
// instance creation. Returns an error if any step of initialization or startup fails.
func (v *IncusVirt) Up(ctx context.Context) error {
	info, err := v.getVMInfo()
	vmAlreadyRunning := err == nil && info.Address != ""

	if !vmAlreadyRunning {
		return v.ColimaVirt.Up(ctx)
	}

	return v.startIncusContainers()
}

// Down stops the Colima Incus daemon and runs the parent's Down() to clean up the VM.
func (v *IncusVirt) Down(ctx context.Context) error {
	if v.configHandler.GetString("platform") != "incus" {
		return v.ColimaVirt.Down(ctx)
	}
	return presenter.Track(ctx, "vm:incus", "Deleting Incus VM", func(ctx context.Context) error {
		contextName := v.configHandler.GetContext()
		profileName := fmt.Sprintf("windsor-%s", contextName)
		_, _ = v.shell.ExecSilent("colima", "daemon", "stop", profileName)
		if err := v.ColimaVirt.Down(ctx); err != nil {
			_ = v.cleanupVMForIncus()
			return err
		}
		return nil
	})
}

// =============================================================================
// Private Methods
// =============================================================================

// startIncusContainers creates Incus instances for all configured services.
// This method assumes the Colima VM is already running and has been validated.
// It ensures required Incus remotes are configured, validates the VM state, and creates
// instances for each service that has an Incus configuration. Network devices and
// non-network devices are configured as specified in each service's configuration.
func (v *IncusVirt) startIncusContainers() error {
	info, err := v.getVMInfo()
	if err == nil {
		var validateErr error
		info, validateErr = v.validateVMForIncus(info)
		if validateErr != nil {
			return validateErr
		}
	}

	remotes := []struct {
		name string
		url  string
	}{
		{"docker", "https://docker.io"},
		{"ghcr", "https://ghcr.io"},
	}

	for _, remote := range remotes {
		if err := v.ensureRemote(remote.name, remote.url); err != nil {
			return werror.Wrap(err, "failed to ensure %s remote", remote.name)
		}
	}

	return nil
}

// validateVMForIncus checks that the VM has an IP address when the platform is incus.
// For other platforms, it returns the VM info without a check.
// When the VM has no IP address, it returns an error with a remediation hint.
func (v *IncusVirt) validateVMForIncus(info VMInfo) (VMInfo, error) {
	if v.configHandler.GetString("platform") != "incus" {
		return info, nil
	}
	if info.Address == "" {
		contextName := v.configHandler.GetContext()
		profileName := fmt.Sprintf("windsor-%s", contextName)
		return VMInfo{}, werror.New(
			"VM is running but has no IP address, possibly because of a stale process or a network problem",
			fmt.Sprintf("Run windsor down --clean. If the problem continues, run colima daemon stop %s.", profileName),
			nil)
	}
	return info, nil
}

// cleanupVMForIncus performs Incus-specific cleanup on the VM before deletion.
// It only runs when workstation runtime is colima and platform is incus.
func (v *IncusVirt) cleanupVMForIncus() error {
	if v.configHandler.GetString("workstation.runtime") != "colima" || v.configHandler.GetString("platform") != "incus" {
		return nil
	}
	contextName := v.configHandler.GetContext()
	profileName := fmt.Sprintf("windsor-%s", contextName)
	limaInstanceName := fmt.Sprintf("colima-%s", profileName)
	v.killStuckProcessesForIncus(profileName, limaInstanceName)
	return nil
}

// killStuckProcessesForIncus cleans up stuck Colima processes for Incus runtime.
// It checks for PID files to determine if processes are stuck, then stops Colima daemons
// and Lima instances using force operations only if PID files indicate stuck processes.
// Removes PID files and socket files to prepare for clean restarts. All operations use
// timeouts to prevent hanging, and failures are silently ignored to allow cleanup to proceed.
func (v *IncusVirt) killStuckProcessesForIncus(profileName, limaInstanceName string) {
	homeDir, err := v.shims.UserHomeDir()
	if err != nil {
		return
	}
	daemonPidPath := filepath.Join(homeDir, ".colima", profileName, "daemon", "daemon.pid")
	_, err = v.shims.Stat(daemonPidPath)
	if !os.IsNotExist(err) {
		_, _ = v.shell.ExecSilentWithTimeout("colima", []string{"daemon", "stop", profileName}, 5*time.Second)
		_ = v.shims.Remove(daemonPidPath)
	}
	vmnetSocketPath := filepath.Join(homeDir, ".colima", profileName, "daemon", "vmnet.sock")
	_, err = v.shims.Stat(vmnetSocketPath)
	if !os.IsNotExist(err) {
		_, _ = v.shell.ExecSilentWithTimeout("limactl", []string{"stop", "--force", limaInstanceName}, 2*time.Second)
		_ = v.shims.Remove(vmnetSocketPath)
	}
}

// pollUntilNotBusy checks at each poll interval whether the instance is busy, and returns when it is not.
// A maxIterations value above zero sets a limit, and a deadline that is not zero sets a limit.
// When the instance is still busy at the limit, it returns an error with a remediation hint.
func (v *IncusVirt) pollUntilNotBusy(instanceName string, maxIterations int, deadline time.Time) error {
	pollInterval := v.pollInterval
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}

	useDeadline := !deadline.IsZero()
	useIterations := maxIterations > 0
	originalMaxIterations := maxIterations

	for {
		busy, err := v.instanceIsBusy(instanceName)
		if err != nil {
			return werror.Wrap(err, "failed to check if instance is busy")
		}
		if !busy {
			return nil
		}

		if useDeadline && time.Now().After(deadline) {
			return werror.New(fmt.Sprintf("instance %s is still busy after deadline", instanceName), instanceBusyHint, nil)
		}

		if useIterations {
			maxIterations--
			if maxIterations <= 0 {
				return werror.New(fmt.Sprintf("instance %s is still busy after %d iterations", instanceName, originalMaxIterations), instanceBusyHint, nil)
			}
		}

		time.Sleep(pollInterval)
	}
}

// ensureRemote ensures a remote is configured in Incus.
// It checks if the remote already exists, and if not, adds it with the specified URL and protocol.
// Uses idempotent behavior - if the remote already exists, the operation succeeds silently.
// Returns an error only if the remote doesn't exist and cannot be added.
func (v *IncusVirt) ensureRemote(name, url string) error {
	exists, err := v.remoteExists(name)
	if err != nil {
		return werror.Wrap(err, "failed to check %s remote", name)
	}
	if exists {
		return nil
	}

	message := fmt.Sprintf("Configuring %s remote", name)
	remoteArgs := []string{"remote", "add", name, url, "--protocol", "oci", "--public"}
	remoteArgs = v.addQuietFlag(remoteArgs)
	_, err = v.execInVMProgress(message, "incus", remoteArgs...)
	if err != nil {
		exists, checkErr := v.remoteExists(name)
		if checkErr == nil && exists {
			return nil
		}
		return werror.Wrap(err, "failed to add %s remote", name)
	}

	return nil
}

// remoteExists checks if an Incus remote exists by querying the list of configured remotes.
// It searches through all remotes to find one matching the specified name.
// Returns true if the remote exists, false if it does not, or an error if the query fails.
func (v *IncusVirt) remoteExists(remoteName string) (bool, error) {
	remotes, err := v.getRemotes()
	if err != nil {
		return false, err
	}
	for _, remote := range remotes {
		if remote.Name == remoteName {
			return true, nil
		}
	}
	return false, nil
}

// getRemotes retrieves all Incus remotes as JSON from the Incus CLI.
// Uses timeout to prevent hanging if SSH connection is broken. Parses the JSON output
// which is a map of remote names to remote configurations, and converts it to a slice
// of IncusRemote structs. Returns an error if the command fails or the JSON cannot be parsed.
func (v *IncusVirt) getRemotes() ([]IncusRemote, error) {
	output, err := v.execInVMQuiet("incus", []string{"remote", "list", "--format", "json"}, sshCommandTimeout)
	if err != nil {
		return nil, err
	}
	var remotes map[string]IncusRemote
	if err := json.Unmarshal([]byte(output), &remotes); err != nil {
		return nil, werror.Wrap(err, "failed to parse remotes JSON")
	}
	result := make([]IncusRemote, 0, len(remotes))
	for name, remote := range remotes {
		remote.Name = name
		result = append(result, remote)
	}
	return result, nil
}

// ensureFileExists ensures a file or directory exists in the VM by checking its existence
// and creating parent directories if necessary. It first checks if the path exists, and if not,
// creates the parent directory structure. Then attempts to create the path itself, handling
// both file and directory creation. Returns an error if the path cannot be created or verified.
func (v *IncusVirt) ensureFileExists(filePath string) error {
	_, err := v.execInVM("test", "-e", filePath)
	if err == nil {
		return nil
	}
	parentDir := filepath.Dir(filePath)
	_, err = v.execInVM("mkdir", "-p", parentDir)
	if err != nil {
		return fmt.Errorf("failed to create parent directory in VM: %s", parentDir)
	}
	_, err = v.execInVM("mkdir", "-p", filePath)
	if err == nil {
		return nil
	}
	_, err = v.execInVM("test", "-e", filePath)
	if err != nil {
		return fmt.Errorf("file or directory does not exist or is not accessible in VM: %s", filePath)
	}
	return nil
}

// waitForInstanceReady waits for an Incus instance to finish any in-progress operations.
// It polls the instance until it is no longer busy, using the specified maximum wait duration.
// If no duration is specified, it uses the configured maxWaitTimeout or the default timeout.
// Returns an error if the instance does not become ready within the timeout period.
func (v *IncusVirt) waitForInstanceReady(name string, maxWait time.Duration) error {
	maxWaitTimeout := v.getMaxWaitTimeout(maxWait)
	deadline := time.Now().Add(maxWaitTimeout)
	if err := v.pollUntilNotBusy(name, 0, deadline); err != nil {
		return werror.Wrap(err, "timeout waiting for instance %s to be ready", name)
	}
	return nil
}

// getMaxWaitTimeout returns the maximum wait timeout, using the provided value if non-zero,
// otherwise falling back to the configured maxWaitTimeout or the default timeout.
func (v *IncusVirt) getMaxWaitTimeout(maxWait time.Duration) time.Duration {
	if maxWait != 0 {
		return maxWait
	}
	if v.maxWaitTimeout != 0 {
		return v.maxWaitTimeout
	}
	return defaultMaxWaitTimeout
}

// addQuietFlag adds --quiet to incus command args when not verbose.
func (v *IncusVirt) addQuietFlag(args []string) []string {
	if !v.shell.IsVerbose() {
		return append(args, "--quiet")
	}
	return args
}

// =============================================================================
// Helpers
// =============================================================================

// isBusyError checks if an error indicates an Incus instance is busy.
// It checks for common busy error patterns including "busy" and "stop operation".
// Returns true if any of these patterns are found in the error message.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "busy") || strings.Contains(errStr, "stop operation")
}

// isSSHError checks if an error is related to SSH connection failure.
// Returns true if the error indicates the VM is unreachable, which is acceptable during cleanup.
// This includes connection timeouts, handshake failures, connection resets, and missing
// client configuration. These errors are treated as non-fatal during teardown operations.
func isSSHError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "failed to connect to SSH") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "handshake failed") ||
		strings.Contains(errStr, "timed out") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "session creation timeout") ||
		strings.Contains(errStr, "client configuration is not set")
}

// sanitizeInstanceName converts a service name to a valid Incus instance name.
// It replaces any characters that are not alphanumeric or hyphens with hyphens to ensure
// the resulting name is valid for Incus instance naming conventions. This ensures
// service names with special characters can be used as instance identifiers.
func sanitizeInstanceName(name string) string {
	result := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return result.String()
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure IncusVirt implements ContainerRuntime
var _ ContainerRuntime = (*IncusVirt)(nil)
