package artifact

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

// writeCachedTar writes a cache directory whose artifact.tar holds files, and returns the directory.
func writeCachedTar(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	file, err := os.Create(filepath.Join(dir, artifactTarFilename))
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestReadCachedBOM(t *testing.T) {
	t.Run("ReturnsTheBOMOfTheCachedArtifact", func(t *testing.T) {
		// Given a cached artifact that holds a BOM
		dir := writeCachedTar(t, map[string]string{"metadata.yaml": "name: core\n", "./" + BOMFileName: "{\"bomFormat\":\"CycloneDX\"}"})

		// When ReadCachedBOM is called
		data, err := ReadCachedBOM(dir)

		// Then it returns the BOM
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if string(data) != "{\"bomFormat\":\"CycloneDX\"}" {
			t.Errorf("expected the BOM, got %q", data)
		}
	})

	t.Run("ReturnsNilForAnArtifactWithNoBOM", func(t *testing.T) {
		// Given a cached artifact with no BOM
		dir := writeCachedTar(t, map[string]string{"metadata.yaml": "name: core\n"})

		// When ReadCachedBOM is called
		data, err := ReadCachedBOM(dir)

		// Then it returns no BOM and no error
		if err != nil || data != nil {
			t.Errorf("expected no BOM and no error, got %q and %v", data, err)
		}
	})

	t.Run("ReturnsErrorWithNoCachedArtifact", func(t *testing.T) {
		// Given a cache directory with no artifact.tar
		dir := t.TempDir()

		// When ReadCachedBOM is called
		_, err := ReadCachedBOM(dir)

		// Then it returns an error
		if err == nil {
			t.Error("expected an error")
		}
	})
}

func TestReadCachedDigest(t *testing.T) {
	t.Run("ReturnsTheCachedDigest", func(t *testing.T) {
		// Given a cache directory with a digest file
		dir := t.TempDir()
		want := "sha256:" + testImageDigestHex
		if err := os.WriteFile(filepath.Join(dir, digestFilename), []byte(want+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		// When ReadCachedDigest is called
		digest, err := ReadCachedDigest(dir)

		// Then it returns the digest
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if digest != want {
			t.Errorf("expected %q, got %q", want, digest)
		}
	})

	t.Run("ReturnsNoDigestForACacheWithNoDigestFile", func(t *testing.T) {
		// Given a cache directory from an older CLI
		dir := t.TempDir()

		// When ReadCachedDigest is called
		digest, err := ReadCachedDigest(dir)

		// Then it returns no digest and no error
		if err != nil || digest != "" {
			t.Errorf("expected no digest and no error, got %q, %v", digest, err)
		}
	})

	t.Run("ReturnsErrorForAnInvalidDigest", func(t *testing.T) {
		// Given a digest file that holds no valid digest
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, digestFilename), []byte("latest"), 0o600); err != nil {
			t.Fatal(err)
		}

		// When ReadCachedDigest is called
		_, err := ReadCachedDigest(dir)

		// Then it fails
		if err == nil {
			t.Error("expected an error")
		}
	})
}
