// The cache functions of the ArtifactBuilder keep pulled artifacts in a local disk cache.
// It provides the cache directory of an artifact, the pull, and the module path extraction.
// Pull downloads only the artifacts that do not have a valid cache entry.
// The extraction rejects a tar entry with an absolute path or a path traversal.

package artifact

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/internal/presenter"
)

// =============================================================================
// Constants
// =============================================================================

const artifactTarFilename = "artifact.tar"

// =============================================================================
// Public Methods
// =============================================================================

// GetCacheDir returns the cache directory path for an OCI artifact identified by registry, repository, and tag.
// The cache directory is located at <projectRoot>/.windsor/cache/oci/<extractionKey> where extractionKey
// is the cacheKey with /, :, and @ replaced with _ for filesystem safety.
// Returns an error if project root is empty.
func (a *ArtifactBuilder) GetCacheDir(registry, repository, tag string) (string, error) {
	if a.runtime == nil || a.runtime.ProjectRoot == "" {
		return "", fmt.Errorf("project root is not set")
	}

	cacheKey := FormatOCIRef(registry, repository, tag)
	extractionKey := strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(cacheKey)
	cacheDir := filepath.Join(a.runtime.ProjectRoot, ".windsor", "cache", "oci", extractionKey)

	return cacheDir, nil
}

// Pull downloads and extracts OCI artifacts to disk cache for use by other components.
// It takes a slice of OCI references and downloads unique artifacts, returning a map
// of cache directory paths keyed by their registry/repository:tag identifier.
// The method provides efficient caching to avoid duplicate downloads of the same artifact.
// It checks disk cache first, then downloads if needed, and extracts artifacts to disk cache.
// Returns cache directory paths instead of binary data.
func (a *ArtifactBuilder) Pull(ociRefs []string) (map[string]string, error) {
	ociArtifacts := make(map[string]string)

	noCacheValue, _ := os.LookupEnv("NO_CACHE")
	noCache := strings.EqualFold(noCacheValue, "true") || noCacheValue == "1"

	uniqueOCIRefs := make(map[string]bool)
	for _, ref := range ociRefs {
		if strings.HasPrefix(ref, "oci://") {
			uniqueOCIRefs[ref] = true
		}
	}

	if len(uniqueOCIRefs) == 0 {
		return ociArtifacts, nil
	}

	var artifactsToDownload []string
	for ref := range uniqueOCIRefs {
		registry, repository, tag, err := a.ParseOCIRef(ref)
		if err != nil {
			return nil, fmt.Errorf("failed to parse OCI reference %s: %w", ref, err)
		}

		cacheKey := FormatOCIRef(registry, repository, tag)

		cacheDir, err := a.GetCacheDir(registry, repository, tag)
		if err != nil {
			return nil, fmt.Errorf("failed to get cache directory: %w", err)
		}

		if noCache {
			artifactsToDownload = append(artifactsToDownload, ref)
			continue
		}

		if validateErr := a.validateOCIDiskCache(cacheDir); validateErr == nil {
			ociArtifacts[cacheKey] = cacheDir
			continue
		} else if !errors.Is(validateErr, os.ErrNotExist) {
			if removeErr := a.shims.RemoveAll(cacheDir); removeErr != nil {
				return nil, fmt.Errorf("failed to remove corrupted OCI cache directory %s: %w", cacheDir, removeErr)
			}
		}

		artifactsToDownload = append(artifactsToDownload, ref)
	}

	if len(artifactsToDownload) > 0 {
		if err := presenter.Track(presenter.Background(), "oci:load", "Loading OCI sources", func(context.Context) error {
			for _, ref := range artifactsToDownload {
				registry, repository, tag, err := a.ParseOCIRef(ref)
				if err != nil {
					return fmt.Errorf("failed to parse OCI reference %s: %w", ref, err)
				}

				cacheKey := FormatOCIRef(registry, repository, tag)

				artifactData, err := a.downloadOCIArtifact(registry, repository, tag)
				if err != nil {
					return fmt.Errorf("failed to download OCI artifact %s: %w", ref, err)
				}

				cacheDir, err := a.GetCacheDir(registry, repository, tag)
				if err != nil {
					return fmt.Errorf("failed to get cache directory: %w", err)
				}

				if err := a.extractArtifactToCache(artifactData, cacheDir); err != nil {
					return fmt.Errorf("failed to extract artifact: %w", err)
				}

				ociArtifacts[cacheKey] = cacheDir
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return ociArtifacts, nil
}

// ExtractModulePath extracts a specific module path from a cached OCI artifact.
// It checks if the module path already exists in the cache directory, and if not,
// extracts it from the cached artifact.tar file. Returns the full path to the extracted module
// or an error if extraction fails.
func (a *ArtifactBuilder) ExtractModulePath(registry, repository, tag, modulePath string) (string, error) {
	cacheDir, err := a.GetCacheDir(registry, repository, tag)
	if err != nil {
		return "", fmt.Errorf("failed to get cache directory: %w", err)
	}

	fullModulePath := filepath.Join(cacheDir, modulePath)
	if _, err := a.shims.Stat(fullModulePath); err == nil {
		return fullModulePath, nil
	}

	artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
	artifactData, err := a.shims.ReadFile(artifactTarPath)
	if err != nil {
		return "", fmt.Errorf("failed to read cached artifact.tar: %w", err)
	}

	reader := a.shims.NewBytesReader(artifactData)
	tarReader := a.shims.NewTarReader(reader)

	if err := a.extractTarEntries(tarReader, cacheDir); err != nil {
		return "", fmt.Errorf("failed to extract module path from artifact: %w", err)
	}

	if _, err := a.shims.Stat(fullModulePath); err != nil {
		return "", fmt.Errorf("module path %s does not exist in artifact", modulePath)
	}

	return fullModulePath, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// validateOCIDiskCache checks that an extracted OCI artifact cache directory is complete and ready for use.
// It verifies that the cache directory exists and contains a non-empty artifact.tar archive file.
// Returns an error if validation fails or incomplete extraction is detected.
func (a *ArtifactBuilder) validateOCIDiskCache(cacheDir string) error {
	if _, err := a.shims.Stat(cacheDir); err != nil {
		return err
	}

	artifactTarPath := filepath.Join(cacheDir, artifactTarFilename)
	info, err := a.shims.Stat(artifactTarPath)
	if err != nil {
		return err
	}

	if info.Size() == 0 {
		return fmt.Errorf("artifact.tar is empty")
	}

	return nil
}

// validateAndSanitizePath sanitizes a file path for safe extraction by removing path traversal sequences
// and rejecting absolute paths. Returns the cleaned path normalized to forward slashes if valid, or an error if the path is unsafe.
// This function checks for absolute paths in a platform-agnostic way since tar archives use Unix-style paths
// regardless of the host OS. It checks the original path for Unix-style absolute paths (starting with /)
// before filepath.Clean, which may convert separators on Windows. The returned path is normalized to forward slashes
// to ensure consistent behavior across platforms.
func (a *ArtifactBuilder) validateAndSanitizePath(path string) (string, error) {
	if strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}
	if len(path) >= 2 && path[1] == ':' && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", path)
	}
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path contains directory traversal sequence: %s", path)
	}
	return filepath.ToSlash(cleanPath), nil
}

// extractTarEntries extracts specific entries from a tar archive into the given destination directory for caching and reuse.
// Only entries within the "_template/", "kustomize/", and "terraform/" directories, as well as the "metadata.yaml" file, are extracted.
// Directories and files are handled appropriately, and file permissions are set as specified in the tar headers.
// The extraction process performs path validation to prevent directory traversal attacks, and the destination directory must preexist.
// Returns an error if any part of the extraction or validation fails.
func (a *ArtifactBuilder) extractTarEntries(tarReader TarReader, destDir string) error {
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		sanitizedPath, err := a.validateAndSanitizePath(header.Name)
		if err != nil {
			return fmt.Errorf("invalid path in tar archive: %w", err)
		}

		if !strings.HasPrefix(sanitizedPath, "_template/") &&
			!strings.HasPrefix(sanitizedPath, "kustomize/") &&
			!strings.HasPrefix(sanitizedPath, "terraform/") &&
			sanitizedPath != "metadata.yaml" {
			if header.Typeflag != tar.TypeDir {
				if _, err := io.Copy(io.Discard, tarReader); err != nil {
					return fmt.Errorf("failed to skip entry %s: %w", header.Name, err)
				}
			}
			continue
		}

		destPath := filepath.Join(destDir, sanitizedPath)

		relPath, err := filepath.Rel(destDir, destPath)
		if err != nil {
			return fmt.Errorf("path traversal attempt detected: %s", header.Name)
		}
		if strings.HasPrefix(relPath, "..") {
			return fmt.Errorf("path traversal attempt detected: %s", header.Name)
		}

		if header.Typeflag == tar.TypeDir {
			if err := a.shims.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			continue
		}

		if err := a.shims.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
		}

		fileWriter, err := a.shims.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", destPath, err)
		}

		_, err = a.shims.Copy(fileWriter, tarReader)
		closeErr := fileWriter.Close()
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close file %s: %w", destPath, closeErr)
		}

		modeValue := header.Mode & 0777
		if modeValue < 0 || modeValue > math.MaxUint32 {
			return fmt.Errorf("invalid file mode value: %d", modeValue)
		}
		fileMode := os.FileMode(uint32(modeValue))

		if err := a.shims.Chmod(destPath, fileMode); err != nil {
			return fmt.Errorf("failed to set file permissions for %s: %w", destPath, err)
		}
	}

	return nil
}

// extractArtifactToCache extracts the entire OCI artifact tar archive into the specified extraction directory.
// The extraction preserves file permissions and is performed atomically: all files are unpacked to a temporary
// directory (extractionDir.tmp) and then renamed to extractionDir upon success, preventing partial or inconsistent state.
// This mechanism ensures all artifact files are present for subsequent resolution steps and enables cache reuse.
// The extractionDir parameter specifies the target directory for extraction. If any extraction phase fails or path
// validations do not pass, an error is returned. On error, temporary directories are cleaned up automatically.
func (a *ArtifactBuilder) extractArtifactToCache(artifactData []byte, extractionDir string) (err error) {
	reader := a.shims.NewBytesReader(artifactData)
	tarReader := a.shims.NewTarReader(reader)

	tmpExtractionDir := extractionDir + ".tmp"

	defer func() {
		if err != nil {
			if _, statErr := a.shims.Stat(tmpExtractionDir); statErr == nil {
				if removeErr := a.shims.RemoveAll(tmpExtractionDir); removeErr != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to clean up temporary extraction directory: %v\n", removeErr)
				}
			}
		}
	}()

	if _, statErr := a.shims.Stat(tmpExtractionDir); statErr == nil {
		if removeErr := a.shims.RemoveAll(tmpExtractionDir); removeErr != nil {
			err = fmt.Errorf("failed to clean up existing temporary extraction directory: %w", removeErr)
			return
		}
	}

	if mkdirErr := a.shims.MkdirAll(tmpExtractionDir, 0755); mkdirErr != nil {
		err = fmt.Errorf("failed to create temporary extraction directory: %w", mkdirErr)
		return
	}

	if err = a.extractTarEntries(tarReader, tmpExtractionDir); err != nil {
		return
	}

	artifactTarPath := filepath.Join(tmpExtractionDir, artifactTarFilename)
	artifactTarFile, tarErr := a.shims.Create(artifactTarPath)
	if tarErr != nil {
		err = fmt.Errorf("failed to create artifact.tar file: %w", tarErr)
		return
	}
	if _, writeErr := artifactTarFile.Write(artifactData); writeErr != nil {
		_ = artifactTarFile.Close()
		err = fmt.Errorf("failed to write artifact.tar file: %w", writeErr)
		return
	}
	if closeErr := artifactTarFile.Close(); closeErr != nil {
		err = fmt.Errorf("failed to close artifact.tar file: %w", closeErr)
		return
	}

	parentDir := filepath.Dir(extractionDir)
	if mkdirErr := a.shims.MkdirAll(parentDir, 0755); mkdirErr != nil {
		err = fmt.Errorf("failed to create parent directory: %w", mkdirErr)
		return
	}

	if _, statErr := a.shims.Stat(extractionDir); statErr == nil {
		if removeErr := a.shims.RemoveAll(extractionDir); removeErr != nil {
			err = fmt.Errorf("failed to remove existing extraction directory before rename: %w", removeErr)
			return
		}
	}

	if renameErr := a.shims.Rename(tmpExtractionDir, extractionDir); renameErr != nil {
		err = fmt.Errorf("failed to rename temporary extraction directory to final location: %w", renameErr)
		return
	}

	return nil
}
