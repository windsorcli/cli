package cmd

import (
	"testing"
)

func TestUpCmd(t *testing.T) {
	originalExitFunc := exitFunc
	exitFunc = mockExit
	t.Cleanup(func() {
		exitFunc = originalExitFunc
	})

	// Common success configuration
	// successConfig := func() mocks.SuperMocks {
	// 	mocks := mocks.CreateSuperMocks()
	// 	mocks.ConfigHandler.GetConfigFunc = func() *config.Context {
	// 		return &config.Context{
	// 			Docker: &config.DockerConfig{
	// 				Enabled:     ptrBool(true),
	// 				NetworkCIDR: ptrString("192.168.5.0/24"),
	// 			},
	// 			VM: &config.VMConfig{
	// 				Driver: ptrString("colima"),
	// 				CPU:    ptrInt(2),
	// 				Memory: ptrInt(4),
	// 				Disk:   ptrInt(10),
	// 			},
	// 		}
	// 	}
	// 	mocks.ColimaVirt.UpFunc = func(verbose ...bool) error {
	// 		return nil
	// 	}
	// 	mocks.ColimaVirt.GetContainerInfoFunc = func() ([]virt.ContainerInfo, error) {
	// 		return []virt.ContainerInfo{
	// 			{
	// 				Name: "mock-vm",
	// 			},
	// 		}, nil
	// 	}
	// 	mocks.SecureShell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
	// 		if command == "sudo" && args[0] == "iptables" && args[1] == "-t" && args[2] == "filter" && args[3] == "-C" {
	// 			return "", fmt.Errorf("Bad rule")
	// 		}
	// 		if command == "ls" && args[0] == "/sys/class/net" {
	// 			return "br-mock-interface", nil
	// 		}
	// 		return "Executed: " + command, nil
	// 	}
	// 	mocks.Shell.ExecFunc = func(verbose bool, message string, command string, args ...string) (string, error) {
	// 		if command == "sudo" && args[0] == "route" {
	// 			return "mock-route-output", nil
	// 		}
	// 		return "Executed: " + command, nil
	// 	}
	// 	mocks.SSHClient.SetClientConfigFileFunc = func(configStr, hostname string) error {
	// 		return nil
	// 	}
	// 	return mocks
	// }
}
