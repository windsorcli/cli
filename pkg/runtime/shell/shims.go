// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

package shell

import (
	"bufio"
	"crypto/rand"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"golang.org/x/term"
)

// =============================================================================
// Types
// =============================================================================

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
	// OS operations
	Getwd      func() (string, error)
	Stat       func(name string) (os.FileInfo, error)
	Executable func() (string, error)

	// Standard I/O operations
	Stderr    func() io.Writer
	SetStderr func(w io.Writer)
	Stdout    func() io.Writer
	SetStdout func(w io.Writer)
	Pipe      func() (*os.File, *os.File, error)

	// Shell operations
	UnsetEnvs  func(envVars []string)
	UnsetAlias func(aliases []string)

	// Exec operations
	Command     func(name string, arg ...string) *exec.Cmd
	Environ     func() []string
	LookPath    func(file string) (string, error)
	OpenFile    func(name string, flag int, perm os.FileMode) (*os.File, error)
	WriteFile   func(name string, data []byte, perm os.FileMode) error
	ReadFile    func(name string) ([]byte, error)
	MkdirAll    func(path string, perm os.FileMode) error
	Remove      func(name string) error
	RemoveAll   func(path string) error
	Chdir       func(dir string) error
	Setenv      func(key, value string) error
	Getenv      func(key string) string
	UserHomeDir func() (string, error)

	// Exec operations
	CmdRun     func(cmd *exec.Cmd) error
	CmdStart   func(cmd *exec.Cmd) error
	CmdWait    func(cmd *exec.Cmd) error
	StdoutPipe func(cmd *exec.Cmd) (io.ReadCloser, error)
	StderrPipe func(cmd *exec.Cmd) (io.ReadCloser, error)
	StdinPipe  func(cmd *exec.Cmd) (io.WriteCloser, error)

	// Template operations
	NewTemplate     func(name string) *template.Template
	TemplateParse   func(tmpl *template.Template, text string) (*template.Template, error)
	TemplateExecute func(tmpl *template.Template, wr io.Writer, data any) error
	ExecuteTemplate func(tmpl *template.Template, data any) error

	// Bufio operations
	NewScanner  func(r io.Reader) *bufio.Scanner
	ScannerScan func(scanner *bufio.Scanner) bool
	ScannerErr  func(scanner *bufio.Scanner) error
	ScannerText func(scanner *bufio.Scanner) string
	NewWriter   func(w io.Writer) *bufio.Writer

	// Filepath operations
	Glob func(pattern string) ([]string, error)
	Join func(elem ...string) string

	// Random operations
	RandRead func(b []byte) (n int, err error)

	// Terminal operations
	IsTerminal func(fd int) bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
	s := &Shims{
		// OS operations
		Getwd:      os.Getwd,
		Stat:       os.Stat,
		Executable: os.Executable,

		// Standard I/O operations
		Stderr: func() io.Writer {
			return os.Stderr
		},
		SetStderr: func(w io.Writer) {
			if f, ok := w.(*os.File); ok {
				os.Stderr = f
			}
		},
		Stdout: func() io.Writer {
			return os.Stdout
		},
		SetStdout: func(w io.Writer) {
			if f, ok := w.(*os.File); ok {
				os.Stdout = f
			}
		},
		Pipe: os.Pipe,

		// Shell operations
		UnsetEnvs:  func(envVars []string) {},
		UnsetAlias: func(aliases []string) {},

		// Exec operations
		Command:     exec.Command,
		Environ:     os.Environ,
		LookPath:    exec.LookPath,
		OpenFile:    os.OpenFile,
		WriteFile:   os.WriteFile,
		ReadFile:    os.ReadFile,
		MkdirAll:    os.MkdirAll,
		Remove:      os.Remove,
		RemoveAll:   os.RemoveAll,
		Chdir:       os.Chdir,
		Setenv:      os.Setenv,
		Getenv:      os.Getenv,
		UserHomeDir: os.UserHomeDir,

		// Exec operations
		CmdRun:     (*exec.Cmd).Run,
		CmdStart:   (*exec.Cmd).Start,
		CmdWait:    (*exec.Cmd).Wait,
		StdoutPipe: (*exec.Cmd).StdoutPipe,
		StderrPipe: (*exec.Cmd).StderrPipe,
		StdinPipe:  (*exec.Cmd).StdinPipe,

		// Template operations
		NewTemplate:     template.New,
		TemplateParse:   (*template.Template).Parse,
		TemplateExecute: (*template.Template).Execute,
		ExecuteTemplate: func(tmpl *template.Template, data any) error {
			return tmpl.Execute(os.Stdout, data)
		},

		// Bufio operations
		NewScanner: bufio.NewScanner,
		ScannerScan: func(scanner *bufio.Scanner) bool {
			return scanner.Scan()
		},
		ScannerErr: func(scanner *bufio.Scanner) error {
			return scanner.Err()
		},
		ScannerText: func(scanner *bufio.Scanner) string {
			return scanner.Text()
		},
		NewWriter: bufio.NewWriter,

		// Filepath operations
		Glob: filepath.Glob,
		Join: filepath.Join,

		// Random operations
		RandRead: func(b []byte) (n int, err error) {
			return rand.Read(b)
		},

		// Terminal operations
		IsTerminal: term.IsTerminal,
	}
	return s
}
