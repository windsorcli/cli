package shell

import "io"

// scrubbingWriter wraps an io.Writer and scrubs secrets from content before writing
type scrubbingWriter struct {
	writer    io.Writer
	scrubFunc func(string) string
}

// Write scrubs secrets from content and writes to the underlying writer.
// If scrubbing changes the content length, it pads with spaces or truncates
// to maintain the original byte length for consistent output formatting.
func (sw *scrubbingWriter) Write(p []byte) (n int, err error) {
	original := string(p)
	scrubbed := sw.scrubFunc(original)

	if scrubbed != original {
		scrubbedBytes := []byte(scrubbed)
		originalLen := len(p)
		scrubbedLen := len(scrubbedBytes)

		if scrubbedLen < originalLen {
			padding := make([]byte, originalLen-scrubbedLen)
			for i := range padding {
				padding[i] = ' '
			}
			scrubbedBytes = append(scrubbedBytes, padding...)
		} else if scrubbedLen > originalLen {
			scrubbedBytes = scrubbedBytes[:originalLen]
		}

		return sw.writer.Write(scrubbedBytes)
	}

	return sw.writer.Write(p)
}
