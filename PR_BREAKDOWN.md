# PR Breakdown for Artifact Caching Refactor

## Overview
This document outlines a 4-PR breakdown for implementing unified artifact caching (OCI and file artifacts) with a new cache structure. Each PR is designed to be self-contained and reviewable, while building toward the final architecture.

## Cache Structure
The new unified cache structure will be:
- `.windsor/cache/oci/` - For OCI artifacts (oci://...)
- `.windsor/cache/file/` - For file artifacts (file://... or .tar.gz paths)
- `.windsor/cache/docker/` - Reserved for future Docker artifacts

This replaces the current:
- `.windsor/.oci_extracted/` - OCI artifacts
- `.windsor/.archive_extracted/` - Archive modules (still used for module extraction)

---

## PR 1: Refactor shared utilities and OCI parsing
**Purpose**: Extract common code and centralize OCI reference parsing

**Files**:
- `pkg/composer/terraform/module_resolver.go` - Add `extractTarEntries`, `extractTarEntriesWithFilter`, `validateAndSanitizePath` methods
- `pkg/composer/terraform/archive_module_resolver.go` - Refactor to use shared methods, remove duplicate code
- `pkg/composer/terraform/archive_module_resolver_test.go` - Update tests
- `pkg/composer/terraform/shims.go` - Add `Rename` shim, remove `RealTarReader` wrapper
- `pkg/composer/artifact/artifact.go` - Add `ParseOCIRef` to interface and implementation
- `pkg/composer/terraform/oci_module_resolver.go` - Use `artifactBuilder.ParseOCIRef`, remove local `parseOCIRef`
- `pkg/composer/artifact/mock_artifact.go` - Add mock method
- `pkg/composer/terraform/oci_module_resolver_private_test.go` - Update tests
- `pkg/composer/terraform/oci_module_resolver_public_test.go` - Update tests

**Changes**:
- Extract tar extraction logic from `ArchiveModuleResolver` to `BaseModuleResolver`
- Add `extractTarEntries` and `extractTarEntriesWithFilter` methods
- Move `validateAndSanitizePath` to shared location
- Simplify `ArchiveModuleResolver.extractModuleFromArchive` to use shared method
- Add `Rename` shim to `terraform.Shims` for extraction operations
- Remove `RealTarReader` wrapper from `terraform.Shims` (use `tar.NewReader` directly)
- Move `parseOCIRef` from `OCIModuleResolver` to `ArtifactBuilder`
- Make it public (`ParseOCIRef`) and add to `Artifact` interface
- Update `OCIModuleResolver` to use shared method
- Move `shouldHandle` method to private section (code organization)

**Rationale**: Self-contained refactor that improves code reuse and centralizes OCI parsing. No behavior changes, just better organization.

---

## PR 2: Add unified cache structure and directory management
**Purpose**: Implement unified cache directory structure and management methods

**Files**:
- `pkg/composer/artifact/artifact.go` - Add `GetCacheDir` with artifact type support, add `runtime` field to `ArtifactBuilder`
- `pkg/composer/artifact/mock_artifact.go` - Add mock method
- `pkg/composer/artifact/shims.go` - Add `MkdirAll`, `NewBytesReader`, `NewTarReader`, `Copy`, `Chmod`, `Rename`, `RemoveAll`, `TarReader` interface
- `pkg/composer/artifact/artifact_private_test.go` - Add tests for `GetCacheDir` with different artifact types
- `pkg/composer/artifact/artifact_public_test.go` - Add tests for `GetCacheDir`

**Changes**:
- Add `runtime` field to `ArtifactBuilder` struct
- Add `GetCacheDir` method to `Artifact` interface with artifact type parameter
- Implement `GetCacheDir` to return paths in `.windsor/cache/{type}/` structure
- Support artifact types: "oci", "file" (and "docker" for future)
- Add `TarReader` interface to `artifact` package
- Add file system operation shims to `artifact.Shims` (MkdirAll, NewBytesReader, NewTarReader, Copy, Chmod, Rename, RemoveAll)
- Wire up default implementations
- Add tests for cache directory generation with different artifact types

**Rationale**: Infrastructure change that establishes the unified cache structure. No behavior changes to existing functionality, just adds the foundation for caching.

---

## PR 3: Implement OCI artifact disk caching
**Purpose**: Add disk caching for OCI artifacts with cache validation and NO_CACHE support

**Files**:
- `pkg/composer/artifact/artifact.go` - Add `extractArtifactToCache`, `validateOCIDiskCache`, update `Pull` to use disk cache, add `NO_CACHE` support, update `GetTemplateData` for OCI artifacts
- `pkg/composer/artifact/artifact_private_test.go` - Add tests for caching logic, cache validation, template data from cache
- `pkg/composer/artifact/artifact_public_test.go` - Add tests for `Pull` with caching, `GetTemplateData` with cache
- `cmd/init.go` - Add `NO_CACHE` env var when `--reset` is used

**Changes**:
- Add `extractArtifactToCache` method to extract full artifacts to disk cache
- Add `validateOCIDiskCache` to validate cache integrity
- Update `Pull` to check disk cache before downloading (respects NO_CACHE)
- Add `getTemplateDataFromCache` to read template data from disk cache
- Add `extractTemplateDataFromTar` helper method
- Add `addMetadataToTemplateData` helper method
- Refactor `GetTemplateData` for OCI artifacts to check cache first, then download if needed
- Extract artifacts atomically (temp dir + rename)
- Store `artifact.tar` in cache directory
- Add `NO_CACHE` environment variable support

**Rationale**: Core caching feature for OCI artifacts. This enables disk caching and improves performance for OCI artifact operations.

---

## PR 4: Add file artifact caching and refactor resolvers
**Purpose**: Add caching for file artifacts and refactor module resolvers to use cache

**Files**:
- `pkg/composer/artifact/artifact.go` - Add file artifact caching support, update `GetTemplateData` for file artifacts
- `pkg/composer/terraform/oci_module_resolver.go` - Refactor `extractOCIModule` to use `GetCacheDir`, remove duplicate extraction code, use `BaseModuleResolver` methods
- `pkg/composer/artifact/artifact_private_test.go` - Add tests for file artifact caching
- `pkg/composer/artifact/artifact_public_test.go` - Add tests for file artifact caching in `GetTemplateData`
- `pkg/composer/terraform/oci_module_resolver_private_test.go` - Update tests
- `pkg/composer/terraform/oci_module_resolver_public_test.go` - Update tests
- `pkg/composer/terraform/composite_module_resolver_test.go` - Update tests
- `pkg/composer/terraform/module_resolver_test.go` - Update tests if needed

**Changes**:
- Add file artifact caching to `GetTemplateData` for `.tar.gz` files
- Cache file artifacts in `.windsor/cache/file/` using file path hash as cache key
- Add `getFileArtifactCacheKey` helper to generate cache keys from file paths
- Add `extractFileArtifactToCache` method for file artifacts
- Update `GetTemplateData` to check file cache before reading from disk
- Refactor `extractOCIModule` to use `artifactBuilder.GetCacheDir` with "oci" type
- Remove duplicate `extractModuleFromArtifact` method from `OCIModuleResolver`
- Remove duplicate `validateAndSanitizePath` method from `OCIModuleResolver`
- Use `BaseModuleResolver.extractTarEntriesWithFilter` for extraction
- Simplify logic to check cache directory instead of extracting from memory
- Update all tests to use new cache structure

**Rationale**: Completes the caching implementation by adding file artifact support and refactoring resolvers to use the unified cache structure. Removes code duplication and completes the architecture.

---

## Summary

**Total PRs**: 4

**Dependencies**:
- PR 1 → PR 3, PR 4 (shared utilities and OCI parsing)
- PR 2 → PR 3, PR 4 (cache directory infrastructure)
- PR 3 → PR 4 (OCI caching implementation)

**Suggested merge order**:
1. PR 1 (refactor - shared utilities and OCI parsing)
2. PR 2 (infrastructure - unified cache structure)
3. PR 3 (feature - OCI artifact caching)
4. PR 4 (feature - file artifact caching and resolver refactor)

**Notes**:
- Each PR should be independently testable
- PRs 1-2 are infrastructure/refactoring
- PR 3 is the main OCI caching feature
- PR 4 completes the architecture with file caching
- Shims are included in the PRs that use them (PR 1 for terraform, PR 2 for artifact)
- Cache structure migration: old `.windsor/.oci_extracted/` paths will be replaced by `.windsor/cache/oci/`
- Archive module extraction still uses `.windsor/.archive_extracted/` (separate from artifact caching)
