// The Shims are the system call abstractions of the shell package.
// It provides the shared shims of internal/shims and the exec, template, scanner, and signal shims.
// Tests replace the shims to simulate command output, file access, and terminal state.
// NewShims returns the shims with the real implementations.

package shell

import (
	"bufio"
	"crypto/rand"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"text/template"

	"golang.org/x/term"

	"github.com/windsorcli/cli/internal/shims"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the shared system call primitives and the shell-specific shims.
type Shims struct {
	shims.Shims
	Executable            func() (string, error)
	Command               func(name string, arg ...string) *exec.Cmd
	Environ               func() []string
	OpenFile              func(name string, flag int, perm os.FileMode) (*os.File, error)
	CmdRun                func(cmd *exec.Cmd) error
	CmdStart              func(cmd *exec.Cmd) error
	CmdWait               func(cmd *exec.Cmd) error
	StdoutPipe            func(cmd *exec.Cmd) (io.ReadCloser, error)
	StderrPipe            func(cmd *exec.Cmd) (io.ReadCloser, error)
	NewTemplate           func(name string) *template.Template
	TemplateParse         func(tmpl *template.Template, text string) (*template.Template, error)
	TemplateExecute       func(tmpl *template.Template, wr io.Writer, data any) error
	NewScanner            func(r io.Reader) *bufio.Scanner
	ScannerScan           func(scanner *bufio.Scanner) bool
	ScannerErr            func(scanner *bufio.Scanner) error
	ScannerText           func(scanner *bufio.Scanner) string
	RandRead              func(b []byte) (n int, err error)
	IsTerminal            func(fd int) bool
	SignalNotify          func(c chan<- os.Signal, sig ...os.Signal)
	SignalStop            func(c chan<- os.Signal)
	InterruptProcessGroup func(cmd *exec.Cmd) error
	KillProcessGroup      func(cmd *exec.Cmd) error
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns a Shims with every field set to its real implementation.
func NewShims() *Shims {
	return &Shims{
		Shims:                 *shims.NewShims(),
		Executable:            os.Executable,
		Command:               exec.Command,
		Environ:               os.Environ,
		OpenFile:              os.OpenFile,
		CmdRun:                (*exec.Cmd).Run,
		CmdStart:              (*exec.Cmd).Start,
		CmdWait:               (*exec.Cmd).Wait,
		StdoutPipe:            (*exec.Cmd).StdoutPipe,
		StderrPipe:            (*exec.Cmd).StderrPipe,
		NewTemplate:           template.New,
		TemplateParse:         (*template.Template).Parse,
		TemplateExecute:       (*template.Template).Execute,
		NewScanner:            bufio.NewScanner,
		ScannerScan:           (*bufio.Scanner).Scan,
		ScannerErr:            (*bufio.Scanner).Err,
		ScannerText:           (*bufio.Scanner).Text,
		RandRead:              rand.Read,
		IsTerminal:            term.IsTerminal,
		SignalNotify:          signal.Notify,
		SignalStop:            signal.Stop,
		InterruptProcessGroup: interruptProcessGroup,
		KillProcessGroup:      killProcessGroup,
	}
}
