package mirror

import (
	"fmt"
	"path/filepath"

	"strings"
	"sync"
	"sync/atomic"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	"github.com/windsorcli/cli/pkg/tui"
	"golang.org/x/sync/errgroup"
)

// The Mirror orchestrates the end-to-end hydration of a local OCI mirror
// registry from the current project's blueprint graph.
// It provides an idempotent Run entrypoint that ensures the distribution/
// distribution container is running, walks the transitive closure of
// `oci://` blueprint sources, and mirrors every reachable blueprint
// artifact, docker image, and helm chart into the local registry.
// Non-OCI manifest entries (github-release, git-tag) are collected for
// reporting rather than failing the operation, since a Docker registry
// cannot host them.

// =============================================================================
// Constants
// =============================================================================

const defaultMirrorConcurrency = 8

// =============================================================================
// Types
// =============================================================================

// Report summarises the outcome of a Mirror.Run invocation. SkippedExisting
// records artifacts bypassed because the destination already held matching
// content; Skipped lists manifest entries that cannot be mirrored through an
// OCI registry at all.
type Report struct {
	Endpoint             string
	MirroredBlueprints   int
	MirroredDockerImages int
	MirroredHelmCharts   int
	SkippedExisting      int
	Skipped              []SkippedEntry
}

// Options customise mirror behaviour. HostPort overrides the default host
// port (5000) used to expose the distribution registry container. Context
// is recorded on the container as a `context` label for ownership tracking.
// Concurrency caps the number of concurrent artifact copies; 0 falls back
// to the package default. Target, when non-empty, points the mirror at an
// existing OCI registry (e.g. "registry.local:5000" or "ghcr.io/me/cache")
// instead of starting a local distribution container; HostPort and Context
// are ignored in that mode.
type Options struct {
	HostPort    int
	Context     string
	Concurrency int
	Target      string
}

// Mirror is the top-level orchestrator exposed by the package.
type Mirror struct {
	runtime       *runtime.Runtime
	shell         shell.Shell
	shims         *Shims
	registry      *Registry
	resolver      *Resolver
	copier        *Copier
	seeds         []string
	localManifest *artifact.ArtifactManifest
	concurrency   int
	resolveSkips  []SkippedEntry
	target        string
	endpoint      string
}

// =============================================================================
// Constructor
// =============================================================================

// NewMirror constructs a Mirror for the supplied runtime, blueprint, and
// optional local manifest. Seeds are derived from the blueprint's `oci://`
// sources; localManifest (when non-nil) contributes the current project's
// own scanned docker and helm entries so base blueprints with no OCI
// sources still hydrate their dependencies.
func NewMirror(rt *runtime.Runtime, bp *blueprintv1alpha1.Blueprint, localManifest *artifact.ArtifactManifest, opts Options) *Mirror {
	if rt == nil {
		panic("runtime is required")
	}
	if rt.Shell == nil {
		panic("shell is required on runtime")
	}

	shims := NewShims()
	m := &Mirror{
		runtime:  rt,
		shell:    rt.Shell,
		shims:    shims,
		resolver: NewResolver(shims),
	}
	if target := normalizeTarget(opts.Target); target != "" {
		m.target = target
		m.endpoint = "https://" + target
		m.copier = NewCopier(shims, target)
	} else {
		cacheDir := filepath.Join(rt.ProjectRoot, ".windsor", "cache", "docker")
		reg := NewRegistry(rt.Shell, shims, cacheDir, opts.HostPort, opts.Context)
		m.registry = reg
		m.endpoint = reg.Endpoint()
		m.copier = NewCopier(shims, fmt.Sprintf("localhost:%d", reg.hostPort))
	}
	m.seeds = extractBlueprintSeeds(bp)
	m.localManifest = localManifest
	m.concurrency = opts.Concurrency
	if m.concurrency <= 0 {
		m.concurrency = defaultMirrorConcurrency
	}
	return m
}

// =============================================================================
// Public Methods
// =============================================================================

// Run performs the full mirror hydration. It starts the local registry if
// necessary, resolves the full artifact graph, then copies every OCI artifact
// and helm chart into the local registry. The returned Report summarises the
// counts of mirrored artifacts and any skipped entries.
func (m *Mirror) Run() (*Report, error) {
	hasLocal := m.localManifest != nil && len(m.localManifest.Artifacts) > 0
	if len(m.seeds) == 0 && !hasLocal {
		return nil, fmt.Errorf("no oci:// sources or local artifacts found — nothing to mirror")
	}

	if m.registry != nil {
		if err := m.ensureCacheDir(); err != nil {
			return nil, err
		}
	}

	var plan *CopyPlan
	if err := tui.WithProgress("Mirroring artifacts", func() error {
		if m.registry != nil {
			tui.Update("starting registry")
			if err := m.registry.EnsureRunning(); err != nil {
				return fmt.Errorf("failed to ensure registry running: %w", err)
			}
		}

		m.resolver.OnStatus = func(s string) { tui.Update(s) }
		var rerr error
		plan, rerr = m.resolver.Resolve(m.seeds)
		if rerr != nil {
			return fmt.Errorf("failed to resolve blueprint graph: %w", rerr)
		}
		m.resolver.IngestManifest(plan, m.localManifest)

		return m.runCopyPlan(plan)
	}); err != nil {
		return nil, err
	}

	return &Report{
		Endpoint:             m.endpoint,
		MirroredBlueprints:   len(plan.Blueprints),
		MirroredDockerImages: len(plan.DockerImages),
		MirroredHelmCharts:   len(plan.HelmHTTPS) + len(plan.HelmOCI),
		SkippedExisting:      int(m.copier.Skipped.Load()),
		Skipped:              append(plan.Skipped, m.resolveSkips...),
	}, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// runCopyPlan executes every copy operation described by plan in parallel,
// bounded by m.concurrency. A tracker of in-flight operations drives progress
// updates so the spinner reflects the oldest still-running artifact — giving
// the user a clear signal when a particular copy is hung instead of masking
// it behind whichever goroutine most recently started.
func (m *Mirror) runCopyPlan(plan *CopyPlan) error {
	total := len(plan.Blueprints) + len(plan.DockerImages) + len(plan.HelmOCI) + len(plan.HelmHTTPS)
	if total == 0 {
		return nil
	}

	var done atomic.Int64
	var mu sync.Mutex
	inFlight := map[int64]string{}
	var seq atomic.Int64

	render := func() {
		mu.Lock()
		defer mu.Unlock()
		oldestID := int64(-1)
		oldestRef := ""
		for id, ref := range inFlight {
			if oldestID == -1 || id < oldestID {
				oldestID = id
				oldestRef = ref
			}
		}
		if oldestRef == "" {
			tui.Update(fmt.Sprintf("[%d/%d] finishing", done.Load(), total))
			return
		}
		tui.Update(fmt.Sprintf("[%d/%d] %d in flight, oldest: %s", done.Load(), total, len(inFlight), oldestRef))
	}

	enter := func(label, ref string) int64 {
		id := seq.Add(1)
		mu.Lock()
		inFlight[id] = label + " " + ref
		mu.Unlock()
		render()
		return id
	}
	leave := func(id int64) {
		mu.Lock()
		delete(inFlight, id)
		mu.Unlock()
		done.Add(1)
		render()
	}

	eg := &errgroup.Group{}
	eg.SetLimit(m.concurrency)

	submit := func(label, ref string, op func() error) {
		eg.Go(func() error {
			id := enter(label, ref)
			defer leave(id)
			if err := op(); err != nil {
				return fmt.Errorf("%s %s: %w", label, ref, err)
			}
			return nil
		})
	}

	for _, ref := range plan.Blueprints {
		r := ref
		submit("blueprint", r, func() error { return m.copier.CopyOCI(r) })
	}
	for _, ref := range plan.DockerImages {
		r := ref
		submit("image", r, func() error { return m.copier.CopyOCI(r) })
	}
	for _, ref := range plan.HelmOCI {
		r := ref
		submit("helm-oci", r, func() error { return m.copier.CopyOCI(r) })
	}
	var softSkipMu sync.Mutex
	for _, h := range plan.HelmHTTPS {
		he := h
		submit("helm", he.ChartName+" "+he.Version, func() error {
			err := m.copier.CopyHelmHTTPS(he)
			if err != nil && strings.Contains(err.Error(), "not found in") {
				softSkipMu.Lock()
				m.resolveSkips = append(m.resolveSkips, SkippedEntry{
					Reference: he.ChartName + " " + he.Version,
					Type:      "helm",
					Reason:    fmt.Sprintf("chart not found in %s (renovate depName likely mismatches chart name)", he.Repository),
				})
				softSkipMu.Unlock()
				return nil
			}
			return err
		})
	}

	return eg.Wait()
}

// ensureCacheDir creates the host-side directory bind-mounted into the
// distribution container so mirrored blobs persist across runs.
func (m *Mirror) ensureCacheDir() error {
	cacheDir := filepath.Join(m.runtime.ProjectRoot, ".windsor", "cache", "docker")
	if _, err := m.shell.ExecSilent("mkdir", "-p", cacheDir); err != nil {
		return fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// normalizeTarget strips any URL scheme and trailing slash from a user-supplied
// target so it can be used directly as the destination authority by Copier.
// Empty input yields empty output, signalling that no external target was set.
func normalizeTarget(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}
	t = strings.TrimPrefix(t, "https://")
	t = strings.TrimPrefix(t, "http://")
	t = strings.TrimPrefix(t, "oci://")
	return strings.TrimRight(t, "/")
}

// extractBlueprintSeeds returns the subset of Blueprint.Sources[].Url entries
// that use the oci:// scheme. These form the roots of the recursive graph
// walk performed by the resolver.
func extractBlueprintSeeds(bp *blueprintv1alpha1.Blueprint) []string {
	if bp == nil {
		return nil
	}
	out := make([]string, 0, len(bp.Sources))
	for _, s := range bp.Sources {
		if canonicalOCI(s.Url) != "" {
			out = append(out, s.Url)
		}
	}
	return out
}
