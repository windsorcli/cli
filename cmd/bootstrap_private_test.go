package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/provisioner"
)

// =============================================================================
// Test Setup
// =============================================================================

// =============================================================================
// Test Public Methods
// =============================================================================

func TestWarnNotReady(t *testing.T) {
	notReady := fmt.Errorf("%w: dns not ready", provisioner.ErrKustomizationsNotReady)
	messages := []blueprintv1alpha1.Message{{Text: "Delegate ns1.example at your registrar."}}

	t.Run("WarnsForNotReadyWithMessages", func(t *testing.T) {
		var buf strings.Builder
		if err := warnNotReady(&buf, notReady, messages); err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
		if !strings.Contains(buf.String(), "dns not ready") {
			t.Errorf("Expected a warning naming the cause, got %q", buf.String())
		}
	})

	t.Run("ReturnsNotReadyWithoutMessages", func(t *testing.T) {
		var buf strings.Builder
		if err := warnNotReady(&buf, notReady, nil); !errors.Is(err, provisioner.ErrKustomizationsNotReady) {
			t.Errorf("Expected the not-ready error, got %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("Expected no warning, got %q", buf.String())
		}
	})

	t.Run("ReturnsOtherErrors", func(t *testing.T) {
		other := fmt.Errorf("apply failed")
		if err := warnNotReady(io.Discard, other, messages); err != other {
			t.Errorf("Expected the other error unchanged, got %v", err)
		}
	})

	t.Run("ReturnsNilForNil", func(t *testing.T) {
		if err := warnNotReady(io.Discard, nil, messages); err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
	})
}

func TestPrintPostRunMessages(t *testing.T) {
	t.Run("EmptyMessagesProducesNoOutput", func(t *testing.T) {
		var buf strings.Builder
		printPostRunMessages(&buf, nil)
		if buf.Len() != 0 {
			t.Errorf("Expected no output for empty messages, got %q", buf.String())
		}
	})

	t.Run("SingleMessageRendersUnderNotesHeader", func(t *testing.T) {
		var buf strings.Builder
		printPostRunMessages(&buf, []blueprintv1alpha1.Message{{Text: "Delegate ns1.example, ns2.example"}})
		out := buf.String()
		if !strings.Contains(out, "Notes\n") {
			t.Errorf("Expected a Notes header, got %q", out)
		}
		if !strings.Contains(out, "═══") {
			t.Errorf("Expected a divider under the header, got %q", out)
		}
		if !strings.Contains(out, "Delegate ns1.example, ns2.example\n") {
			t.Errorf("Expected the message text, got %q", out)
		}
	})

	t.Run("MultipleMessagesSeparatedByBlankLine", func(t *testing.T) {
		var buf strings.Builder
		printPostRunMessages(&buf, []blueprintv1alpha1.Message{{Text: "first"}, {Text: "second"}})
		if !strings.Contains(buf.String(), "first\n\nsecond\n") {
			t.Errorf("Expected messages separated by a blank line, got %q", buf.String())
		}
	})
}
