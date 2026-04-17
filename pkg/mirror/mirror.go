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
	"github.com/windsorcli/cli/pkg/tui"
	"golang.org/x/sync/errgroup"
)

// The Mirror orchestrates the end-to-end hydration of a Talos-compatible
// image cache or an external OCI registry from the current project's blueprint
// graph. It resolves the transitive closure of oci:// blueprint sources and
// mirrors every reachable blueprint artifact, docker image, and helm chart.
// Non-OCI manifest entries (github-release, git-tag) are collected for
// reporting rather than failing the operation.

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

// Options customise mirror behaviour. Concurrency caps the number of concurrent
// artifact copies; 0 falls back to the package default. Target, when non-empty,
// pushes artifacts to an existing OCI registry instead of writing to disk.
type Options struct {
	Concurrency int
	Target      string
}

// Mirror is the top-level orchestrator exposed by the package.
type Mirror struct {
	runtime      *runtime.Runtime
	shims        *Shims
	resolver     *Resolver
	copier       *Copier
	cacheWriter  *CacheWriter
	seeds        []string
	localManifest *artifact.ArtifactManifest
	concurrency  int
	resolveSkips []SkippedEntry
	target       string
	cacheDir     string
}

// =============================================================================
// Constructor
// =============================================================================

// NewMirror constructs a Mirror for the supplied runtime, blueprint, and
// optional local manifest. Seeds are derived from the blueprint's `oci://`
// sources; localManifest (when non-nil) contributes the current project's
// own scanned docker and helm entries so base blueprints with no OCI
// sources still hydrate their dependencies.
//
// When Target is set, artifacts are pushed to an external registry via Copier.
// Otherwise, artifacts are written to a Talos-compatible image cache directory
// at .windsor/cache/image-cache.
func NewMirror(rt *runtime.Runtime, bp *blueprintv1alpha1.Blueprint, localManifest *artifact.ArtifactManifest, opts Options) *Mirror {
	if rt == nil {
		panic("runtime is required")
	}

	shims := NewShims()
	m := &Mirror{
		runtime:  rt,
		shims:    shims,
		resolver: NewResolver(shims),
	}
	if target := normalizeTarget(opts.Target); target != "" {
		m.target = target
		m.copier = NewCopier(shims, target)
	} else {
		m.cacheDir = filepath.Join(rt.ProjectRoot, ".windsor", "cache", "image-cache")
		m.cacheWriter = NewCacheWriter(shims, m.cacheDir)
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

// Run performs the full mirror operation. It resolves the blueprint artifact
// graph, then either writes to a Talos image cache directory or pushes to an
// external registry. The returned Report summarises artifact counts and skips.
func (m *Mirror) Run() (*Report, error) {
	hasLocal := m.localManifest != nil && len(m.localManifest.Artifacts) > 0
	if len(m.seeds) == 0 && !hasLocal {
		return nil, fmt.Errorf("no oci:// sources or local artifacts found — nothing to mirror")
	}

	var plan *CopyPlan
	if err := tui.WithProgress("Mirroring artifacts", func() error {
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

	skippedExisting := int64(0)
	endpoint := m.cacheDir
	if m.copier != nil {
		skippedExisting = m.copier.Skipped.Load()
		endpoint = "https://" + m.target
	} else if m.cacheWriter != nil {
		skippedExisting = m.cacheWriter.Skipped.Load()
	}

	return &Report{
		Endpoint:             endpoint,
		MirroredBlueprints:   len(plan.Blueprints),
		MirroredDockerImages: len(plan.DockerImages),
		MirroredHelmCharts:   len(plan.HelmHTTPS) + len(plan.HelmOCI),
		SkippedExisting:      int(skippedExisting),
		Skipped:              append(plan.Skipped, m.resolveSkips...),
	}, nil
}

// Resolve returns the resolved copy plan without executing any copies.
// Used by --list mode to output image refs.
func (m *Mirror) Resolve() (*CopyPlan, error) {
	hasLocal := m.localManifest != nil && len(m.localManifest.Artifacts) > 0
	if len(m.seeds) == 0 && !hasLocal {
		return nil, fmt.Errorf("no oci:// sources or local artifacts found — nothing to mirror")
	}
	plan, err := m.resolver.Resolve(m.seeds)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve blueprint graph: %w", err)
	}
	m.resolver.IngestManifest(plan, m.localManifest)
	return plan, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// runCopyPlan executes every copy operation described by plan in parallel,
// bounded by m.concurrency. Progress updates show the most recently completed
// artifact.
func (m *Mirror) runCopyPlan(plan *CopyPlan) error {
	total := len(plan.Blueprints) + len(plan.DockerImages) + len(plan.HelmOCI) + len(plan.HelmHTTPS)
	if total == 0 {
		return nil
	}

	var done atomic.Int64
	var lastCompleted atomic.Value

	render := func() {
		d := done.Load()
		if last, ok := lastCompleted.Load().(string); ok && last != "" {
			tui.Update(fmt.Sprintf("[%d/%d] %s", d, total, last))
		} else {
			tui.Update(fmt.Sprintf("[%d/%d]", d, total))
		}
	}

	enter := func(_, _ string) int64 {
		render()
		return 0
	}
	leave := func(_ int64, label, ref string) {
		done.Add(1)
		lastCompleted.Store(label + " " + ref)
		render()
	}

	eg := &errgroup.Group{}
	eg.SetLimit(m.concurrency)

	submit := func(label, ref string, op func() error) {
		eg.Go(func() error {
			id := enter(label, ref)
			defer leave(id, label, ref)
			if err := op(); err != nil {
				return fmt.Errorf("%s %s: %w", label, ref, err)
			}
			return nil
		})
	}

	// Dispatch to either copier (--target) or cacheWriter (default)
	if m.copier != nil {
		m.submitCopierOps(submit, plan)
	} else {
		m.submitCacheOps(submit, plan)
	}

	return eg.Wait()
}

// submitCopierOps dispatches copy operations to the Copier (--target mode).
func (m *Mirror) submitCopierOps(submit func(string, string, func() error), plan *CopyPlan) {
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
		submit("helm-oci", r, func() error { return m.copier.CopyHelmOCI(r) })
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
}

// submitCacheOps dispatches write operations to the CacheWriter (default mode).
func (m *Mirror) submitCacheOps(submit func(string, string, func() error), plan *CopyPlan) {
	for _, ref := range plan.Blueprints {
		r := ref
		submit("blueprint", r, func() error { return m.cacheWriter.WriteOCI(r) })
	}
	for _, ref := range plan.DockerImages {
		r := ref
		submit("image", r, func() error { return m.cacheWriter.WriteOCI(r) })
	}
	for _, ref := range plan.HelmOCI {
		r := ref
		submit("helm-oci", r, func() error { return m.cacheWriter.WriteHelmOCI(r) })
	}
	var softSkipMu sync.Mutex
	for _, h := range plan.HelmHTTPS {
		he := h
		submit("helm", he.ChartName+" "+he.Version, func() error {
			err := m.cacheWriter.WriteHelmHTTPS(he)
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
