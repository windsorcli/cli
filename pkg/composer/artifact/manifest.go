// The artifact manifest is the CycloneDX BOM that windsor bundle writes at the root of a blueprint artifact.
// It lists each chart and image that an artifact tag names in the bundled files and in each oci:// source.
// The artifact package scans the tags, and a BOMWriter from the composer encodes them.
// ReadCachedBOM and ReadCachedDigest read the manifest and the digest of a pulled source from its cache.

package artifact

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

// BOMFileName is the name of the CycloneDX BOM at the root of a blueprint artifact.
const BOMFileName = "artifact-manifest.cdx.json"

// maxBOMSize is the largest BOM, in bytes, that ReadCachedBOM reads.
const maxBOMSize = 64 << 20

// =============================================================================
// Types
// =============================================================================

// BOMWriter returns the BOM of a blueprint with the given name, version and artifact tags.
// blueprint is the content of the bundled _template/blueprint.yaml, or nil when the bundle has none.
type BOMWriter func(name, version string, tags []ArtifactTag, blueprint []byte) ([]byte, error)

// =============================================================================
// Helpers
// =============================================================================

// ReadCachedBOM returns the BOM in the artifact.tar of the cache directory cacheDir.
// It returns nil and no error when the archive holds no BOM, as in an artifact from an older CLI.
// It returns an error when the archive is missing or cannot be read, or when the BOM is larger than maxBOMSize.
func ReadCachedBOM(cacheDir string) ([]byte, error) {
	file, err := os.Open(filepath.Join(cacheDir, artifactTarFilename)) // #nosec G304 -- archive in the OCI cache directory of a source
	if err != nil {
		return nil, werror.Wrap(err, "failed to open the cached artifact in %s", cacheDir)
	}
	defer file.Close()
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, werror.Wrap(err, "failed to read the cached artifact in %s", cacheDir)
		}
		if path.Clean(strings.TrimPrefix(header.Name, "./")) != BOMFileName {
			continue
		}
		if header.Size > maxBOMSize {
			return nil, fmt.Errorf("%s in the cached artifact in %s is larger than %d bytes", BOMFileName, cacheDir, maxBOMSize)
		}
		data, err := io.ReadAll(io.LimitReader(reader, maxBOMSize+1))
		if err == nil && len(data) > maxBOMSize {
			err = fmt.Errorf("the entry is larger than %d bytes", maxBOMSize)
		}
		if err != nil {
			return nil, werror.Wrap(err, "failed to read %s from the cached artifact in %s", BOMFileName, cacheDir)
		}
		return data, nil
	}
}

// ReadCachedDigest returns the manifest digest of the artifact in the cache directory cacheDir.
// It returns an empty digest when the cache holds no digest, as in a cache from an older CLI.
// It returns an error when the digest file cannot be read or does not hold a valid digest.
func ReadCachedDigest(cacheDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(cacheDir, digestFilename)) // #nosec G304 -- digest file in the OCI cache directory of a source
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", werror.Wrap(err, "failed to read the cached digest in %s", cacheDir)
	}
	hash, err := v1.NewHash(strings.TrimSpace(string(data)))
	if err != nil {
		return "", werror.Wrap(err, "failed to parse the cached digest in %s", cacheDir)
	}
	return hash.String(), nil
}
