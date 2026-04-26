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

	t.Run("AwsPlatformDefaultsBackendToS3", func(t *testing.T) {
		// Given --platform aws and no explicit terraform.backend.type override
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "aws")

		// Then terraform.backend.type defaults to "s3" since AWS contexts overwhelmingly
		// store terraform state in S3; the operator can still override via --set.
		if overrides["terraform.backend.type"] != "s3" {
			t.Errorf("Expected terraform.backend.type=s3, got %v", overrides["terraform.backend.type"])
		}
	})

	t.Run("AzurePlatformDefaultsBackendToAzurerm", func(t *testing.T) {
		// Given --platform azure and no explicit terraform.backend.type override
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "azure")

		// Then terraform.backend.type defaults to "azurerm"
		if overrides["terraform.backend.type"] != "azurerm" {
			t.Errorf("Expected terraform.backend.type=azurerm, got %v", overrides["terraform.backend.type"])
		}
	})

	t.Run("ExplicitBackendTypePreservedOverDefault", func(t *testing.T) {
		// Given --platform aws AND a pre-set terraform.backend.type (as if --set
		// terraform.backend.type=local had already been merged into the map). Note:
		// callers actually merge --set after this helper runs, so this test exercises
		// the symmetric guard against a future change in merge ordering — the helper
		// must never clobber a value already present.
		overrides := map[string]any{
			"terraform.backend.type": "local",
		}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "aws")

		// Then the explicit value wins; no platform-default takes effect
		if overrides["terraform.backend.type"] != "local" {
			t.Errorf("Expected explicit terraform.backend.type=local to be preserved, got %v", overrides["terraform.backend.type"])
		}
	})

	t.Run("MetalPlatformDefaultsBackendToKubernetes", func(t *testing.T) {
		// Given --platform metal (bare-metal Talos cluster). For platforms where
		// the cluster is the natural state store (no canonical cloud bucket
		// service), the kubernetes backend is the right default — each component's
		// state lives as a Secret in the cluster it manages. The bootstrap dance
		// runs the same shape as for s3: apply the cluster/backend component with
		// local state, migrate to kubernetes once the cluster is up, then up rest.
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "metal")

		// Then terraform.backend.type defaults to "kubernetes"
		if overrides["terraform.backend.type"] != "kubernetes" {
			t.Errorf("Expected terraform.backend.type=kubernetes, got %v", overrides["terraform.backend.type"])
		}
	})

	t.Run("DockerPlatformDefaultsBackendToKubernetes", func(t *testing.T) {
		// Given --platform docker (local cluster on a docker workstation), the
		// in-cluster kubernetes backend mirrors the metal case — the cluster
		// Windsor brings up is also the state store for everything that follows.
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "docker")

		// Then terraform.backend.type defaults to "kubernetes"
		if overrides["terraform.backend.type"] != "kubernetes" {
			t.Errorf("Expected terraform.backend.type=kubernetes, got %v", overrides["terraform.backend.type"])
		}
	})

	t.Run("IncusPlatformDefaultsBackendToKubernetes", func(t *testing.T) {
		// Given --platform incus (the colima-incus inferred platform), same
		// kubernetes-backend default applies — incus workstations run a cluster.
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "incus")

		// Then terraform.backend.type defaults to "kubernetes"
		if overrides["terraform.backend.type"] != "kubernetes" {
			t.Errorf("Expected terraform.backend.type=kubernetes, got %v", overrides["terraform.backend.type"])
		}
	})

	t.Run("VmDriverInferenceFlowsThroughToBackendDefault", func(t *testing.T) {
		// Given --vm-driver docker-desktop with no --platform, the helper infers
		// platform=docker, and the backend default must then key off that inferred
		// platform. Guards the order of operations: vmDriver inference must run
		// before the backend-default switch reads overrides["platform"], otherwise
		// a driver-only invocation (no explicit --platform) wouldn't get the
		// kubernetes default it needs.
		overrides := map[string]any{}

		// When the helper is applied with only a vm driver
		applyWorkstationFlagOverrides(overrides, "docker-desktop", "")

		// Then platform=docker is inferred AND backend defaults to kubernetes
		if overrides["platform"] != "docker" {
			t.Errorf("Expected inferred platform=docker, got %v", overrides["platform"])
		}
		if overrides["terraform.backend.type"] != "kubernetes" {
			t.Errorf("Expected backend default to follow inferred platform, got %v", overrides["terraform.backend.type"])
		}
	})

	t.Run("UnmappedPlatformDoesNotDefaultBackendType", func(t *testing.T) {
		// Given --platform gcp (not yet wired up — GCSBackend schema is missing)
		// the default switch must not invent a value. Operators on gcp are
		// expected to configure terraform.backend.type explicitly until the
		// schema lands.
		overrides := map[string]any{}

		// When the helper is applied
		applyWorkstationFlagOverrides(overrides, "", "gcp")

		// Then no backend default is injected
		if _, set := overrides["terraform.backend.type"]; set {
			t.Errorf("Expected no backend default for unmapped platform, got %v", overrides["terraform.backend.type"])
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
