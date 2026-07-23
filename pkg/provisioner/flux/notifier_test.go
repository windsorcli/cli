package flux

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/shell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Test Setup
// =============================================================================

// patchCall captures the arguments the test's mock KubernetesClient saw, so
// assertions can verify both the targeted GVR and the patch payload.
type patchCall struct {
	gvr       schema.GroupVersionResource
	namespace string
	name      string
	patchType types.PatchType
	data      []byte
}

// notifierMocks bundles the dependencies a test needs to drive BaseNotifier
// through Notify without a real cluster. kubeClient captures PATCH calls;
// patches receives each call in order so tests can assert fan-out ordering.
type notifierMocks struct {
	runtime    *runtime.Runtime
	kubeClient *client.MockKubernetesClient
	shims      *NotifierShims
	logs       *bytes.Buffer
	patches    *[]patchCall
}

// setupNotifierMocks builds a notifierMocks with a mock kube client, a real
// runtime pointing at mock config + shell, deterministic time via shims, and
// a capturing log writer so tests can assert on warnings.
func setupNotifierMocks(t *testing.T) *notifierMocks {
	t.Helper()

	rt := runtime.NewRuntime(&runtime.Runtime{
		ConfigHandler: config.NewMockConfigHandler(),
		Shell:         shell.NewMockShell(),
	})

	kc := client.NewMockKubernetesClient()
	patches := &[]patchCall{}
	kc.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
		*patches = append(*patches, patchCall{gvr: gvr, namespace: ns, name: name, patchType: pt, data: append([]byte(nil), data...)})
		return &unstructured.Unstructured{}, nil
	}

	return &notifierMocks{
		runtime:    rt,
		kubeClient: kc,
		shims:      &NotifierShims{Now: func() time.Time { return time.Date(2026, 4, 19, 20, 0, 0, 0, time.UTC) }},
		logs:       &bytes.Buffer{},
		patches:    patches,
	}
}

// newTestNotifier builds a BaseNotifier wired to the mocks' shims, kube
// client, and log writer.
func newTestNotifier(m *notifierMocks) *BaseNotifier {
	return NewNotifier(m.runtime, m.kubeClient, &BaseNotifier{
		shims:     m.shims,
		logWriter: m.logs,
	}).(*BaseNotifier)
}

// testNotifyBlueprint builds a blueprint with one primary Repository (git) and
// one additional Sources entry (oci) so tests can cover both routing paths.
func testNotifyBlueprint() *blueprintv1alpha1.Blueprint {
	return &blueprintv1alpha1.Blueprint{
		Metadata: blueprintv1alpha1.Metadata{Name: "core"},
		Repository: blueprintv1alpha1.Repository{
			Url: "https://git.test/git/core",
		},
		Sources: []blueprintv1alpha1.Source{
			{Name: "addon", Url: "oci://registry.test/addon"},
		},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

func TestNotifier_Notify(t *testing.T) {
	ctx := context.Background()

	t.Run("AnnotatesAllRemoteSources", func(t *testing.T) {
		// Given a blueprint with a git Repository and an oci Source
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then one PATCH is issued per remote source, routed to the right GVR
		if len(*m.patches) != 2 {
			t.Fatalf("expected 2 patches, got %d", len(*m.patches))
		}
		first, second := (*m.patches)[0], (*m.patches)[1]
		if first.gvr.Resource != "gitrepositories" {
			t.Errorf("expected first patch to target gitrepositories, got %s", first.gvr.Resource)
		}
		if first.name != "core" {
			t.Errorf("expected first patch to target 'core', got %s", first.name)
		}
		if second.gvr.Resource != "ocirepositories" {
			t.Errorf("expected second patch to target ocirepositories, got %s", second.gvr.Resource)
		}
		if second.name != "addon" {
			t.Errorf("expected second patch to target 'addon', got %s", second.name)
		}
		for _, p := range *m.patches {
			if p.patchType != types.MergePatchType {
				t.Errorf("expected MergePatchType, got %v", p.patchType)
			}
			if !strings.Contains(string(p.data), "reconcile.fluxcd.io/requestedAt") {
				t.Errorf("expected patch payload to contain reconcile annotation, got %s", p.data)
			}
			if !strings.Contains(string(p.data), "2026-04-19T20:00:00Z") {
				t.Errorf("expected patch payload to contain injected timestamp, got %s", p.data)
			}
		}
	})

	t.Run("UsesGitopsNamespaceFromConfig", func(t *testing.T) {
		// Given a config handler that returns a non-default gitops namespace
		m := setupNotifierMocks(t)
		mockCH := m.runtime.ConfigHandler.(*config.MockConfigHandler)
		mockCH.GetStringFunc = func(key string, defaultValue ...string) string {
			if key == "gitops.namespace" {
				return "flux-system"
			}
			if len(defaultValue) > 0 {
				return defaultValue[0]
			}
			return ""
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the patch is issued in the configured namespace, not the default
		if len(*m.patches) == 0 {
			t.Fatal("expected at least one patch")
		}
		if (*m.patches)[0].namespace != "flux-system" {
			t.Errorf("expected namespace flux-system, got %s", (*m.patches)[0].namespace)
		}
	})

	t.Run("SkipsLocalTemplateSource", func(t *testing.T) {
		// Given a blueprint whose only source is the local template (no URL)
		m := setupNotifierMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata: blueprintv1alpha1.Metadata{Name: "core"},
			Sources: []blueprintv1alpha1.Source{
				{Name: "template"},
			},
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, bp); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then no patch is issued and no warning is logged
		if len(*m.patches) != 0 {
			t.Errorf("expected no patches, got %d", len(*m.patches))
		}
		if m.logs.Len() != 0 {
			t.Errorf("expected silent no-op, got logs: %q", m.logs.String())
		}
	})

	t.Run("NoSourcesIsSilentNoOp", func(t *testing.T) {
		// Given a blueprint with no Repository and no Sources
		m := setupNotifierMocks(t)
		bp := &blueprintv1alpha1.Blueprint{Metadata: blueprintv1alpha1.Metadata{Name: "core"}}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, bp); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then no patch is issued
		if len(*m.patches) != 0 {
			t.Errorf("expected no patches, got %d", len(*m.patches))
		}
	})

	t.Run("PatchFailureOnOneSourceDoesNotAbortOthers", func(t *testing.T) {
		// Given a kube client where the first PATCH fails and the second succeeds
		m := setupNotifierMocks(t)
		var calls int
		m.kubeClient.PatchResourceFunc = func(ctx context.Context, gvr schema.GroupVersionResource, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			calls++
			*m.patches = append(*m.patches, patchCall{gvr: gvr, namespace: ns, name: name, patchType: pt, data: append([]byte(nil), data...)})
			if calls == 1 {
				return nil, fmt.Errorf("boom")
			}
			return &unstructured.Unstructured{}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then both sources were attempted and a warning mentions the failed one
		if calls != 2 {
			t.Errorf("expected 2 patch attempts, got %d", calls)
		}
		if !strings.Contains(m.logs.String(), "GitRepository/core") {
			t.Errorf("expected warning to identify failed source, got %q", m.logs.String())
		}
	})

	t.Run("NilBlueprintReturnsError", func(t *testing.T) {
		// Given a notifier
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When Notify is called with nil
		err := n.Notify(ctx, nil)

		// Then an error is returned (nil blueprint is a programmer mistake, not cluster state)
		if err == nil {
			t.Fatal("expected error for nil blueprint")
		}
	})

	t.Run("CancelledContextShortCircuits", func(t *testing.T) {
		// Given an already-cancelled context
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		// When Notify is called
		if err := n.Notify(cancelled, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil (best-effort), got %v", err)
		}

		// Then no patches are issued and a warning mentions the abort
		if len(*m.patches) != 0 {
			t.Errorf("expected no patches on cancelled ctx, got %d", len(*m.patches))
		}
		if !strings.Contains(m.logs.String(), "aborted") {
			t.Errorf("expected abort warning, got %q", m.logs.String())
		}
	})

	t.Run("PatchCallsReceiveNotifyScopedContext", func(t *testing.T) {
		// Given a kube client that records the ctx it sees on each PATCH
		m := setupNotifierMocks(t)
		var seenCtxs []context.Context
		m.kubeClient.PatchResourceFunc = func(patchCtx context.Context, gvr schema.GroupVersionResource, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
			seenCtxs = append(seenCtxs, patchCtx)
			return &unstructured.Unstructured{}, nil
		}
		n := newTestNotifier(m)

		// When Notify is called with a parent ctx
		parent, parentCancel := context.WithCancel(ctx)
		defer parentCancel()
		if err := n.Notify(parent, testNotifyBlueprint()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then each PATCH receives a ctx that is derived from the parent (so
		// parent cancellation propagates into an in-flight PATCH) — verified by
		// cancelling the parent after the fact and confirming every captured
		// ctx observes the cancellation. Without ctx threading this would fail
		// because PatchResource would have built its own Background-derived ctx.
		if len(seenCtxs) != 2 {
			t.Fatalf("expected 2 patch contexts captured, got %d", len(seenCtxs))
		}
		parentCancel()
		for i, patchCtx := range seenCtxs {
			if err := patchCtx.Err(); err == nil {
				t.Errorf("patch ctx %d did not observe parent cancellation; ctx threading broken", i)
			}
		}
	})

	t.Run("OnlyPrimaryRepositoryAnnotatedWhenNoAdditionalSources", func(t *testing.T) {
		// Given a blueprint with only a git Repository (no Sources)
		m := setupNotifierMocks(t)
		bp := &blueprintv1alpha1.Blueprint{
			Metadata:   blueprintv1alpha1.Metadata{Name: "core"},
			Repository: blueprintv1alpha1.Repository{Url: "https://git.test/git/core"},
		}
		n := newTestNotifier(m)

		// When Notify is called
		if err := n.Notify(ctx, bp); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then exactly one GitRepository PATCH is issued
		if len(*m.patches) != 1 {
			t.Fatalf("expected 1 patch, got %d", len(*m.patches))
		}
		if (*m.patches)[0].gvr.Resource != "gitrepositories" {
			t.Errorf("expected gitrepositories, got %s", (*m.patches)[0].gvr.Resource)
		}
	})
}

func TestNotifier_ReconcileKustomizations(t *testing.T) {
	ctx := context.Background()

	t.Run("AnnotatesEachNamedKustomization", func(t *testing.T) {
		// Given a set of kustomization names
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When ReconcileKustomizations is called
		if err := n.ReconcileKustomizations(ctx, []string{"lb-install", "gateway-install"}); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then one PATCH per name targets the kustomizations GVR carrying the reconcile annotation
		if len(*m.patches) != 2 {
			t.Fatalf("expected 2 patches, got %d", len(*m.patches))
		}
		for _, p := range *m.patches {
			if p.gvr.Resource != "kustomizations" || p.gvr.Group != "kustomize.toolkit.fluxcd.io" {
				t.Errorf("expected kustomizations GVR, got %s/%s", p.gvr.Group, p.gvr.Resource)
			}
			if !strings.Contains(string(p.data), "reconcile.fluxcd.io/requestedAt") || !strings.Contains(string(p.data), "2026-04-19T20:00:00Z") {
				t.Errorf("expected reconcile annotation with injected timestamp, got %s", p.data)
			}
		}
		if (*m.patches)[0].name != "lb-install" || (*m.patches)[1].name != "gateway-install" {
			t.Errorf("expected the named kustomizations patched, got %s, %s", (*m.patches)[0].name, (*m.patches)[1].name)
		}
	})

	t.Run("EmptyNamesIsNoOp", func(t *testing.T) {
		// Given no names
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When ReconcileKustomizations is called with an empty slice, nothing is patched
		if err := n.ReconcileKustomizations(ctx, nil); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if len(*m.patches) != 0 {
			t.Errorf("expected no patches, got %d", len(*m.patches))
		}
	})
}

func TestNotifier_ReconcileHelmReleases(t *testing.T) {
	ctx := context.Background()

	t.Run("ForceAddsForceAtAnnotation", func(t *testing.T) {
		// Given a stalled HelmRelease reference
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When ReconcileHelmReleases is called with force
		if err := n.ReconcileHelmReleases(ctx, []HelmReleaseRef{{Namespace: "system-lb", Name: "hcloud-cloud-controller-manager"}}, true); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then the HelmRelease GVR is patched in its own namespace with both requestedAt and forceAt
		if len(*m.patches) != 1 {
			t.Fatalf("expected 1 patch, got %d", len(*m.patches))
		}
		p := (*m.patches)[0]
		if p.gvr.Resource != "helmreleases" || p.gvr.Group != "helm.toolkit.fluxcd.io" {
			t.Errorf("expected helmreleases GVR, got %s/%s", p.gvr.Group, p.gvr.Resource)
		}
		if p.namespace != "system-lb" || p.name != "hcloud-cloud-controller-manager" {
			t.Errorf("expected the HR patched in system-lb, got %s/%s", p.namespace, p.name)
		}
		if !strings.Contains(string(p.data), "reconcile.fluxcd.io/requestedAt") || !strings.Contains(string(p.data), "reconcile.fluxcd.io/forceAt") {
			t.Errorf("expected both requestedAt and forceAt, got %s", p.data)
		}
	})

	t.Run("WithoutForceOmitsForceAt", func(t *testing.T) {
		// Given a HelmRelease reference reconciled without force
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)

		// When ReconcileHelmReleases is called without force
		if err := n.ReconcileHelmReleases(ctx, []HelmReleaseRef{{Namespace: "system-lb", Name: "hr"}}, false); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		// Then only requestedAt is set, not forceAt
		p := (*m.patches)[0]
		if !strings.Contains(string(p.data), "reconcile.fluxcd.io/requestedAt") || strings.Contains(string(p.data), "forceAt") {
			t.Errorf("expected requestedAt without forceAt, got %s", p.data)
		}
	})

	t.Run("EmptyRefsIsNoOp", func(t *testing.T) {
		m := setupNotifierMocks(t)
		n := newTestNotifier(m)
		if err := n.ReconcileHelmReleases(ctx, nil, true); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if len(*m.patches) != 0 {
			t.Errorf("expected no patches, got %d", len(*m.patches))
		}
	})
}

func TestNewNotifier(t *testing.T) {
	t.Run("PanicsOnNilRuntime", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on nil runtime")
			}
		}()
		NewNotifier(nil, client.NewMockKubernetesClient())
	})

	t.Run("PanicsOnNilKubeClient", func(t *testing.T) {
		rt := runtime.NewRuntime(&runtime.Runtime{
			ConfigHandler: config.NewMockConfigHandler(),
			Shell:         shell.NewMockShell(),
		})
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on nil kubeClient")
			}
		}()
		NewNotifier(rt, nil)
	})
}
