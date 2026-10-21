// The instance methods of the IncusVirt manage the lifecycle of Incus instances.
// They launch, start, stop, and delete instances, and read the instance and operation state.
// They wait while an instance is busy and handle a failed launch.

package virt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Private Methods
// =============================================================================

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
		return nil, werror.Wrap(err, "failed to parse instances JSON")
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
		return nil, werror.Wrap(err, "failed to parse operations JSON")
	}
	return operations, nil
}

// launchInstance launches a new Incus instance or ensures an existing one is ready.
// If the instance already exists, it waits for any in-progress operations to complete.
// If the instance does not exist, it launches a new instance with the provided configuration
// and waits for the launch operation to complete. Handles launch errors gracefully with retry logic.
func (v *IncusVirt) launchInstance(config *IncusInstanceConfig) error {
	exists, err := v.instanceExists(config.Name)
	if err != nil {
		return werror.Wrap(err, "failed to check if instance exists")
	}

	if exists {
		deadline := time.Now().Add(instanceExistsWaitTimeout)
		if err := v.pollUntilNotBusy(config.Name, 0, deadline); err != nil {
			return werror.Wrap(err, "instance exists but is still busy after waiting")
		}
		return nil
	}

	args := v.buildLaunchArgs(config)
	message := fmt.Sprintf("Creating Incus instance %s", config.Name)
	launchArgs := v.addQuietFlag(args)
	_, err = v.execInVMProgress(message, "incus", launchArgs...)
	if err != nil {
		return v.handleInstanceLaunchError(config.Name, message, args, err)
	}

	if err := v.pollUntilNotBusy(config.Name, maxIterationsForLaunch, time.Time{}); err != nil {
		return werror.Wrap(err, "instance is still busy after launch, cannot proceed")
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
			return werror.Wrap(err, "instance is busy")
		}
		updateArgs := v.addQuietFlag(args)
		_, err := v.execInVMProgress(message, "incus", updateArgs...)
		if err != nil {
			exists, checkErr := v.instanceExists(instanceName)
			if checkErr == nil && exists {
				return v.ensureInstanceNotBusy(instanceName, "instance exists but is busy")
			}
			return werror.Wrap(err, "failed to launch Incus instance after wait")
		}
		return nil
	}

	return werror.Wrap(launchErr, "failed to launch Incus instance")
}

// ensureInstanceNotBusy ensures an instance is not busy, waiting if necessary.
// It polls the instance until it is no longer busy, using the configured maximum wait timeout.
// Returns an error with the provided error prefix if the instance does not become available
// within the timeout period.
func (v *IncusVirt) ensureInstanceNotBusy(instanceName, errorPrefix string) error {
	maxWait := v.getMaxWaitTimeout(0)
	deadline := time.Now().Add(maxWait)
	if err := v.pollUntilNotBusy(instanceName, 0, deadline); err != nil {
		return werror.Wrap(err, "%s", errorPrefix)
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
		return false, werror.Wrap(err, "failed to check if instance is running")
	}

	running := v.isRunning(instances, instanceName)
	if !running {
		return false, nil
	}

	_, err = v.execInVM("incus", v.addQuietFlag([]string{"stop", instanceName})...)
	if err != nil {
		instances, checkErr := v.getInstances()
		if checkErr != nil {
			return false, werror.Wrap(checkErr, "failed to verify instance state after stop attempt")
		}
		if v.isRunning(instances, instanceName) {
			return false, werror.Wrap(err, "%s", errorPrefix)
		}
	}

	if err := v.pollUntilNotBusy(instanceName, maxIterationsForStop, time.Time{}); err != nil {
		return false, werror.Wrap(err, "instance is still busy after stop")
	}

	instances, err = v.getInstances()
	if err != nil {
		return false, werror.Wrap(err, "failed to verify instance stopped")
	}
	if v.isRunning(instances, instanceName) {
		return false, fmt.Errorf("instance is still running after stop")
	}

	return true, nil
}

// startInstance starts an Incus instance using the incus start command.
// It executes the start command and returns an error with the provided error prefix
// if the start operation fails. This method is used to restart instances after
// configuration changes that require the instance to be stopped.
func (v *IncusVirt) startInstance(instanceName, errorPrefix string) error {
	_, err := v.execInVM("incus", v.addQuietFlag([]string{"start", instanceName})...)
	if err != nil {
		return werror.Wrap(err, "%s", errorPrefix)
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
		return werror.Wrap(err, "failed to check if instance is running")
	}
	if !v.isRunning(instances, instanceName) {
		_, err := v.execInVM("incus", v.addQuietFlag([]string{"start", instanceName})...)
		if err != nil {
			instances, checkErr := v.getInstances()
			if checkErr == nil && v.isRunning(instances, instanceName) {
				return nil
			}
			return werror.Wrap(err, "failed to start instance")
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
		return werror.Wrap(err, "failed to check if instance exists")
	}
	if !exists {
		return nil
	}

	deadline := time.Now().Add(deleteInstanceWaitTimeout)
	if err := v.pollUntilNotBusy(name, 0, deadline); err != nil {
		return werror.Wrap(err, "failed to wait for instance to be ready")
	}

	args := []string{"delete", name, "--force"}
	args = v.addQuietFlag(args)
	message := fmt.Sprintf("Deleting Incus instance %s", name)
	_, err = v.execInVMProgress(message, "incus", args...)
	if err != nil {
		if isSSHError(err) {
			return nil
		}
		exists, checkErr := v.instanceExists(name)
		if checkErr == nil && !exists {
			return nil
		}
		return werror.Wrap(err, "failed to delete Incus instance")
	}

	return nil
}
