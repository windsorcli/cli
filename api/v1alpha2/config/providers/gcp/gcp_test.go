package gcp

import (
	"testing"
)

func TestGCPConfig_Merge(t *testing.T) {
	t.Run("MergeWithNoNils", func(t *testing.T) {
		base := &GCPConfig{
			Enabled:         ptrBool(true),
			ProjectID:       ptrString("base-project"),
			CredentialsPath: ptrString("/base/credentials.json"),
			QuotaProject:    ptrString("base-quota"),
		}

		overlay := &GCPConfig{
			Enabled:         ptrBool(false),
			ProjectID:       ptrString("overlay-project"),
			CredentialsPath: ptrString("/overlay/credentials.json"),
			QuotaProject:    ptrString("overlay-quota"),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected false, got %v", *base.Enabled)
		}
		if base.ProjectID == nil || *base.ProjectID != "overlay-project" {
			t.Errorf("ProjectID mismatch: expected 'overlay-project', got '%s'", *base.ProjectID)
		}
		if base.CredentialsPath == nil || *base.CredentialsPath != "/overlay/credentials.json" {
			t.Errorf("CredentialsPath mismatch: expected '/overlay/credentials.json', got '%s'", *base.CredentialsPath)
		}
		if base.QuotaProject == nil || *base.QuotaProject != "overlay-quota" {
			t.Errorf("QuotaProject mismatch: expected 'overlay-quota', got '%s'", *base.QuotaProject)
		}
	})

	t.Run("MergeWithAllNils", func(t *testing.T) {
		base := &GCPConfig{
			Enabled:         nil,
			ProjectID:       nil,
			CredentialsPath: nil,
			QuotaProject:    nil,
		}

		overlay := &GCPConfig{
			Enabled:         nil,
			ProjectID:       nil,
			CredentialsPath: nil,
			QuotaProject:    nil,
		}

		base.Merge(overlay)

		if base.Enabled != nil {
			t.Errorf("Enabled mismatch: expected nil, got %v", base.Enabled)
		}
		if base.ProjectID != nil {
			t.Errorf("ProjectID mismatch: expected nil, got '%s'", *base.ProjectID)
		}
		if base.CredentialsPath != nil {
			t.Errorf("CredentialsPath mismatch: expected nil, got '%s'", *base.CredentialsPath)
		}
		if base.QuotaProject != nil {
			t.Errorf("QuotaProject mismatch: expected nil, got '%s'", *base.QuotaProject)
		}
	})

	t.Run("MergeWithPartialOverlay", func(t *testing.T) {
		base := &GCPConfig{
			Enabled:         ptrBool(true),
			ProjectID:       ptrString("base-project"),
			CredentialsPath: ptrString("/base/credentials.json"),
			QuotaProject:    ptrString("base-quota"),
		}

		overlay := &GCPConfig{
			Enabled: ptrBool(false),
		}

		base.Merge(overlay)

		if base.Enabled == nil || *base.Enabled != false {
			t.Errorf("Enabled mismatch: expected false, got %v", *base.Enabled)
		}
		if base.ProjectID == nil || *base.ProjectID != "base-project" {
			t.Errorf("ProjectID mismatch: expected 'base-project', got '%s'", *base.ProjectID)
		}
		if base.CredentialsPath == nil || *base.CredentialsPath != "/base/credentials.json" {
			t.Errorf("CredentialsPath mismatch: expected '/base/credentials.json', got '%s'", *base.CredentialsPath)
		}
		if base.QuotaProject == nil || *base.QuotaProject != "base-quota" {
			t.Errorf("QuotaProject mismatch: expected 'base-quota', got '%s'", *base.QuotaProject)
		}
	})

	t.Run("MergeWithNilOverlay", func(t *testing.T) {
		base := &GCPConfig{
			Enabled:         ptrBool(true),
			ProjectID:       ptrString("base-project"),
			CredentialsPath: ptrString("/base/credentials.json"),
			QuotaProject:    ptrString("base-quota"),
		}
		original := base.DeepCopy()

		base.Merge(nil)

		if base.Enabled == nil || *base.Enabled != *original.Enabled {
			t.Errorf("Enabled mismatch: expected unchanged")
		}
		if base.ProjectID == nil || *base.ProjectID != *original.ProjectID {
			t.Errorf("ProjectID mismatch: expected unchanged")
		}
		if base.CredentialsPath == nil || *base.CredentialsPath != *original.CredentialsPath {
			t.Errorf("CredentialsPath mismatch: expected unchanged")
		}
		if base.QuotaProject == nil || *base.QuotaProject != *original.QuotaProject {
			t.Errorf("QuotaProject mismatch: expected unchanged")
		}
	})
}

func TestGCPConfig_DeepCopy(t *testing.T) {
	t.Run("DeepCopyWithNonNilValues", func(t *testing.T) {
		original := &GCPConfig{
			Enabled:         ptrBool(true),
			ProjectID:       ptrString("test-project"),
			CredentialsPath: ptrString("/test/credentials.json"),
			QuotaProject:    ptrString("test-quota"),
		}

		copy := original.DeepCopy()

		if original.Enabled == nil || copy.Enabled == nil || *original.Enabled != *copy.Enabled {
			t.Errorf("Enabled mismatch: expected %v, got %v", *original.Enabled, *copy.Enabled)
		}
		if original.ProjectID == nil || copy.ProjectID == nil || *original.ProjectID != *copy.ProjectID {
			t.Errorf("ProjectID mismatch: expected %v, got %v", *original.ProjectID, *copy.ProjectID)
		}
		if original.CredentialsPath == nil || copy.CredentialsPath == nil || *original.CredentialsPath != *copy.CredentialsPath {
			t.Errorf("CredentialsPath mismatch: expected %v, got %v", *original.CredentialsPath, *copy.CredentialsPath)
		}
		if original.QuotaProject == nil || copy.QuotaProject == nil || *original.QuotaProject != *copy.QuotaProject {
			t.Errorf("QuotaProject mismatch: expected %v, got %v", *original.QuotaProject, *copy.QuotaProject)
		}

		copy.Enabled = ptrBool(false)
		if original.Enabled == nil || *original.Enabled == *copy.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Enabled)
		}

		copy.ProjectID = ptrString("modified-project")
		if original.ProjectID == nil || *original.ProjectID == *copy.ProjectID {
			t.Errorf("Original ProjectID was modified: expected %v, got %v", "test-project", *copy.ProjectID)
		}

		if copy.Enabled == original.Enabled {
			t.Error("Expected Enabled pointers to be different")
		}
		if copy.ProjectID == original.ProjectID {
			t.Error("Expected ProjectID pointers to be different")
		}
		if copy.CredentialsPath == original.CredentialsPath {
			t.Error("Expected CredentialsPath pointers to be different")
		}
		if copy.QuotaProject == original.QuotaProject {
			t.Error("Expected QuotaProject pointers to be different")
		}
	})

	t.Run("DeepCopyNil", func(t *testing.T) {
		var original *GCPConfig = nil
		mockCopy := original.DeepCopy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})

	t.Run("DeepCopyWithSomeFields", func(t *testing.T) {
		original := &GCPConfig{
			Enabled: ptrBool(true),
		}

		copy := original.DeepCopy()

		if copy == nil {
			t.Fatal("Expected non-nil copy")
		}
		if copy.Enabled == nil || *copy.Enabled != *original.Enabled {
			t.Errorf("Expected Enabled to be copied correctly")
		}
		if copy.ProjectID != nil {
			t.Error("Expected ProjectID to be nil")
		}
		if copy.CredentialsPath != nil {
			t.Error("Expected CredentialsPath to be nil")
		}
		if copy.QuotaProject != nil {
			t.Error("Expected QuotaProject to be nil")
		}
	})
}

func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

