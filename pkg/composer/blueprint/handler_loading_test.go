package blueprint

import (
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
)

// =============================================================================
// Test Helpers
// =============================================================================

func TestOrderLoadersByDependency(t *testing.T) {
	// newLoader builds a mock loader named `name` whose blueprint lists `deps` as sources.
	newLoader := func(name string, deps ...string) BlueprintLoader {
		sources := make([]blueprintv1alpha1.Source, 0, len(deps))
		for _, d := range deps {
			sources = append(sources, blueprintv1alpha1.Source{Name: d})
		}
		return &mockLoaderImpl{
			getSourceNameFunc: func() string { return name },
			getBlueprintFunc: func() *blueprintv1alpha1.Blueprint {
				return &blueprintv1alpha1.Blueprint{Sources: sources}
			},
		}
	}
	order := func(loaders []BlueprintLoader) []string {
		names := make(map[BlueprintLoader]string, len(loaders))
		for _, l := range loaders {
			names[l] = l.GetSourceName()
		}
		result := orderLoadersByDependency(loaders, names, "user")
		out := make([]string, len(result))
		for i, l := range result {
			out[i] = l.GetSourceName()
		}
		return out
	}

	t.Run("DependencyBeforeDependent", func(t *testing.T) {
		// Given a chain where user lists core+consumer and consumer lists core
		core := newLoader("core")
		consumer := newLoader("consumer", "core")
		user := newLoader("user", "core", "consumer")

		// When ordering (input deliberately reversed to prove sorting, not input order)
		got := order([]BlueprintLoader{user, consumer, core})

		// Then dependencies come before dependents and user is last
		want := []string{"core", "consumer", "user"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("Expected %v, got %v", want, got)
		}
	})

	t.Run("IndependentSourcesAreDeterministicWithUserLast", func(t *testing.T) {
		// Given independent sources and a user loader with no sources
		got := order([]BlueprintLoader{newLoader("zeta"), newLoader("user"), newLoader("alpha")})

		// Then independents sort alphabetically and user is forced last
		want := []string{"alpha", "zeta", "user"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("Expected %v, got %v", want, got)
		}
	})

	t.Run("ForcesUserLoaderLastByGivenName", func(t *testing.T) {
		// Given a user loader whose name is not the literal "user"
		loaders := []BlueprintLoader{newLoader("zeta"), newLoader("overlay"), newLoader("alpha")}
		names := make(map[BlueprintLoader]string, len(loaders))
		for _, l := range loaders {
			names[l] = l.GetSourceName()
		}

		// When ordering with "overlay" declared as the user loader name
		result := orderLoadersByDependency(loaders, names, "overlay")
		got := make([]string, len(result))
		for i, l := range result {
			got[i] = l.GetSourceName()
		}

		// Then the named loader is forced last regardless of alphabetical order
		want := []string{"alpha", "zeta", "overlay"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("Expected %v, got %v", want, got)
		}
	})

	t.Run("CycleFallsBackToStableOrderWithoutDroppingLoaders", func(t *testing.T) {
		// Given a dependency cycle a->b->a
		a := newLoader("a", "b")
		b := newLoader("b", "a")

		// When ordering
		got := order([]BlueprintLoader{b, a})

		// Then no loader is dropped and the fallback order is stable
		want := []string{"a", "b"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("Expected %v (no loaders dropped), got %v", want, got)
		}
	})

	t.Run("IgnoresEdgesToUnknownSources", func(t *testing.T) {
		// Given a source that lists a dependency not present among the loaders
		got := order([]BlueprintLoader{newLoader("only", "missing")})

		// Then the unknown edge is ignored and the loader still appears
		if strings.Join(got, ",") != "only" {
			t.Errorf("Expected [only], got %v", got)
		}
	})
}
