package presenter_test

import (
	"context"
	"testing"

	"github.com/windsorcli/cli/internal/presenter"
)

// =============================================================================
// Test Setup
// =============================================================================

type pausingRecorder struct {
	recorder
	calls []string
}

func (p *pausingRecorder) Pause()  { p.calls = append(p.calls, "pause") }
func (p *pausingRecorder) Resume() { p.calls = append(p.calls, "resume") }

// =============================================================================
// Test Public Methods
// =============================================================================

func TestDefault(t *testing.T) {
	t.Cleanup(func() { presenter.SetDefault(nil) })

	t.Run("ReturnsDiscardUntilSet", func(t *testing.T) {
		// Given no default presenter
		presenter.SetDefault(nil)

		// When the default is read
		got := presenter.Default()

		// Then it is Discard
		if got != presenter.Discard {
			t.Errorf("expected Discard, got %T", got)
		}
	})

	t.Run("CarriesTheDefaultInTheBackgroundContext", func(t *testing.T) {
		// Given a default presenter
		rec := &recorder{}
		presenter.SetDefault(rec)

		// When an operation runs in the background context
		_ = presenter.Track(presenter.Background(), "oci", "Loading OCI sources", func(context.Context) error { return nil })

		// Then the default presenter receives it
		if len(rec.events) != 2 || rec.events[0].Message != "Loading OCI sources" {
			t.Errorf("unexpected events %+v", rec.events)
		}
	})
}

func TestHandOff(t *testing.T) {
	t.Cleanup(func() { presenter.SetDefault(nil) })

	t.Run("PausesAndResumesTheDefault", func(t *testing.T) {
		// Given a default presenter that can pause
		p := &pausingRecorder{}
		presenter.SetDefault(p)

		// When the terminal is handed off and taken back
		resume := presenter.HandOff()
		resume()

		// Then the presenter pauses and then resumes
		if len(p.calls) != 2 || p.calls[0] != "pause" || p.calls[1] != "resume" {
			t.Errorf("unexpected calls %v", p.calls)
		}
	})

	t.Run("DoesNothingForAPresenterThatCannotPause", func(t *testing.T) {
		// Given a default presenter that cannot pause
		presenter.SetDefault(&recorder{})

		// When the terminal is handed off and taken back
		resume := presenter.HandOff()

		// Then the resume function runs without effect
		resume()
	})
}

func TestGuardInterrupts(t *testing.T) {
	t.Run("ReportsGuardedUntilEveryGuardIsReleased", func(t *testing.T) {
		// Given two open guards
		first := presenter.GuardInterrupts()
		second := presenter.GuardInterrupts()

		// When the first is released twice and then the second once
		first()
		first()
		stillGuarded := presenter.InterruptsGuarded()
		second()

		// Then interrupts stay guarded until the last guard is released
		if !stillGuarded || presenter.InterruptsGuarded() {
			t.Errorf("expected guarded then unguarded, got %v then %v", stillGuarded, presenter.InterruptsGuarded())
		}
	})
}
