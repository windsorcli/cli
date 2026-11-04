// The Kustomization render is the flux build step of the Lister.
// It provides the rendered objects of one Flux Kustomization, built offline with flux build --dry-run.
// The flux CLI builds with the package that kustomize-controller uses, so patches and substitutions apply.
// Context substitution values go inline into postBuild.substitute, and deferred values become a marker.

package inventory

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	blueprintcomposer "github.com/windsorcli/cli/pkg/composer/blueprint"
	"github.com/windsorcli/cli/pkg/constants"
)

// =============================================================================
// Constants
// =============================================================================

// minFluxMajor and minFluxMinor are the minimum flux CLI version, the same that windsor plan checks.
const (
	minFluxMajor = 2
	minFluxMinor = 3
)

// deferredMarker replaces a substitution value that is not known before apply.
const deferredMarker = "windsor-deferred-value"

// =============================================================================
// Types
// =============================================================================

// renderedObject is one Kubernetes object from a render and the Kustomization that produced it.
// parent names the object that an extractor derived it from.
// A Terraform object has no Kustomization and names the file of its resource.
type renderedObject struct {
	kustomization string
	file          string
	object        map[string]any
	parent        string
}

// =============================================================================
// Private Methods
// =============================================================================

// checkFlux returns an error with a remediation hint when the flux CLI is missing or older than v2.3.
func (l *BaseLister) checkFlux() error {
	if _, err := l.shims.LookPath("flux"); err != nil {
		return werror.New("the flux CLI is required for 'show artifacts'", "Install the flux CLI from https://fluxcd.io/flux/installation/.", nil)
	}
	out, err := l.runtime.Shell.ExecSilent("flux", "version", "--client")
	if err != nil {
		return werror.Wrap(err, "failed to get flux version")
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "flux:" {
			continue
		}
		ver := strings.TrimPrefix(fields[1], "v")
		segments := strings.SplitN(ver, ".", 3)
		if len(segments) < 2 {
			break
		}
		major, err1 := strconv.Atoi(segments[0])
		minor, err2 := strconv.Atoi(segments[1])
		if err1 != nil || err2 != nil {
			break
		}
		if major > minFluxMajor || (major == minFluxMajor && minor >= minFluxMinor) {
			return nil
		}
		return werror.New(fmt.Sprintf("flux CLI v%d.%d or later is required (found v%s)", minFluxMajor, minFluxMinor, ver), "Upgrade the flux CLI from https://fluxcd.io/flux/installation/.", nil)
	}
	return fmt.Errorf("could not determine flux CLI version from output: %q", out)
}

// renderKustomization writes the Flux Kustomization for k to a temporary file and runs
// flux build kustomization --dry-run --in-memory-build on its path in the source root.
// It returns the rendered objects. A CRD layer renders without substitution, as in the cluster.
func (l *BaseLister) renderKustomization(blueprint *blueprintv1alpha1.Blueprint, k blueprintv1alpha1.Kustomization) ([]renderedObject, error) {
	namespace := l.runtime.ConfigHandler.GetString("gitops.namespace", constants.DefaultGitopsNamespace)
	mode := constants.ParseGitopsMode(l.runtime.ConfigHandler.GetString("gitops.mode", ""))
	fluxK := k.ToFluxKustomization(namespace, blueprint.Metadata.Name, blueprint.Sources, mode)
	fluxK.Spec.PostBuild = nil
	if !blueprintv1alpha1.IsCrdLayerName(k.Name) {
		if values := substituteValues(blueprint, k); len(values) > 0 {
			fluxK.Spec.PostBuild = &kustomizev1.PostBuild{Substitute: values}
		}
	}

	sourceRoot, err := l.sourceRoot(blueprint, k.Source, "kustomization "+k.Name)
	if err != nil {
		return nil, err
	}

	data, err := sigsyaml.Marshal(fluxK)
	if err != nil {
		return nil, werror.Wrap(err, "failed to marshal kustomization %q", k.Name)
	}
	tmpDir, err := l.shims.MkdirTemp("", "windsor-artifacts-")
	if err != nil {
		return nil, werror.Wrap(err, "failed to create a temporary directory")
	}
	defer l.shims.RemoveAll(tmpDir)
	kustomizationFile := filepath.Join(tmpDir, k.Name+".yaml")
	if err := l.shims.WriteFile(kustomizationFile, data, 0600); err != nil {
		return nil, werror.Wrap(err, "failed to write kustomization %q", k.Name)
	}

	out, err := l.runtime.Shell.ExecCaptureWithEnv("flux", map[string]string{"NO_COLOR": "1"},
		"build", "kustomization", k.Name,
		"--namespace", fluxK.Namespace,
		"--path", filepath.Join(sourceRoot, filepath.FromSlash(fluxK.Spec.Path)),
		"--kustomization-file", kustomizationFile,
		"--dry-run", "--in-memory-build")
	if err != nil {
		return nil, werror.Wrap(err, "failed to render kustomization %q", k.Name)
	}

	objects, err := decodeObjects(out)
	if err != nil {
		return nil, werror.Wrap(err, "failed to read the render of kustomization %q", k.Name)
	}
	rendered := make([]renderedObject, 0, len(objects))
	for _, obj := range objects {
		rendered = append(rendered, renderedObject{kustomization: k.Name, object: obj})
	}
	return rendered, nil
}

// sourceRoot returns the local directory that holds the source sourceName of owner. An oci://
// source resolves to its directory in the OCI cache. Every other source resolves to the project root.
func (l *BaseLister) sourceRoot(blueprint *blueprintv1alpha1.Blueprint, sourceName, owner string) (string, error) {
	if sourceName == "" || (sourceName == "template" && !blueprintv1alpha1.HasRemoteTemplateSource(blueprint.Sources)) {
		sourceName = blueprint.Metadata.Name
	}
	for _, source := range blueprint.Sources {
		if source.Name != sourceName || !strings.HasPrefix(source.Url, "oci://") {
			continue
		}
		registry, repository, tag, err := l.artifactBuilder.ParseOCIRef(source.Url)
		if err != nil {
			return "", werror.Wrap(err, "failed to parse source %q of %s", source.Name, owner)
		}
		return l.artifactBuilder.GetCacheDir(registry, repository, tag)
	}
	return l.runtime.ProjectRoot, nil
}

// =============================================================================
// Helpers
// =============================================================================

// substituteValues returns the substitution values of k in the order that postBuild.substituteFrom
// lists them: the substitutions of k, then every blueprint ConfigMap in name order. A later source
// wins, as in Flux. A deferred value becomes the deferred marker.
func substituteValues(blueprint *blueprintv1alpha1.Blueprint, k blueprintv1alpha1.Kustomization) map[string]string {
	values := make(map[string]string)
	for key, value := range k.Substitutions {
		values[key] = value
	}
	names := make([]string, 0, len(blueprint.ConfigMaps))
	for name := range blueprint.ConfigMaps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for key, value := range blueprint.ConfigMaps[name] {
			values[key] = value
		}
	}
	for key, value := range values {
		if value == blueprintcomposer.DeferredPlaceholder {
			values[key] = deferredMarker
		}
	}
	return values
}

// decodeObjects decodes multi-document YAML into objects. It skips empty documents.
func decodeObjects(data string) ([]map[string]any, error) {
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(data)), 4096)
	var objects []map[string]any
	for {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			if err == io.EOF {
				return objects, nil
			}
			return nil, err
		}
		if len(obj) == 0 {
			continue
		}
		objects = append(objects, obj)
	}
}
