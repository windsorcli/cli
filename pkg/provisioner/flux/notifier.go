// Package flux provides Flux kustomization stack management functionality.
// The Notifier piece implements best-effort flux reconcile requests. After a
// successful apply/bootstrap it annotates each of the blueprint's flux sources
// (GitRepository / OCIRepository) with reconcile.fluxcd.io/requestedAt so
// source-controller re-fetches them immediately instead of waiting for the
// next scheduled interval. When the artifact revision changes, kustomize-
// controller reconciles the dependent Kustomizations automatically via its
// watch on source status, so only sources need annotating — not Kustomizations.
// This approach is receiver-type-agnostic, requires no cluster-side secret or
// webhook receiver, and works uniformly against any flux installation.

package flux

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/constants"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes/client"
	"github.com/windsorcli/cli/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// =============================================================================
// Constants
// =============================================================================

const (
	// reconcileAnnotation is the key flux's controllers watch for to trigger an
	// immediate reconciliation. The associated value is any string that differs
	// from the previous value; RFC3339Nano timestamps give a human-readable,
	// monotonically-increasing choice that matches flux's own notification-
	// controller convention when it handles webhook POSTs.
	reconcileAnnotation = "reconcile.fluxcd.io/requestedAt"

	// fieldManager identifies windsor-cli as the writer of the annotation under
	// server-side apply accounting. Keeps kustomize-controller from considering
	// its own field ownership conflicted when it later rewrites other fields.
	fieldManager = "windsor-cli"

	// notifyTimeout bounds the whole annotation fan-out so a stuck API server
	// cannot block install/bootstrap. Ten seconds is generous for a handful of
	// PATCH calls against a healthy cluster; the deadline is shared across all
	// sources, so a slow cluster still makes forward progress on at least some
	// of them before aborting.
	notifyTimeout = 10 * time.Second
)

// =============================================================================
// Interface
// =============================================================================

// Notifier requests an immediate flux reconcile for the blueprint's sources.
// Implementations are best-effort: Notify returns nil whenever the blueprint
// has no remote sources, the cluster is unreachable, or individual PATCH calls
// fail. Hard errors are only returned for programming mistakes (nil blueprint),
// never for cluster state.
type Notifier interface {
	Notify(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error
}

// =============================================================================
// Types
// =============================================================================

// NotifierShims provides mockable wrappers around I/O the Notifier performs
// outside of its KubernetesClient. Only the clock is injectable here — the
// patch call goes through KubernetesClient which already has its own mock.
type NotifierShims struct {
	Now func() time.Time
}

// BaseNotifier implements Notifier by PATCHing the blueprint's flux source
// resources in the configured gitops namespace. Each call constructs its own
// ctx-bounded fan-out; instances should not be shared across goroutines.
type BaseNotifier struct {
	runtime    *runtime.Runtime
	kubeClient client.KubernetesClient
	shims      *NotifierShims
	logWriter  io.Writer
}

// sourceTarget identifies a flux source resource in the cluster: its in-cluster
// name, its Kind (for log output), and the GVR used to PATCH it.
type sourceTarget struct {
	name string
	kind string
	gvr  schema.GroupVersionResource
}

// =============================================================================
// Constructor
// =============================================================================

// NewNotifierShims builds NotifierShims wired to the real clock.
func NewNotifierShims() *NotifierShims {
	return &NotifierShims{Now: time.Now}
}

// NewNotifier creates a BaseNotifier. Panics if runtime or kubeClient are nil.
// Accepts an optional override struct for injecting test shims and a log writer;
// matches the opts-pattern used by NewStack.
func NewNotifier(rt *runtime.Runtime, kubeClient client.KubernetesClient, opts ...*BaseNotifier) Notifier {
	if rt == nil {
		panic("runtime is required")
	}
	if kubeClient == nil {
		panic("kubernetes client is required")
	}

	n := &BaseNotifier{
		runtime:    rt,
		kubeClient: kubeClient,
		shims:      NewNotifierShims(),
	}

	if len(opts) > 0 && opts[0] != nil {
		if opts[0].shims != nil {
			n.shims = opts[0].shims
		}
		if opts[0].logWriter != nil {
			n.logWriter = opts[0].logWriter
		}
	}

	return n
}

// =============================================================================
// Public Methods
// =============================================================================

// Notify annotates every flux source declared by the blueprint with the current
// timestamp under reconcile.fluxcd.io/requestedAt, causing source-controller to
// re-fetch them immediately instead of waiting for the next scheduled interval.
// The blueprint.Repository entry is annotated under blueprint.Metadata.Name;
// each blueprint.Sources entry under its own Name. Local template sources (no
// URL) are skipped. OCI URLs (oci://) route to OCIRepository; every other
// protocol to GitRepository. Per-source PATCH errors are logged and swallowed
// so one unreachable source does not abort the rest. Returns nil for every
// cluster-state condition; returns an error only for nil blueprint.
func (n *BaseNotifier) Notify(ctx context.Context, blueprint *blueprintv1alpha1.Blueprint) error {
	if blueprint == nil {
		return fmt.Errorf("blueprint not provided")
	}

	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()

	targets := collectSourceTargets(blueprint)
	if len(targets) == 0 {
		return nil
	}

	namespace := n.runtime.ConfigHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)
	ts := n.shims.Now().UTC().Format(time.RFC3339Nano)
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, reconcileAnnotation, ts))
	opts := metav1.PatchOptions{FieldManager: fieldManager}

	var notified []string
	for _, t := range targets {
		if err := ctx.Err(); err != nil {
			n.logf("flux reconcile request aborted: %v", err)
			return nil
		}
		if _, err := n.kubeClient.PatchResource(ctx, t.gvr, namespace, t.name, types.MergePatchType, patch, opts); err != nil {
			n.logf("flux reconcile request for %s/%s skipped: %v", t.kind, t.name, err)
			continue
		}
		notified = append(notified, fmt.Sprintf("%s/%s", t.kind, t.name))
	}
	if len(notified) > 0 {
		n.logf("flux reconcile requested in namespace %s: %s", namespace, strings.Join(notified, ", "))
	}
	return nil
}

// =============================================================================
// Private Methods
// =============================================================================

// logf writes a warning to the notifier's log writer if one is configured.
// Using a no-op default keeps Notify quiet in contexts that haven't wired up
// an explicit writer.
func (n *BaseNotifier) logf(format string, args ...any) {
	if n.logWriter == nil {
		return
	}
	fmt.Fprintf(n.logWriter, format+"\n", args...)
}

// =============================================================================
// Helpers
// =============================================================================

// collectSourceTargets enumerates the GitRepository/OCIRepository resources
// ApplyBlueprint creates: the primary blueprint.Repository (named after
// blueprint.Metadata.Name) plus each blueprint.Sources entry except the local
// template source. Returns an empty slice when the blueprint declares no
// remote sources (common for bootstraps that only apply local kustomizations).
func collectSourceTargets(blueprint *blueprintv1alpha1.Blueprint) []sourceTarget {
	var targets []sourceTarget
	if blueprint.Repository.Url != "" {
		targets = append(targets, sourceTargetFromURL(blueprint.Metadata.Name, blueprint.Repository.Url))
	}
	for _, s := range blueprint.Sources {
		if blueprintv1alpha1.IsLocalTemplateSource(s) {
			continue
		}
		if s.Url == "" {
			continue
		}
		targets = append(targets, sourceTargetFromURL(s.Name, s.Url))
	}
	return targets
}

// sourceTargetFromURL maps a blueprint source URL to its flux source GVR. An
// oci:// prefix routes to OCIRepository; everything else (https, ssh, git, or
// bare HTTP) routes to GitRepository, matching applyBlueprintSource's routing
// in kubernetes_manager.go so the annotation target always corresponds to the
// resource actually created on the cluster.
func sourceTargetFromURL(name, url string) sourceTarget {
	if strings.HasPrefix(url, "oci://") {
		return sourceTarget{
			name: name,
			kind: "OCIRepository",
			gvr: schema.GroupVersionResource{
				Group:    "source.toolkit.fluxcd.io",
				Version:  "v1",
				Resource: "ocirepositories",
			},
		}
	}
	return sourceTarget{
		name: name,
		kind: "GitRepository",
		gvr: schema.GroupVersionResource{
			Group:    "source.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "gitrepositories",
		},
	}
}
