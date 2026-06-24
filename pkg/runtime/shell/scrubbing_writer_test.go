package shell

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	t.Run("MasksInlinePEMBlock", func(t *testing.T) {
		// Given a line carrying a PEM block rendered with escaped newlines, as terraform prints it
		input := `ca_certificate = "-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIJAOZ\n-----END CERTIFICATE-----\n"`

		// When the line is redacted
		result := redactSecrets(input)

		// Then no PEM material survives
		if strings.Contains(result, "BEGIN CERTIFICATE") || strings.Contains(result, "MIIBkTCB") {
			t.Errorf("Expected PEM block to be masked, got %q", result)
		}
		if !strings.Contains(result, "********") {
			t.Errorf("Expected mask marker in output, got %q", result)
		}
	})

	t.Run("MasksBase64SecretField", func(t *testing.T) {
		// Given a secret-bearing field name followed by a long base64 value
		input := `        secret: dGhpc2lzYXZlcnlsb25nYmFzZTY0c2VjcmV0dmFsdWVhYmNkZWZn`

		// When the line is redacted
		result := redactSecrets(input)

		// Then the value is masked while the field name remains visible
		if strings.Contains(result, "dGhpc2lzYXZlcnlsb25n") {
			t.Errorf("Expected base64 value to be masked, got %q", result)
		}
		if !strings.Contains(result, "secret:") || !strings.Contains(result, "********") {
			t.Errorf("Expected field name kept and value masked, got %q", result)
		}
	})

	t.Run("MasksTerraformAttributeNames", func(t *testing.T) {
		// Given terraform plan lines for Talos PKI attributes whose names embed the secret word
		for _, input := range []string{
			`        ca_certificate     = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tQUFBQUFBQUFBQUFB"`,
			`        client_key         = "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLUFBQUFBQUFB"`,
		} {
			// When the line is redacted
			result := redactSecrets(input)

			// Then the base64 value is masked
			if strings.Contains(result, "LS0tLS1") {
				t.Errorf("Expected attribute value masked, got %q", result)
			}
			if !strings.Contains(result, "********") {
				t.Errorf("Expected mask marker, got %q", result)
			}
		}
	})

	t.Run("MasksTalosBootstrapToken", func(t *testing.T) {
		// Given a Talos bootstrap-token field
		input := `token: abcdef.0123456789abcdef`

		// When the line is redacted
		result := redactSecrets(input)

		// Then the token value is masked
		if strings.Contains(result, "0123456789abcdef") {
			t.Errorf("Expected token to be masked, got %q", result)
		}
	})

	t.Run("LeavesOrdinaryOutputUntouched", func(t *testing.T) {
		// Given a line with a short identifier that is not a secret
		input := `id = "abc123"`

		// When the line is redacted
		result := redactSecrets(input)

		// Then the line is unchanged
		if result != input {
			t.Errorf("Expected unchanged line, got %q", result)
		}
	})

	t.Run("LeavesLongOpaqueIdUntouched", func(t *testing.T) {
		// Given a non-secret id whose value runs past the base64 length floor
		input := `id = "` + strings.Repeat("A", 48) + `"`

		// When the line is redacted
		result := redactSecrets(input)

		// Then the id is left intact: id is a ubiquitous terraform field, not a secret
		if result != input {
			t.Errorf("Expected long id left unchanged, got %q", result)
		}
	})
}

func TestScrubbingWriter_Write(t *testing.T) {
	// setup builds a scrubbingWriter over a buffer with the given registered secrets.
	setup := func(secrets ...string) (*scrubbingWriter, *bytes.Buffer) {
		var sink bytes.Buffer
		scrub := func(in string) string {
			out := in
			for _, sec := range secrets {
				out = strings.ReplaceAll(out, sec, "********")
			}
			return out
		}
		return &scrubbingWriter{writer: &sink, scrubFunc: scrub}, &sink
	}

	t.Run("MasksMultiLinePEMBlockAsSingleUnit", func(t *testing.T) {
		// Given a real multi-line PEM block split across writes
		sw, sink := setup()

		// When the block is written line by line
		for _, line := range []string{
			"machine config:\n",
			"-----BEGIN RSA PRIVATE KEY-----\n",
			"MIIEpAIBAAKCAQEA1234567890\n",
			"abcdefghijklmnopqrstuvwxyz\n",
			"-----END RSA PRIVATE KEY-----\n",
			"done\n",
		} {
			if _, err := sw.Write([]byte(line)); err != nil {
				t.Fatalf("unexpected write error: %v", err)
			}
		}

		// Then no key material reaches the sink and the surrounding lines survive
		out := sink.String()
		if strings.Contains(out, "BEGIN RSA PRIVATE KEY") || strings.Contains(out, "MIIEpAIBAAKCAQEA") {
			t.Errorf("Expected PEM body to be masked, got %q", out)
		}
		if !strings.Contains(out, "machine config:") || !strings.Contains(out, "done") {
			t.Errorf("Expected surrounding lines preserved, got %q", out)
		}
		if !strings.Contains(out, "********") {
			t.Errorf("Expected mask marker, got %q", out)
		}
	})

	t.Run("BuffersPartialLineUntilFlush", func(t *testing.T) {
		// Given a line delivered without a trailing newline
		sw, sink := setup()

		// When a partial line is written
		if _, err := sw.Write([]byte("no newline yet")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}

		// Then nothing is emitted until Flush
		if sink.Len() != 0 {
			t.Errorf("Expected no output before flush, got %q", sink.String())
		}
		if err := sw.Flush(); err != nil {
			t.Fatalf("unexpected flush error: %v", err)
		}
		if sink.String() != "no newline yet" {
			t.Errorf("Expected flushed content, got %q", sink.String())
		}
	})

	t.Run("AppliesRegisteredSecretDenylist", func(t *testing.T) {
		// Given a writer with a registered secret
		sw, sink := setup("s3cr3t")

		// When a line containing the secret is written
		if _, err := sw.Write([]byte("value is s3cr3t here\n")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}

		// Then the registered secret is masked
		if strings.Contains(sink.String(), "s3cr3t") {
			t.Errorf("Expected registered secret masked, got %q", sink.String())
		}
	})

	t.Run("ReportsFullInputConsumed", func(t *testing.T) {
		// Given a writer whose redaction shortens the content
		sw, _ := setup()
		input := []byte(`key: ` + strings.Repeat("A", 60) + "\n")

		// When the input is written
		n, err := sw.Write(input)

		// Then the writer reports the full input length consumed despite masking
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if n != len(input) {
			t.Errorf("Expected %d bytes consumed, got %d", len(input), n)
		}
	})
}
