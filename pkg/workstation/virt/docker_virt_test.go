package virt_test

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/windsorcli/cli/pkg/workstation/virt"
)

// =============================================================================
// Test Setup
// =============================================================================

// setupDockerVirt returns a fixture and a DockerVirt that uses the stub shims of the fixture.
func setupDockerVirt(t *testing.T) (*fixture, *virt.DockerVirt) {
	t.Helper()
	f := newFixture(t)
	dockerVirt := virt.NewDockerVirt(f.runtime)
	dockerVirt.SetShims(f.shims)
	return f, dockerVirt
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewDockerVirt(t *testing.T) {
	t.Run("CreatesDockerVirtWithRuntime", func(t *testing.T) {
		// Given a runtime
		f := newFixture(t)

		// When creating DockerVirt
		dockerVirt := virt.NewDockerVirt(f.runtime)

		// Then DockerVirt should be non-nil with BaseVirt set
		if dockerVirt == nil {
			t.Fatal("Expected non-nil DockerVirt")
		}
		if dockerVirt.BaseVirt == nil {
			t.Fatal("Expected non-nil BaseVirt")
		}
	})

	t.Run("PanicsWhenRuntimeNil", func(t *testing.T) {
		// Given no runtime (nil)

		// When/Then NewDockerVirt panics
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when runtime is nil")
			}
		}()
		virt.NewDockerVirt(nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDockerVirt_Up(t *testing.T) {
	t.Run("SucceedsWithNoError", func(t *testing.T) {
		// Given a DockerVirt
		f, dockerVirt := setupDockerVirt(t)

		// When calling Up
		err := dockerVirt.Up(f.ctx)

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestDockerVirt_WriteConfig(t *testing.T) {
	t.Run("SucceedsWithNoError", func(t *testing.T) {
		// Given a DockerVirt
		_, dockerVirt := setupDockerVirt(t)

		// When calling WriteConfig
		err := dockerVirt.WriteConfig()

		// Then no error should be returned
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

func TestDockerVirt_Down(t *testing.T) {
	t.Run("WhenDockerHostSocketMissingSkipsCleanupSilently", func(t *testing.T) {
		// Given a DOCKER_HOST unix socket that does not exist
		var execCalls []string
		f, dockerVirt := setupDockerVirt(t)
		f.shims.Getenv = func(key string) string { return "unix:///home/u/.colima/windsor-local/docker.sock" }
		f.shims.Stat = func(name string) (os.FileInfo, error) { return nil, fs.ErrNotExist }
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execCalls = append(execCalls, command+" "+strings.Join(args, " "))
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then no docker command runs and no event is emitted
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
		if len(execCalls) != 0 {
			t.Errorf("Expected no docker calls, got %v", execCalls)
		}
		if len(f.recorder.events) != 0 {
			t.Errorf("Expected no events, got %v", f.recorder.events)
		}
	})

	t.Run("WhenDockerHostSocketExistsButDaemonFailsWarns", func(t *testing.T) {
		// Given a DOCKER_HOST unix socket that exists while docker ps fails
		f, dockerVirt := setupDockerVirt(t)
		f.shims.Getenv = func(key string) string { return "unix:///var/run/docker.sock" }
		f.shims.Stat = func(name string) (os.FileInfo, error) { return nil, nil }
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "", fmt.Errorf("ps failed")
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then the failure is a warning and Down returns nil
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
		warnings := f.recorder.warnings()
		if len(warnings) != 1 || !strings.Contains(warnings[0], "could not list containers") {
			t.Errorf("Expected one container list warning, got %q", warnings)
		}
	})

	t.Run("WhenDockerHostIsNotUnixSocketProceedsWithCleanup", func(t *testing.T) {
		// Given DOCKER_HOST set to a Windows named pipe and a Stat that would report not-exist
		var execCalls []string
		f, dockerVirt := setupDockerVirt(t)
		f.shims.Getenv = func(key string) string { return "npipe:////./pipe/docker_engine" }
		f.shims.Stat = func(name string) (os.FileInfo, error) { return nil, fs.ErrNotExist }
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			execCalls = append(execCalls, command+" "+strings.Join(args, " "))
			return "", nil
		}

		// When calling Down
		if err := dockerVirt.Down(f.ctx); err != nil {
			t.Errorf("Expected nil, got %v", err)
		}

		// Then cleanup runs
		if len(execCalls) == 0 {
			t.Error("Expected docker cleanup calls for a non-unix DOCKER_HOST")
		}
	})

	t.Run("WhenNetworkListFailsReturnsNil", func(t *testing.T) {
		// Given a DockerVirt with shell that fails on docker network ls (in removeNetworkIfExists)
		f, dockerVirt := setupDockerVirt(t)
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "", fmt.Errorf("docker unavailable")
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then Down returns nil (best-effort)
		if err != nil {
			t.Errorf("Expected nil (best-effort), got %v", err)
		}
	})

	t.Run("WhenNoWindsorNetworksReturnsNil", func(t *testing.T) {
		// Given a DockerVirt with shell that returns no project containers and non-Windsor networks only
		f, dockerVirt := setupDockerVirt(t)
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "", nil
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\nhost\nother_net\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then no error (no project containers; context network not present)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("WhenProjectContainersAndNetworkExistCleansBoth", func(t *testing.T) {
		// Given a DockerVirt with shell that returns project container IDs and context network present
		var execCalls []string
		f, dockerVirt := setupDockerVirt(t)
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			call := command + " " + strings.Join(args, " ")
			execCalls = append(execCalls, call)
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "id1 id2", nil
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\nwindsor-local\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And ps -a -q --filter label=com.docker.compose.project=, stop, rm, network rm were invoked
		hasProjectLabel := false
		hasStop := false
		hasRm := false
		hasNetworkRm := false
		for _, c := range execCalls {
			if strings.Contains(c, "ps") && strings.Contains(c, "label=com.docker.compose.project=workstation-windsor-local") {
				hasProjectLabel = true
			}
			if strings.Contains(c, "docker stop") {
				hasStop = true
			}
			if strings.Contains(c, "docker rm") && strings.Contains(c, "-f") && strings.Contains(c, "-v") {
				hasRm = true
			}
			if strings.Contains(c, "network rm") {
				hasNetworkRm = true
			}
		}
		if !hasProjectLabel {
			t.Error("Expected docker ps -a -q --filter label=com.docker.compose.project= to be called for project")
		}
		if !hasStop {
			t.Error("Expected docker stop to be called for containers")
		}
		if !hasRm {
			t.Error("Expected docker rm -f to be called for containers")
		}
		if !hasNetworkRm {
			t.Error("Expected docker network rm to be called")
		}
	})

	t.Run("WhenOnlyOtherContextsWindsorNetworksExistCleansNone", func(t *testing.T) {
		// Given a DockerVirt (context local) with shell that returns other contexts' Windsor networks only
		var execCalls []string
		f, dockerVirt := setupDockerVirt(t)
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			call := command + " " + strings.Join(args, " ")
			execCalls = append(execCalls, call)
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "", nil
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\nwindsor-staging\nwindsor-dev\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And network rm was not called (current context's network not in list)
		for _, c := range execCalls {
			if strings.Contains(c, "network rm") {
				t.Errorf("Expected no network rm when context network does not exist, got exec call: %s", c)
			}
		}
	})

	t.Run("WhenListContainersForProjectFailsReturnsNil", func(t *testing.T) {
		// Given a DockerVirt but ps -a -q --filter label=com.docker.compose.project= fails
		f, dockerVirt := setupDockerVirt(t)
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "", fmt.Errorf("ps failed")
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "windsor-local\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then Down returns nil (best-effort; list failure is logged, network still removed if present)
		if err != nil {
			t.Errorf("Expected nil (best-effort), got %v", err)
		}
	})

	t.Run("WhenProjectVolumesExistRemovesThem", func(t *testing.T) {
		// Given a DockerVirt with shell that returns no containers, project volumes, and no context network
		var execCalls []string
		f, dockerVirt := setupDockerVirt(t)
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			call := command + " " + strings.Join(args, " ")
			execCalls = append(execCalls, call)
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "", nil
			}
			if command == "docker" && len(args) >= 2 && args[0] == "volume" && args[1] == "ls" {
				return "controlplane_1_etc_kubernetes\ncontrolplane_1_var\n", nil
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And volume ls with project label was called
		var hasVolumeLs bool
		for _, c := range execCalls {
			if strings.Contains(c, "volume ls") && strings.Contains(c, "label=com.docker.compose.project=workstation-windsor-local") {
				hasVolumeLs = true
				break
			}
		}
		if !hasVolumeLs {
			t.Error("Expected docker volume ls with project label to be called")
		}
		// And volume rm was called for each volume
		var volumeRmCalls []string
		for _, c := range execCalls {
			if strings.Contains(c, "volume rm") {
				volumeRmCalls = append(volumeRmCalls, c)
			}
		}
		if len(volumeRmCalls) != 2 {
			t.Errorf("Expected 2 volume rm calls, got %d: %v", len(volumeRmCalls), volumeRmCalls)
		}
		volumeNames := []string{"controlplane_1_etc_kubernetes", "controlplane_1_var"}
		for _, name := range volumeNames {
			found := false
			for _, c := range volumeRmCalls {
				if strings.Contains(c, name) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected volume rm to be called for %q", name)
			}
		}
	})
	t.Run("WhenVolumeListFailsWarnsAndContinues", func(t *testing.T) {
		// Given a docker volume ls that fails
		f, dockerVirt := setupDockerVirt(t)
		var networkListed bool
		f.shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "volume" && args[1] == "ls" {
				return "", fmt.Errorf("volume ls failed")
			}
			if len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				networkListed = true
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down(f.ctx)

		// Then one volume warning shows and the network cleanup still runs
		if err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
		warnings := f.recorder.warnings()
		if len(warnings) != 1 || !strings.Contains(warnings[0], "could not list volumes for project workstation-windsor-local") {
			t.Errorf("Expected one volume warning, got %q", warnings)
		}
		if !networkListed {
			t.Error("Expected the network cleanup to run")
		}
	})
}
