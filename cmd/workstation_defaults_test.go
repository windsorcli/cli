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
		urls, err := resolveBlueprintURL("oci://custom/blueprint:v1", "docker", "local", "/does/not/matter")

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
		urls, err := resolveBlueprintURL("", "docker", "aws", "/does/not/matter")

		// Then the default blueprint URL is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(urls) != 1 || urls[0] == "" {
			t.Errorf("Expected a non-empty default blueprint URL, got %v", urls)
		}
	})

	t.Run("LocalContextWithoutTemplateUsesDefault", func(t *testing.T) {
		// Given context=local and a template dir that does not exist
		tmpDir := t.TempDir()
		missingTemplate := filepath.Join(tmpDir, "contexts", "_template")

		urls, err := resolveBlueprintURL("", "", "local", missingTemplate)

		// Then the default blueprint URL is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(urls) != 1 || urls[0] == "" {
			t.Errorf("Expected a non-empty default blueprint URL, got %v", urls)
		}
	})

	t.Run("LocalContextWithExistingTemplateReturnsNil", func(t *testing.T) {
		// Given context=local and a template dir that DOES exist
		tmpDir := t.TempDir()
		templateDir := filepath.Join(tmpDir, "contexts", "_template")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template dir: %v", err)
		}

		urls, err := resolveBlueprintURL("", "", "local", templateDir)

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
		urls, err := resolveBlueprintURL("", "", "aws", "/does/not/matter")

		// Then no URL is returned
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if urls != nil {
			t.Errorf("Expected nil URLs for non-local context, got %v", urls)
		}
	})

	t.Run("PermissionErrorOnStatIsWrapped", func(t *testing.T) {
		// Given a template path whose parent is not traversable. We simulate this
		// with a path containing an embedded NUL which os.Stat treats as invalid.
		badPath := "\x00invalid"

		_, err := resolveBlueprintURL("", "", "local", badPath)

		// Then a wrapped error is returned (not os.IsNotExist)
		if err == nil {
			t.Fatal("Expected an error from os.Stat on an invalid path, got nil")
		}
		if !strings.Contains(err.Error(), "error checking template root") {
			t.Errorf("Expected wrapped 'error checking template root' error, got: %v", err)
		}
	})
}
