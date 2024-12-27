package git

import (
	"reflect"
	"testing"
)

func TestGitConfig_Merge(t *testing.T) {
	t.Run("MergeWithNonNilValues", func(t *testing.T) {
		base := &GitConfig{}
		if base.Livereload == nil {
			base.Livereload = &GitLivereloadConfig{}
		}
		*base.Livereload = GitLivereloadConfig{
			Enabled:      ptrBool(true),
			RsyncExclude: ptrString("exclude-pattern"),
			RsyncProtect: ptrString("protect-pattern"),
			Username:     ptrString("user"),
			Password:     ptrString("pass"),
			WebhookUrl:   ptrString("http://webhook.url"),
			VerifySsl:    ptrBool(true),
			Image:        ptrString("image-name"),
		}

		overlay := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:      ptrBool(false),
				RsyncExclude: ptrString("new-exclude-pattern"),
				RsyncProtect: ptrString("new-protect-pattern"),
				Username:     ptrString("new-user"),
				Password:     ptrString("new-pass"),
				WebhookUrl:   ptrString("http://new-webhook.url"),
				VerifySsl:    ptrBool(false),
				Image:        ptrString("new-image-name"),
			},
		}

		base.Merge(overlay)

		if base.Livereload.Enabled == nil || *base.Livereload.Enabled != false {
			t.Errorf("Enabled mismatch: expected %v, got %v", false, *base.Livereload.Enabled)
		}
		if base.Livereload.RsyncExclude == nil || *base.Livereload.RsyncExclude != "new-exclude-pattern" {
			t.Errorf("RsyncExclude mismatch: expected %v, got %v", "new-exclude-pattern", *base.Livereload.RsyncExclude)
		}
		if base.Livereload.RsyncProtect == nil || *base.Livereload.RsyncProtect != "new-protect-pattern" {
			t.Errorf("RsyncProtect mismatch: expected %v, got %v", "new-protect-pattern", *base.Livereload.RsyncProtect)
		}
		if base.Livereload.Username == nil || *base.Livereload.Username != "new-user" {
			t.Errorf("Username mismatch: expected %v, got %v", "new-user", *base.Livereload.Username)
		}
		if base.Livereload.Password == nil || *base.Livereload.Password != "new-pass" {
			t.Errorf("Password mismatch: expected %v, got %v", "new-pass", *base.Livereload.Password)
		}
		if base.Livereload.WebhookUrl == nil || *base.Livereload.WebhookUrl != "http://new-webhook.url" {
			t.Errorf("WebhookUrl mismatch: expected %v, got %v", "http://new-webhook.url", *base.Livereload.WebhookUrl)
		}
		if base.Livereload.VerifySsl == nil || *base.Livereload.VerifySsl != false {
			t.Errorf("VerifySsl mismatch: expected %v, got %v", false, *base.Livereload.VerifySsl)
		}
		if base.Livereload.Image == nil || *base.Livereload.Image != "new-image-name" {
			t.Errorf("Image mismatch: expected %v, got %v", "new-image-name", *base.Livereload.Image)
		}
	})

	t.Run("MergeWithNilValues", func(t *testing.T) {
		base := &GitConfig{
			Livereload: nil,
		}

		overlay := &GitConfig{
			Livereload: &GitLivereloadConfig{
				Enabled:      ptrBool(true),
				RsyncExclude: ptrString("exclude-pattern"),
				RsyncProtect: ptrString("protect-pattern"),
				Username:     ptrString("user"),
				Password:     ptrString("pass"),
				WebhookUrl:   ptrString("http://webhook.url"),
				VerifySsl:    ptrBool(true),
				Image:        ptrString("image-name"),
			},
		}

		base.Merge(overlay)

		if base.Livereload.Enabled == nil || *base.Livereload.Enabled != true {
			t.Errorf("Enabled mismatch: expected %v, got %v", true, *base.Livereload.Enabled)
		}
		if base.Livereload.RsyncExclude == nil || *base.Livereload.RsyncExclude != "exclude-pattern" {
			t.Errorf("RsyncExclude mismatch: expected %v, got %v", "exclude-pattern", *base.Livereload.RsyncExclude)
		}
		if base.Livereload.RsyncProtect == nil || *base.Livereload.RsyncProtect != "protect-pattern" {
			t.Errorf("RsyncProtect mismatch: expected %v, got %v", "protect-pattern", *base.Livereload.RsyncProtect)
		}
		if base.Livereload.Username == nil || *base.Livereload.Username != "user" {
			t.Errorf("Username mismatch: expected %v, got %v", "user", *base.Livereload.Username)
		}
		if base.Livereload.Password == nil || *base.Livereload.Password != "pass" {
			t.Errorf("Password mismatch: expected %v, got %v", "pass", *base.Livereload.Password)
		}
		if base.Livereload.WebhookUrl == nil || *base.Livereload.WebhookUrl != "http://webhook.url" {
			t.Errorf("WebhookUrl mismatch: expected %v, got %v", "http://webhook.url", *base.Livereload.WebhookUrl)
		}
		if base.Livereload.VerifySsl == nil || *base.Livereload.VerifySsl != true {
			t.Errorf("VerifySsl mismatch: expected %v, got %v", true, *base.Livereload.VerifySsl)
		}
		if base.Livereload.Image == nil || *base.Livereload.Image != "image-name" {
			t.Errorf("Image mismatch: expected %v, got %v", "image-name", *base.Livereload.Image)
		}
	})
}

func TestGitConfig_Copy(t *testing.T) {
	t.Run("CopyWithNonNilValues", func(t *testing.T) {
		original := &GitConfig{}
		if original.Livereload == nil {
			original.Livereload = &GitLivereloadConfig{}
		}
		*original.Livereload = GitLivereloadConfig{
			Enabled:      ptrBool(true),
			RsyncExclude: ptrString("exclude-pattern"),
			RsyncProtect: ptrString("protect-pattern"),
			Username:     ptrString("user"),
			Password:     ptrString("pass"),
			WebhookUrl:   ptrString("http://webhook.url"),
			VerifySsl:    ptrBool(true),
			Image:        ptrString("image-name"),
		}

		copy := original.Copy()

		if !reflect.DeepEqual(original, copy) {
			t.Errorf("Copy mismatch: expected %v, got %v", original, copy)
		}

		// Modify the copy and ensure original is unchanged
		copy.Livereload.Enabled = ptrBool(false)
		if original.Livereload.Enabled == nil || *original.Livereload.Enabled == *copy.Livereload.Enabled {
			t.Errorf("Original Enabled was modified: expected %v, got %v", true, *copy.Livereload.Enabled)
		}
		copy.Livereload.Username = ptrString("modified-user")
		if original.Livereload.Username == nil || *original.Livereload.Username == *copy.Livereload.Username {
			t.Errorf("Original Username was modified: expected %v, got %v", "user", *copy.Livereload.Username)
		}
	})

	t.Run("CopyWithNilValues", func(t *testing.T) {
		original := &GitConfig{
			Livereload: nil,
		}

		copy := original.Copy()

		if copy.Livereload != nil {
			t.Errorf("Livereload mismatch: expected nil, got %v", copy.Livereload)
		}
	})

	t.Run("CopyNil", func(t *testing.T) {
		var original *GitConfig = nil
		mockCopy := original.Copy()
		if mockCopy != nil {
			t.Errorf("Mock copy should be nil, got %v", mockCopy)
		}
	})

	t.Run("MergeWithAllNils", func(t *testing.T) {
		base := &GitConfig{
			Livereload: nil,
		}

		overlay := &GitConfig{
			Livereload: nil,
		}

		base.Merge(overlay)

		if base.Livereload != nil {
			t.Errorf("Livereload mismatch: expected nil, got %v", base.Livereload)
		}
	})
}

// Helper functions to create pointers for basic types
func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}
