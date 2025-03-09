package shell

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
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
func (s *DockerShell) Exec(command string, args ...string) (string, int, error) {
	containerID, err := s.getWindsorExecContainerID()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get Windsor exec container ID: %w", err)
	}

	workDir, err := s.getWorkDir()
	if err != nil {
		return "", 0, err
	}

	shellCmd := s.buildShellCommand(workDir, command, args...)
	cmdArgs := []string{"exec", "-i", containerID, "sh", "-c", shellCmd}

	// Directly write the output to os.Stdout and os.Stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutWriter := io.MultiWriter(&stdoutBuf, os.Stdout)
	stderrWriter := io.MultiWriter(&stderrBuf, os.Stderr)

	return s.runDockerCommand(cmdArgs, stdoutWriter, stderrWriter)
}

// ExecProgress runs a command in a Docker container labeled "role=windsor_exec" with a progress indicator.
func (s *DockerShell) ExecProgress(message string, command string, args ...string) (string, int, error) {
	if s.verbose {
		return s.Exec(command, args...)
	}

	containerID, err := s.getWindsorExecContainerID()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get Windsor exec container ID: %w", err)
	}

	workDir, err := s.getWorkDir()
	if err != nil {
		return "", 0, err
	}

	// Adjust the shell command to change directory first, then execute within 'windsor exec'
	shellCmd := s.buildShellCommand(workDir, command, args...)
	cmdArgs := []string{"exec", "-i", containerID, "sh", "-c", shellCmd}

	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout, exitCode, err := s.runDockerCommand(cmdArgs, &stdoutBuf, &stderrBuf)
	spin.Stop()

	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n%s", message, stderrBuf.String())
		return stdout, exitCode, fmt.Errorf("Error: %w\n%s", err, stderrBuf.String())
	}

	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
	return stdout, exitCode, nil
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

// getWorkDir calculates the working directory inside the container.
func (s *DockerShell) getWorkDir() (string, error) {
	projectRoot, err := s.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get project root: %w", err)
	}

	currentDir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	relativeDir, err := filepathRel(projectRoot, currentDir)
	if err != nil {
		return "", fmt.Errorf("failed to determine relative directory: %w", err)
	}

	return filepath.ToSlash(filepath.Join(constants.CONTAINER_EXEC_WORKDIR, relativeDir)), nil
}

// buildShellCommand constructs the shell command to be executed in the container.
func (s *DockerShell) buildShellCommand(workDir, command string, args ...string) string {
	combinedCmd := command
	if len(args) > 0 {
		combinedCmd += " " + strings.Join(args, " ")
	}
	finalCmd := fmt.Sprintf("cd %s && windsor exec -- %s", workDir, combinedCmd)
	return finalCmd
}

// runDockerCommand executes the Docker command and writes the output to provided writers.
func (s *DockerShell) runDockerCommand(cmdArgs []string, stdoutWriter, stderrWriter io.Writer) (string, int, error) {
	cmd := execCommand("docker", cmdArgs...)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	if err := cmdStart(cmd); err != nil {
		return "", 1, fmt.Errorf("command start failed: %w", err)
	}

	if err := cmdWait(cmd); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return "", processStateExitCode(exitError.ProcessState), fmt.Errorf("Error: %w", err)
		}
		return "", 1, fmt.Errorf("unexpected error during command execution: %w", err)
	}

	exitCode := processStateExitCode(cmd.ProcessState)
	if exitCode != 0 {
		return "", exitCode, fmt.Errorf("command execution failed with exit code %d", exitCode)
	}
	return "", exitCode, nil
}

// Ensure DockerShell implements the Shell interface
var _ Shell = (*DockerShell)(nil)
