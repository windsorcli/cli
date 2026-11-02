package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	cliCmd "github.com/windsorcli/cli/cmd"

	"github.com/windsorcli/cli/pkg/composer"
	"github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/project"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestExplainCmd(t *testing.T) {
	t.Run("RejectsInvalidPath", func(t *testing.T) {
		// Given a project with a mock blueprint handler
		mocks := setupShowTest(t)

		// When explain runs with a path that has no known prefix
		_, err := runExplain(t, mocks, nil, "bogus.path")

		// Then an invalid path error is returned
		if err == nil || !strings.Contains(err.Error(), `invalid path "bogus.path"`) {
			t.Fatalf("Expected an invalid path error, got %v", err)
		}
	})

	t.Run("ReturnsErrorWhenDirectoryIsNotTrusted", func(t *testing.T) {
		// Given a shell that reports an untrusted directory
		mocks := setupShowTest(t)
		mocks.Shell.CheckTrustedDirectoryFunc = func() error { return fmt.Errorf("untrusted") }

		// When explain runs
		_, err := runExplain(t, mocks, nil, "substitutions.domain")

		// Then a trusted directory error is returned
		if err == nil || !strings.Contains(err.Error(), "not in a trusted directory") {
			t.Fatalf("Expected a trusted directory error, got %v", err)
		}
	})

	t.Run("ReturnsExplainError", func(t *testing.T) {
		// Given a blueprint handler that cannot explain the path
		mocks := setupShowTest(t)
		mocks.BlueprintHandler.ExplainFunc = func(string) (*blueprint.ExplainTrace, error) {
			return nil, fmt.Errorf("path not found")
		}

		// When explain runs
		_, err := runExplain(t, mocks, nil, "substitutions.domain")

		// Then the explain error is returned
		if err == nil || !strings.Contains(err.Error(), "path not found") {
			t.Fatalf("Expected the explain error, got %v", err)
		}
	})

	t.Run("PrintsScalarTraceWithEffectiveContributions", func(t *testing.T) {
		// Given a trace with effective and overridden contributions
		mocks := setupShowTest(t)
		trace := &blueprint.ExplainTrace{
			Path:  "substitutions.domain",
			Value: "example.com",
			Contributions: []blueprint.ExplainContribution{
				{AbsFacetPath: "/repo/facets/dns.yaml", Line: 7, Effective: true, ScopeRefs: []blueprint.ExplainScopeRef{
					{Name: "dns.domain", Source: "/repo/facets/config.yaml", Line: 3, Nested: []blueprint.ExplainScopeRef{
						{Name: "dns.zone", Status: "not set"},
					}},
					{Name: "dns.deferred", Status: "deferred"},
					{Name: "dns.loop", Status: "cycle"},
				}},
				{FacetPath: "facets/override.yaml", Effective: true},
				{SourceName: "core", Effective: true},
				{SourceName: "overridden", Effective: false},
			},
		}

		// When explain runs
		out, err := runExplain(t, mocks, trace, "substitutions.domain")

		// Then the value and each effective contribution are printed
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		for _, want := range []string{
			"substitutions.domain = example.com\n",
			"  /repo/facets/dns.yaml:7\n",
			"    dns.domain\n",
			"      /repo/facets/config.yaml:3\n",
			"      dns.zone (not set)\n",
			"    dns.deferred (deferred)\n",
			"    dns.loop (cycle)\n",
			"  facets/override.yaml\n",
			"  core\n",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("Expected output to contain %q, got:\n%s", want, out)
			}
		}
		if strings.Contains(out, "overridden") {
			t.Errorf("Expected overridden contributions to be hidden, got:\n%s", out)
		}
	})

	t.Run("MarksEmptyDeferredAndLongValues", func(t *testing.T) {
		// Given traces with an empty, an expression, and a long value
		cases := map[string]string{
			"":                      "substitutions.domain (empty)\n",
			"${dns.domain}":         "substitutions.domain (deferred)\n",
			strings.Repeat("x", 61): "substitutions.domain\n",
		}
		for value, want := range cases {
			mocks := setupShowTest(t)
			trace := &blueprint.ExplainTrace{Path: "substitutions.domain", Value: value}

			// When explain runs
			out, err := runExplain(t, mocks, trace, "substitutions.domain")

			// Then the path line carries the matching marker
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if !strings.HasPrefix(out, want) {
				t.Errorf("Expected output to start with %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("PrintsEffectiveComponentsWithIndex", func(t *testing.T) {
		// Given a trace for a components list
		mocks := setupShowTest(t)
		trace := &blueprint.ExplainTrace{
			Path: "kustomize.dns.components",
			Contributions: []blueprint.ExplainContribution{
				{Expression: "coredns", SourceName: "core", Effective: true},
				{Expression: "dropped", SourceName: "core", Effective: false},
				{Expression: "external-dns", FacetPath: "facets/dns.yaml", Effective: true},
			},
		}

		// When explain runs
		out, err := runExplain(t, mocks, trace, "kustomize.dns.components")

		// Then each effective component is printed with its index
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		for _, want := range []string{"kustomize.dns.components\n", "  [0] coredns\n", "  [1] external-dns\n"} {
			if !strings.Contains(out, want) {
				t.Errorf("Expected output to contain %q, got:\n%s", want, out)
			}
		}
		if strings.Contains(out, "dropped") {
			t.Errorf("Expected overridden components to be hidden, got:\n%s", out)
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// runExplain runs windsor explain for path against a project built from mocks and returns process stdout.
// A non-nil trace replaces the blueprint handler's Explain result.
func runExplain(t *testing.T, mocks *ShowMocks, trace *blueprint.ExplainTrace, path string) (string, error) {
	t.Helper()
	if trace != nil {
		mocks.BlueprintHandler.ExplainFunc = func(string) (*blueprint.ExplainTrace, error) { return trace, nil }
	}
	comp := composer.NewComposer(mocks.Runtime)
	comp.BlueprintHandler = mocks.BlueprintHandler
	proj := project.NewProject("", &project.Project{Runtime: mocks.Runtime, Composer: comp})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	root := cliCmd.RootCmd()
	root.SetContext(context.WithValue(context.Background(), cliCmd.ProjectOverridesKey, proj))
	root.SetArgs([]string{"explain", path})
	root.SetErr(io.Discard)
	t.Cleanup(func() {
		root.SetContext(context.Background())
		root.SetArgs([]string{})
		root.SetErr(nil)
	})
	execErr := cliCmd.Execute()

	w.Close()
	os.Stdout = origStdout
	return <-done, execErr
}
