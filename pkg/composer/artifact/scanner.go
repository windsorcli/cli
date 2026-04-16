package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The Scanner walks blueprint source files for Renovate annotations and emits
// ManifestEntry records describing the artifacts they pin. It relies solely on
// Renovate's declarative comment convention — `# renovate: datasource=... depName=...`
// followed by the pinned value on a nearby line — which makes the scan
// deterministic, offline, and independent of adaptive composition logic or
// Helm chart rendering.

// =============================================================================
// Constants
// =============================================================================

// annotationLookahead is the number of lines examined after a Renovate
// comment before giving up on locating the pinned value. A small window keeps
// the scanner robust to ordinary intervening whitespace or minor comments
// without drifting into unrelated declarations.
const annotationLookahead = 5

// renovateCommentPattern matches a Renovate annotation comment in either YAML
// (`# renovate: ...`) or HCL (`// renovate: ...` / `# renovate: ...`) style.
var renovateCommentPattern = regexp.MustCompile(`(?:#|//)\s*renovate:\s*(.+)$`)

// renovateKeyValuePattern splits a single `key=value` pair inside a Renovate
// annotation. Values may contain colons (URLs) but never whitespace.
var renovateKeyValuePattern = regexp.MustCompile(`(\w+)=(\S+)`)

// pinnedValuePattern matches YAML (`key: value`) or HCL (`key = value`) lines
// that carry the pinned version/tag value following a Renovate annotation.
// Supported keys are constrained to the set Renovate emits annotations for.
// Commas are excluded from the value class so HCL trailing-comma assignments
// (e.g. `tag = "v1.0.0",` in tuple contexts) do not absorb the comma into
// the captured version.
var pinnedValuePattern = regexp.MustCompile(`^\s*(tag|version|default|chart|image)\s*[:=]\s*["']?([^"'\s,]+)["']?`)

// =============================================================================
// Types
// =============================================================================

// renovateAnnotation captures the key=value pairs parsed from a single
// `# renovate:` comment line, along with the file location that produced it.
type renovateAnnotation struct {
	datasource string
	depName    string
	pkg        string
	helmRepo   string
	file       string
}

// =============================================================================
// Public Methods
// =============================================================================

// ScanProject walks the `kustomize/` and `terraform/` directories under the
// given project root for Renovate annotations and returns an ArtifactManifest
// covering every docker image and helm chart declared in the local project.
// Unlike Bundle-based scanning, this path is read-only and skips the full
// artifact construction pipeline so callers (e.g. `windsor mirror`) can
// discover local dependencies without composing blueprints or building tarballs.
// Missing directories are treated as empty; .terraform caches are skipped.
func ScanProject(projectRoot string) (*ArtifactManifest, error) {
	entries := make([]ManifestEntry, 0)
	for _, dir := range []string{"kustomize", "terraform"} {
		root := filepath.Join(projectRoot, dir)
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to stat %s: %w", root, err)
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == ".terraform" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel(projectRoot, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			if !isScannablePath(rel) {
				return nil
			}
			data, readErr := os.ReadFile(path) // #nosec G304 -- path is constrained to projectRoot kustomize/terraform subtrees
			if readErr != nil {
				return readErr
			}
			entries = append(entries, scanRenovateAnnotations(rel, data)...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan %s: %w", root, err)
		}
	}
	sortManifestEntries(entries)
	entries = dedupeManifestEntries(entries)
	return &ArtifactManifest{Version: ManifestVersion, Artifacts: entries}, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// scanRenovateAnnotations walks the content of a single blueprint source file
// and returns one ManifestEntry per Renovate annotation for which a pinned
// value can be located in the next few lines. File paths are passed through
// unchanged so callers can record blueprint-relative provenance.
func scanRenovateAnnotations(filePath string, content []byte) []ManifestEntry {
	lines := strings.Split(string(content), "\n")
	entries := make([]ManifestEntry, 0)

	for i, line := range lines {
		match := renovateCommentPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		annotation := parseRenovateAnnotation(match[1], filePath)
		if annotation.datasource == "" {
			continue
		}
		value, valueLine := findPinnedValue(lines, i+1, annotationLookahead)
		if value == "" {
			continue
		}
		entry := buildManifestEntry(annotation, value, valueLine)
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	return entries
}

// dedupeManifestEntries collapses duplicate entries — same reference and
// digest — into a single record, preserving the first occurrence so
// provenance points at the primary declaration.
func dedupeManifestEntries(entries []ManifestEntry) []ManifestEntry {
	seen := make(map[string]struct{}, len(entries))
	result := make([]ManifestEntry, 0, len(entries))
	for _, e := range entries {
		key := string(e.Type) + "|" + e.Reference + "|" + e.Digest
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, e)
	}
	return result
}

// isScannablePath reports whether a bundled file path is a source file the
// scanner should inspect. Only YAML and Terraform sources under the
// `kustomize/` or `terraform/` bundle roots carry Renovate annotations in
// Windsor blueprints; template and context files are excluded so that
// unrelated YAML fragments cannot leak into the artifact manifest.
func isScannablePath(path string) bool {
	if !strings.HasPrefix(path, "kustomize/") && !strings.HasPrefix(path, "terraform/") {
		return false
	}
	return strings.HasSuffix(path, ".yaml") ||
		strings.HasSuffix(path, ".yml") ||
		strings.HasSuffix(path, ".tf")
}

// sortManifestEntries orders entries deterministically by type, reference,
// and source file so that manifests produced from the same inputs are
// byte-identical regardless of filesystem walk order.
func sortManifestEntries(entries []ManifestEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type < entries[j].Type
		}
		if entries[i].Reference != entries[j].Reference {
			return entries[i].Reference < entries[j].Reference
		}
		return entries[i].Source.File < entries[j].Source.File
	})
}

// =============================================================================
// Helpers
// =============================================================================

// parseRenovateAnnotation extracts recognized key=value pairs from the body of
// a Renovate comment. Unknown keys are ignored so future Renovate extensions
// do not break scanning.
func parseRenovateAnnotation(body, filePath string) renovateAnnotation {
	a := renovateAnnotation{file: filePath}
	for _, kv := range renovateKeyValuePattern.FindAllStringSubmatch(body, -1) {
		switch kv[1] {
		case "datasource":
			a.datasource = kv[2]
		case "depName":
			a.depName = kv[2]
		case "package":
			a.pkg = kv[2]
		case "helmRepo":
			a.helmRepo = kv[2]
		}
	}
	return a
}

// findPinnedValue scans up to lookahead lines after start, returning the first
// pinned value it locates and the 1-based line number on which it appeared.
// Empty string indicates no match within the window.
func findPinnedValue(lines []string, start, lookahead int) (string, int) {
	end := min(start+lookahead, len(lines))
	for j := start; j < end; j++ {
		if m := pinnedValuePattern.FindStringSubmatch(lines[j]); m != nil {
			return m[2], j + 1
		}
	}
	return "", 0
}

// buildManifestEntry constructs a ManifestEntry from a parsed annotation and
// its discovered pinned value, applying datasource-specific conventions for
// reference, version, digest, repository, and transport fields. When the
// pinned value already looks like a full image reference (contains a path
// separator or tag colon), it is adopted verbatim rather than concatenated
// onto depName, which prevents references like "alpine:alpine:3.21.5".
func buildManifestEntry(a renovateAnnotation, value string, valueLine int) *ManifestEntry {
	var typ ArtifactType
	var transport ArtifactTransport
	switch a.datasource {
	case "docker":
		typ = ArtifactTypeDocker
		transport = ArtifactTransportOCI
	case "helm":
		typ = ArtifactTypeHelm
		transport = ArtifactTransportOCI
	case "github-releases":
		typ = ArtifactTypeGitHubRelease
		transport = ArtifactTransportTarball
	case "git-tags":
		typ = ArtifactTypeGitTag
		transport = ArtifactTransportGit
	default:
		return nil
	}

	entry := ManifestEntry{
		Type:      typ,
		Transport: transport,
		Source: ArtifactSource{
			File:       a.file,
			Line:       valueLine,
			Datasource: a.datasource,
			DepName:    a.depName,
			Package:    a.pkg,
			HelmRepo:   a.helmRepo,
		},
	}

	tag, digest := splitTagAndDigest(value)

	if isIncusAliasReference(tag) {
		return nil
	}

	switch typ {
	case ArtifactTypeDocker:
		entry.Repository = a.depName
		entry.Digest = digest
		entry.Reference, entry.Version = composeDockerReference(a.depName, tag)
	case ArtifactTypeHelm:
		entry.Repository = a.helmRepo
		entry.Version = tag
		entry.Reference = a.depName
	case ArtifactTypeGitHubRelease, ArtifactTypeGitTag:
		ref := a.pkg
		if ref == "" {
			ref = a.depName
		}
		entry.Reference = ref
		entry.Version = tag
	}

	return &entry
}

// composeDockerReference returns the canonical image reference and the
// extracted version for a captured docker pin. When the pinned value is a
// bare tag, it is appended to depName and also returned as the version.
// When the value already encodes a full reference (contains "/" or ":" —
// as happens with `image: alpine:3.21.5` in Helm values), it is used
// verbatim as the reference, and the version is the portion following the
// last colon so the Version field does not duplicate the repo path.
func composeDockerReference(depName, tag string) (reference, version string) {
	if strings.ContainsAny(tag, "/:") {
		if idx := strings.LastIndex(tag, ":"); idx >= 0 {
			return tag, tag[idx+1:]
		}
		return tag, ""
	}
	return depName + ":" + tag, tag
}

// isIncusAliasReference reports whether a pinned value is an Incus-style
// remote-alias reference of the form "<alias>:<path>:<version>" rather than
// a canonical OCI ref. Incus remote aliases are chosen locally by each user
// (e.g. "ghcr", "registryk8s"), so they are not mirrorable by a generic OCI
// hydrate path and must be excluded from the manifest.
//
// Heuristic: two or more colons and a first segment that contains no "." or
// "/". The common false positive is an OCI "host:port/repo:tag" reference
// (e.g. "localhost:5000/foo:v1"), which also has two colons and a bare
// prefix — but the segment between the first and second colons is a numeric
// port followed by a path separator, which distinguishes it from an Incus
// alias body (typically a package path).
func isIncusAliasReference(value string) bool {
	if strings.Count(value, ":") < 2 {
		return false
	}
	firstColon := strings.Index(value, ":")
	prefix := value[:firstColon]
	if strings.ContainsAny(prefix, "./") {
		return false
	}
	after := value[firstColon+1:]
	if slash := strings.Index(after, "/"); slash > 0 && isAllDigits(after[:slash]) {
		return false
	}
	return true
}

// isAllDigits reports whether s is non-empty and composed solely of ASCII
// digit characters. Used to detect OCI host:port prefixes during Incus alias
// disambiguation.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// splitTagAndDigest separates an `@sha256:...` suffix from the tag portion of
// a container image reference value. Returns (tag, digest); digest is empty
// when no suffix is present.
func splitTagAndDigest(value string) (string, string) {
	if idx := strings.Index(value, "@sha256:"); idx >= 0 {
		return value[:idx], value[idx+1:]
	}
	return value, ""
}
