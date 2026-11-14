// The RegistryDigests looks up the digest that the tag of an artifact names in its registry now.
// It provides the digests that windsor bundle writes into the BOM for tags with no @sha256 suffix.
// It reads an OCI artifact from the registry that its reference names, and an HTTP chart from the repository index.
// It keeps each chart index for the life of the value, so one repository costs one download.

package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/composer/artifact"
)

// =============================================================================
// Constants
// =============================================================================

// maxDigestLookups is the largest number of digest lookups that run at the same time.
const maxDigestLookups = 8

// httpTimeout is the time limit of one registry lookup, and of one download of a chart index or archive.
const httpTimeout = 2 * time.Minute

// =============================================================================
// Types
// =============================================================================

// RegistryDigests is the default DigestLookup.
type RegistryDigests struct {
	client  *http.Client
	mu      sync.Mutex
	indexes map[string]*chartIndex
}

// chartIndex is one downloaded Helm repository index, or the error of its download.
type chartIndex struct {
	once    sync.Once
	entries map[string][]chartVersion
	err     error
}

// chartVersion is one chart version in a Helm repository index.
type chartVersion struct {
	Version string   `yaml:"version"`
	Digest  string   `yaml:"digest"`
	URLs    []string `yaml:"urls"`
}

// =============================================================================
// Interfaces
// =============================================================================

// DigestLookup returns the digest that the tag of an artifact names in its registry now.
type DigestLookup interface {
	Digest(e Entry) (string, error)
}

// =============================================================================
// Constructor
// =============================================================================

// NewRegistryDigests creates a RegistryDigests.
func NewRegistryDigests() *RegistryDigests {
	return &RegistryDigests{
		client:  &http.Client{Timeout: httpTimeout},
		indexes: make(map[string]*chartIndex),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Digest returns the sha256 digest of e. An OCI image or chart gets the digest of its manifest or index.
// An HTTP chart gets the digest of its archive from the repository index, or the sha256 of the archive
// when the index gives none. It returns an error for an artifact that it cannot read, and for a Git chart.
func (d *RegistryDigests) Digest(e Entry) (string, error) {
	switch e.Transport {
	case TransportOCI:
		return d.ociDigest(e.Reference + ":" + e.Version)
	case TransportHTTP:
		return d.chartDigest(e.Reference, e.Name, e.Version)
	default:
		return "", fmt.Errorf("artifact %s has no registry to read a digest from", e.Upstream())
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// ociDigest returns the digest of the manifest or index that ref names.
// It uses the Docker keychain, and tries again without credentials when the registry rejects them.
// The lookup stops after httpTimeout.
func (d *RegistryDigests) ociDigest(ref string) (string, error) {
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return "", werror.Wrap(err, "invalid reference %s", ref)
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	desc, err := remote.Head(parsed, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil && artifact.IsAuthenticationError(err) {
		if anon, anonErr := remote.Head(parsed, remote.WithContext(ctx), remote.WithAuth(authn.Anonymous)); anonErr == nil {
			desc, err = anon, nil
		}
	}
	if err != nil {
		return "", werror.Wrap(err, "failed to read the digest of %s", parsed.String())
	}
	return desc.Digest.String(), nil
}

// chartDigest returns the digest of the archive of chart chartName at version in the Helm repository repo.
// A version with a leading v matches an index version without it, and the other way around.
func (d *RegistryDigests) chartDigest(repo, chartName, version string) (string, error) {
	index := d.index(repo)
	if index.err != nil {
		return "", index.err
	}
	for _, v := range index.entries[chartName] {
		if strings.TrimPrefix(v.Version, "v") != strings.TrimPrefix(version, "v") {
			continue
		}
		if v.Digest != "" {
			return "sha256:" + strings.TrimPrefix(v.Digest, "sha256:"), nil
		}
		if len(v.URLs) == 0 {
			return "", fmt.Errorf("chart %s %s in %s has no digest and no URL", chartName, version, repo)
		}
		return d.archiveDigest(repo, v.URLs[0])
	}
	return "", fmt.Errorf("chart %s %s is not in the index of %s", chartName, version, repo)
}

// index returns the index of the Helm repository repo. It downloads each index once.
func (d *RegistryDigests) index(repo string) *chartIndex {
	d.mu.Lock()
	index, ok := d.indexes[repo]
	if !ok {
		index = &chartIndex{}
		d.indexes[repo] = index
	}
	d.mu.Unlock()
	index.once.Do(func() {
		data, err := d.download(repo + "/index.yaml")
		if err != nil {
			index.err = err
			return
		}
		var parsed struct {
			Entries map[string][]chartVersion `yaml:"entries"`
		}
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			index.err = werror.Wrap(err, "failed to parse the index of %s", repo)
			return
		}
		index.entries = parsed.Entries
	})
	return index
}

// archiveDigest returns the sha256 of the chart archive at archiveURL. A relative URL is relative to repo.
func (d *RegistryDigests) archiveDigest(repo, archiveURL string) (string, error) {
	base, err := url.Parse(repo + "/")
	if err != nil {
		return "", werror.Wrap(err, "invalid chart repository %s", repo)
	}
	ref, err := url.Parse(archiveURL)
	if err != nil {
		return "", werror.Wrap(err, "invalid chart URL %s", archiveURL)
	}
	data, err := d.download(base.ResolveReference(ref).String())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// download returns the body of a GET of target. It fails on a status other than 200.
func (d *RegistryDigests) download(target string) ([]byte, error) {
	resp, err := d.client.Get(target) // #nosec G107 -- the URL is a chart repository that a tag names
	if err != nil {
		return nil, werror.Wrap(err, "failed to download %s", target)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download %s: %s", target, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, werror.Wrap(err, "failed to download %s", target)
	}
	return data, nil
}

// =============================================================================
// Helpers
// =============================================================================

// resolveDigests returns a copy of tree in which each chart and image with an exact version and no digest
// has the digest that lookup gives. It reads each artifact once, at most maxDigestLookups at a time. It
// leaves out the components of a source BOM, a blueprint source, and a Git chart. It returns the error of
// the first artifact in tree order that lookup cannot read.
func resolveDigests(tree SourceTree, lookup DigestLookup) (SourceTree, error) {
	var keys []string
	pending := make(map[string]Entry)
	collectPending(tree, pending, &keys)

	digests := make([]string, len(keys))
	errs := make([]error, len(keys))
	slots := make(chan struct{}, maxDigestLookups)
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		slots <- struct{}{}
		go func(i int, e Entry) {
			defer wg.Done()
			defer func() { <-slots }()
			digests[i], errs[i] = lookup.Digest(e)
		}(i, pending[key])
	}
	wg.Wait()

	found := make(map[string]string, len(keys))
	for i, key := range keys {
		if errs[i] != nil {
			e := pending[key]
			return SourceTree{}, werror.Wrap(errs[i], "failed to read the digest of %s %s from its registry", e.Kind, e.Upstream())
		}
		found[key] = digests[i]
	}
	return withDigests(tree, found), nil
}

// collectPending adds each artifact of tree that needs a digest to pending, and its key to keys in tree order.
func collectPending(tree SourceTree, pending map[string]Entry, keys *[]string) {
	if tree.bom == nil {
		for _, e := range tree.Artifacts {
			if !needsDigest(e) {
				continue
			}
			key := digestKey(e)
			if _, ok := pending[key]; !ok {
				pending[key] = e
				*keys = append(*keys, key)
			}
		}
	}
	for _, source := range tree.Sources {
		collectPending(source, pending, keys)
	}
}

// withDigests returns a copy of tree in which each artifact whose key is in found has that digest.
func withDigests(tree SourceTree, found map[string]string) SourceTree {
	out := tree
	out.Artifacts = make([]Entry, len(tree.Artifacts))
	for i, e := range tree.Artifacts {
		if digest, ok := found[digestKey(e)]; ok && needsDigest(e) {
			e.Digest = digest
		}
		out.Artifacts[i] = e
	}
	out.Sources = make([]SourceTree, len(tree.Sources))
	for i, source := range tree.Sources {
		out.Sources[i] = withDigests(source, found)
	}
	return out
}

// needsDigest reports whether e is a chart or image with an exact version and no digest, in a registry.
func needsDigest(e Entry) bool {
	if e.Digest != "" || e.Kind == KindBlueprint || e.Transport == TransportGit {
		return false
	}
	return isExactVersion(e.Version)
}

// digestKey returns the key of the artifact that e names, the same for each tag of one artifact.
func digestKey(e Entry) string {
	return strings.Join([]string{string(e.Transport), e.Reference, e.Name, e.Version}, "\x00")
}
