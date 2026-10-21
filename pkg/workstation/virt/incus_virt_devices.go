// The device methods of the IncusVirt add and update the devices of Incus instances.
// They configure the network device of an instance and add the other configured devices.

package virt

import (
	"fmt"
	"strings"
	"time"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Private Methods
// =============================================================================

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

// configureNetworkDevice configures the network device for an instance.
// It checks if the network device exists, and if not, adds it. If it already exists,
// updates the device configuration. This method handles both initial device creation
// and device modification scenarios.
func (v *IncusVirt) configureNetworkDevice(config *IncusInstanceConfig) error {
	exists, err := v.deviceExists(config.Name, "eth0")
	if err != nil {
		return werror.Wrap(err, "failed to check if network device exists")
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
		return werror.Wrap(err, "instance is busy, cannot add network device")
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
				return werror.Wrap(originalErr, "instance is still busy after waiting, cannot add device")
			}

			_, retryErr := v.execInVM("incus", v.addQuietFlag(deviceArgs)...)
			if retryErr != nil {
				exists, checkErr := v.deviceExists(config.Name, "eth0")
				if checkErr != nil {
					return werror.Wrap(checkErr, "failed to check if device exists after retry")
				}
				if !exists {
					return werror.Wrap(retryErr, "failed to add network device after retry")
				}
			}
		} else {
			exists, checkErr := v.deviceExists(config.Name, "eth0")
			if checkErr != nil {
				return werror.Wrap(checkErr, "failed to check if device exists")
			}
			if !exists {
				return werror.Wrap(originalErr, "failed to add network device")
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
		return werror.Wrap(err, "instance is busy, cannot update network device")
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
				return werror.Wrap(originalErr, "instance is still busy after waiting, cannot set network device")
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
				return werror.Wrap(err, "instance is still busy, cannot override network device")
			}
			overrideArgs := []string{"config", "device", "override", config.Name, "eth0", fmt.Sprintf("ipv4.address=%s", config.IPv4)}
			overrideArgs = v.addQuietFlag(overrideArgs)
			_, err = v.execInVM("incus", overrideArgs...)
			if err != nil {
				if err := v.pollUntilNotBusy(config.Name, maxIterationsForLaunch, time.Time{}); err != nil {
					return werror.Wrap(err, "instance is still busy, cannot retry override")
				}
				_, retryErr := v.execInVM("incus", v.addQuietFlag(overrideArgs)...)
				if retryErr != nil {
					return werror.Wrap(retryErr, "failed to override network device with static IP")
				}
			}
		}
	}

	if wasRunning {
		return v.startInstance(config.Name, "failed to start instance after configuring network")
	}

	return nil
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
		return werror.Wrap(err, "failed to check if device %s exists", deviceName)
	}
	if exists {
		return nil
	}

	deviceType := deviceConfig["type"]
	if deviceType == "disk" {
		if source, ok := deviceConfig["source"]; ok {
			if err := v.ensureFileExists(source); err != nil {
				return werror.Wrap(err, "failed to ensure source file exists for device %s", deviceName)
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
			return werror.Wrap(err, "failed to add device %s", deviceName)
		}
	}
	return nil
}
