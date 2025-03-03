package shell

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/pkg/constants"
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

// Exec runs a command in a Docker container labeled "role=windsor_exec".
// It retrieves the container ID, calculates the relative path, and executes
// the command inside the container, streaming the output to stdout and stderr,
// and also returning the output as a string.
func (s *DockerShell) Exec(command string, args ...string) (string, int, error) {
	containerID, err := s.getWindsorExecContainerID()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get Windsor exec container ID: %w", err)
	}

	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get project root: %w", err)
	}

	currentDir, err := getwd()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get current working directory: %w", err)
	}

	relativeDir, err := filepathRel(projectRoot, currentDir)
	if err != nil {
		return "", 0, fmt.Errorf("failed to determine relative directory: %w", err)
	}

	relativeDir = filepath.ToSlash(relativeDir)

	workDir := filepath.ToSlash(filepath.Join(constants.CONTAINER_EXEC_WORKDIR, relativeDir))

	combinedCmd := command
	if len(args) > 0 {
		combinedCmd += " " + strings.Join(args, " ")
	}
	shellCmd := fmt.Sprintf("cd %s && windsor --silent exec -- sh -c '%s'", workDir, combinedCmd)

	cmdArgs := []string{"exec", "-i", containerID, "sh", "-c", shellCmd}

	cmd := execCommand("docker", cmdArgs...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	// Start the command
	if err := cmdStart(cmd); err != nil {
		return "", 1, fmt.Errorf("command start failed: %w", err)
	}

	// Wait for the command to finish and capture the exit code
	if err := cmdWait(cmd); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return stdoutBuf.String(), exitError.ExitCode(), err
		}
		return "", 1, fmt.Errorf("unexpected error during command execution: %w", err)
	}

	// Capture the exit code from the process state
	exitCode := cmd.ProcessState.ExitCode()
	if exitCode != 0 {
		return stdoutBuf.String(), exitCode, fmt.Errorf("command execution failed with exit code %d", exitCode)
	}
	return stdoutBuf.String(), exitCode, nil
}

// getWindsorExecContainerID retrieves the container ID of the Windsor exec container.
func (s *DockerShell) getWindsorExecContainerID() (string, error) {
	cmd := execCommand("docker", "ps", "--filter", "label=role=windsor_exec", "--format", "{{.ID}}")
	output, err := cmdOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to list Docker containers: %w", err)
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		return "", fmt.Errorf("no Windsor exec container found")
	}

	return containerID, nil
}

// Ensure DockerShell implements the Shell interface
var _ Shell = (*DockerShell)(nil)
