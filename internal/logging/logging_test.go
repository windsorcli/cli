package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/windsorcli/cli/internal/logging"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Test Constructor
// =============================================================================

func TestNewHandler(t *testing.T) {
	t.Run("JSONWritesOneObjectPerRecord", func(t *testing.T) {
		// Given a JSON handler at info level
		var buf bytes.Buffer
		logger := slog.New(logging.NewHandler(&buf, logging.FormatJSON, slog.LevelInfo))

		// When a record with an attribute is logged
		logger.Info("applied", "component", "cluster")

		// Then the output is a JSON object with the message and attribute
		var record map[string]any
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("expected JSON, got %q: %v", buf.String(), err)
		}
		if record["msg"] != "applied" || record["component"] != "cluster" {
			t.Errorf("unexpected record %v", record)
		}
	})

	t.Run("TextWritesLogfmt", func(t *testing.T) {
		// Given a text handler at info level
		var buf bytes.Buffer
		logger := slog.New(logging.NewHandler(&buf, logging.FormatText, slog.LevelInfo))

		// When a record with an attribute is logged
		logger.Info("applied", "component", "cluster")

		// Then the output is a logfmt line with the full level name, the message, and the attribute
		out := buf.String()
		if !strings.Contains(out, "level=INFO") || !strings.Contains(out, "msg=applied") || !strings.Contains(out, "component=cluster") {
			t.Errorf("unexpected output %q", out)
		}
	})

	t.Run("DropsRecordsBelowTheLevel", func(t *testing.T) {
		// Given handlers at info level for each format
		for _, format := range []logging.Format{logging.FormatText, logging.FormatJSON} {
			var buf bytes.Buffer
			logger := slog.New(logging.NewHandler(&buf, format, slog.LevelInfo))

			// When a debug record is logged
			logger.Debug("hidden")

			// Then nothing is written
			if buf.Len() != 0 {
				t.Errorf("%s: expected no output, got %q", format, buf.String())
			}
		}
	})

	t.Run("WritesDebugRecordsAtDebugLevel", func(t *testing.T) {
		// Given handlers at debug level for each format
		for _, format := range []logging.Format{logging.FormatText, logging.FormatJSON} {
			var buf bytes.Buffer
			logger := slog.New(logging.NewHandler(&buf, format, slog.LevelDebug))

			// When a debug record is logged
			logger.Debug("shown")

			// Then the record is written
			if !strings.Contains(buf.String(), "shown") {
				t.Errorf("%s: expected debug output, got %q", format, buf.String())
			}
		}
	})

	t.Run("ExpandsWindsorErrorFields", func(t *testing.T) {
		// Given handlers for each format and a WindsorError with a remediation
		we := werror.New("context value is malformed", "Run windsor set to fix it.", nil)
		for _, format := range []logging.Format{logging.FormatText, logging.FormatJSON} {
			var buf bytes.Buffer
			logger := slog.New(logging.NewHandler(&buf, format, slog.LevelInfo))

			// When the error is logged as an attribute
			logger.Error("load failed", "error", we)

			// Then the remediation appears as a structured field
			if !strings.Contains(buf.String(), "remediation") || !strings.Contains(buf.String(), "Run windsor set to fix it.") {
				t.Errorf("%s: expected remediation field, got %q", format, buf.String())
			}
		}
	})
}

func TestNewHandler_Derived(t *testing.T) {
	t.Run("TextResolvesWindsorErrorFieldsAddedWithWith", func(t *testing.T) {
		// Given a text logger that carries a WindsorError from With
		var buf bytes.Buffer
		we := werror.New("token expired", "", nil)
		logger := slog.New(logging.NewHandler(&buf, logging.FormatText, slog.LevelInfo)).With("error", we)

		// When a record is logged
		logger.Info("resolving")

		// Then the message appears as a structured field
		if !strings.Contains(buf.String(), `error.message="token expired"`) {
			t.Errorf("expected message field, got %q", buf.String())
		}
	})

	t.Run("TextKeepsAttributesUnderAGroup", func(t *testing.T) {
		// Given a text logger with an open group
		var buf bytes.Buffer
		logger := slog.New(logging.NewHandler(&buf, logging.FormatText, slog.LevelInfo)).WithGroup("terraform")

		// When a record with an attribute is logged
		logger.Info("applying", "component", "cluster")

		// Then the group name prefixes the attribute key
		if !strings.Contains(buf.String(), "msg=applying") || !strings.Contains(buf.String(), "terraform.component=cluster") {
			t.Errorf("expected grouped attribute, got %q", buf.String())
		}
	})
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestParseFormat(t *testing.T) {
	t.Run("AcceptsKnownFormats", func(t *testing.T) {
		// Given the names of both formats
		names := []string{"text", "json"}

		// When each name is parsed
		for _, name := range names {
			format, err := logging.ParseFormat(name)

			// Then the matching format is returned
			if err != nil || string(format) != name {
				t.Errorf("expected %q, got %q (%v)", name, format, err)
			}
		}
	})

	t.Run("RejectsUnknownFormats", func(t *testing.T) {
		// Given an unknown format name
		name := "yaml"

		// When it is parsed
		_, err := logging.ParseFormat(name)

		// Then the error names the valid formats
		if err == nil || !strings.Contains(err.Error(), `"text"`) || !strings.Contains(err.Error(), `"json"`) {
			t.Errorf("expected error listing valid formats, got %v", err)
		}
	})
}

func TestFromContext(t *testing.T) {
	t.Run("ReturnsTheInjectedLogger", func(t *testing.T) {
		// Given a context that carries a logger
		logger := slog.New(slog.DiscardHandler)
		ctx := logging.NewContext(context.Background(), logger)

		// When the logger is read back
		got := logging.FromContext(ctx)

		// Then it is the same logger
		if got != logger {
			t.Error("expected the injected logger")
		}
	})

	t.Run("ReturnsASilentLoggerWhenNoneIsInjected", func(t *testing.T) {
		// Given a context without a logger
		ctx := context.Background()

		// When the logger is read
		got := logging.FromContext(ctx)

		// Then a non-nil logger is returned that discards every level
		if got == nil {
			t.Fatal("expected a logger, got nil")
		}
		if got.Enabled(ctx, slog.LevelError) {
			t.Error("expected the fallback logger to discard records")
		}
	})
}
