package werror_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Setup
// =============================================================================

var errLeaf = errors.New("exit status 1")

// =============================================================================
// Test Constructor
// =============================================================================

func TestNew(t *testing.T) {
	t.Run("DerivesCategoryAndDocsURLFromCode", func(t *testing.T) {
		// Given a code with a domain prefix
		code := "CONFIG-002"

		// When a WindsorError is created
		we := werror.New(code, "context value is malformed", "Run windsor set to fix it.", errLeaf)

		// Then the category and docs URL come from the code
		if we.Category != "CONFIG" {
			t.Errorf("expected category CONFIG, got %q", we.Category)
		}
		if we.DocsURL != "https://www.windsorcli.dev/errors/config-002" {
			t.Errorf("expected lowercased docs URL, got %q", we.DocsURL)
		}
		if we.Remediation != "Run windsor set to fix it." {
			t.Errorf("expected remediation to be kept, got %q", we.Remediation)
		}
	})

	t.Run("ErrorReturnsMessageWithoutCause", func(t *testing.T) {
		// Given a WindsorError with a cause
		we := werror.New("SHELL-001", "terraform is not installed", "", errLeaf)

		// When its text is read
		text := we.Error()

		// Then only the message is returned
		if text != "terraform is not installed" {
			t.Errorf("expected message only, got %q", text)
		}
	})

	t.Run("AllowsNilCause", func(t *testing.T) {
		// Given a WindsorError with no cause
		we := werror.New("CLI-001", "unknown flag", "", nil)

		// When it is unwrapped
		cause := errors.Unwrap(we)

		// Then the cause is nil
		if cause != nil {
			t.Errorf("expected nil cause, got %v", cause)
		}
	})
}

func TestWrap(t *testing.T) {
	t.Run("ReturnsNilForNilError", func(t *testing.T) {
		// Given no error
		var err error

		// When it is wrapped
		wrapped := werror.Wrap(err, "applying %s", "cluster")

		// Then nil is returned
		if wrapped != nil {
			t.Errorf("expected nil, got %v", wrapped)
		}
	})

	t.Run("PrefixesTheFormattedMessage", func(t *testing.T) {
		// Given an error
		err := errLeaf

		// When it is wrapped with a formatted message
		wrapped := werror.Wrap(err, "applying component %s", "cluster")

		// Then the text matches fmt.Errorf wrapping
		if wrapped.Error() != "applying component cluster: exit status 1" {
			t.Errorf("unexpected text %q", wrapped.Error())
		}
	})

	t.Run("KeepsTheChainWalkable", func(t *testing.T) {
		// Given a sentinel inside a WindsorError inside a frame
		we := werror.New("TERRAFORM-001", "apply failed", "", errLeaf)
		wrapped := werror.Wrap(we, "applying component %s", "cluster")

		// When the chain is inspected
		var got *werror.WindsorError
		found := errors.As(wrapped, &got)

		// Then both the sentinel and the WindsorError are reachable
		if !errors.Is(wrapped, errLeaf) {
			t.Error("expected errors.Is to find the sentinel")
		}
		if !found || got != we {
			t.Error("expected errors.As to find the WindsorError")
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWindsorError_LogValue(t *testing.T) {
	t.Run("ExpandsToStructuredFields", func(t *testing.T) {
		// Given a JSON logger and a WindsorError
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		we := werror.New("BLUEPRINT-014", "facet not found", "", nil)

		// When the error is logged as an attribute
		logger.Error("compose failed", "error", we)

		// Then the record holds code, category, and message fields
		var record struct {
			Error map[string]string `json:"error"`
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("expected JSON record, got %q: %v", buf.String(), err)
		}
		want := map[string]string{"code": "BLUEPRINT-014", "category": "BLUEPRINT", "message": "facet not found"}
		if !reflect.DeepEqual(record.Error, want) {
			t.Errorf("expected %v, got %v", want, record.Error)
		}
	})
}

func TestIsCode(t *testing.T) {
	t.Run("MatchesThroughFramesAndErrorf", func(t *testing.T) {
		// Given a coded error under a frame and an fmt.Errorf wrap
		err := fmt.Errorf("up: %w", werror.Wrap(werror.New("CLUSTER-003", "node not ready", "", nil), "waiting"))

		// When the code is checked
		matched := werror.IsCode(err, "CLUSTER-003")

		// Then it matches
		if !matched {
			t.Error("expected code to match")
		}
	})

	t.Run("MatchesAnInnerWindsorError", func(t *testing.T) {
		// Given one coded error wrapping another
		inner := werror.New("SECRETS-002", "token expired", "", nil)
		outer := werror.New("CONFIG-005", "cannot resolve value", "", inner)

		// When the inner code is checked
		matched := werror.IsCode(outer, "SECRETS-002")

		// Then it matches
		if !matched {
			t.Error("expected inner code to match")
		}
	})

	t.Run("ReturnsFalseForUntypedOrNilErrors", func(t *testing.T) {
		// Given an untyped error and a nil error
		untyped := fmt.Errorf("wrap: %w", errLeaf)

		// When the code is checked on each
		matchedUntyped := werror.IsCode(untyped, "CONFIG-001")
		matchedNil := werror.IsCode(nil, "CONFIG-001")

		// Then neither matches
		if matchedUntyped || matchedNil {
			t.Errorf("expected no match, got untyped=%v nil=%v", matchedUntyped, matchedNil)
		}
	})
}

func TestIsCategory(t *testing.T) {
	t.Run("MatchesTheDomainPrefix", func(t *testing.T) {
		// Given a wrapped coded error
		err := werror.Wrap(werror.New("ARTIFACT-007", "pull denied", "", nil), "pulling blueprint")

		// When categories are checked
		matched := werror.IsCategory(err, "ARTIFACT")
		other := werror.IsCategory(err, "CONFIG")

		// Then only the code's domain matches
		if !matched || other {
			t.Errorf("expected ARTIFACT only, got ARTIFACT=%v CONFIG=%v", matched, other)
		}
	})
}

func TestBreadcrumbs(t *testing.T) {
	t.Run("ListsFramesOutermostFirst", func(t *testing.T) {
		// Given frames around a coded error with a leaf cause
		we := werror.New("TERRAFORM-002", "terraform apply failed", "", errLeaf)
		err := werror.Wrap(werror.Wrap(we, "running terraform apply"), "applying component cluster")

		// When the breadcrumbs are read
		crumbs := werror.Breadcrumbs(err)

		// Then each frame, the coded message, and the leaf appear in order
		want := []string{"applying component cluster", "running terraform apply", "terraform apply failed", "exit status 1"}
		if !reflect.DeepEqual(crumbs, want) {
			t.Errorf("expected %v, got %v", want, crumbs)
		}
	})

	t.Run("SkipsIntermediateErrorfWraps", func(t *testing.T) {
		// Given an fmt.Errorf wrap between a frame and the leaf
		err := werror.Wrap(fmt.Errorf("reading state: %w", errLeaf), "planning")

		// When the breadcrumbs are read
		crumbs := werror.Breadcrumbs(err)

		// Then the fmt.Errorf text is not repeated
		want := []string{"planning", "exit status 1"}
		if !reflect.DeepEqual(crumbs, want) {
			t.Errorf("expected %v, got %v", want, crumbs)
		}
	})

	t.Run("ReturnsNothingForNil", func(t *testing.T) {
		// Given no error
		var err error

		// When the breadcrumbs are read
		crumbs := werror.Breadcrumbs(err)

		// Then the list is empty
		if len(crumbs) != 0 {
			t.Errorf("expected no breadcrumbs, got %v", crumbs)
		}
	})
}
