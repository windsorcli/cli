package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Set up the Windsor environment",
	Long:  "Set up the Windsor environment by executing necessary shell commands.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the context
		contextName, err := contextInstance.GetContext()
		if err != nil {
			return fmt.Errorf("Error getting context: %w", err)
		}

		// Get the context configuration
		contextConfig, err := cliConfigHandler.GetConfig()
		if err != nil {
			if verbose {
				return fmt.Errorf("Error getting context configuration: %w", err)
			}
			return nil
		}

		// Ensure VM is set before continuing
		if contextConfig.VM == nil {
			if verbose {
				fmt.Println("VM configuration is not set, skipping VM start")
			}
			return nil
		}

		// Collect environment variables
		envVars, err := collectEnvVars()
		if err != nil {
			return err
		}

		// Set environment variables for the command
		for k, v := range envVars {
			if err := osSetenv(k, v); err != nil {
				return fmt.Errorf("Error setting environment variable %s: %w", k, err)
			}
		}

		// Check the VM.Driver value and start the virtual machine if necessary
		if *contextConfig.VM.Driver == "colima" {
			if err := colimaHelper.WriteConfig(); err != nil {
				return fmt.Errorf("Error writing colima config: %w", err)
			}

			colimaCommand := "colima"
			colimaArgs := []string{"start", fmt.Sprintf("windsor-%s", contextName)}
			output, err := shellInstance.Exec(verbose, "Executing colima start command", colimaCommand, colimaArgs...)
			if err != nil {
				return fmt.Errorf("Error executing command %s %v: %w\n%s", colimaCommand, colimaArgs, err, output)
			}
		}

		// Check if Docker is enabled and run "docker-compose up" in daemon mode if necessary
		if contextConfig.Docker != nil && *contextConfig.Docker.Enabled {
			// Ensure Docker daemon is running
			if err := checkDockerDaemon(); err != nil {
				return fmt.Errorf("Docker daemon is not running: %w", err)
			}

			// Retry logic for docker-compose up
			retries := 3
			var lastErr error
			var lastOutput string
			for i := 0; i < retries; i++ {
				dockerComposeCommand := "docker-compose"
				dockerComposeArgs := []string{"up", "-d"}
				output, err := shellInstance.Exec(verbose, "Executing docker-compose up command", dockerComposeCommand, dockerComposeArgs...)
				if err == nil {
					lastErr = nil
					break
				}

				lastErr = err
				lastOutput = output

				if i < retries-1 {
					fmt.Println("Retrying docker-compose up...")
					time.Sleep(2 * time.Second)
				}
			}

			if lastErr != nil {
				return fmt.Errorf("Error executing command %s %v: %w\n%s", "docker-compose", []string{"up", "-d"}, lastErr, lastOutput)
			}
		}

		// Print welcome status page
		printWelcomeStatus(contextName)

		return nil
	},
}

// checkDockerDaemon checks if the Docker daemon is running
func checkDockerDaemon() error {
	command := "docker"
	args := []string{"info"}
	_, err := shellInstance.Exec(verbose, "Checking Docker daemon", command, args...)
	return err
}

func printWelcomeStatus(contextName string) {
	// Define ANSI color codes
	const (
		Reset  = "\033[0m"
		Green  = "\033[32m"
		Yellow = "\033[33m"
		Cyan   = "\033[36m"
	)

	fmt.Println(Green + "Welcome to the Windsor Environment!" + " 🎀 " + Reset)
	fmt.Println(strings.Repeat("=", 40))

	// Fetch and print Colima machine info
	fmt.Println(Cyan + "Colima Machine Info:" + Reset)
	colimaInfo, err := getColimaInfo(contextName)
	if err != nil {
		fmt.Println(Yellow + "Error fetching Colima info: " + err.Error() + Reset)
	} else {
		fmt.Println(colimaInfo)
	}

	// Fetch and print Docker service info
	fmt.Println(Cyan + "\nAccessible Docker Services:" + Reset)
	dockerInfo, err := getDockerServicesInfo()
	if err != nil {
		fmt.Println(Yellow + "Error fetching Docker service info: " + err.Error() + Reset)
	} else {
		fmt.Println(dockerInfo)
	}
}

func getColimaInfo(contextName string) (string, error) {
	cmd := exec.Command("colima", "ls", "--profile", fmt.Sprintf("windsor-%s", contextName), "--json")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}

	var colimaData struct {
		Address string `json:"address"`
		Arch    string `json:"arch"`
		CPUs    int    `json:"cpus"`
		Disk    int64  `json:"disk"`
		Memory  int64  `json:"memory"`
		Name    string `json:"name"`
		Runtime string `json:"runtime"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(out.Bytes(), &colimaData); err != nil {
		return "", err
	}

	// Format the Colima info for display
	colimaInfo := fmt.Sprintf(
		"Name: %s\nIP Address: %s\nArchitecture: %s\nCPUs: %d\nMemory: %.2f GB\nDisk: %.2f GB\nRuntime: %s\nStatus: %s",
		colimaData.Name,
		colimaData.Address,
		colimaData.Arch,
		colimaData.CPUs,
		float64(colimaData.Memory)/(1024*1024*1024),
		float64(colimaData.Disk)/(1024*1024*1024),
		colimaData.Runtime,
		colimaData.Status,
	)

	return colimaInfo, nil
}

func getDockerServicesInfo() (string, error) {
	cmd := exec.Command("docker", "ps", "--filter", "label=managed_by=windsor", "--format", "{{.ID}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}

	containerIDs := strings.Split(strings.TrimSpace(out.String()), "\n")
	var serviceInfo strings.Builder

	for _, containerID := range containerIDs {
		if containerID == "" {
			continue
		}

		inspectCmd := exec.Command("docker", "inspect", containerID, "--format", "{{ index .Config.Labels \"com.docker.compose.service\" }}")
		var inspectOut bytes.Buffer
		inspectCmd.Stdout = &inspectOut
		if err := inspectCmd.Run(); err != nil {
			return "", err
		}

		serviceName := strings.TrimSpace(inspectOut.String())
		if serviceName != "" {
			serviceInfo.WriteString(fmt.Sprintf("- http://%s\n", serviceName))
		}
	}

	return serviceInfo.String(), nil
}

func init() {
	rootCmd.AddCommand(upCmd)
}
