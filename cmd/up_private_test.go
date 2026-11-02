package cmd

import (
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/workstation"
)

// =============================================================================
// Test Setup
// =============================================================================

// =============================================================================
// Test Cases
// =============================================================================

func TestBuildUpFlagOverrides(t *testing.T) {
	// Package-level flag vars are shared; reset after each case.
	resetFlags := func() {
		upVmDriver = ""
		upPlatform = ""
		upBlueprint = ""
		upSetFlags = nil
	}

	t.Run("EmptyFlagsYieldEmptyMap", func(t *testing.T) {
		resetFlags()
		t.Cleanup(resetFlags)

		overrides, err := buildUpFlagOverrides()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(overrides) != 0 {
			t.Errorf("Expected empty overrides, got %v", overrides)
		}
	})

	t.Run("VmDriverDockerDesktopInfersDockerPlatform", func(t *testing.T) {
		resetFlags()
		t.Cleanup(resetFlags)
		upVmDriver = "docker-desktop"

		overrides, err := buildUpFlagOverrides()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if overrides["workstation.runtime"] != "docker-desktop" {
			t.Errorf("Expected workstation.runtime=docker-desktop, got %v", overrides["workstation.runtime"])
		}
		if overrides["platform"] != "docker" {
			t.Errorf("Expected inferred platform=docker, got %v", overrides["platform"])
		}
	})

	t.Run("VmDriverColimaIncusRemapsToColimaRuntimeAndIncusPlatform", func(t *testing.T) {
		resetFlags()
		t.Cleanup(resetFlags)
		upVmDriver = "colima-incus"

		overrides, err := buildUpFlagOverrides()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if overrides["workstation.runtime"] != "colima" {
			t.Errorf("Expected workstation.runtime=colima (remapped from colima-incus), got %v", overrides["workstation.runtime"])
		}
		if overrides["platform"] != "incus" {
			t.Errorf("Expected platform=incus, got %v", overrides["platform"])
		}
	})

	t.Run("ExplicitPlatformOverridesVmDriverInference", func(t *testing.T) {
		resetFlags()
		t.Cleanup(resetFlags)
		upVmDriver = "docker-desktop"
		upPlatform = "aws"

		overrides, err := buildUpFlagOverrides()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if overrides["platform"] != "aws" {
			t.Errorf("Expected explicit platform=aws to win, got %v", overrides["platform"])
		}
		if overrides["workstation.runtime"] != "docker-desktop" {
			t.Errorf("Expected workstation.runtime=docker-desktop, got %v", overrides["workstation.runtime"])
		}
	})

	t.Run("SetFlagsParsedAsKeyValuePairs", func(t *testing.T) {
		resetFlags()
		t.Cleanup(resetFlags)
		upSetFlags = []string{"cluster.driver=talos", "cluster.endpoint=https://localhost:6443"}

		overrides, err := buildUpFlagOverrides()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if overrides["cluster.driver"] != "talos" {
			t.Errorf("Expected cluster.driver=talos, got %v", overrides["cluster.driver"])
		}
		if overrides["cluster.endpoint"] != "https://localhost:6443" {
			t.Errorf("Expected cluster.endpoint=https://localhost:6443, got %v", overrides["cluster.endpoint"])
		}
	})

	t.Run("InvalidSetFlagReturnsError", func(t *testing.T) {
		resetFlags()
		t.Cleanup(resetFlags)
		upSetFlags = []string{"no-equals-sign"}

		_, err := buildUpFlagOverrides()
		if err == nil {
			t.Fatal("Expected error for malformed --set, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --set format") {
			t.Errorf("Expected 'invalid --set format' error, got: %v", err)
		}
		if werr := werror.Find(err); werr == nil || werr.Remediation != setFormatHint {
			t.Errorf("Expected the --set format hint, got %+v", werr)
		}
	})
}

func TestPrintDeferredWork(t *testing.T) {
	divider := strings.Repeat("═", 47)
	actionRequired := "\nAction required\n" + divider + "\n"
	nextSteps := "\nNext steps\n" + divider + "\n"

	t.Run("EmptyItemsProducesNoOutput", func(t *testing.T) {
		var buf strings.Builder
		printDeferredWork(&buf, nil, "darwin")
		if buf.Len() != 0 {
			t.Errorf("Expected no output for empty items, got %q", buf.String())
		}
	})

	t.Run("RequiredItemRendersHaltSentenceWithSudoParenOnUnix", func(t *testing.T) {
		var buf strings.Builder
		printDeferredWork(&buf, []workstation.DeferredWorkItem{
			{Required: true, Command: "windsor configure network"},
		}, "darwin")
		want := actionRequired + "Run 'windsor configure network' (asks for sudo), then re-run 'windsor up'.\n"
		if buf.String() != want {
			t.Errorf("Expected %q, got %q", want, buf.String())
		}
	})

	t.Run("RequiredItemRendersHaltSentenceWithAdministratorParenOnWindows", func(t *testing.T) {
		var buf strings.Builder
		printDeferredWork(&buf, []workstation.DeferredWorkItem{
			{Required: true, Command: "windsor configure network"},
		}, "windows")
		want := actionRequired + "Run 'windsor configure network' (Administrator PowerShell), then re-run 'windsor up'.\n"
		if buf.String() != want {
			t.Errorf("Expected %q, got %q", want, buf.String())
		}
	})

	t.Run("OptionalItemRendersOutcomeSentence", func(t *testing.T) {
		var buf strings.Builder
		printDeferredWork(&buf, []workstation.DeferredWorkItem{
			{Required: false, Command: "windsor configure network", Outcome: "use *.local.test in your browser"},
		}, "darwin")
		want := nextSteps + "Run 'windsor configure network' (asks for sudo) to use *.local.test in your browser.\n"
		if buf.String() != want {
			t.Errorf("Expected %q, got %q", want, buf.String())
		}
	})

	t.Run("RequiredItemFoldsMatchingOptionalOutcomeIntoSingleLine", func(t *testing.T) {
		// Given a halt (cluster-privilege) paired with an optional outcome (DNS hint) sharing
		// the same command — the typical first-run colima case where 'configure network' will
		// produce both. The operator must see one line that names the outcome AND the re-run
		// guidance, not the same command printed twice.
		var buf strings.Builder
		printDeferredWork(&buf, []workstation.DeferredWorkItem{
			{Required: false, Command: "windsor configure network", Outcome: "use *.local.test in your browser"},
			{Required: true, Command: "windsor configure network"},
		}, "darwin")
		want := actionRequired + "Run 'windsor configure network' (asks for sudo) to use *.local.test in your browser, then re-run 'windsor up'.\n"
		if buf.String() != want {
			t.Errorf("Expected folded output %q, got %q", want, buf.String())
		}
	})

	t.Run("OptionalItemWithDistinctCommandStillRendersSeparatelyAlongsideHalt", func(t *testing.T) {
		var buf strings.Builder
		printDeferredWork(&buf, []workstation.DeferredWorkItem{
			{Required: true, Command: "windsor configure network"},
			{Required: false, Command: "windsor configure network --revert", Outcome: "remove host configuration"},
		}, "darwin")
		want := actionRequired + "Run 'windsor configure network' (asks for sudo), then re-run 'windsor up'.\n" +
			"Run 'windsor configure network --revert' (asks for sudo) to remove host configuration.\n"
		if buf.String() != want {
			t.Errorf("Expected halt + distinct optional %q, got %q", want, buf.String())
		}
	})

	t.Run("SecondOptionalOutcomeSharingRequiredCommandStillRenders", func(t *testing.T) {
		// Given a required halt plus TWO optional outcomes sharing its command — only the first
		// folds into the halt line; the second must still render on its own line rather than
		// being silently dropped.
		var buf strings.Builder
		printDeferredWork(&buf, []workstation.DeferredWorkItem{
			{Required: true, Command: "windsor configure network"},
			{Required: false, Command: "windsor configure network", Outcome: "use *.local.test in your browser"},
			{Required: false, Command: "windsor configure network", Outcome: "reach the cluster ingress"},
		}, "darwin")
		want := actionRequired + "Run 'windsor configure network' (asks for sudo) to use *.local.test in your browser, then re-run 'windsor up'.\n" +
			"Run 'windsor configure network' (asks for sudo) to reach the cluster ingress.\n"
		if buf.String() != want {
			t.Errorf("Expected second outcome preserved %q, got %q", want, buf.String())
		}
	})

	t.Run("MultipleOptionalItemsEachRenderOnTheirOwnLine", func(t *testing.T) {
		var buf strings.Builder
		printDeferredWork(&buf, []workstation.DeferredWorkItem{
			{Required: false, Command: "windsor configure network", Outcome: "use *.local.test in your browser"},
			{Required: false, Command: "windsor configure network --revert", Outcome: "remove host configuration"},
		}, "darwin")
		want := nextSteps + "Run 'windsor configure network' (asks for sudo) to use *.local.test in your browser.\n" +
			"Run 'windsor configure network --revert' (asks for sudo) to remove host configuration.\n"
		if buf.String() != want {
			t.Errorf("Expected %q, got %q", want, buf.String())
		}
	})
}
