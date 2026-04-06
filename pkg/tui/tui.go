package tui

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/briandowns/spinner"
)

// The tui package provides centralized terminal user interface primitives for the Windsor CLI.
// It provides a Spinner interface for displaying progress messages during long-running operations.
// Active is the live implementation and can be replaced for testing or alternative UI backends.
// WithProgress is the primary convenience entry point for wrapping operations with progress display.

// =============================================================================
// Types
// =============================================================================

// termSpinner is the default terminal Spinner implementation backed by an animated spinner.
type termSpinner struct {
	spin    *spinner.Spinner
	message string
}

// verboseSpinner is the Spinner implementation used in verbose mode.
type verboseSpinner struct{}

// =============================================================================
// Interfaces
// =============================================================================

// Spinner is the interface for terminal progress feedback.
type Spinner interface {
	Start(message string)
	Update(message string)
	Done()
	Fail()
}

// =============================================================================
// Constructor
// =============================================================================

// Active is the live Spinner instance used by package-level functions.
// Replace this in tests or when switching to an alternative UI implementation.
var Active Spinner = &termSpinner{}

// activeDepth tracks nesting of WithProgress calls. When greater than zero,
// package-level Start/Done/Fail are suppressed so only the outermost layer
// produces a terminal output line.
var activeDepth int32

// Init configures Active for the current run mode.
// In verbose mode, Active is set to a verboseSpinner that prints messages without animation.
func Init(verbose bool) {
	if verbose {
		Active = &verboseSpinner{}
		return
	}
	Active = &termSpinner{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Start begins a new progress spinner with the given message.
// When called inside a WithProgress block, it is a no-op so the layer message is preserved.
func Start(message string) {
	if atomic.LoadInt32(&activeDepth) > 0 {
		return
	}
	Active.Start(message)
}

// Update changes the suffix on the currently active spinner.
func Update(message string) { Active.Update(message) }

// Done stops the active spinner and prints a success line.
// When called inside a WithProgress block, it is a no-op.
func Done() {
	if atomic.LoadInt32(&activeDepth) > 0 {
		return
	}
	Active.Done()
}

// Fail stops the active spinner and prints a failure line.
// When called inside a WithProgress block, it is a no-op.
func Fail() {
	if atomic.LoadInt32(&activeDepth) > 0 {
		return
	}
	Active.Fail()
}

// WithProgress runs fn with a progress spinner showing message.
// Increments the nesting depth so that any Start/Done/Fail calls inside fn
// are suppressed — only this layer's Done or Fail line is printed.
// When already inside a WithProgress block, fn is run directly without spinner management.
func WithProgress(message string, fn func() error) error {
	if atomic.LoadInt32(&activeDepth) > 0 {
		return fn()
	}
	Active.Start(message)
	atomic.AddInt32(&activeDepth, 1)
	failed := true
	defer func() {
		atomic.AddInt32(&activeDepth, -1)
		if failed {
			Active.Fail()
		}
	}()
	err := fn()
	if err != nil {
		return err
	}
	failed = false
	Active.Done()
	return nil
}

// Start stops any existing spinner and begins a new one with the given message.
func (s *termSpinner) Start(message string) {
	if s.spin != nil {
		s.spin.Stop()
	}
	s.message = message
	s.spin = spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithColor("green"), spinner.WithWriter(os.Stderr))
	s.spin.Suffix = " " + message
	s.spin.Start()
}

// Update changes the spinner suffix to the given message without altering the stored message.
func (s *termSpinner) Update(message string) {
	if s.spin != nil {
		s.spin.Suffix = " " + message
	}
}

// Done stops the spinner and prints a green success line to stderr.
func (s *termSpinner) Done() {
	if s.spin != nil {
		s.spin.Stop()
		s.spin = nil
	}
	fmt.Fprintf(os.Stderr, "\033[32m✔\033[0m %s - \033[32mDone\033[0m\n", s.message)
}

// Fail stops the spinner and prints a red failure line to stderr.
func (s *termSpinner) Fail() {
	if s.spin != nil {
		s.spin.Stop()
		s.spin = nil
	}
	fmt.Fprintf(os.Stderr, "\033[31m✗ %s - Failed\033[0m\n", s.message)
}

// Start prints the message directly to stderr without animation.
func (s *verboseSpinner) Start(message string) {
	fmt.Fprintln(os.Stderr, message)
}

// Update prints the updated message directly to stderr.
func (s *verboseSpinner) Update(message string) {
	fmt.Fprintln(os.Stderr, message)
}

// Done is a no-op in verbose mode since output is already visible.
func (s *verboseSpinner) Done() {}

// Fail is a no-op in verbose mode since errors are surfaced through return values.
func (s *verboseSpinner) Fail() {}
