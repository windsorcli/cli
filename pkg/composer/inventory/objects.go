// The object scan is the read step of the Lister.
// It provides the chart and image entries that a set of rendered Kubernetes objects names.
// A chart comes from a chart source object, and an image comes from a pod template.
// The scan reads no annotations and knows no blueprint value names.

package inventory

import (
	"fmt"
	"strings"
)

// =============================================================================
// Constants
// =============================================================================

// helmChartMediaType is the layer media type of a Helm chart in an OCI artifact.
const helmChartMediaType = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"

// =============================================================================
// Types
// =============================================================================

// imageReference is a container image reference split into its parts.
// repository holds the registry host, and Docker Hub images use docker.io.
type imageReference struct {
	repository string
	tag        string
	digest     string
}

// =============================================================================
// Private Methods
// =============================================================================

// id returns the object as Kind/namespace/name, or Kind/name when it has no namespace.
func (o renderedObject) id() string {
	kind, _ := o.object["kind"].(string)
	return objectID(kind, nestedString(o.object, "metadata", "namespace"), nestedString(o.object, "metadata", "name"))
}

// source returns the Source of an entry that o declares. A derived object names its parent.
func (o renderedObject) source() Source {
	object := o.id()
	if o.parent != "" {
		object = o.parent
	}
	return Source{Kustomization: o.kustomization, Object: object, File: o.file}
}

// where names the Kustomization or the Terraform file that declares o, for a message.
func (o renderedObject) where() string {
	return o.source().where()
}

// where names the Terraform file of s, or its Kustomization, for a message.
func (s Source) where() string {
	if s.File != "" {
		return s.File
	}
	return fmt.Sprintf("kustomization %q", s.Kustomization)
}

// =============================================================================
// Helpers
// =============================================================================

// scanObjects returns the chart and image entries of objects, and a note for each chart source
// that it cannot pin to one version or resolve to a repository.
func scanObjects(objects []renderedObject) ([]Entry, []string) {
	index := indexObjects(objects)
	chartRefs := make(map[string]bool)
	for _, obj := range objects {
		if kind, _ := obj.object["kind"].(string); kind == "HelmRelease" {
			if ref := chartRefID(obj); ref != "" {
				chartRefs[ref] = true
			}
		}
	}

	var entries []Entry
	var notes []string
	for _, obj := range objects {
		kind, _ := obj.object["kind"].(string)
		switch kind {
		case "OCIRepository":
			if nestedString(obj.object, "spec", "layerSelector", "mediaType") != helmChartMediaType && !chartRefs[obj.id()] {
				continue
			}
			entry, note := ociChartEntry(obj)
			entries = append(entries, entry)
			notes = appendNote(notes, note)
		case "HelmRelease":
			entry, ok, note := helmReleaseChartEntry(obj, index)
			if ok {
				entries = append(entries, entry)
			}
			notes = appendNote(notes, note)
		}
		for _, image := range podTemplateImages(obj.object) {
			ref := parseImageReference(image)
			entries = append(entries, Entry{
				Kind:      KindImage,
				Transport: TransportOCI,
				Reference: ref.repository,
				Version:   ref.tag,
				Digest:    ref.digest,
				Source:    obj.source(),
			})
		}
	}
	return entries, notes
}

// releaseChart returns the chart entry of a HelmRelease. A spec.chartRef names an OCIRepository in
// index, and a spec.chart names a HelmRepository or GitRepository in index. It returns false when
// the source is not in index.
func releaseChart(hr renderedObject, index map[string]renderedObject) (Entry, bool) {
	if ref := chartRefID(hr); ref != "" {
		source, found := index[ref]
		if !found {
			return Entry{}, false
		}
		entry, _ := ociChartEntry(source)
		return entry, true
	}
	entry, ok, _ := helmReleaseChartEntry(hr, index)
	return entry, ok
}

// indexObjects returns objects by their id. A later object with the same id replaces an earlier one.
func indexObjects(objects []renderedObject) map[string]renderedObject {
	index := make(map[string]renderedObject, len(objects))
	for _, obj := range objects {
		index[obj.id()] = obj
	}
	return index
}

// ociChartEntry returns the chart entry of an OCIRepository. The chart name is the last path
// segment of its URL, and the version is its ref.tag. A ref.semver range gives a note.
func ociChartEntry(obj renderedObject) (Entry, string) {
	repository := strings.TrimSuffix(strings.TrimPrefix(nestedString(obj.object, "spec", "url"), "oci://"), "/")
	entry := Entry{
		Kind:      KindChart,
		Transport: TransportOCI,
		Reference: repository,
		Name:      repository[strings.LastIndex(repository, "/")+1:],
		Version:   nestedString(obj.object, "spec", "ref", "tag"),
		Digest:    nestedString(obj.object, "spec", "ref", "digest"),
		Source:    obj.source(),
	}
	if entry.Version == "" {
		return entry, fmt.Sprintf("%s in %s has no ref.tag, so the chart has no destination.", obj.id(), obj.where())
	}
	return entry, ""
}

// helmReleaseChartEntry returns the chart entry of a HelmRelease that uses spec.chart. The
// HelmRepository or GitRepository it names must be in index. A HelmRelease with spec.chartRef
// returns false, because its OCIRepository gives the entry.
func helmReleaseChartEntry(obj renderedObject, index map[string]renderedObject) (Entry, bool, string) {
	chart := nestedString(obj.object, "spec", "chart", "spec", "chart")
	if chart == "" {
		return Entry{}, false, ""
	}
	sourceKind := nestedString(obj.object, "spec", "chart", "spec", "sourceRef", "kind")
	sourceName := nestedString(obj.object, "spec", "chart", "spec", "sourceRef", "name")
	sourceNamespace := nestedString(obj.object, "spec", "chart", "spec", "sourceRef", "namespace")
	if sourceNamespace == "" {
		sourceNamespace = nestedString(obj.object, "metadata", "namespace")
	}
	sourceID := objectID(sourceKind, sourceNamespace, sourceName)
	source, found := index[sourceID]
	if !found {
		return Entry{}, false, fmt.Sprintf("%s in %s uses %s, which no rendered Kustomization holds.", obj.id(), obj.kustomization, sourceID)
	}

	version := nestedString(obj.object, "spec", "chart", "spec", "version")
	url := nestedString(source.object, "spec", "url")
	entry := Entry{Kind: KindChart, Name: chart, Version: version, Source: obj.source()}
	switch {
	case sourceKind == "GitRepository":
		entry.Transport = TransportGit
		entry.Reference = url
		entry.Version = gitRevision(source.object)
		return entry, true, fmt.Sprintf("%s in %s builds chart %q from Git, so the chart has no destination.", obj.id(), obj.kustomization, chart)
	case sourceKind != "HelmRepository":
		return Entry{}, false, fmt.Sprintf("%s in %s uses a %s, which the inventory does not read.", obj.id(), obj.kustomization, sourceKind)
	case nestedString(source.object, "spec", "type") == "oci" || strings.HasPrefix(url, "oci://"):
		entry.Transport = TransportOCI
		entry.Reference = strings.TrimSuffix(strings.TrimPrefix(url, "oci://"), "/") + "/" + chart
	default:
		entry.Transport = TransportHTTP
		entry.Reference = strings.TrimSuffix(url, "/")
	}
	if !isExactVersion(version) {
		return entry, true, fmt.Sprintf("%s in %s does not pin chart %q to one version, so the chart has no destination.", obj.id(), obj.kustomization, chart)
	}
	return entry, true, ""
}

// chartRefID returns the OCIRepository that a HelmRelease names in spec.chartRef, or "".
func chartRefID(obj renderedObject) string {
	if nestedString(obj.object, "spec", "chartRef", "kind") != "OCIRepository" {
		return ""
	}
	namespace := nestedString(obj.object, "spec", "chartRef", "namespace")
	if namespace == "" {
		namespace = nestedString(obj.object, "metadata", "namespace")
	}
	return objectID("OCIRepository", namespace, nestedString(obj.object, "spec", "chartRef", "name"))
}

// objectID returns Kind/namespace/name, or Kind/name when namespace is empty.
func objectID(kind, namespace, name string) string {
	if namespace == "" {
		return kind + "/" + name
	}
	return kind + "/" + namespace + "/" + name
}

// gitRevision returns the tag, semver, commit or branch of a GitRepository, in that order.
func gitRevision(obj map[string]any) string {
	for _, field := range []string{"tag", "semver", "commit", "branch"} {
		if v := nestedString(obj, "spec", "ref", field); v != "" {
			return v
		}
	}
	return ""
}

// podTemplateImages returns every container, init container and ephemeral container image in the
// pod spec of a Pod, Deployment, StatefulSet, DaemonSet, ReplicaSet, Job or CronJob.
func podTemplateImages(obj map[string]any) []string {
	var podSpec map[string]any
	switch kind, _ := obj["kind"].(string); kind {
	case "Pod":
		podSpec = nestedMap(obj, "spec")
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		podSpec = nestedMap(obj, "spec", "template", "spec")
	case "CronJob":
		podSpec = nestedMap(obj, "spec", "jobTemplate", "spec", "template", "spec")
	}
	if podSpec == nil {
		return nil
	}
	var images []string
	for _, field := range []string{"initContainers", "containers", "ephemeralContainers"} {
		containers, _ := podSpec[field].([]any)
		for _, c := range containers {
			container, _ := c.(map[string]any)
			if image, _ := container["image"].(string); image != "" {
				images = append(images, image)
			}
		}
	}
	return images
}

// parseImageReference splits an image reference into repository, tag and digest. A reference
// with no registry host gets docker.io, and a one-segment Docker Hub path gets library/.
func parseImageReference(ref string) imageReference {
	var out imageReference
	if at := strings.Index(ref, "@"); at >= 0 {
		out.digest = ref[at+1:]
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		out.tag = ref[colon+1:]
		ref = ref[:colon]
	}
	first, rest, hasSlash := strings.Cut(ref, "/")
	if !hasSlash || (!strings.ContainsAny(first, ".:") && first != "localhost") {
		first, rest = "docker.io", ref
	}
	if first == "docker.io" && !strings.Contains(rest, "/") {
		rest = "library/" + rest
	}
	out.repository = first + "/" + rest
	return out
}

// isExactVersion reports whether version names one chart version and not a semver range.
func isExactVersion(version string) bool {
	if version == "" || strings.ContainsAny(version, "<>=~^*|, ") {
		return false
	}
	for _, part := range strings.Split(version, ".") {
		if part == "x" || part == "X" {
			return false
		}
	}
	return true
}

// appendNote appends note to notes when note is not empty.
func appendNote(notes []string, note string) []string {
	if note == "" {
		return notes
	}
	return append(notes, note)
}

// nestedMap returns the map at path in obj, or nil.
func nestedMap(obj map[string]any, path ...string) map[string]any {
	current := obj
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// nestedString returns the string at path in obj, or "".
func nestedString(obj map[string]any, path ...string) string {
	if len(path) == 0 {
		return ""
	}
	parent := nestedMap(obj, path[:len(path)-1]...)
	if parent == nil {
		return ""
	}
	s, _ := parent[path[len(path)-1]].(string)
	return s
}
