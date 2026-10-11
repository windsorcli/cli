// The process file holds the presenter state that belongs to the whole process: the terminal.
// It provides the default presenter, terminal handoff, and interrupt guard tracking.
// Code that hands the terminal to another program, such as a sudo password prompt, uses HandOff.
// Code that has no context yet reaches the default presenter through Background.

package presenter

import (
	"context"
	"sync/atomic"
)

// =============================================================================
// Types
// =============================================================================

// pauser is a Presenter that can hand the terminal to another writer and take it back.
type pauser interface {
	Pause()
	Resume()
}

// defaultHolder wraps the default presenter for atomic storage.
type defaultHolder struct {
	p Presenter
}

// =============================================================================
// Constructor
// =============================================================================

// defaultPresenter is the presenter that owns the terminal for this process.
var defaultPresenter atomic.Pointer[defaultHolder]

// interruptGuards counts the open interrupt guards, which handle interrupt signals themselves.
var interruptGuards atomic.Int32

// =============================================================================
// Public Methods
// =============================================================================

// SetDefault makes p the presenter that owns the terminal for this process. Nil clears it.
func SetDefault(p Presenter) {
	if p == nil {
		defaultPresenter.Store(nil)
		return
	}
	defaultPresenter.Store(&defaultHolder{p: p})
}

// Default returns the presenter that owns the terminal, or Discard when none is set.
func Default() Presenter {
	if h := defaultPresenter.Load(); h != nil {
		return h.p
	}
	return Discard
}

// Background returns a background context that carries the default presenter. It is for code
// that has no context of its own yet; code with a context uses that context instead.
func Background() context.Context {
	return NewContext(context.Background(), Default())
}

// HandOff pauses the default presenter so another program can use the terminal, and returns a
// function that resumes it. It does nothing for a presenter that cannot pause. Call the returned
// function once the other program is done.
func HandOff() func() {
	p, ok := Default().(pauser)
	if !ok {
		return func() {}
	}
	p.Pause()
	return p.Resume
}

// GuardInterrupts marks that a caller handles interrupt signals itself until the returned release
// function runs. Release runs once.
func GuardInterrupts() func() {
	interruptGuards.Add(1)
	var released atomic.Bool
	return func() {
		if released.CompareAndSwap(false, true) {
			interruptGuards.Add(-1)
		}
	}
}

// InterruptsGuarded reports whether an interrupt guard is open.
func InterruptsGuarded() bool {
	return interruptGuards.Load() > 0
}
