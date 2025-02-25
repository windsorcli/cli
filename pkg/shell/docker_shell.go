package shell

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
)

// DockerShell implements the Shell interface using Docker.
type DockerShell struct {
	DefaultShell
}

// NewDockerShell creates a new instance of DockerShell.
func NewDockerShell(injector di.Injector) *DockerShell {
	return &DockerShell{
		DefaultShell: DefaultShell{
			injector: injector,
		},
	}
}

// Exec executes a command inside a Docker container with the label "role=windsor_exec".
func (s *DockerShell) Exec(command string, args ...string) (string, error) {
	// Get the container ID
	containerID, err := s.getWindsorExecContainerID()
	if err != nil {
		return "", fmt.Errorf("failed to get Windsor exec container ID: %w", err)
	}

	// Get the project root
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	// Determine the current working directory relative to the project root
	currentDir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	relativeDir, err := filepathRel(projectRoot, currentDir)
	if err != nil {
		return "", fmt.Errorf("failed to determine relative directory: %w", err)
	}

	// Construct the working directory inside the container and build the shell command.
	workDir := filepath.Join("/work", relativeDir)

	// Combine 'command' and its 'args' into a single command string.
	combinedCmd := command
	if len(args) > 0 {
		combinedCmd += " " + strings.Join(args, " ")
	}
	shellCmd := fmt.Sprintf("cd %s && windsor exec -- %s", workDir, combinedCmd)

	// Execute the command using the execCommand shim for better testability.
	cmd := execCommand("docker", "exec", "-it", containerID, "sh", "-c", shellCmd)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmdStart(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command start failed: %w", err)
	}
	if err := cmdWait(cmd); err != nil {
		return stdoutBuf.String(), fmt.Errorf("command execution failed: %w\n%s", err, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// getWindsorExecContainerID retrieves the container ID of the Windsor exec container.
func (s *DockerShell) getWindsorExecContainerID() (string, error) {
	cmd := execCommand("docker", "ps", "--filter", "label=role=windsor_exec", "--format", "{{.ID}}")
	output, err := cmdOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to list Docker containers: %w", err)
	}

	containerID := strings.TrimSpace(output)
	if containerID == "" {
		return "", fmt.Errorf("no Windsor exec container found")
	}

	return containerID, nil
}

// Ensure DockerShell implements the Shell interface
var _ Shell = (*DockerShell)(nil)
