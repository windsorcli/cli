package shell

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/windsorcli/cli/pkg/di"
)

// setSafeDockerShellMocks creates a safe "supermock" where all components are mocked except for DockerShell.
func setSafeDockerShellMocks(injector ...di.Injector) struct {
	Injector di.Injector
} {
	if len(injector) == 0 {
		injector = []di.Injector{di.NewMockInjector()}
	}

	i := injector[0]

	// Mock the execCommand to simulate successful command execution for specific Docker command
	execCommand = func(name string, arg ...string) *exec.Cmd {
		if name == "docker" && len(arg) > 0 && arg[0] == "ps" {
			cmd := exec.Command("echo", "mock-container-id")
			return cmd
		}
		cmd := exec.Command("echo", "mock output")
		return cmd
	}

	// Mock the getwd to simulate a specific working directory
	getwd = func() (string, error) {
		return "/mock/project/root", nil
	}

	return struct {
		Injector di.Injector
	}{
		Injector: i,
	}
}

// TestDockerShell_Exec tests the Exec method of DockerShell.
func TestDockerShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original cmdOutput and execCommand functions
		originalCmdOutput := cmdOutput
		originalExecCommand := execCommand

		// Defer restoring the original functions
		defer func() {
			cmdOutput = originalCmdOutput
			execCommand = originalExecCommand
		}()

		// Mock the necessary functions to simulate a successful execution
		cmdOutput = func(cmd *exec.Cmd) (string, error) {
			if cmd.Path == "/bin/echo" && len(cmd.Args) == 2 && cmd.Args[1] == "mock-container-id" {
				return "mock-container-id", nil
			}
			return "", fmt.Errorf("unexpected command %s with args %v", cmd.Path, cmd.Args)
		}

		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "docker" && len(arg) > 0 {
				switch {
				case arg[0] == "ps" && len(arg) > 4 && arg[1] == "--filter" && arg[2] == "label=role=windsor_exec" && arg[3] == "--format" && arg[4] == "{{.ID}}":
					return exec.Command("/bin/echo", "mock-container-id")
				case len(arg) > 5 && arg[0] == "exec" && arg[1] == "-i" && arg[2] == "mock-container-id" && arg[3] == "sh" && arg[4] == "-c":
					expectedCmd := "cd /work && windsor exec -- echo hello"
					if arg[5] == expectedCmd {
						return exec.Command("/bin/echo", "mock output")
					}
				}
			}
			t.Fatalf("unexpected command %s with args %v", name, arg)
			return nil
		}

		output, _, err := dockerShell.Exec("echo", "hello")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedOutput := "mock output\n"
		if output != expectedOutput {
			t.Errorf("expected %q, got %q", expectedOutput, output)
		}
	})

	t.Run("CommandError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original cmdOutput function
		originalCmdOutput := cmdOutput

		// Defer restoring the original function
		defer func() {
			cmdOutput = originalCmdOutput
		}()

		// Mock cmdOutput to simulate a command execution failure
		cmdOutput = func(cmd *exec.Cmd) (string, error) {
			return "", fmt.Errorf("command execution failed")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil {
			t.Fatalf("expected an error, got none")
		}
	})

	t.Run("ContainerIDError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original execCommand function
		originalExecCommand := execCommand

		// Defer restoring the original function
		defer func() {
			execCommand = originalExecCommand
		}()

		// Mock execCommand to simulate an empty container ID
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "docker" && len(arg) > 0 && arg[0] == "ps" {
				return exec.Command("/bin/echo", "")
			}
			return exec.Command(name, arg...)
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "failed to get Windsor exec container ID: no Windsor exec container found" {
			t.Fatalf("expected error 'failed to get Windsor exec container ID: no Windsor exec container found', got %v", err)
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original getwd function
		originalGetwd := getwd

		// Defer restoring the original function
		defer func() {
			getwd = originalGetwd
		}()

		// Mock getwd to simulate an error
		getwd = func() (string, error) {
			return "", fmt.Errorf("failed to get project root")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "failed to get project root: failed to get project root" {
			t.Fatalf("expected error 'failed to get project root: failed to get project root', got %v", err)
		}
	})

	t.Run("ErrorGettingWorkingDirectory", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original getwd function
		originalGetwd := getwd

		// Defer restoring the original function
		defer func() {
			getwd = originalGetwd
		}()

		// Counter to track the number of calls to getwd
		callCount := 0

		// Mock getwd to simulate an error on the second call
		getwd = func() (string, error) {
			callCount++
			if callCount == 2 {
				return "", fmt.Errorf("failed to get working directory on second call")
			}
			return "/mock/path", nil
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "failed to get current working directory: failed to get working directory on second call" {
			t.Fatalf("expected error 'failed to get current working directory: failed to get working directory on second call', got %v", err)
		}
	})

	t.Run("ErrorDeterminingRelativeDirectory", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original filepathRel function
		originalFilepathRel := filepathRel

		// Defer restoring the original function
		defer func() {
			filepathRel = originalFilepathRel
		}()

		// Mock filepathRel to simulate an error
		filepathRel = func(basepath, targpath string) (string, error) {
			return "", fmt.Errorf("failed to determine relative directory")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "failed to determine relative directory: failed to determine relative directory" {
			t.Fatalf("expected error 'failed to determine relative directory: failed to determine relative directory', got %v", err)
		}
	})

	t.Run("CommandStartError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original cmdStart function
		originalCmdStart := cmdStart

		// Defer restoring the original function
		defer func() {
			cmdStart = originalCmdStart
		}()

		// Mock cmdStart to simulate a command start error
		cmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command start failed")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "command start failed: command start failed" {
			t.Fatalf("expected error 'command start failed: command start failed', got %v", err)
		}
	})

	t.Run("CommandWaitError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Save the original cmdWait function
		originalCmdWait := cmdWait

		// Defer restoring the original function
		defer func() {
			cmdWait = originalCmdWait
		}()

		// Mock cmdWait to simulate a command wait error
		cmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command execution failed")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "command execution failed: command execution failed\n" {
			t.Fatalf("expected error 'command execution failed: command execution failed', got %v", err)
		}
	})
}

// TestDockerShell_GetWindsorExecContainerID tests the getWindsorExecContainerID method of DockerShell.
func TestDockerShell_GetWindsorExecContainerID(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Setup for getWindsorExecContainerID success test
		// Test successful container ID retrieval
	})

	t.Run("NoContainerFound", func(t *testing.T) {
		// Setup for getWindsorExecContainerID no container found test
		// Test scenario where no container with the specified label is found
	})

	t.Run("DockerCommandError", func(t *testing.T) {
		// Setup for getWindsorExecContainerID Docker command error test
		// Test error when Docker command execution fails
	})
}
