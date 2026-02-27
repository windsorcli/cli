// The IncusVirt is a container runtime implementation
// It provides Incus container management capabilities through the Incus API/CLI
// It serves as the primary container orchestration layer for Incus-based services
// It handles container lifecycle, configuration, and networking for Incus containers and VMs

package virt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
func (v *IncusVirt) Up() error {
	info, err := v.getVMInfo()
	vmAlreadyRunning := err == nil && info.Address != ""

	if !vmAlreadyRunning {
		return v.ColimaVirt.Up()
	}

	return v.startIncusContainers()
}

// Down stops the Colima Incus daemon and runs the parent's Down() to clean up the VM.
func (v *IncusVirt) Down() error {
	if v.configHandler.GetString("platform") != "incus" {
		return v.ColimaVirt.Down()
	}
	contextName := v.configHandler.GetContext()
	profileName := fmt.Sprintf("windsor-%s", contextName)
	_, _ = v.shell.ExecProgress("🦙 Stopping Colima daemon", "colima", "daemon", "stop", profileName)

	err := v.ColimaVirt.Down()
	if err != nil {
		_ = v.cleanupVMForIncus()
	}
	return err
}

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
			return fmt.Errorf("failed to ensure %s remote: %w", remote.name, err)
		}
	}

	return nil
}

// validateVMForIncus validates that the VM is ready for Incus operations.
// It checks that the VM has a valid IP address when running in incus mode.
// Returns an error with helpful troubleshooting information if the VM is in an invalid state.
func (v *IncusVirt) validateVMForIncus(info VMInfo) (VMInfo, error) {
	if v.configHandler.GetString("platform") != "incus" {
		return info, nil
	}
	if info.Address == "" {
		contextName := v.configHandler.GetContext()
		profileName := fmt.Sprintf("windsor-%s", contextName)
		return VMInfo{}, fmt.Errorf(
			"VM is running but has no IP address. This may indicate a stale process or network issue.\n"+
				"To fix this, run: windsor down --clean\n"+
				"Or manually: colima daemon stop %s",
			profileName)
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	daemonPidPath := filepath.Join(homeDir, ".colima", profileName, "daemon", "daemon.pid")
	_, err = os.Stat(daemonPidPath)
	if !os.IsNotExist(err) {
		_, _ = v.shell.ExecSilentWithTimeout("colima", []string{"daemon", "stop", profileName}, 5*time.Second)
		_ = os.Remove(daemonPidPath)
	}
	vmnetSocketPath := filepath.Join(homeDir, ".colima", profileName, "daemon", "vmnet.sock")
	_, err = os.Stat(vmnetSocketPath)
	if !os.IsNotExist(err) {
		_, _ = v.shell.ExecSilentWithTimeout("limactl", []string{"stop", "--force", limaInstanceName}, 2*time.Second)
		_ = os.Remove(vmnetSocketPath)
	}
}

// pollUntilNotBusy polls until an instance is not busy, using either iteration count or deadline.
// It checks the instance busy status at regular intervals and returns when the instance is
// no longer busy or when the maximum iterations or deadline is reached. Supports both
// iteration-based and time-based polling strategies for flexible timeout handling.
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
			return fmt.Errorf("failed to check if instance is busy: %w", err)
		}
		if !busy {
			return nil
		}

		if useDeadline && time.Now().After(deadline) {
			return fmt.Errorf("instance %s is still busy after deadline", instanceName)
		}

		if useIterations {
			maxIterations--
			if maxIterations <= 0 {
				return fmt.Errorf("instance %s is still busy after %d iterations", instanceName, originalMaxIterations)
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
		return fmt.Errorf("failed to check %s remote: %w", name, err)
	}
	if exists {
		return nil
	}

	message := fmt.Sprintf("🔧 Configuring %s remote", name)
	remoteArgs := []string{"remote", "add", name, url, "--protocol", "oci", "--public"}
	remoteArgs = v.addQuietFlag(remoteArgs)
	_, err = v.execInVMProgress(message, "incus", remoteArgs...)
	if err != nil {
		exists, checkErr := v.remoteExists(name)
		if checkErr == nil && exists {
			return nil
		}
		return fmt.Errorf("failed to add %s remote: %w", name, err)
	}

	return nil
}

// instanceExists checks if an Incus instance exists by querying the Incus instance list.
// It retrieves all instances and searches for one matching the provided name.
// Returns true if the instance exists, false if it does not, or an error if the query fails.
func (v *IncusVirt) instanceExists(name string) (bool, error) {
	instances, err := v.getInstances()
	if err != nil {
		return false, err
	}
	for _, instance := range instances {
		if instance.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// instanceIsRunning checks if an Incus instance is currently running.
// It queries the instance status and returns true if the status is "Running" or status code is 103.
// Returns false if the instance is not running or does not exist, or an error if the query fails.
func (v *IncusVirt) instanceIsRunning(name string) (bool, error) {
	instances, err := v.getInstances()
	if err != nil {
		return false, err
	}
	for _, instance := range instances {
		if instance.Name == name {
			return instance.Status == "Running" || instance.StatusCode == 103, nil
		}
	}
	return false, nil
}

// instanceIsBusy checks if an Incus instance has an operation in progress.
// It queries all Incus operations and checks if any are currently running or were recently
// completed (within 3 seconds) for the specified instance. This helps detect transient busy
// states that may not be immediately visible in the instance status.
func (v *IncusVirt) instanceIsBusy(name string) (bool, error) {
	operations, err := v.getOperations()
	if err != nil {
		return false, err
	}
	now := time.Now()
	for _, op := range operations {
		if instances, ok := op.Resources["instances"].([]any); ok {
			for _, inst := range instances {
				if instStr, ok := inst.(string); ok {
					if instStr == name || strings.HasSuffix(instStr, "/"+name) {
						if op.Status == "running" {
							return true, nil
						}
						if op.Status == "Success" || op.Status == "Failure" {
							if op.UpdatedAt != "" {
								updatedAt, err := time.Parse(time.RFC3339, op.UpdatedAt)
								if err == nil {
									timeDiff := now.Sub(updatedAt)
									if timeDiff < recentOperationThreshold {
										return true, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return false, nil
}

// getInstances retrieves all Incus instances as JSON from the Incus CLI.
// Uses timeout to prevent hanging if SSH connection is broken. Parses the JSON output
// into a slice of IncusInstance structs. Returns an error if the command fails or
// the JSON cannot be parsed.
func (v *IncusVirt) getInstances() ([]IncusInstance, error) {
	output, err := v.execInVMQuiet("incus", []string{"list", "--format", "json"}, sshCommandTimeout)
	if err != nil {
		return nil, err
	}
	var instances []IncusInstance
	if err := json.Unmarshal([]byte(output), &instances); err != nil {
		return nil, fmt.Errorf("failed to parse instances JSON: %w", err)
	}
	return instances, nil
}

// getOperations retrieves all Incus operations as JSON from the Incus CLI.
// Uses timeout to prevent hanging if SSH connection is broken. Parses the JSON output
// into a slice of IncusOperation structs. Returns an error if the command fails or
// the JSON cannot be parsed.
func (v *IncusVirt) getOperations() ([]IncusOperation, error) {
	output, err := v.execInVMQuiet("incus", []string{"operation", "list", "--format", "json"}, sshCommandTimeout)
	if err != nil {
		return nil, err
	}
	var operations []IncusOperation
	if err := json.Unmarshal([]byte(output), &operations); err != nil {
		return nil, fmt.Errorf("failed to parse operations JSON: %w", err)
	}
	return operations, nil
}

// waitForInstanceReady waits for an Incus instance to finish any in-progress operations.
// It polls the instance until it is no longer busy, using the specified maximum wait duration.
// If no duration is specified, it uses the configured maxWaitTimeout or the default timeout.
// Returns an error if the instance does not become ready within the timeout period.
func (v *IncusVirt) waitForInstanceReady(name string, maxWait time.Duration) error {
	maxWaitTimeout := v.getMaxWaitTimeout(maxWait)
	deadline := time.Now().Add(maxWaitTimeout)
	if err := v.pollUntilNotBusy(name, 0, deadline); err != nil {
		return fmt.Errorf("timeout waiting for instance %s to be ready: %w", name, err)
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

// deviceExists checks if a device exists on an Incus instance by querying the instance's
// expanded devices configuration. It searches through the instance's device list to find
// a device matching the specified name. Returns true if found, false if not, or an error
// if the instance cannot be queried.
func (v *IncusVirt) deviceExists(instanceName, deviceName string) (bool, error) {
	instances, err := v.getInstances()
	if err != nil {
		return false, err
	}
	for _, instance := range instances {
		if instance.Name == instanceName {
			if instance.ExpandedDevices != nil {
				_, exists := instance.ExpandedDevices[deviceName]
				return exists, nil
			}
			return false, nil
		}
	}
	return false, nil
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
		return nil, fmt.Errorf("failed to parse remotes JSON: %w", err)
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

// launchInstance launches a new Incus instance or ensures an existing one is ready.
// If the instance already exists, it waits for any in-progress operations to complete.
// If the instance does not exist, it launches a new instance with the provided configuration
// and waits for the launch operation to complete. Handles launch errors gracefully with retry logic.
func (v *IncusVirt) launchInstance(config *IncusInstanceConfig) error {
	exists, err := v.instanceExists(config.Name)
	if err != nil {
		return fmt.Errorf("failed to check if instance exists: %w", err)
	}

	if exists {
		deadline := time.Now().Add(instanceExistsWaitTimeout)
		if err := v.pollUntilNotBusy(config.Name, 0, deadline); err != nil {
			return fmt.Errorf("instance exists but is still busy after waiting: %w", err)
		}
		return nil
	}

	args := v.buildLaunchArgs(config)
	message := fmt.Sprintf("📦 Creating Incus instance %s", config.Name)
	launchArgs := v.addQuietFlag(args)
	_, err = v.execInVMProgress(message, "incus", launchArgs...)
	if err != nil {
		return v.handleInstanceLaunchError(config.Name, message, args, err)
	}

	if err := v.pollUntilNotBusy(config.Name, maxIterationsForLaunch, time.Time{}); err != nil {
		return fmt.Errorf("instance is still busy after launch, cannot proceed: %w", err)
	}

	return nil
}

// buildLaunchArgs builds the command-line arguments for launching an Incus instance.
// It constructs the incus launch command with all necessary flags including instance type,
// network configuration, custom config values, resource limits, and profiles. Handles
// proper quoting of config values that contain spaces. Adds --quiet flag when not verbose.
func (v *IncusVirt) buildLaunchArgs(config *IncusInstanceConfig) []string {
	args := []string{"launch", config.Image, config.Name}
	if !v.shell.IsVerbose() {
		args = append(args, "--quiet")
	}

	if config.Type == "vm" {
		args = append(args, "--vm")
	}

	if config.Network != "" && config.IPv4 == "" {
		args = append(args, "--network", config.Network)
	}

	for key, value := range config.Config {
		configValue := fmt.Sprintf("%s=%s", key, value)
		if strings.Contains(value, " ") {
			configValue = fmt.Sprintf("%s=\"%s\"", key, value)
		}
		args = append(args, "--config", configValue)
	}

	if len(config.Resources) > 0 {
		for key, value := range config.Resources {
			args = append(args, "--config", fmt.Sprintf("%s=%s", key, value))
		}
	}

	if len(config.Profiles) > 0 {
		for _, profile := range config.Profiles {
			args = append(args, "--profile", profile)
		}
	}

	return args
}

// handleInstanceLaunchError handles errors that occur during instance launch.
// It checks if the instance already exists or is busy, and retries the launch operation
// after waiting if the instance becomes available. Provides detailed error messages
// based on the specific failure scenario encountered.
func (v *IncusVirt) handleInstanceLaunchError(instanceName, message string, args []string, launchErr error) error {
	exists, checkErr := v.instanceExists(instanceName)
	if checkErr == nil && exists {
		return v.ensureInstanceNotBusy(instanceName, "instance exists but is busy")
	}

	busy, checkErr := v.instanceIsBusy(instanceName)
	if checkErr == nil && busy {
		if err := v.waitForInstanceReady(instanceName, 0); err != nil {
			return fmt.Errorf("instance is busy: %w", err)
		}
		updateArgs := v.addQuietFlag(args)
		_, err := v.execInVMProgress(message, "incus", updateArgs...)
		if err != nil {
			exists, checkErr := v.instanceExists(instanceName)
			if checkErr == nil && exists {
				return v.ensureInstanceNotBusy(instanceName, "instance exists but is busy")
			}
			return fmt.Errorf("failed to launch Incus instance after wait: %w", err)
		}
		return nil
	}

	return fmt.Errorf("failed to launch Incus instance: %w", launchErr)
}

// ensureInstanceNotBusy ensures an instance is not busy, waiting if necessary.
// It polls the instance until it is no longer busy, using the configured maximum wait timeout.
// Returns an error with the provided error prefix if the instance does not become available
// within the timeout period.
func (v *IncusVirt) ensureInstanceNotBusy(instanceName, errorPrefix string) error {
	maxWait := v.getMaxWaitTimeout(0)
	deadline := time.Now().Add(maxWait)
	if err := v.pollUntilNotBusy(instanceName, 0, deadline); err != nil {
		return fmt.Errorf("%s: %w", errorPrefix, err)
	}
	return nil
}

// configureNetworkDevice configures the network device for an instance.
// It checks if the network device exists, and if not, adds it. If it already exists,
// updates the device configuration. This method handles both initial device creation
// and device modification scenarios.
func (v *IncusVirt) configureNetworkDevice(config *IncusInstanceConfig) error {
	exists, err := v.deviceExists(config.Name, "eth0")
	if err != nil {
		return fmt.Errorf("failed to check if network device exists: %w", err)
	}

	if !exists {
		return v.addNetworkDevice(config)
	}

	return v.updateNetworkDevice(config)
}

// addNetworkDevice adds a network device to an instance.
// It ensures the instance is not busy, stops it if running, adds the network device with
// the specified network and IP address, and restarts the instance if it was previously running.
// Handles busy state errors with retry logic and proper error reporting.
func (v *IncusVirt) addNetworkDevice(config *IncusInstanceConfig) error {
	if err := v.pollUntilNotBusy(config.Name, maxIterationsForDevice, time.Time{}); err != nil {
		return fmt.Errorf("instance is busy, cannot add network device: %w", err)
	}

	wasRunning, err := v.stopInstanceIfRunning(config.Name, "failed to stop instance to add network device")
	if err != nil {
		return err
	}

	deviceArgs := []string{"config", "device", "add", config.Name, "eth0", "nic", fmt.Sprintf("network=%s", config.Network), fmt.Sprintf("ipv4.address=%s", config.IPv4)}
	deviceArgs = v.addQuietFlag(deviceArgs)
	_, err = v.execInVM("incus", deviceArgs...)
	if err != nil {
		originalErr := err

		if isBusyError(err) {
			if err := v.pollUntilNotBusy(config.Name, maxIterationsForLaunch, time.Time{}); err != nil {
				return fmt.Errorf("instance is still busy after waiting, cannot add device: %w", originalErr)
			}

			_, retryErr := v.execInVM("incus", v.addQuietFlag(deviceArgs)...)
			if retryErr != nil {
				exists, checkErr := v.deviceExists(config.Name, "eth0")
				if checkErr != nil {
					return fmt.Errorf("failed to check if device exists after retry: %w", checkErr)
				}
				if !exists {
					return fmt.Errorf("failed to add network device after retry: %w", retryErr)
				}
			}
		} else {
			exists, checkErr := v.deviceExists(config.Name, "eth0")
			if checkErr != nil {
				return fmt.Errorf("failed to check if device exists: %w", checkErr)
			}
			if !exists {
				return fmt.Errorf("failed to add network device: %w", originalErr)
			}
		}
	}

	if wasRunning {
		return v.startInstance(config.Name, "failed to start instance after adding network device")
	}

	return nil
}

// updateNetworkDevice updates the network device IP address for an instance.
// It checks the current IP address and skips update if it already matches. Otherwise,
// it ensures the instance is not busy, stops it if running, updates the device IP address,
// and restarts the instance if it was previously running. Uses device override as a fallback
// if device set fails, with retry logic for busy state errors.
func (v *IncusVirt) updateNetworkDevice(config *IncusInstanceConfig) error {
	getArgs := []string{"config", "device", "get", config.Name, "eth0", "ipv4.address"}
	getArgs = v.addQuietFlag(getArgs)
	currentIP, err := v.execInVMQuiet("incus", getArgs, sshCommandTimeout)
	if err == nil && strings.TrimSpace(currentIP) == config.IPv4 {
		return nil
	}

	if err := v.pollUntilNotBusy(config.Name, maxIterationsForDevice, time.Time{}); err != nil {
		return fmt.Errorf("instance is busy, cannot update network device: %w", err)
	}

	wasRunning, err := v.stopInstanceIfRunning(config.Name, "failed to stop instance to configure network")
	if err != nil {
		return err
	}

	setArgs := []string{"config", "device", "set", config.Name, "eth0", fmt.Sprintf("ipv4.address=%s", config.IPv4)}
	setArgs = v.addQuietFlag(setArgs)
	_, err = v.execInVM("incus", setArgs...)
	if err != nil {
		originalErr := err

		if isBusyError(err) {
			if err := v.pollUntilNotBusy(config.Name, maxIterationsForLaunch, time.Time{}); err != nil {
				return fmt.Errorf("instance is still busy after waiting, cannot set network device: %w", originalErr)
			}

			_, retryErr := v.execInVM("incus", v.addQuietFlag(setArgs)...)
			if retryErr != nil {
				err = retryErr
			} else {
				err = nil
			}
		}

		if err != nil {
			if err := v.pollUntilNotBusy(config.Name, maxIterationsForLaunch, time.Time{}); err != nil {
				return fmt.Errorf("instance is still busy, cannot override network device: %w", err)
			}
			overrideArgs := []string{"config", "device", "override", config.Name, "eth0", fmt.Sprintf("ipv4.address=%s", config.IPv4)}
			overrideArgs = v.addQuietFlag(overrideArgs)
			_, err = v.execInVM("incus", overrideArgs...)
			if err != nil {
				if err := v.pollUntilNotBusy(config.Name, maxIterationsForLaunch, time.Time{}); err != nil {
					return fmt.Errorf("instance is still busy, cannot retry override: %w", err)
				}
				_, retryErr := v.execInVM("incus", v.addQuietFlag(overrideArgs)...)
				if retryErr != nil {
					return fmt.Errorf("failed to override network device with static IP: %w", retryErr)
				}
			}
		}
	}

	if wasRunning {
		return v.startInstance(config.Name, "failed to start instance after configuring network")
	}

	return nil
}

// stopInstanceIfRunning stops an instance if it is running and waits for it to be ready.
// It checks if the instance is running, and if so, stops it and waits for the stop operation
// to complete. Verifies that the instance is actually stopped after the operation.
// Returns true if the instance was stopped, false if it was not running, or an error
// if the stop operation fails or the instance remains running.
func (v *IncusVirt) stopInstanceIfRunning(instanceName, errorPrefix string) (bool, error) {
	instances, err := v.getInstances()
	if err != nil {
		return false, fmt.Errorf("failed to check if instance is running: %w", err)
	}

	running := v.isRunning(instances, instanceName)
	if !running {
		return false, nil
	}

	_, err = v.execInVM("incus", v.addQuietFlag([]string{"stop", instanceName})...)
	if err != nil {
		instances, checkErr := v.getInstances()
		if checkErr != nil {
			return false, fmt.Errorf("failed to verify instance state after stop attempt: %w", checkErr)
		}
		if v.isRunning(instances, instanceName) {
			return false, fmt.Errorf("%s: %w", errorPrefix, err)
		}
	}

	if err := v.pollUntilNotBusy(instanceName, maxIterationsForStop, time.Time{}); err != nil {
		return false, fmt.Errorf("instance is still busy after stop: %w", err)
	}

	instances, err = v.getInstances()
	if err != nil {
		return false, fmt.Errorf("failed to verify instance stopped: %w", err)
	}
	if v.isRunning(instances, instanceName) {
		return false, fmt.Errorf("instance is still running after stop")
	}

	return true, nil
}

// isRunning checks if an instance is running by looking it up in a provided instances list.
// It searches the list for the instance and returns true if the status is "Running" or status code is 103.
func (v *IncusVirt) isRunning(instances []IncusInstance, name string) bool {
	for _, instance := range instances {
		if instance.Name == name {
			return instance.Status == "Running" || instance.StatusCode == 103
		}
	}
	return false
}

// addNonNetworkDevices adds all non-network devices to an instance.
// It iterates through all devices in the configuration, skipping the eth0 network device,
// and adds each device that has a valid type specified. This method handles device
// creation for storage, disk, and other non-network device types.
func (v *IncusVirt) addNonNetworkDevices(config *IncusInstanceConfig) error {
	for deviceName, deviceConfig := range config.Devices {
		if deviceName == "eth0" {
			continue
		}
		deviceType := deviceConfig["type"]
		if deviceType == "" {
			continue
		}
		if err := v.addDevice(config.Name, deviceName, deviceConfig); err != nil {
			return err
		}
	}
	return nil
}

// addDevice adds a single device to an instance.
// It checks if the device already exists, and if not, ensures any required source files
// exist for disk devices, then adds the device with the specified configuration.
// Uses idempotent behavior - if the device already exists, the operation succeeds silently.
func (v *IncusVirt) addDevice(instanceName, deviceName string, deviceConfig map[string]string) error {
	exists, err := v.deviceExists(instanceName, deviceName)
	if err != nil {
		return fmt.Errorf("failed to check if device %s exists: %w", deviceName, err)
	}
	if exists {
		return nil
	}

	deviceType := deviceConfig["type"]
	if deviceType == "disk" {
		if source, ok := deviceConfig["source"]; ok {
			if err := v.ensureFileExists(source); err != nil {
				return fmt.Errorf("failed to ensure source file exists for device %s: %w", deviceName, err)
			}
		}
	}

	deviceArgs := []string{"config", "device", "add", instanceName, deviceName, deviceType}
	for k, v := range deviceConfig {
		if k != "type" {
			deviceArgs = append(deviceArgs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	deviceArgs = v.addQuietFlag(deviceArgs)
	_, err = v.execInVM("incus", deviceArgs...)
	if err != nil {
		exists, checkErr := v.deviceExists(instanceName, deviceName)
		if checkErr != nil || !exists {
			return fmt.Errorf("failed to add device %s: %w", deviceName, err)
		}
	}
	return nil
}

// startInstance starts an Incus instance using the incus start command.
// It executes the start command and returns an error with the provided error prefix
// if the start operation fails. This method is used to restart instances after
// configuration changes that require the instance to be stopped.
func (v *IncusVirt) startInstance(instanceName, errorPrefix string) error {
	_, err := v.execInVM("incus", v.addQuietFlag([]string{"start", instanceName})...)
	if err != nil {
		return fmt.Errorf("%s: %w", errorPrefix, err)
	}
	return nil
}

// ensureInstanceRunning ensures an instance is running, starting it if necessary.
// It checks the instance status, and if not running, attempts to start it.
// Uses idempotent behavior - if the instance is already running, the operation succeeds.
// Verifies the instance is actually running after start attempts.
func (v *IncusVirt) ensureInstanceRunning(instanceName string) error {
	instances, err := v.getInstances()
	if err != nil {
		return fmt.Errorf("failed to check if instance is running: %w", err)
	}
	if !v.isRunning(instances, instanceName) {
		_, err := v.execInVM("incus", v.addQuietFlag([]string{"start", instanceName})...)
		if err != nil {
			instances, checkErr := v.getInstances()
			if checkErr == nil && v.isRunning(instances, instanceName) {
				return nil
			}
			return fmt.Errorf("failed to start instance: %w", err)
		}
	}
	return nil
}

// deleteInstance deletes the Incus instance with the given name.
// Attempts deletion of the specified Incus instance, handling SSH connection failures
// gracefully to allow cleanup to proceed. Returns nil if the VM is unreachable or
// already deleted. Waits for the instance to become ready before deletion. If deletion
// cannot be confirmed and no handled error applies, returns an error indicating deletion failure.
func (v *IncusVirt) deleteInstance(name string) error {
	exists, err := v.instanceExists(name)
	if err != nil {
		if isSSHError(err) {
			return nil
		}
		return fmt.Errorf("failed to check if instance exists: %w", err)
	}
	if !exists {
		return nil
	}

	deadline := time.Now().Add(deleteInstanceWaitTimeout)
	if err := v.pollUntilNotBusy(name, 0, deadline); err != nil {
		return fmt.Errorf("failed to wait for instance to be ready: %w", err)
	}

	args := []string{"delete", name, "--force"}
	args = v.addQuietFlag(args)
	message := fmt.Sprintf("📦 Deleting Incus instance %s", name)
	_, err = v.execInVMProgress(message, "incus", args...)
	if err != nil {
		if isSSHError(err) {
			return nil
		}
		exists, checkErr := v.instanceExists(name)
		if checkErr == nil && !exists {
			return nil
		}
		return fmt.Errorf("failed to delete Incus instance: %w", err)
	}

	return nil
}

// addQuietFlag adds --quiet to incus command args when not verbose.
func (v *IncusVirt) addQuietFlag(args []string) []string {
	if !v.shell.IsVerbose() {
		return append(args, "--quiet")
	}
	return args
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure IncusVirt implements ContainerRuntime
var _ ContainerRuntime = (*IncusVirt)(nil)

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
