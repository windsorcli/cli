package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// maxDepth is the maximum depth to search for the project root
const maxDepth = 10

// getwd is a variable that points to os.Getwd, allowing it to be overridden in tests
var getwd = os.Getwd

// execCommand is a variable that points to exec.Command, allowing it to be overridden in tests
var execCommand = exec.Command

type Shell interface {
	PrintEnvVars(envVars map[string]string)
	GetProjectRoot() (string, error)
}

type DefaultShell struct {
	projectRoot string
	mu          sync.Mutex
}

func NewDefaultShell() *DefaultShell {
	return &DefaultShell{}
}

func (d *DefaultShell) GetProjectRoot() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.projectRoot != "" {
		return d.projectRoot, nil
	}

	// Try to get the git root first
	cmd := execCommand("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		d.projectRoot = strings.TrimSpace(string(output))
		return d.projectRoot, nil
	}

	// If git command fails, search for windsor.yaml or windsor.yml
	currentDir, err := getwd()
	if err != nil {
		return "", err
	}

	depth := 0
	for {
		if depth > maxDepth {
			return "", nil
		}

		windsorYaml := filepath.Join(currentDir, "windsor.yaml")
		windsorYml := filepath.Join(currentDir, "windsor.yml")

		if _, err := os.Stat(windsorYaml); err == nil {
			d.projectRoot = currentDir
			return d.projectRoot, nil
		}
		if _, err := os.Stat(windsorYml); err == nil {
			d.projectRoot = currentDir
			return d.projectRoot, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// We've reached the root of the file system
			return "", nil
		}
		currentDir = parentDir
		depth++
	}
}
