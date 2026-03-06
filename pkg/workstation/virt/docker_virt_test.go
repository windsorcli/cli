// The docker_virt_test package is a test suite for the DockerVirt ContainerRuntime implementation.
// It provides test coverage for Docker-native lifecycle behavior (Up, WriteConfig, Down).
// It verifies public method behavior only in a TDD fashion.

package virt

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

func setupDockerVirt(t *testing.T, opts ...func(*VirtTestMocks)) (*VirtTestMocks, *DockerVirt) {
	t.Helper()
	mocks := setupVirtMocks(t, opts...)
	mocks.Runtime.ContextName = "mock-context"
	if err := mocks.ConfigHandler.SetContext("mock-context"); err != nil {
		t.Fatalf("Failed to set context: %v", err)
	}
	dockerVirt := NewDockerVirt(mocks.Runtime)
	return mocks, dockerVirt
}

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewDockerVirt(t *testing.T) {
	t.Run("CreatesDockerVirtWithRuntime", func(t *testing.T) {
		// Given virt mocks
		mocks := setupVirtMocks(t)
		mocks.Runtime.ContextName = "mock-context"

		// When creating DockerVirt
		dockerVirt := NewDockerVirt(mocks.Runtime)

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
		NewDockerVirt(nil)
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDockerVirt_Up(t *testing.T) {
	t.Run("SucceedsWithNoError", func(t *testing.T) {
		// Given a DockerVirt
		_, dockerVirt := setupDockerVirt(t)

		// When calling Up
		err := dockerVirt.Up()

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
	t.Run("WhenNetworkListFailsReturnsNil", func(t *testing.T) {
		// Given a DockerVirt with shell that fails on docker network ls (in removeNetworkIfExists)
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "", fmt.Errorf("docker unavailable")
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then Down returns nil (best-effort)
		if err != nil {
			t.Errorf("Expected nil (best-effort), got %v", err)
		}
	})

	t.Run("WhenNoWindsorNetworksReturnsNil", func(t *testing.T) {
		// Given a DockerVirt with shell that returns no project containers and non-Windsor networks only
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "", nil
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\nhost\nother_net\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then no error (no project containers; context network not present)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("WhenProjectContainersAndNetworkExistCleansBoth", func(t *testing.T) {
		// Given a DockerVirt with shell that returns project container IDs and context network present
		var execCalls []string
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			call := command + " " + strings.Join(args, " ")
			execCalls = append(execCalls, call)
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "id1 id2", nil
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "bridge\nwindsor-mock-context\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

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
			if strings.Contains(c, "ps") && strings.Contains(c, "label=com.docker.compose.project=workstation-windsor-mock-context") {
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
		// Given a DockerVirt (context mock-context) with shell that returns other contexts' Windsor networks only
		var execCalls []string
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
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
		err := dockerVirt.Down()

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
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
			if command == "docker" && len(args) >= 1 && args[0] == "ps" {
				return "", fmt.Errorf("ps failed")
			}
			if command == "docker" && len(args) >= 2 && args[0] == "network" && args[1] == "ls" {
				return "windsor-mock-context\n", nil
			}
			return "", nil
		}

		// When calling Down
		err := dockerVirt.Down()

		// Then Down returns nil (best-effort; list failure is logged, network still removed if present)
		if err != nil {
			t.Errorf("Expected nil (best-effort), got %v", err)
		}
	})

	t.Run("WhenProjectVolumesExistRemovesThem", func(t *testing.T) {
		// Given a DockerVirt with shell that returns no containers, project volumes, and no context network
		var execCalls []string
		mocks, dockerVirt := setupDockerVirt(t)
		mocks.Shell.ExecSilentFunc = func(command string, args ...string) (string, error) {
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
		err := dockerVirt.Down()

		// Then no error
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		// And volume ls with project label was called
		var hasVolumeLs bool
		for _, c := range execCalls {
			if strings.Contains(c, "volume ls") && strings.Contains(c, "label=com.docker.compose.project=workstation-windsor-mock-context") {
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
}
