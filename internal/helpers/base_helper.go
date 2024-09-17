package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/windsor-hotel/cli/internal/config"
)

type BaseHelper struct {
	ConfigHandler config.ConfigHandler
}

func NewBaseHelper(configHandler config.ConfigHandler) *BaseHelper {
	return &BaseHelper{ConfigHandler: configHandler}
}

var goos = runtime.GOOS

// Function variable to allow mocking exec.Command in tests
var execCommand = exec.Command

func getParentProcessName() (string, error) {
	ppid := os.Getppid()
	cmd := execCommand("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", ppid), "get", "ParentProcessId,ExecutablePath", "/format:csv")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("unexpected output from command")
	}
	fields := strings.Split(lines[1], ",")
	if len(fields) < 2 {
		return "", fmt.Errorf("unexpected output from wmic")
	}
	return strings.TrimSpace(fields[1]), nil
}

var getShellType = func() string {
	if goos == "windows" {
		parentProcess, err := getParentProcessName()
		if err != nil {
			return "unknown"
		}
		parentProcess = strings.ToLower(parentProcess)
		if strings.Contains(parentProcess, "powershell") || strings.Contains(parentProcess, "pwsh") {
			return "powershell"
		}
		if strings.Contains(parentProcess, "cmd.exe") {
			return "cmd"
		}
	}
	return "unix"
}

func (h *BaseHelper) GetEnvVars() (map[string]string, error) {
	context, err := h.ConfigHandler.GetConfigValue("context")
	if err != nil {
		return nil, fmt.Errorf("error retrieving context: %w", err)
	}

	envVars, err := h.ConfigHandler.GetNestedMap(fmt.Sprintf("contexts.%s.environment", context))
	if err != nil {
		return map[string]string{}, nil
	}

	stringEnvVars := make(map[string]string)
	for k, v := range envVars {
		if strVal, ok := v.(string); ok {
			stringEnvVars[strings.ToUpper(k)] = strVal // Capitalize the key
		} else {
			return nil, fmt.Errorf("non-string value found in environment variables for context %s", context)
		}
	}

	// Add WINDSORCONTEXT to the environment variables
	stringEnvVars["WINDSORCONTEXT"] = context

	return stringEnvVars, nil
}

func (h *BaseHelper) PrintEnvVars() error {
	envVars, err := h.GetEnvVars()
	if err != nil {
		return err
	}
	shellType := getShellType()
	for key, value := range envVars {
		switch shellType {
		case "powershell":
			fmt.Printf("$env:%s='%s'\n", key, value)
		case "cmd":
			fmt.Printf("set %s=%s\n", key, value)
		default:
			fmt.Printf("export %s='%s'\n", key, value)
		}
	}
	return nil
}

// Ensure BaseHelper implements Helper interface
var _ Helper = (*BaseHelper)(nil)
