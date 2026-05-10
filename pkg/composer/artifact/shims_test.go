package artifact

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestNewShims(t *testing.T) {
	t.Run("ParseReferenceForwardsOptions", func(t *testing.T) {
		// Given the default shims
		shims := NewShims()

		// When parsing a bare repo:tag with a custom default registry option
		ref, err := shims.ParseReference("repo:tag", name.WithDefaultRegistry("custom.example.com"))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Then the option should be applied to the parsed reference
		if got := ref.Context().RegistryStr(); got != "custom.example.com" {
			t.Errorf("expected registry custom.example.com, got %s", got)
		}
	})

	t.Run("RemoteImageForwardsOptions", func(t *testing.T) {
		// Given an httptest registry that records the User-Agent of inbound requests
		var gotUA string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.UserAgent()
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		// When calling RemoteImage with a User-Agent option against that registry
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatalf("parse server url: %v", err)
		}
		shims := NewShims()
		ref, err := name.ParseReference(u.Host + "/repo:tag")
		if err != nil {
			t.Fatalf("parse ref: %v", err)
		}
		_, _ = shims.RemoteImage(ref, remote.WithUserAgent("windsor-test-ua"))

		// Then the User-Agent should have been forwarded to the outbound request
		// (go-containerregistry appends its own library identifier after our value)
		if !strings.HasPrefix(gotUA, "windsor-test-ua") {
			t.Errorf("expected User-Agent prefix windsor-test-ua, got %q", gotUA)
		}
	})
}
