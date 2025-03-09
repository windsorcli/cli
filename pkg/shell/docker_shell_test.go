package shell

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
			return mockEchoCommand("mock-container-id")
		}
		return mockEchoCommand("mock output")
	}

	// Mock the cmdOutput to return a specific container ID
	cmdOutput = func(cmd *exec.Cmd) (string, error) {
		if cmd.Path == "docker" && len(cmd.Args) > 1 && cmd.Args[1] == "ps" {
			return "mock-container-id", nil
		}
		return "mock output", nil
	}

	// Mock the getwd to simulate a specific working directory
	getwd = func() (string, error) {
		return "/mock/project/root", nil
	}

	// Mock the processStateExitCode to always return 0
	processStateExitCode = func(state *os.ProcessState) int {
		return 0
	}

	return struct {
		Injector di.Injector
	}{
		Injector: i,
	}
}

// mockEchoCommand returns a cross-platform echo command
func mockEchoCommand(output string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", "echo", output)
	}
	return exec.Command("echo", output)
}

// TestDockerShell_Exec tests the Exec method of DockerShell.
func TestDockerShell_Exec(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Preserve the original execCommand function
		originalExecCommand := execCommand
		defer func() { execCommand = originalExecCommand }() // Restore it after the test

		// Flag to verify if execCommand is invoked with 'docker exec'
		execCommandCalled := false
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "docker" && len(arg) > 0 && arg[0] == "exec" {
				execCommandCalled = true
			}
			return mockEchoCommand("mock output")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !execCommandCalled {
			t.Fatalf("expected execCommand to be called with 'docker exec', but it was not")
		}
	})

	t.Run("CommandError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Backup the original cmdOutput function to restore it later
		originalCmdOutput := cmdOutput
		defer func() { cmdOutput = originalCmdOutput }()

		// Mock cmdOutput to simulate a command execution failure
		cmdOutput = func(cmd *exec.Cmd) (string, error) {
			return "", fmt.Errorf("command execution failed")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil {
			t.Fatalf("expected an error, got none")
		}
	})

	t.Run("ErrorGettingProjectRoot", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Backup the original getwd function to restore it later
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()

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

		// Preserve the original getwd function to ensure it is restored after the test
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()

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

		// Preserve the original filepathRel function to ensure it is restored after the test
		originalFilepathRel := filepathRel
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

		// Preserve the original cmdStart function and ensure it's restored after the test
		originalCmdStart := cmdStart
		defer func() { cmdStart = originalCmdStart }()

		// Mock cmdStart to simulate a command start error
		cmdStart = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command start failed")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "command start failed: command start failed" {
			t.Fatalf("expected error 'command start failed: command start failed', got %v", err)
		}
	})

	t.Run("CommandWaitUnexpectedError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Preserve the original cmdWait function and ensure it's restored after the test
		originalCmdWait := cmdWait
		defer func() { cmdWait = originalCmdWait }()

		// Mock cmdWait to simulate an unexpected error during command wait
		cmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("unexpected error during command execution")
		}

		_, _, err := dockerShell.Exec("echo", "hello")
		if err == nil || err.Error() != "unexpected error during command execution: unexpected error during command execution" {
			t.Fatalf("expected error 'unexpected error during command execution: unexpected error during command execution', got %v", err)
		}
	})
}

// TestDockerShell_ExecProgress tests the ExecProgress method of DockerShell.
func TestDockerShell_ExecProgress(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Preserve the original execCommand function
		originalExecCommand := execCommand
		defer func() { execCommand = originalExecCommand }() // Restore it after the test

		// Flag to verify if execCommand is invoked with 'docker exec'
		execCommandCalled := false
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "docker" && len(arg) > 0 && arg[0] == "exec" {
				execCommandCalled = true
			}
			return mockEchoCommand("mock output")
		}

		_, _, err := dockerShell.ExecProgress("Running command", "echo", "hello")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !execCommandCalled {
			t.Fatalf("expected execCommand to be called with 'docker exec', but it was not")
		}
	})

	t.Run("ExecProgressWithVerbose", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)
		dockerShell.verbose = true

		// Preserve the original execCommand function
		originalExecCommand := execCommand
		defer func() { execCommand = originalExecCommand }() // Restore it after the test

		// Flag to verify if execCommand is invoked with 'docker exec'
		execCommandCalled := false
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "docker" && len(arg) > 0 && arg[0] == "exec" {
				execCommandCalled = true
			}
			return mockEchoCommand("mock output")
		}

		_, _, err := dockerShell.ExecProgress("Running command", "echo", "hello")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !execCommandCalled {
			t.Fatalf("expected execCommand to be called with 'docker exec', but it was not")
		}
	})

	t.Run("GetWindsorExecContainerIDError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Backup the original cmdOutput function to restore it later
		originalCmdOutput := cmdOutput
		defer func() { cmdOutput = originalCmdOutput }()

		// Mock cmdOutput to simulate a failure in retrieving the container ID
		cmdOutput = func(cmd *exec.Cmd) (string, error) {
			return "", fmt.Errorf("failed to get Windsor exec container ID")
		}

		_, _, err := dockerShell.ExecProgress("Running command", "echo", "hello")
		if err == nil || !strings.Contains(err.Error(), "failed to get Windsor exec container ID") {
			t.Fatalf("expected error containing 'failed to get Windsor exec container ID', got %v", err)
		}
	})

	t.Run("GetWorkDirError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Backup the original getwd function to restore it later
		originalGetwd := getwd
		defer func() { getwd = originalGetwd }()

		// Mock getwd to simulate an error in retrieving the current working directory
		getwd = func() (string, error) {
			return "", fmt.Errorf("failed to get current working directory")
		}

		_, _, err := dockerShell.ExecProgress("Running command", "echo", "hello")
		if err == nil || !strings.Contains(err.Error(), "failed to get current working directory") {
			t.Fatalf("expected error containing 'failed to get current working directory', got %v", err)
		}
	})

	t.Run("ErrorRunningDockerCommand", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Backup the original execCommand function to restore it later
		originalExecCommand := execCommand
		defer func() { execCommand = originalExecCommand }()

		// Mock execCommand to simulate a failure inside runDockerCommand
		execCommand = func(name string, arg ...string) *exec.Cmd {
			if name == "docker" && len(arg) > 0 && arg[0] == "exec" {
				return exec.Command("false") // Simulate a command that fails
			}
			return mockEchoCommand("mock output")
		}

		_, _, err := dockerShell.ExecProgress("Running command", "echo", "hello")
		if err == nil || !strings.Contains(err.Error(), "Error: exit status 1") {
			t.Fatalf("expected error containing 'Error: exit status 1', got %v", err)
		}
	})
}

// TestDockerShell_GetWindsorExecContainerID tests the getWindsorExecContainerID method of DockerShell.
func TestDockerShell_GetWindsorExecContainerID(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Mock the cmdOutput function to simulate successful retrieval of container ID
		originalCmdOutput := cmdOutput
		defer func() { cmdOutput = originalCmdOutput }()
		cmdOutput = func(cmd *exec.Cmd) (string, error) {
			return "mock-container-id", nil
		}

		containerID, err := dockerShell.getWindsorExecContainerID()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if containerID != "mock-container-id" {
			t.Fatalf("expected container ID 'mock-container-id', got %v", containerID)
		}
	})

	t.Run("NoContainerFound", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Mock the cmdOutput function to simulate no container found
		originalCmdOutput := cmdOutput
		defer func() { cmdOutput = originalCmdOutput }()
		cmdOutput = func(cmd *exec.Cmd) (string, error) {
			return "", nil
		}

		_, err := dockerShell.getWindsorExecContainerID()
		if err == nil || err.Error() != "no Windsor exec container found" {
			t.Fatalf("expected error 'no Windsor exec container found', got %v", err)
		}
	})

	t.Run("DockerCommandError", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Mock the cmdOutput function to simulate a Docker command error
		originalCmdOutput := cmdOutput
		defer func() { cmdOutput = originalCmdOutput }()
		cmdOutput = func(cmd *exec.Cmd) (string, error) {
			return "", fmt.Errorf("failed to list Docker containers")
		}

		_, err := dockerShell.getWindsorExecContainerID()
		if err == nil || !strings.Contains(err.Error(), "failed to list Docker containers") {
			t.Fatalf("expected error containing 'failed to list Docker containers', got %v", err)
		}
	})
}

// TestDockerShell_runDockerCommand tests the runDockerCommand method of DockerShell.
func TestDockerShell_runDockerCommand(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		var stdoutBuf, stderrBuf strings.Builder
		_, exitCode, err := dockerShell.runDockerCommand([]string{"echo", "hello"}, &stdoutBuf, &stderrBuf)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if stdoutBuf.String() != "mock output\n" {
			t.Fatalf("expected stdout 'mock output', got %s", stdoutBuf.String())
		}
	})

	t.Run("CommandWaitFailed", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Mock the cmdWait function to simulate a command wait failure
		originalCmdWait := cmdWait
		defer func() { cmdWait = originalCmdWait }()
		cmdWait = func(cmd *exec.Cmd) error {
			return fmt.Errorf("command wait failed")
		}

		var stdoutBuf, stderrBuf strings.Builder
		_, _, err := dockerShell.runDockerCommand([]string{"echo", "hello"}, &stdoutBuf, &stderrBuf)
		if err == nil || !strings.Contains(err.Error(), "command wait failed") {
			t.Fatalf("expected error containing 'command wait failed', got %v", err)
		}
	})

	t.Run("CommandExecutionFailed", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Mock the cmdWait function to simulate a command execution failure
		originalCmdWait := cmdWait
		defer func() { cmdWait = originalCmdWait }()
		cmdWait = func(cmd *exec.Cmd) error {
			return &exec.ExitError{ProcessState: &os.ProcessState{}}
		}

		// Mock the processStateExitCode function to return a non-zero exit code
		originalProcessStateExitCode := processStateExitCode
		defer func() { processStateExitCode = originalProcessStateExitCode }()
		processStateExitCode = func(ps *os.ProcessState) int {
			return 1
		}

		var stdoutBuf, stderrBuf strings.Builder
		_, exitCode, err := dockerShell.runDockerCommand([]string{"echo", "hello"}, &stdoutBuf, &stderrBuf)
		if err == nil || exitCode == 0 {
			t.Fatalf("expected command execution failure with non-zero exit code, got error: %v, exit code: %d", err, exitCode)
		}
	})

	t.Run("CommandExecutionFailedWithNonZeroExitCode", func(t *testing.T) {
		injector := di.NewMockInjector()
		mocks := setSafeDockerShellMocks(injector)
		dockerShell := NewDockerShell(mocks.Injector)

		// Mock the processStateExitCode function to return a non-zero exit code
		originalProcessStateExitCode := processStateExitCode
		defer func() { processStateExitCode = originalProcessStateExitCode }()
		processStateExitCode = func(ps *os.ProcessState) int {
			return 2
		}

		var stdoutBuf, stderrBuf strings.Builder
		_, exitCode, err := dockerShell.runDockerCommand([]string{"echo", "hello"}, &stdoutBuf, &stderrBuf)
		if err == nil || exitCode == 0 {
			t.Fatalf("expected command execution failure with non-zero exit code, got error: %v, exit code: %d", err, exitCode)
		}
		if exitCode != 2 {
			t.Fatalf("expected exit code 2, got %d", exitCode)
		}
	})
}
