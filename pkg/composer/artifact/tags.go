// The artifact tag scanner reads the `artifact` comment tags of a blueprint source tree.
// It provides one ArtifactTag for each chart or image that a tag names, with its version.
// A tag binds to one line: the next code line below a tag-only comment, or the line that ends with the tag.
// It reads kustomize, terraform, facet and values files. It needs no rendering and no network.

package artifact

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
)

// =============================================================================
// Constants
// =============================================================================

// ArtifactTagType is the type of artifact that a tag names.
type ArtifactTagType string

const (
	// ArtifactTagImage is a container image. Its name is the image repository.
	ArtifactTagImage ArtifactTagType = "image"

	// ArtifactTagChart is a Helm chart. Its name is the chart name and its repo is the repository URL.
	ArtifactTagChart ArtifactTagType = "chart"
)

// artifactTagPattern matches an artifact tag comment in YAML (`# artifact: ...`) or HCL (`// artifact: ...`).
var artifactTagPattern = regexp.MustCompile(`(?:#|//)\s*artifact:\s+(\w+=\S.*)$`)

// versionLinePattern splits a `key: value` or `key = value` line into its value.
var versionLinePattern = regexp.MustCompile(`^\s*(?:-\s*)?[\w.-]+\s*[:=]\s*(.+)$`)

// referrersPattern matches the referrers attribute of a tag body. It also matches an empty value.
var referrersPattern = regexp.MustCompile(`(?:^|\s)referrers=(\S*)`)

// tagRoots are the directories of a source that the scan reads. The contexts directory holds the
// facets and values files of a project. The _template directory holds them in a bundled source.
var tagRoots = []string{"kustomize", "terraform", "contexts", "_template"}

// =============================================================================
// Types
// =============================================================================

// ArtifactTag is one chart or image that an `artifact` tag names.
type ArtifactTag struct {
	Type        ArtifactTagType
	Name        string
	Repo        string
	Version     string
	Digest      string
	NoReferrers bool
	File        string
	Line        int
}

// =============================================================================
// Public Methods
// =============================================================================

// ScanTags returns the artifact tags in the content of one file. A tag on a comment line of its own
// binds to the next line that is not blank and not a comment. A tag at the end of a line binds to
// that line. The value is the last scalar on the bound line, and an @sha256 suffix gives the digest.
// It returns an error for a tag with no value, an unknown type, no name and no full image reference,
// a chart with no repo, a referrers value that is not true or false, or a chart with referrers.
func ScanTags(filePath string, content []byte) ([]ArtifactTag, error) {
	lines := strings.Split(string(content), "\n")
	var tags []ArtifactTag
	for i, line := range lines {
		loc := artifactTagPattern.FindStringSubmatchIndex(line)
		if loc == nil {
			continue
		}
		tag, err := parseTag(line[loc[2]:loc[3]])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filePath, i+1, err)
		}
		value, valueLine := "", 0
		if code := strings.TrimSpace(line[:loc[0]]); code != "" {
			value, valueLine = scalarOf(code), i+1
		} else {
			value, valueLine = nextValueLine(lines, i+1)
		}
		if value == "" {
			return nil, fmt.Errorf("%s:%d: the artifact tag has no value on the line it binds to", filePath, i+1)
		}
		if err := tag.bind(value); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filePath, i+1, err)
		}
		tag.File = filePath
		tag.Line = valueLine
		tags = append(tags, tag)
	}
	return tags, nil
}

// ScanTagsDir returns the artifact tags in the .yaml, .yml and .tf files under the kustomize,
// terraform, contexts and _template directories of root. This covers facets and values files.
// A root with none of these directories gives no tags. File paths are relative to root.
func ScanTagsDir(root string) ([]ArtifactTag, error) {
	fsys := os.DirFS(root)
	var tags []ArtifactTag
	for _, dir := range tagRoots {
		err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				if d.Name() == ".terraform" || d.Name() == ".git" {
					return fs.SkipDir
				}
				return nil
			}
			if !isTagPath(path) {
				return nil
			}
			content, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			found, err := ScanTags(path, content)
			if err != nil {
				return err
			}
			tags = append(tags, found...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].File != tags[j].File {
			return tags[i].File < tags[j].File
		}
		return tags[i].Line < tags[j].Line
	})
	return tags, nil
}

// =============================================================================
// Helpers
// =============================================================================

// isTagPath reports whether the tag scan reads the file at path. It accepts YAML and Terraform
// files under the kustomize, terraform, contexts and _template directories.
func isTagPath(path string) bool {
	for _, dir := range tagRoots {
		if strings.HasPrefix(path, dir+"/") {
			return hasTagExtension(path)
		}
	}
	return false
}

// hasTagExtension reports whether the file at path is a YAML or Terraform file.
func hasTagExtension(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".tf")
}

// parseTag reads the key=value pairs of a tag body and checks them.
func parseTag(body string) (ArtifactTag, error) {
	var tag ArtifactTag
	for _, kv := range renovateKeyValuePattern.FindAllStringSubmatch(body, -1) {
		switch kv[1] {
		case "type":
			tag.Type = ArtifactTagType(kv[2])
		case "name":
			tag.Name = kv[2]
		case "repo":
			tag.Repo = kv[2]
		}
	}
	if tag.Type != ArtifactTagImage && tag.Type != ArtifactTagChart {
		return tag, fmt.Errorf("the artifact tag has type %q, and the type must be image or chart", tag.Type)
	}
	if tag.Type == ArtifactTagChart {
		if tag.Name == "" {
			return tag, fmt.Errorf("the artifact tag for a chart has no name")
		}
		if !strings.HasPrefix(tag.Repo, "https://") && !strings.HasPrefix(tag.Repo, "http://") && !strings.HasPrefix(tag.Repo, "oci://") {
			return tag, fmt.Errorf("the artifact tag for chart %q needs a repo that starts with https:// or oci://", tag.Name)
		}
	}
	if m := referrersPattern.FindStringSubmatch(body); m != nil {
		if tag.Type == ArtifactTagChart {
			return tag, fmt.Errorf("the artifact tag for chart %q has referrers, and only an image tag can have referrers", tag.Name)
		}
		if m[1] != "true" && m[1] != "false" {
			return tag, fmt.Errorf("the artifact tag has referrers %q, and the value must be true or false", m[1])
		}
		tag.NoReferrers = m[1] == "false"
	}
	return tag, nil
}

// bind sets the version, the digest and, for an image tag with no name, the name from value.
// An image value is a tag or a full reference with a tag. A tag with no name needs a full reference.
func (t *ArtifactTag) bind(value string) error {
	ref, digest := splitTagAndDigest(value)
	t.Digest = digest
	if t.Type == ArtifactTagChart {
		t.Version = ref
		return nil
	}
	idx := strings.LastIndex(ref, ":")
	if idx <= strings.LastIndex(ref, "/") {
		idx = -1
	}
	if idx < 0 && digest != "" && strings.Contains(ref, "/") {
		if t.Name == "" {
			t.Name = ref
		}
		return nil
	}
	if idx < 0 && t.Name == "" && strings.Contains(ref, "/") {
		return fmt.Errorf("the artifact tag value %q has no tag", value)
	}
	if idx >= 0 {
		t.Version = ref[idx+1:]
		if t.Name == "" {
			t.Name = ref[:idx]
		}
	} else {
		t.Version = ref
	}
	if t.Name == "" {
		return fmt.Errorf("the artifact tag has no name and %q is not a full image reference", value)
	}
	if t.Version == "" {
		return fmt.Errorf("the artifact tag value %q has no version", value)
	}
	return nil
}

// nextValueLine returns the last scalar on the first line from start that is not blank or a comment,
// and its 1-based line number.
func nextValueLine(lines []string, start int) (string, int) {
	for j := start; j < len(lines); j++ {
		line := strings.TrimSpace(lines[j])
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		return scalarOf(line), j + 1
	}
	return "", 0
}

// scalarOf returns the last scalar of a line of code: the text after the key, without a trailing
// comment, quotes or comma. A line with no key is a list item or a bare value.
func scalarOf(line string) string {
	value := strings.TrimPrefix(strings.TrimSpace(line), "- ")
	if m := versionLinePattern.FindStringSubmatch(line); m != nil {
		value = m[1]
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = value[:idx]
	}
	value = strings.Trim(strings.TrimSpace(value), `"',`)
	if fields := strings.Fields(value); len(fields) > 1 {
		value = strings.Trim(fields[len(fields)-1], `"',`)
	}
	return value
}
