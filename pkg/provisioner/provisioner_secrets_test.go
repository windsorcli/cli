package provisioner_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/windsorcli/cli/pkg/provisioner"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/presenter"
	"github.com/windsorcli/cli/internal/werror"
	fluxinfra "github.com/windsorcli/cli/pkg/provisioner/flux"
	"github.com/windsorcli/cli/pkg/provisioner/kubernetes"
	"github.com/windsorcli/cli/pkg/runtime/config"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestProvisioner_ResolveSecrets(t *testing.T) {
	newProvisioner := func(mocks *ProvisionerTestMocks) *provisioner.Provisioner {
		return provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
			ClusterClient:     mocks.ClusterClient,
			FluxStack:         mocks.FluxStack,
		})
	}
	withValues := func(mocks *ProvisionerTestMocks, values map[string]any) {
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) { return values, nil }
	}
	bp := func(secrets map[string]blueprintv1alpha1.SecretEntry) *blueprintv1alpha1.Blueprint {
		return &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "cdn-install", Secrets: secrets}}}
	}
	entry := func(data map[string]string) blueprintv1alpha1.SecretEntry {
		return blueprintv1alpha1.SecretEntry{Data: data}
	}

	t.Run("ResolvesValueKeyedByKustomization", func(t *testing.T) {
		// Given a resolvable reference
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{"cdn": map[string]any{"cloudflare_api_key": "resolved-token"}})

		// When resolving secrets
		resolved, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{"cloudflare-creds": entry(map[string]string{"api_token": "${cdn.cloudflare_api_key}"})}))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the value is keyed by owning kustomization -> secret -> data key
		if resolved["cdn-install"]["cloudflare-creds"].Data["api_token"] != "resolved-token" {
			t.Errorf("Expected resolved token keyed by kustomization, got %v", resolved)
		}
	})

	t.Run("DereferencesComputedConfigBlockFromComposedScope", func(t *testing.T) {
		// Given a computed config block present only in the composed scope
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{})
		mocks.Runtime.Evaluator.SetConfigScope(map[string]any{
			"grafana_effective": map[string]any{"client_secret": "grafana-oidc-dev"},
		})

		// When resolving a secret whose value dereferences the computed block
		resolved, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{
			"sso": entry(map[string]string{"clientSecret": "${grafana_effective.client_secret}"}),
		}))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the secret resolves against the composed scope, consistent with substitutions
		if resolved["cdn-install"]["sso"].Data["clientSecret"] != "grafana-oidc-dev" {
			t.Errorf("Expected secret to dereference the computed config block, got %v", resolved)
		}
	})

	t.Run("CarriesTheEntryType", func(t *testing.T) {
		// Given a secret entry with an explicit type
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{"gw": map[string]any{"cert": "CERT", "key": "KEY"}})
		tls := entry(map[string]string{"tls.crt": "${gw.cert}", "tls.key": "${gw.key}"})
		tls.Type = "kubernetes.io/tls"

		// When resolving secrets
		resolved, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{"web-tls": tls}))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the resolved secret keeps the type for placement
		if got := resolved["cdn-install"]["web-tls"].Type; got != "kubernetes.io/tls" {
			t.Errorf("Expected type kubernetes.io/tls, got %q", got)
		}
	})

	t.Run("RegistersResolvedValueWithScrubber", func(t *testing.T) {
		// Given a resolvable secret reference and a shell that records scrubber registrations
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{"cdn": map[string]any{"cloudflare_api_key": "resolved-token"}})
		var registered []string
		mocks.Shell.RegisterSecretFunc = func(v string) { registered = append(registered, v) }

		// When resolving secrets
		_, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{"cloudflare-creds": entry(map[string]string{"api_token": "${cdn.cloudflare_api_key}"})}))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the resolved value is registered with the shell scrubber
		found := false
		for _, v := range registered {
			if v == "resolved-token" {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected resolved secret value registered with scrubber, got %v", registered)
		}
	})

	t.Run("ResolvesSecretFromEnvWhenConfigUnsetAndScrubsIt", func(t *testing.T) {
		// Given a secret whose unset config value falls back to env()
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{})
		t.Setenv("HETZNER_TOKEN", "env-token")
		var registered []string
		mocks.Shell.RegisterSecretFunc = func(v string) { registered = append(registered, v) }

		// When resolving secrets
		resolved, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{
			"hcloud": entry(map[string]string{"token": `${hetzner.token ?? env("HETZNER_TOKEN")}`}),
		}))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the token is sourced from the environment and masked in output
		if got := resolved["cdn-install"]["hcloud"].Data["token"]; got != "env-token" {
			t.Errorf("Expected token sourced from env, got %q", got)
		}
		found := false
		for _, v := range registered {
			if v == "env-token" {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected env-sourced secret registered with scrubber, got %v", registered)
		}
	})

	t.Run("FailsWhenRequiredReferenceResolvesToNil", func(t *testing.T) {
		// Given a required reference to a value absent from configuration
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{})

		// When resolving secrets
		_, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{"creds": entry(map[string]string{"token": "${hetzner.token}"})}))
		// Then resolution fails with a remediation that names ?? default
		if err == nil || !strings.Contains(err.Error(), "resolved to empty") {
			t.Errorf("Expected nil-required error, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "?? default") {
			t.Errorf("Expected a remediation that names ?? default, got: %v", err)
		}
	})

	t.Run("OmitsNilOptionalKeyAndDropsWhollyEmptySecret", func(t *testing.T) {
		// Given an optional reference (?? default) to a value absent from configuration
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{})

		// When resolving secrets
		resolved, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{"creds": entry(map[string]string{"token": "${hetzner.token ?? ''}"})}))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the optional key is omitted and the empty secret is dropped
		if len(resolved) != 0 {
			t.Errorf("Expected no secret from nil-resolving optional reference, got %v", resolved)
		}
	})

	t.Run("FailsWhenRequiredReferenceResolvesEmpty", func(t *testing.T) {
		// Given a required reference whose value is empty
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{"cdn": map[string]any{"token": ""}})

		// When resolving secrets
		_, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{"creds": entry(map[string]string{"token": "${cdn.token}"})}))
		// Then resolution fails closed
		if err == nil || !strings.Contains(err.Error(), "resolved to empty") {
			t.Errorf("Expected empty-required error, got %v", err)
		}
	})

	t.Run("OmitsEmptyOptionalKeyAndDropsWhollyEmptySecret", func(t *testing.T) {
		// Given an optional empty key alongside a required one, and a wholly-empty optional secret
		mocks := setupProvisionerMocks(t)
		withValues(mocks, map[string]any{
			"cdn":       map[string]any{"token": "real", "extra": ""},
			"telemetry": map[string]any{"alerts": map[string]any{"slack": map[string]any{"webhook_url": nil}}},
		})

		// When resolving
		resolved, err := newProvisioner(mocks).ResolveSecrets(bp(map[string]blueprintv1alpha1.SecretEntry{
			"creds":            entry(map[string]string{"token": "${cdn.token}", "extra": "${cdn.extra ?? ''}"}),
			"optional-webhook": entry(map[string]string{"url": "${telemetry.alerts.slack.webhook_url ?? ''}"}),
		}))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the optional empty key is omitted and the wholly-empty optional secret is dropped
		creds := resolved["cdn-install"]["creds"].Data
		if creds["token"] != "real" {
			t.Errorf("Expected token resolved, got %v", creds)
		}
		if _, exists := creds["extra"]; exists {
			t.Errorf("Expected empty optional key omitted, got %v", creds)
		}
		if _, exists := resolved["cdn-install"]["optional-webhook"]; exists {
			t.Errorf("Expected wholly-empty secret dropped, got %v", resolved["cdn-install"])
		}
	})

	t.Run("NoSecretsReturnsEmpty", func(t *testing.T) {
		// Given a kustomization that declares no secrets
		mocks := setupProvisionerMocks(t)
		// When resolving secrets
		resolved, err := newProvisioner(mocks).ResolveSecrets(&blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "plain"}}})
		// Then the resolved set is empty
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(resolved) != 0 {
			t.Errorf("Expected empty resolved secrets, got %v", resolved)
		}
	})
}

func TestProvisioner_PlaceSecrets(t *testing.T) {
	newProvisioner := func(mocks *ProvisionerTestMocks) *provisioner.Provisioner {
		return provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{
			TerraformStack:    mocks.TerraformStack,
			KubernetesManager: mocks.KubernetesManager,
			KubernetesClient:  mocks.KubernetesClient,
			ClusterClient:     mocks.ClusterClient,
			FluxStack:         mocks.FluxStack,
		})
	}
	resolved := func() provisioner.ResolvedSecrets {
		return provisioner.ResolvedSecrets{"cdn-install": {"cloudflare-creds": {Data: map[string]string{"api_token": "resolved-token"}}}}
	}

	t.Run("PlacesIntoInventoryNamespace", func(t *testing.T) {
		// Given a kustomization whose inventory holds exactly one applied namespace
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-dns"}}, nil
		}
		var gotName, gotNs, gotOwner string
		var gotData map[string]string
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			gotName, gotNs, gotData, gotOwner = name, namespace, stringData, owner
			return nil
		}

		// When placing the resolved secrets
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the secret lands in the inventory namespace with the resolved value, owned by its kustomization
		if gotName != "cloudflare-creds" || gotNs != "system-dns" || gotData["api_token"] != "resolved-token" || gotOwner != "cdn-install" {
			t.Errorf("Unexpected ApplySecret args: name=%q ns=%q data=%v owner=%q", gotName, gotNs, gotData, gotOwner)
		}
	})

	t.Run("EndsReadyLayersWhileASecretStillWaits", func(t *testing.T) {
		// Given a ready kustomization and a secret whose namespace appears only on the second check
		mocks := setupProvisionerMocks(t)
		mocks.ConfigHandler.(*config.MockConfigHandler).GetContextValuesFunc = func() (map[string]any, error) {
			return map[string]any{"cdn": map[string]any{"cloudflare_api_key": "resolved-token"}}, nil
		}
		var inventoryReads int
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			inventoryReads++
			if inventoryReads < 2 {
				return nil, nil
			}
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-dns"}}, nil
		}
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			return map[string]bool{"app": true, "cdn-install": false}, nil
		}
		prov := newProvisioner(mocks)
		prov.PollInterval = time.Millisecond
		prov.Notifier = fluxinfra.NewMockNotifier()
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "app"},
			{Name: "cdn-install", Secrets: map[string]blueprintv1alpha1.SecretEntry{
				"cloudflare-creds": {Data: map[string]string{"api_token": "${cdn.cloudflare_api_key}"}},
			}},
		}}
		rec := &eventRecorder{}

		// When Install places the secret
		if err := prov.Install(presenter.NewContext(context.Background(), rec), bp, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the ready layer ends while the secret waits, before the secret is placed
		waiting, appDone, placed := -1, -1, -1
		for i, e := range rec.events {
			switch {
			case e.Kind == presenter.KindProgress && e.Message == "waiting on a namespace for secret cloudflare-creds" && waiting < 0:
				waiting = i
			case e.Kind == presenter.KindApplied && e.Message == "app" && appDone < 0:
				appDone = i
			case e.Kind == presenter.KindProgress && e.Message == "placed secret cloudflare-creds" && placed < 0:
				placed = i
			}
		}
		if waiting < 0 || appDone < 0 || placed < 0 || !(waiting < placed && appDone < placed) {
			t.Errorf("Expected wait, app done, then placement; got indexes %d, %d, %d in %+v", waiting, appDone, placed, rec.events)
		}
	})

	t.Run("ReportsPlacementAsProgressOnTheCurrentStep", func(t *testing.T) {
		// Given a kustomization whose inventory holds one applied namespace, inside a recorded step
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-dns"}}, nil
		}
		rec := &eventRecorder{}
		ctx, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "kustomize:cdn-install", "Applying kustomization cdn-install")
		defer end(nil)

		// When placing the resolved secrets
		if err := newProvisioner(mocks).PlaceSecrets(ctx, resolved(), nil, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then placement is progress on that step, not a step of its own
		if ops := rec.operations(); len(ops) != 1 {
			t.Fatalf("Expected no extra steps, got %v", ops)
		}
		if e := rec.events[1]; e.Kind != presenter.KindProgress || e.ID != rec.events[0].ID || e.Message != "placed secret cloudflare-creds" {
			t.Errorf("Expected a placement progress update on the step, got %+v", e)
		}
	})

	t.Run("RollsConsumersWithContentDigest", func(t *testing.T) {
		// Given a kustomization whose inventory holds exactly one applied namespace
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-dns"}}, nil
		}
		var rolledNs, rolledSecret, rolledDigest string
		mocks.KubernetesManager.RollWorkloadsForSecretFunc = func(ctx context.Context, namespace, secretName, digest string) error {
			rolledNs, rolledSecret, rolledDigest = namespace, secretName, digest
			return nil
		}

		// When placing the resolved secrets
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then consumers are rolled in the inventory namespace, keyed by the resolved content's digest
		wantDigest := "4730801c532957c07f8434818b406568a62ce237c9c2caf7d777dd1cecfa0deb"
		if rolledNs != "system-dns" || rolledSecret != "cloudflare-creds" || rolledDigest != wantDigest {
			t.Errorf("Unexpected roll args: ns=%q secret=%q digest=%q (want digest %q)", rolledNs, rolledSecret, rolledDigest, wantDigest)
		}
	})

	t.Run("RollFailureSurfaces", func(t *testing.T) {
		// Given placement succeeds but rolling consumers fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-dns"}}, nil
		}
		mocks.KubernetesManager.RollWorkloadsForSecretFunc = func(ctx context.Context, namespace, secretName, digest string) error {
			return fmt.Errorf("api down")
		}

		// When placing the resolved secrets
		err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, true)
		// Then the roll failure surfaces
		if err == nil || !strings.Contains(err.Error(), "rolling workloads for secret") {
			t.Errorf("Expected roll-failure error, got %v", err)
		}
	})

	t.Run("FailsClosedWhenMultipleNamespaces", func(t *testing.T) {
		// Given an inventory that holds two namespaces
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "ns-a"}, {Kind: "Namespace", Name: "ns-b"}}, nil
		}

		// When placing the resolved secrets
		err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, true)
		// Then placement fails closed
		if err == nil || !strings.Contains(err.Error(), "multiple namespaces") {
			t.Errorf("Expected multiple-namespaces error, got %v", err)
		}
	})

	t.Run("AutoResolvesFromResourceNamespaceWhenNoNamespaceCreated", func(t *testing.T) {
		// Given an inventory that holds only a resource in system-csi
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "DaemonSet", Name: "hcloud-csi-node", Namespace: "system-csi"}}, nil
		}
		var gotNs string
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			gotNs = namespace
			return nil
		}

		// When placing the resolved secrets
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the secret lands in the namespace of that resource
		if gotNs != "system-csi" {
			t.Errorf("Expected placement into system-csi, got %q", gotNs)
		}
	})

	t.Run("FailsFastWhenNothingToInferNamespaceFrom", func(t *testing.T) {
		// Given an inventory that holds only a cluster-scoped resource
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "ClusterRole", Name: "csi-role"}}, nil
		}

		// When placing under a live context
		err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, true)
		// Then it fails fast with a remediation that names namespaces:
		if err == nil || !strings.Contains(err.Error(), "creates no namespace and deploys no namespaced resources") {
			t.Errorf("Expected fail-fast no-namespace error, got %v", err)
		}
		if we := werror.Find(err); we == nil || !strings.Contains(we.Remediation, "namespaces:") {
			t.Errorf("Expected a remediation that names namespaces:, got: %v", err)
		}
	})

	t.Run("TimesOutWhileInventoryStillEmpty", func(t *testing.T) {
		// Given a kustomization that has not reconciled yet (empty inventory)
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return nil, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When placing under an ended context
		err := newProvisioner(mocks).PlaceSecrets(ctx, resolved(), nil, true)
		// Then a namespace timeout error names the pending kustomization
		if err == nil || !strings.Contains(err.Error(), "before placing secrets") || !strings.Contains(err.Error(), "cdn-install") {
			t.Errorf("Expected namespace-timeout error naming the pending secret, got %v", err)
		}
	})

	t.Run("PlacesIntoExplicitNamespaceThatExists", func(t *testing.T) {
		// Given a secret naming its namespace explicitly, and that namespace exists in the cluster
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) {
			return name == "system-pki", nil
		}
		var gotNs string
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			gotNs = namespace
			return nil
		}
		explicit := provisioner.ResolvedSecrets{"pki-install": {"hetzner-dns": {Namespaces: []string{"system-pki"}, Data: map[string]string{"token": "resolved"}}}}

		// When placing the explicit secret
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), explicit, nil, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the secret lands in the named namespace without an inventory read
		if gotNs != "system-pki" {
			t.Errorf("Expected placement into system-pki, got %q", gotNs)
		}
	})

	t.Run("PlacesIntoExplicitNamespaceCreatedByAnotherKustomization", func(t *testing.T) {
		// Given lb-install names system-lb, which another kustomization created
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{}, nil
		}
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) {
			return name == "system-lb", nil
		}
		var gotNs string
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			gotNs = namespace
			return nil
		}
		explicit := provisioner.ResolvedSecrets{"lb-install": {"hcloud": {Namespaces: []string{"system-lb"}, Data: map[string]string{"token": "resolved"}}}}

		// When placing the explicit secret
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), explicit, nil, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then namespace existence, not owner inventory, gates placement
		if gotNs != "system-lb" {
			t.Errorf("Expected placement into system-lb, got %q", gotNs)
		}
	})

	t.Run("PlacesReadySecretWithoutBlockingOnAPendingOne", func(t *testing.T) {
		// Given one secret whose namespace exists and one whose namespace never appears
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) {
			return name == "system-lb", nil
		}
		placed := map[string]bool{}
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			placed[namespace] = true
			return nil
		}
		secrets := provisioner.ResolvedSecrets{
			"lb-install":  {"hcloud": {Namespaces: []string{"system-lb"}, Data: map[string]string{"token": "t"}}},
			"dns-install": {"hetzner-dns": {Namespaces: []string{"system-dns"}, Data: map[string]string{"token": "t"}}},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When placing under an ended context
		err := newProvisioner(mocks).PlaceSecrets(ctx, secrets, nil, false)
		// Then the ready secret lands and the timeout names only the pending one
		if !placed["system-lb"] {
			t.Error("Expected the ready secret to be placed into system-lb despite a pending peer")
		}
		if err == nil || !strings.Contains(err.Error(), "system-dns") || strings.Contains(err.Error(), "system-lb") {
			t.Errorf("Expected timeout naming only the pending system-dns, got %v", err)
		}
	})

	t.Run("PlacesSecretOnceItsNamespaceAppears", func(t *testing.T) {
		// Given a secret whose namespace appears on the third existence check
		mocks := setupProvisionerMocks(t)
		var checks int
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) {
			checks++
			return checks >= 3, nil
		}
		var gotNs string
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			gotNs = namespace
			return nil
		}
		secrets := provisioner.ResolvedSecrets{"dns-install": {"hetzner-dns": {Namespaces: []string{"system-dns"}, Data: map[string]string{"token": "t"}}}}

		prov := newProvisioner(mocks)
		prov.PollInterval = time.Millisecond

		// When placing the secret
		if err := prov.PlaceSecrets(context.Background(), secrets, nil, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the secret lands once its namespace appears
		if gotNs != "system-dns" {
			t.Errorf("Expected placement into system-dns once it appeared, got %q", gotNs)
		}
	})

	t.Run("ReconcilesOwnerAfterPlacingItsSecret", func(t *testing.T) {
		// Given a secret whose namespace exists, so it is placed on the first round
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) { return name == "system-lb", nil }
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error { return nil }
		notifier := fluxinfra.NewMockNotifier()
		var reconciled [][]string
		notifier.ReconcileKustomizationsFunc = func(ctx context.Context, names []string) error {
			reconciled = append(reconciled, names)
			return nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, Notifier: notifier})
		secrets := provisioner.ResolvedSecrets{"lb-install": {"hcloud": {Namespaces: []string{"system-lb"}, Data: map[string]string{"token": "t"}}}}
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{{Name: "lb-install"}}}

		// When placing the secret
		if err := prov.PlaceSecrets(context.Background(), secrets, bp, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the owning kustomization is reconciled after placement
		if len(reconciled) == 0 || !slices.Contains(reconciled[0], "lb-install") {
			t.Errorf("Expected lb-install to be reconciled after placement, got %v", reconciled)
		}
	})

	t.Run("NudgesOnlyTheFrontierWhilePending", func(t *testing.T) {
		// Given a pending secret behind a not-Ready chain rooted at lb-install
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) { return false, nil }
		mocks.KubernetesManager.GetKustomizationReadinessFunc = func(names []string) (map[string]bool, error) {
			return map[string]bool{"dns-install": false, "gateway-install": false, "lb-install": false}, nil
		}
		notifier := fluxinfra.NewMockNotifier()
		var reconciled [][]string
		notifier.ReconcileKustomizationsFunc = func(ctx context.Context, names []string) error {
			reconciled = append(reconciled, names)
			return nil
		}
		prov := provisioner.NewProvisioner(mocks.Runtime, mocks.BlueprintHandler, &provisioner.Provisioner{KubernetesManager: mocks.KubernetesManager, Notifier: notifier})
		secrets := provisioner.ResolvedSecrets{"dns-install": {"hetzner-dns": {Namespaces: []string{"system-dns"}, Data: map[string]string{"token": "t"}}}}
		bp := &blueprintv1alpha1.Blueprint{Kustomizations: []blueprintv1alpha1.Kustomization{
			{Name: "dns-install", DependsOn: []string{"gateway-install"}},
			{Name: "gateway-install", DependsOn: []string{"lb-install"}},
			{Name: "lb-install"},
		}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When placing under an ended context
		_ = prov.PlaceSecrets(ctx, secrets, bp, false)
		// Then only the frontier kustomization lb-install is reconciled
		if len(reconciled) == 0 {
			t.Fatalf("expected a frontier reconcile, got none")
		}
		if got := reconciled[0]; !slices.Equal(got, []string{"lb-install"}) {
			t.Errorf("expected only frontier lb-install nudged, got %v", got)
		}
	})

	t.Run("FansOutToEveryNamedNamespace", func(t *testing.T) {
		// Given a secret naming two namespaces that both exist in the cluster
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) {
			return name == "system-pki" || name == "system-dns", nil
		}
		placed := map[string]bool{}
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			placed[namespace] = true
			return nil
		}
		explicit := provisioner.ResolvedSecrets{"pki-install": {"acme-dns": {Namespaces: []string{"system-pki", "system-dns"}, Data: map[string]string{"token": "resolved"}}}}

		// When placing the explicit secret
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), explicit, nil, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the secret lands in each named namespace
		if !placed["system-pki"] || !placed["system-dns"] || len(placed) != 2 {
			t.Errorf("Expected fan-out into system-pki and system-dns, got %v", placed)
		}
	})

	t.Run("TimesOutNamingANonexistentExplicitNamespace", func(t *testing.T) {
		// Given a named namespace that does not exist in the cluster
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) {
			return false, nil
		}
		explicit := provisioner.ResolvedSecrets{"pki-install": {"hetzner-dns": {Namespaces: []string{"system-pki"}, Data: map[string]string{"token": "resolved"}}}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When placing under an ended context
		err := newProvisioner(mocks).PlaceSecrets(ctx, explicit, nil, true)
		// Then a timeout error names the missing namespace
		if err == nil || !strings.Contains(err.Error(), "system-pki") || !strings.Contains(err.Error(), "before placing secrets") {
			t.Errorf("Expected missing-namespace timeout error naming system-pki, got %v", err)
		}
	})

	t.Run("EmptyResolvedPlacesNothingButStillReconciles", func(t *testing.T) {
		// Given a blueprint that declares no secrets
		mocks := setupProvisionerMocks(t)
		queried := false
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			queried = true
			return nil, nil
		}
		var pruned bool
		var prunedDesired map[string]map[string]bool
		mocks.KubernetesManager.PruneSecretsFunc = func(desired map[string]map[string]bool) error {
			pruned, prunedDesired = true, desired
			return nil
		}

		// When placing an empty set of secrets
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), provisioner.ResolvedSecrets{}, nil, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then no inventory is read and prune runs with an empty desired set
		if queried {
			t.Error("Expected no inventory query when nothing is placed")
		}
		if !pruned || len(prunedDesired) != 0 {
			t.Errorf("Expected reconcile with empty desired set, pruned=%v desired=%v", pruned, prunedDesired)
		}
	})

	t.Run("ReconcilesFanOutPlacementIntoDesiredSet", func(t *testing.T) {
		// Given a secret that fans out into two namespaces that exist in the cluster
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.NamespaceExistsFunc = func(name string) (bool, error) {
			return name == "system-pki" || name == "system-dns", nil
		}
		var gotDesired map[string]map[string]bool
		mocks.KubernetesManager.PruneSecretsFunc = func(desired map[string]map[string]bool) error {
			gotDesired = desired
			return nil
		}
		fanned := provisioner.ResolvedSecrets{"pki-install": {"acme-dns": {Namespaces: []string{"system-pki", "system-dns"}, Data: map[string]string{"token": "resolved"}}}}

		// When placing the fanned-out secret
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), fanned, nil, true); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the prune desired set names both placements
		if !gotDesired["system-pki"]["acme-dns"] || !gotDesired["system-dns"]["acme-dns"] {
			t.Errorf("Expected desired set to include both placements, got %v", gotDesired)
		}
	})

	t.Run("PruneFailureSurfaces", func(t *testing.T) {
		// Given placement succeeds but the reconcile delete fails
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-dns"}}, nil
		}
		mocks.KubernetesManager.PruneSecretsFunc = func(desired map[string]map[string]bool) error {
			return fmt.Errorf("api down")
		}

		// When placing the resolved secrets
		err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, true)
		// Then the prune failure surfaces
		if err == nil || !strings.Contains(err.Error(), "pruning orphaned secrets") {
			t.Errorf("Expected prune-failure error, got %v", err)
		}
	})

	t.Run("PruneDisabledPlacesWithoutReconciling", func(t *testing.T) {
		// Given a place-only flow (prune not requested)
		mocks := setupProvisionerMocks(t)
		mocks.KubernetesManager.GetKustomizationInventoryFunc = func(name, namespace string) ([]kubernetes.InventoryEntry, error) {
			return []kubernetes.InventoryEntry{{Kind: "Namespace", Name: "system-dns"}}, nil
		}
		placedName := ""
		mocks.KubernetesManager.ApplySecretFunc = func(name, namespace, secretType string, stringData map[string]string, owner string) error {
			placedName = name
			return nil
		}
		reconciled := false
		mocks.KubernetesManager.PruneSecretsFunc = func(desired map[string]map[string]bool) error {
			reconciled = true
			return nil
		}

		// When placing with prune disabled
		if err := newProvisioner(mocks).PlaceSecrets(context.Background(), resolved(), nil, false); err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Then the secret is placed and prune never runs
		if placedName != "cloudflare-creds" {
			t.Errorf("Expected the secret to still be placed, got %q", placedName)
		}
		if reconciled {
			t.Error("Expected no reconcile when prune is disabled")
		}
	})
}
