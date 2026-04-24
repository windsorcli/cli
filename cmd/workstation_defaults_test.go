package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestApplyWorkstationFlagOverrides(t *testing.T) {
	t.Run("NoFlagsYieldNoOverrides", func(t *testing.T) {
		// Given no flags
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "")

		// Then no overrides are injected
		if len(overrides) != 0 {
			t.Errorf("Expected empty overrides, got %v", overrides)
		}
	})

	t.Run("VmDriverDockerDesktopInfersDockerPlatform", func(t *testing.T) {
		// Given --vm-driver docker-desktop and no --platform
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "docker-desktop", "")

		// Then workstation.runtime=docker-desktop and platform=docker are set
		if overrides["workstation.runtime"] != "docker-desktop" {
			t.Errorf("Expected workstation.runtime=docker-desktop, got %v", overrides["workstation.runtime"])
		}
		if overrides["platform"] != "docker" {
			t.Errorf("Expected inferred platform=docker, got %v", overrides["platform"])
		}
	})

	t.Run("VmDriverColimaIncusRemapsRuntimeAndPlatform", func(t *testing.T) {
		// Given --vm-driver colima-incus
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "colima-incus", "")

		// Then runtime is remapped to colima and platform is incus
		if overrides["workstation.runtime"] != "colima" {
			t.Errorf("Expected workstation.runtime=colima (remapped), got %v", overrides["workstation.runtime"])
		}
		if overrides["platform"] != "incus" {
			t.Errorf("Expected platform=incus, got %v", overrides["platform"])
		}
	})

	t.Run("ExplicitPlatformOverridesInference", func(t *testing.T) {
		// Given --vm-driver docker-desktop AND --platform aws
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "docker-desktop", "aws")

		// Then platform=aws wins and workstation.runtime=docker-desktop
		if overrides["platform"] != "aws" {
			t.Errorf("Expected explicit platform=aws, got %v", overrides["platform"])
		}
		if overrides["workstation.runtime"] != "docker-desktop" {
			t.Errorf("Expected workstation.runtime=docker-desktop, got %v", overrides["workstation.runtime"])
		}
	})

	t.Run("PlatformAloneIsSetWithoutVmDriver", func(t *testing.T) {
		// Given --platform metal with no --vm-driver
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "metal")

		// Then only platform is set, workstation.runtime is untouched
		if overrides["platform"] != "metal" {
			t.Errorf("Expected platform=metal, got %v", overrides["platform"])
		}
		if _, set := overrides["workstation.runtime"]; set {
			t.Errorf("Expected workstation.runtime to remain unset, got %v", overrides["workstation.runtime"])
		}
	})
}

func TestResolveBlueprintURL(t *testing.T) {
	t.Run("ExplicitBlueprintWins", func(t *testing.T) {
		// Given an explicit --blueprint value
		urls, err := resolveBlueprintURL("oci://custom/blueprint:v1", "docker", "local", "/does/not/matter", true)

		// Then the explicit URL is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(urls) != 1 || urls[0] != "oci://custom/blueprint:v1" {
			t.Errorf("Expected [oci://custom/blueprint:v1], got %v", urls)
		}
	})

	t.Run("PlatformFallsBackToDefaultURL", func(t *testing.T) {
		// Given --platform but no --blueprint
		urls, err := resolveBlueprintURL("", "docker", "aws", "/does/not/matter", true)

		// Then the default blueprint URL is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(urls) != 1 || urls[0] == "" {
			t.Errorf("Expected a non-empty default blueprint URL, got %v", urls)
		}
	})

	t.Run("PlatformWithExistingTemplateReturnsNilWhenBootstrapAllowed", func(t *testing.T) {
		// Given --platform and a contexts/_template directory that exists on disk, init
		// flow (allowLocalBootstrap=true). The local template is authoritative and the
		// OCI fallback must NOT be layered on top — otherwise repos like windsorcli/core,
		// where the template and the default OCI source are effectively the same
		// blueprint, end up with duplicate template/core entries that Initialize
		// rejects as ambiguous.
		tmpDir := t.TempDir()
		templateDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template dir: %v", err)
		}

		urls, err := resolveBlueprintURL("", "aws", "aws", templateDir, true)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if urls != nil {
			t.Errorf("Expected nil URLs when --platform is set and local template exists, got %v", urls)
		}
	})

	t.Run("PlatformFallsBackToDefaultURLWhenBootstrapDisallowed", func(t *testing.T) {
		// Given --platform, a template dir that exists on disk, but allowLocalBootstrap=false
		// (the `up` flow). The existing-template guard intentionally does NOT kick in here;
		// `up` preserves the prior unconditional-URL behavior on --platform so its
		// semantics don't shift based on disk contents.
		tmpDir := t.TempDir()
		templateDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template dir: %v", err)
		}

		urls, err := resolveBlueprintURL("", "aws", "aws", templateDir, false)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(urls) != 1 || urls[0] == "" {
			t.Errorf("Expected default URL for --platform on up flow even when template exists, got %v", urls)
		}
	})

	t.Run("LocalContextWithoutTemplateUsesDefaultWhenBootstrapAllowed", func(t *testing.T) {
		// Given context=local, a missing template dir, and bootstrap allowed (init flow)
		tmpDir := t.TempDir()
		missingTemplate := filepath.Join(tmpDir, "contexts", "_template")

		urls, err := resolveBlueprintURL("", "", "local", missingTemplate, true)

		// Then the default blueprint URL is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(urls) != 1 || urls[0] == "" {
			t.Errorf("Expected a non-empty default blueprint URL, got %v", urls)
		}
	})

	t.Run("LocalContextWithoutTemplateReturnsNilWhenBootstrapDisallowed", func(t *testing.T) {
		// Given context=local, a missing template dir, and bootstrap disallowed (up flow)
		tmpDir := t.TempDir()
		missingTemplate := filepath.Join(tmpDir, "contexts", "_template")

		urls, err := resolveBlueprintURL("", "", "local", missingTemplate, false)

		// Then no URL is returned — up must not silently pull from OCI
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if urls != nil {
			t.Errorf("Expected nil URLs when bootstrap disallowed, got %v", urls)
		}
	})

	t.Run("LocalContextWithExistingTemplateReturnsNil", func(t *testing.T) {
		// Given context=local and a template dir that DOES exist
		tmpDir := t.TempDir()
		templateDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template dir: %v", err)
		}

		urls, err := resolveBlueprintURL("", "", "local", templateDir, true)

		// Then no URL is returned (use existing blueprint state on disk)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if urls != nil {
			t.Errorf("Expected nil URLs when template exists, got %v", urls)
		}
	})

	t.Run("NonLocalContextWithoutFlagsReturnsNil", func(t *testing.T) {
		// Given context != local and no flags
		urls, err := resolveBlueprintURL("", "", "aws", "/does/not/matter", true)

		// Then no URL is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if urls != nil {
			t.Errorf("Expected nil URLs for non-local context, got %v", urls)
		}
	})

	t.Run("BootstrapDisallowedSkipsStat", func(t *testing.T) {
		// Given an invalid template path that would fail os.Stat
		badPath := "\x00invalid"

		// When bootstrap is disallowed, the stat is never reached
		urls, err := resolveBlueprintURL("", "", "local", badPath, false)

		// Then no error and no URL — stat short-circuited by the gate
		if err != nil {
			t.Fatalf("Expected no error when bootstrap is disallowed, got %v", err)
		}
		if urls != nil {
			t.Errorf("Expected nil URLs, got %v", urls)
		}
	})

	t.Run("PermissionErrorOnStatIsWrapped", func(t *testing.T) {
		// Given a template path whose parent is not traversable. We simulate this
		// with a path containing an embedded NUL which os.Stat treats as invalid.
		badPath := "\x00invalid"

		_, err := resolveBlueprintURL("", "", "local", badPath, true)

		// Then a wrapped error is returned (not os.IsNotExist)
		if err == nil {
			t.Fatal("Expected an error from os.Stat on an invalid path, got nil")
		}
		if !strings.Contains(err.Error(), "error checking template root") {
			t.Errorf("Expected wrapped 'error checking template root' error, got: %v", err)
		}
	})
}
