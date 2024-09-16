package helpers

import (
	"fmt"
	"os"
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

var getEnv = os.Getenv

var isPowerShell = func() bool {
	shell := getEnv("SHELL")
	if shell == "" {
		shell = getEnv("ComSpec")
	}
	return strings.Contains(strings.ToLower(shell), "powershell") || strings.Contains(strings.ToLower(shell), "pwsh")
}

func getShellType() string {
	if goos == "windows" {
		if isPowerShell() {
			return "powershell"
		}
		return "cmd"
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
			stringEnvVars[k] = strVal
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

// Ensure BaseHelper implements CLIHelperInterface
var _ Helper = (*BaseHelper)(nil)
