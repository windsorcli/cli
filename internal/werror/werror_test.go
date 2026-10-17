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
	t.Run("KeepsTheMessageRemediationAndCause", func(t *testing.T) {
		// Given a message, a remediation, and a cause
		message, remediation := "context value is malformed", "Run windsor set to fix it."

		// When a WindsorError is created
		we := werror.New(message, remediation, errLeaf)

		// Then each field holds its input
		if we.Message != message || we.Remediation != remediation || we.Cause != errLeaf {
			t.Errorf("unexpected fields %+v", we)
		}
	})

	t.Run("ErrorReturnsMessageWithoutCause", func(t *testing.T) {
		// Given a WindsorError with a cause
		we := werror.New("terraform is not installed", "", errLeaf)

		// When its text is read
		text := we.Error()

		// Then only the message is returned
		if text != "terraform is not installed" {
			t.Errorf("expected message only, got %q", text)
		}
	})

	t.Run("AllowsNilCause", func(t *testing.T) {
		// Given a WindsorError with no cause
		we := werror.New("unknown flag", "", nil)

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
		we := werror.New("apply failed", "", errLeaf)
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
		we := werror.New("facet not found", "", nil)

		// When the error is logged as an attribute
		logger.Error("compose failed", "error", we)

		// Then the record holds the message field
		var record struct {
			Error map[string]string `json:"error"`
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("expected JSON record, got %q: %v", buf.String(), err)
		}
		want := map[string]string{"message": "facet not found"}
		if !reflect.DeepEqual(record.Error, want) {
			t.Errorf("expected %v, got %v", want, record.Error)
		}
	})

	t.Run("AddsTheRemediationWhenSet", func(t *testing.T) {
		// Given a JSON logger and a WindsorError with a remediation
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		we := werror.New("facet not found", "Add the facet.", nil)

		// When the error is logged as an attribute
		logger.Error("compose failed", "error", we)

		// Then the record holds the message and remediation fields
		var record struct {
			Error map[string]string `json:"error"`
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("expected JSON record, got %q: %v", buf.String(), err)
		}
		want := map[string]string{"message": "facet not found", "remediation": "Add the facet."}
		if !reflect.DeepEqual(record.Error, want) {
			t.Errorf("expected %v, got %v", want, record.Error)
		}
	})
}

func TestBreadcrumbs(t *testing.T) {
	t.Run("ListsFramesOutermostFirst", func(t *testing.T) {
		// Given frames around a WindsorError with a leaf cause
		we := werror.New("terraform apply failed", "", errLeaf)
		err := werror.Wrap(werror.Wrap(we, "running terraform apply"), "applying component cluster")

		// When the breadcrumbs are read
		crumbs := werror.Breadcrumbs(err)

		// Then each frame, the WindsorError message, and the leaf appear in order
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

	t.Run("SkipsAMessageThePreviousEntryContains", func(t *testing.T) {
		// Given a frame around a WindsorError whose message quotes its cause
		we := werror.New("file is not valid YAML: exit status 1", "", errors.New("exit status 1\n"))
		err := werror.Wrap(we, "loading blueprint")

		// When the breadcrumbs are read
		crumbs := werror.Breadcrumbs(err)

		// Then the cause text is not repeated
		want := []string{"loading blueprint", "file is not valid YAML: exit status 1"}
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
