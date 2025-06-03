// Package kubernetes provides Kubernetes resource management functionality
// It implements server-side apply patterns for managing Kubernetes resources
// and provides a clean interface for kustomization and resource management

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/config"
	"github.com/windsorcli/cli/pkg/di"
	"github.com/windsorcli/cli/pkg/shell"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// =============================================================================
// Types
// =============================================================================

// KubeRequestConfig defines configuration for Kubernetes API requests
type KubeRequestConfig struct {
	Method    string
	ApiPath   string
	Namespace string
	Resource  string
	Name      string
	Body      any
	Response  runtime.Object
	Headers   map[string]string
}

// GitRepository represents a Flux GitRepository resource
type GitRepository struct {
	Name      string
	Namespace string
	URL       string
	Interval  time.Duration
	Timeout   time.Duration
	Reference *GitRepositoryRef
	SecretRef *LocalObjectReference
}

type GitRepositoryRef struct {
	Branch string
	Tag    string
	SemVer string
	Commit string
}

type LocalObjectReference struct {
	Name string
}

// Kustomization represents a Flux Kustomization resource
type Patch struct {
	Patch  string       `json:"patch" yaml:"patch"`
	Target *PatchTarget `json:"target,omitempty" yaml:"target,omitempty"`
}

type PatchTarget struct {
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type Kustomization struct {
	Name       string
	Namespace  string
	Source     string
	Path       string
	DependsOn  []string
	Force      bool
	Components []string
	Cleanup    []string
	Patches    []Patch
	PostBuild  *kustomizev1.PostBuild
}

// HelmRelease represents a Flux HelmRelease resource
type HelmRelease struct {
	Name      string
	Namespace string
}

// ResourceOperationConfig defines configuration for resource operations
// Matches blueprint ResourceOperationConfig
type ResourceOperationConfig struct {
	ApiPath              string
	Namespace            string
	ResourceName         string
	ResourceInstanceName string
	ResourceObject       runtime.Object
	ResourceType         func() runtime.Object
}

// =============================================================================
// Interfaces
// =============================================================================

// KubernetesManager defines methods for Kubernetes resource management
type KubernetesManager interface {
	Initialize() error
	ApplyKustomization(kustomization Kustomization) error
	DeleteKustomization(name, namespace string) error
	WaitForKustomizations(message string, names ...string) error
	GetKustomizationStatus(names []string) (map[string]bool, error)
	CreateNamespace(name string) error
	DeleteNamespace(name string) error
	ApplyConfigMap(name, namespace string, data map[string]string) error
	GetHelmReleasesForKustomization(name, namespace string) ([]HelmRelease, error)
	SuspendKustomization(name, namespace string) error
	SuspendHelmRelease(name, namespace string) error
	ApplyGitRepository(repo *GitRepository) error
}

// =============================================================================
// Base Implementation
// =============================================================================

// BaseKubernetesManager implements KubernetesManager interface
type BaseKubernetesManager struct {
	injector      di.Injector
	shell         shell.Shell
	configHandler config.ConfigHandler
	shims         *Shims

	kustomizationWaitPollInterval time.Duration
	kustomizationReconcileTimeout time.Duration
	kustomizationReconcileSleep   time.Duration
}

// NewKubernetesManager creates a new instance of BaseKubernetesManager
func NewKubernetesManager(injector di.Injector) *BaseKubernetesManager {
	return &BaseKubernetesManager{
		injector:                      injector,
		shims:                         NewShims(),
		kustomizationWaitPollInterval: 2 * time.Second,
		kustomizationReconcileTimeout: 5 * time.Minute,
		kustomizationReconcileSleep:   2 * time.Second,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize sets up the BaseKubernetesManager by resolving dependencies
func (k *BaseKubernetesManager) Initialize() error {
	shellInstance, ok := k.injector.Resolve("shell").(shell.Shell)
	if !ok {
		return fmt.Errorf("error resolving shell")
	}
	k.shell = shellInstance

	configHandler, ok := k.injector.Resolve("configHandler").(config.ConfigHandler)
	if !ok {
		return fmt.Errorf("error resolving configHandler")
	}
	k.configHandler = configHandler

	return nil
}

// ApplyKustomization creates or updates a Kustomization resource in the cluster.
// Matches blueprint logic: get-or-create-or-update, all fields, context/source resolution.
func (k *BaseKubernetesManager) ApplyKustomization(kustomization Kustomization) error {
	// Source resolution
	if kustomization.Source == "" {
		if k.configHandler != nil {
			kustomization.Source = k.configHandler.GetContext()
		}
	}

	interval := 2 * time.Minute
	timeout := 5 * time.Minute
	retryInterval := 30 * time.Second
	prune := true
	wait := true
	suspend := false
	dependsOn := []map[string]interface{}{}
	for _, dep := range kustomization.DependsOn {
		dependsOn = append(dependsOn, map[string]interface{}{"name": dep})
	}

	// Local struct matching blueprint and Flux YAML
	kustomizeObj := map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      kustomization.Name,
			"namespace": kustomization.Namespace,
		},
		"spec": map[string]interface{}{
			"interval":      interval.String(),
			"timeout":       timeout.String(),
			"retryInterval": retryInterval.String(),
			"path":          kustomization.Path,
			"prune":         prune,
			"wait":          wait,
			"suspend":       suspend,
			"dependsOn":     dependsOn,
			"sourceRef": map[string]interface{}{
				"kind":      "GitRepository",
				"name":      kustomization.Source,
				"namespace": kustomization.Namespace,
			},
			"patches":    kustomization.Patches,
			"components": kustomization.Components,
			"postBuild":  kustomization.PostBuild,
		},
	}

	config := ResourceOperationConfig{
		ApiPath:              "/apis/kustomize.toolkit.fluxcd.io/v1",
		Namespace:            kustomization.Namespace,
		ResourceName:         "kustomizations",
		ResourceInstanceName: kustomization.Name,
		ResourceObject:       &unstructured.Unstructured{Object: kustomizeObj},
		ResourceType: func() runtime.Object {
			return &unstructured.Unstructured{}
		},
	}

	return kubeClientResourceOperation(os.Getenv("KUBECONFIG"), config)
}

// DeleteKustomization removes a Kustomization resource
func (k *BaseKubernetesManager) DeleteKustomization(name, namespace string) error {
	return kubeClient(os.Getenv("KUBECONFIG"), KubeRequestConfig{
		Method:    "DELETE",
		ApiPath:   "/apis/kustomize.toolkit.fluxcd.io/v1",
		Namespace: namespace,
		Resource:  "kustomizations",
		Name:      name,
	})
}

// WaitForKustomizations waits for kustomizations to be ready
func (k *BaseKubernetesManager) WaitForKustomizations(message string, names ...string) error {
	spin := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"))
	spin.Suffix = " " + message
	spin.Start()
	defer spin.Stop()

	timeout := time.After(k.kustomizationReconcileTimeout)
	ticker := time.NewTicker(k.kustomizationWaitPollInterval)
	defer ticker.Stop()

	consecutiveFailures := 0
	for {
		select {
		case <-timeout:
			spin.Stop()
			fmt.Fprintf(os.Stderr, "\033[31m✗ %s - \033[31mFailed\033[0m\n", message)
			return fmt.Errorf("timeout waiting for kustomizations")
		case <-ticker.C:
			kubeconfig := os.Getenv("KUBECONFIG")
			if err := checkGitRepositoryStatus(kubeconfig); err != nil {
				consecutiveFailures++
				if consecutiveFailures >= 3 { // DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES
					spin.Stop()
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s - \033[31mFailed\033[0m\n", message)
					return fmt.Errorf("git repository error after %d consecutive failures: %w", consecutiveFailures, err)
				}
				continue
			}
			status, err := k.GetKustomizationStatus(names)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures >= 3 { // DEFAULT_KUSTOMIZATION_WAIT_MAX_FAILURES
					spin.Stop()
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s - \033[31mFailed\033[0m\n", message)
					return fmt.Errorf("kustomization error after %d consecutive failures: %w", consecutiveFailures, err)
				}
				continue
			}

			allReady := true
			for _, ready := range status {
				if !ready {
					allReady = false
					break
				}
			}

			if allReady {
				spin.Stop()
				fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", message)
				return nil
			}

			// Reset consecutive failures on successful check
			consecutiveFailures = 0
		}
	}
}

// GetKustomizationStatus checks the status of kustomizations
func (k *BaseKubernetesManager) GetKustomizationStatus(names []string) (map[string]bool, error) {
	return checkKustomizationStatus(os.Getenv("KUBECONFIG"), names)
}

// CreateNamespace creates a new namespace
func (k *BaseKubernetesManager) CreateNamespace(name string) error {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "windsor-cli",
			},
		},
	}

	return kubeClient(os.Getenv("KUBECONFIG"), KubeRequestConfig{
		Method:   "POST",
		ApiPath:  "/api/v1",
		Resource: "namespaces",
		Body:     ns,
	})
}

// DeleteNamespace removes a namespace
func (k *BaseKubernetesManager) DeleteNamespace(name string) error {
	return kubeClient(os.Getenv("KUBECONFIG"), KubeRequestConfig{
		Method:   "DELETE",
		ApiPath:  "/api/v1",
		Resource: "namespaces",
		Name:     name,
	})
}

// ApplyConfigMap creates or updates a ConfigMap
func (k *BaseKubernetesManager) ApplyConfigMap(name, namespace string, data map[string]string) error {
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}

	return kubeClientResourceOperation(os.Getenv("KUBECONFIG"), ResourceOperationConfig{
		ApiPath:              "/api/v1",
		Namespace:            namespace,
		ResourceName:         "configmaps",
		ResourceInstanceName: name,
		ResourceObject:       configMap,
		ResourceType: func() runtime.Object {
			return &corev1.ConfigMap{}
		},
	})
}

// GetHelmReleasesForKustomization gets HelmReleases associated with a Kustomization
func (k *BaseKubernetesManager) GetHelmReleasesForKustomization(name, namespace string) ([]HelmRelease, error) {
	var kustomization kustomizev1.Kustomization

	err := kubeClient(os.Getenv("KUBECONFIG"), KubeRequestConfig{
		Method:    "GET",
		ApiPath:   "/apis/kustomize.toolkit.fluxcd.io/v1",
		Namespace: namespace,
		Resource:  "kustomizations",
		Name:      name,
		Response:  &kustomization,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get kustomization: %w", err)
	}

	var helmReleases []HelmRelease
	if kustomization.Status.Inventory == nil {
		return helmReleases, nil
	}

	for _, entry := range kustomization.Status.Inventory.Entries {
		parts := strings.Split(entry.ID, "_")
		if len(parts) >= 4 && parts[2] == "helm.toolkit.fluxcd.io" && parts[3] == "HelmRelease" {
			helmReleases = append(helmReleases, HelmRelease{
				Name:      parts[1],
				Namespace: parts[0],
			})
		}
	}

	return helmReleases, nil
}

// SuspendKustomization suspends a Kustomization
func (k *BaseKubernetesManager) SuspendKustomization(name, namespace string) error {
	patch := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"suspend": true,
			},
		},
	}

	return kubeClient(os.Getenv("KUBECONFIG"), KubeRequestConfig{
		Method:    "PATCH",
		ApiPath:   "/apis/kustomize.toolkit.fluxcd.io/v1",
		Namespace: namespace,
		Resource:  "kustomizations",
		Name:      name,
		Body:      patch,
		Headers: map[string]string{
			"Content-Type": "application/merge-patch+json",
		},
	})
}

// SuspendHelmRelease suspends a Flux HelmRelease by setting its suspend field to true.
// This prevents the helmrelease from reconciling during teardown.
func (k *BaseKubernetesManager) SuspendHelmRelease(name, namespace string) error {
	patch := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"suspend": true,
			},
		},
	}

	return kubeClient(os.Getenv("KUBECONFIG"), KubeRequestConfig{
		Method:    "PATCH",
		ApiPath:   "/apis/helm.toolkit.fluxcd.io/v2",
		Namespace: namespace,
		Resource:  "helmreleases",
		Name:      name,
		Body:      patch,
		Headers: map[string]string{
			"Content-Type": "application/merge-patch+json",
		},
	})
}

// ApplyGitRepository creates or updates a GitRepository resource in the cluster
func (k *BaseKubernetesManager) ApplyGitRepository(repo *GitRepository) error {
	gitRepo := &sourcev1.GitRepository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "source.toolkit.fluxcd.io/v1",
			Kind:       "GitRepository",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      repo.Name,
			Namespace: repo.Namespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: repo.URL,
			Interval: metav1.Duration{
				Duration: repo.Interval,
			},
			Timeout: &metav1.Duration{
				Duration: repo.Timeout,
			},
			Reference: &sourcev1.GitRepositoryRef{
				Branch: repo.Reference.Branch,
				Tag:    repo.Reference.Tag,
				SemVer: repo.Reference.SemVer,
				Commit: repo.Reference.Commit,
			},
		},
	}

	if repo.SecretRef != nil {
		gitRepo.Spec.SecretRef = &meta.LocalObjectReference{
			Name: repo.SecretRef.Name,
		}
	}

	return kubeClientResourceOperation(os.Getenv("KUBECONFIG"), ResourceOperationConfig{
		ApiPath:              "/apis/source.toolkit.fluxcd.io/v1",
		Namespace:            repo.Namespace,
		ResourceName:         "gitrepositories",
		ResourceInstanceName: repo.Name,
		ResourceObject:       gitRepo,
		ResourceType: func() runtime.Object {
			return &sourcev1.GitRepository{}
		},
	})
}

// =============================================================================
// Private Methods
// =============================================================================

// kubeClient performs a Kubernetes API request using the provided configuration
func kubeClient(kubeconfigPath string, config KubeRequestConfig) error {
	var kubeConfig *rest.Config
	var err error

	if kubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Parse API path to get group, version, resource
	parts := strings.Split(strings.TrimPrefix(config.ApiPath, "/"), "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid API path: %s", config.ApiPath)
	}

	var gvr schema.GroupVersionResource
	if parts[0] == "api" {
		// Core API group
		gvr = schema.GroupVersionResource{
			Group:    "",
			Version:  parts[1],
			Resource: config.Resource,
		}
	} else if parts[0] == "apis" {
		// Custom resource
		if len(parts) < 3 {
			return fmt.Errorf("invalid API path for custom resource: %s", config.ApiPath)
		}
		gvr = schema.GroupVersionResource{
			Group:    parts[1],
			Version:  parts[2],
			Resource: config.Resource,
		}
	} else {
		return fmt.Errorf("invalid API path format: %s", config.ApiPath)
	}

	var resourceClient dynamic.ResourceInterface
	if config.Namespace != "" {
		resourceClient = dynamicClient.Resource(gvr).Namespace(config.Namespace)
	} else {
		resourceClient = dynamicClient.Resource(gvr)
	}

	switch config.Method {
	case "GET":
		if config.Name != "" {
			obj, err := resourceClient.Get(context.Background(), config.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if config.Response != nil {
				return runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), config.Response)
			}
		} else {
			list, err := resourceClient.List(context.Background(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			if config.Response != nil {
				return runtime.DefaultUnstructuredConverter.FromUnstructured(list.UnstructuredContent(), config.Response)
			}
		}
	case "POST":
		unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(config.Body)
		if err != nil {
			return fmt.Errorf("failed to convert object to unstructured: %w", err)
		}
		_, err = resourceClient.Create(context.Background(), &unstructured.Unstructured{Object: unstructuredMap}, metav1.CreateOptions{})
		return err
	case "DELETE":
		return resourceClient.Delete(context.Background(), config.Name, metav1.DeleteOptions{})
	case "PATCH":
		unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(config.Body)
		if err != nil {
			return fmt.Errorf("failed to convert object to unstructured: %w", err)
		}
		patchBytes, err := json.Marshal(unstructured)
		if err != nil {
			return fmt.Errorf("failed to marshal patch: %w", err)
		}
		_, err = resourceClient.Patch(context.Background(), config.Name, "application/merge-patch+json", patchBytes, metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("unsupported method: %s", config.Method)
	}

	return nil
}

// checkKustomizationStatus checks the status of kustomizations
func checkKustomizationStatus(kubeconfigPath string, names []string) (map[string]bool, error) {
	var kubeConfig *rest.Config
	var err error

	if kubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	objList, err := dynamicClient.Resource(gvr).Namespace("flux-system").
		List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list kustomizations: %w", err)
	}

	status := make(map[string]bool)
	found := make(map[string]bool)

	for _, obj := range objList.Items {
		var kustomizeObj kustomizev1.Kustomization
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &kustomizeObj); err != nil {
			return nil, fmt.Errorf("failed to convert kustomization %s: %w", kustomizeObj.Name, err)
		}

		found[kustomizeObj.Name] = true
		ready := false
		for _, condition := range kustomizeObj.Status.Conditions {
			if condition.Type == "Ready" {
				if condition.Status == "True" {
					ready = true
				} else if condition.Status == "False" && condition.Reason == "ReconciliationFailed" {
					return nil, fmt.Errorf("kustomization %s failed: %s", kustomizeObj.Name, condition.Message)
				}
				break
			}
		}
		status[kustomizeObj.Name] = ready
	}

	for _, name := range names {
		if !found[name] {
			status[name] = false
			continue
		}
	}

	return status, nil
}

// kubeClientResourceOperation implements get-or-create-or-update pattern for resources
func kubeClientResourceOperation(kubeconfigPath string, config ResourceOperationConfig) error {
	existingObj := config.ResourceType()
	err := kubeClient(kubeconfigPath, KubeRequestConfig{
		Method:    "GET",
		ApiPath:   config.ApiPath,
		Namespace: config.Namespace,
		Resource:  config.ResourceName,
		Name:      config.ResourceInstanceName,
		Response:  existingObj,
	})
	if err != nil {
		// If not found, create
		if strings.Contains(err.Error(), "not found") {
			return kubeClient(kubeconfigPath, KubeRequestConfig{
				Method:    "POST",
				ApiPath:   config.ApiPath,
				Namespace: config.Namespace,
				Resource:  config.ResourceName,
				Body:      config.ResourceObject,
			})
		}
		return err
	}
	// Update: set resourceVersion
	metaObj, ok := existingObj.(metav1.Object)
	if !ok {
		return fmt.Errorf("existing object does not implement metav1.Object")
	}
	newMetaObj, ok := config.ResourceObject.(metav1.Object)
	if !ok {
		return fmt.Errorf("new object does not implement metav1.Object")
	}
	newMetaObj.SetResourceVersion(metaObj.GetResourceVersion())
	return kubeClient(kubeconfigPath, KubeRequestConfig{
		Method:    "PATCH",
		ApiPath:   config.ApiPath,
		Namespace: config.Namespace,
		Resource:  config.ResourceName,
		Name:      config.ResourceInstanceName,
		Body:      config.ResourceObject,
		Headers: map[string]string{
			"Content-Type": "application/merge-patch+json",
		},
	})
}

// checkGitRepositoryStatus checks the status of all GitRepository resources in the cluster.
// It returns an error if any repository is not ready or has failed. The function queries all
// GitRepository resources in the default namespace, converts them to typed objects, and inspects
// their status conditions for readiness or failure. If any repository is not ready, it returns an error
// with the repository name and failure message.
func checkGitRepositoryStatus(kubeconfigPath string) error {
	var kubeConfig *rest.Config
	var err error

	if kubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	objList, err := dynamicClient.Resource(gvr).Namespace("flux-system").
		List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list git repositories: %w", err)
	}

	for _, obj := range objList.Items {
		var gitRepo sourcev1.GitRepository
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &gitRepo); err != nil {
			return fmt.Errorf("failed to convert git repository %s: %w", gitRepo.Name, err)
		}

		for _, condition := range gitRepo.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "False" {
				return fmt.Errorf("%s: %s", gitRepo.Name, condition.Message)
			}
		}
	}

	return nil
}

// ToKubernetesKustomization converts a blueprint kustomization to a kubernetes kustomization
func ToKubernetesKustomization(k blueprintv1alpha1.Kustomization, namespace string) Kustomization {
	kk := Kustomization{
		Name:       k.Name,
		Namespace:  namespace,
		Path:       k.Path,
		Source:     k.Source,
		DependsOn:  k.DependsOn,
		Patches:    make([]Patch, len(k.Patches)),
		Components: k.Components,
		PostBuild:  nil,
	}
	if k.PostBuild != nil {
		kk.PostBuild = &kustomizev1.PostBuild{
			SubstituteFrom: make([]kustomizev1.SubstituteReference, len(k.PostBuild.SubstituteFrom)),
		}
		for i, ref := range k.PostBuild.SubstituteFrom {
			kk.PostBuild.SubstituteFrom[i] = kustomizev1.SubstituteReference{
				Kind:     ref.Kind,
				Name:     ref.Name,
				Optional: ref.Optional,
			}
		}
	}
	for i, p := range k.Patches {
		kk.Patches[i] = Patch{
			Patch: p.Patch,
			Target: &PatchTarget{
				Kind: p.Target.Kind,
				Name: p.Target.Name,
			},
		}
	}
	return kk
}
