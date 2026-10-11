package presenter_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/windsorcli/cli/internal/presenter"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestGroup(t *testing.T) {
	kinds := func(rec *recorder, from int) []string {
		var got []string
		for _, e := range rec.events[from:] {
			got = append(got, string(e.Kind)+":"+e.Message)
		}
		return got
	}

	t.Run("StartsEachChildOnceUnderTheCurrentOperation", func(t *testing.T) {
		// Given a parent operation with a recording presenter
		rec := &recorder{}
		ctx, end := presenter.Begin(presenter.NewContext(context.Background(), rec), "wait", "Waiting")
		defer end(nil)
		parentID := rec.events[0].ID
		g := presenter.NewGroup(ctx, "kustomization:")

		// When children start, one twice
		g.Start("a")
		g.Start("b")
		g.Start("a")

		// Then each child starts once, under the parent, with its subject and name
		var children []string
		for _, e := range rec.events[1:] {
			if e.Kind != presenter.KindApplying || e.ParentID != parentID {
				t.Fatalf("unexpected event %+v", e)
			}
			children = append(children, e.Subject+"="+e.Message)
		}
		if !reflect.DeepEqual(children, []string{"kustomization:a=a", "kustomization:b=b"}) {
			t.Errorf("unexpected children %v", children)
		}
	})

	t.Run("ShowsNothingForAChildThatHasNotStarted", func(t *testing.T) {
		// Given a group with no started children
		rec := &recorder{}
		g := presenter.NewGroup(presenter.NewContext(context.Background(), rec), "k:")

		// When progress arrives for a child and the group closes
		g.Progress("blocked", "waiting on a namespace")
		g.Close(errors.New("timed out"))

		// Then no event is emitted
		if len(rec.events) != 0 {
			t.Errorf("expected no events, got %v", kinds(rec, 0))
		}
	})

	t.Run("ShowsHeldProgressWhenTheChildStarts", func(t *testing.T) {
		// Given progress held for a child that has not started
		rec := &recorder{}
		g := presenter.NewGroup(presenter.NewContext(context.Background(), rec), "k:")
		g.Progress("dns", "waiting on a namespace")

		// When the child starts
		g.Start("dns")

		// Then it starts and then shows the held progress
		if got := kinds(rec, 0); !reflect.DeepEqual(got, []string{"applying:dns", "progress:waiting on a namespace"}) {
			t.Errorf("unexpected events %v", got)
		}
	})

	t.Run("StartsAChildThatEndsBeforeItWasStarted", func(t *testing.T) {
		// Given a group with no started children
		rec := &recorder{}
		g := presenter.NewGroup(presenter.NewContext(context.Background(), rec), "k:")

		// When one child is done and another fails without a start
		g.Done("fast")
		g.Fail("broken", errors.New("boom"))
		g.Done("fast")

		// Then each still starts and ends once
		want := []string{"applying:fast", "applied:fast", "applying:broken", "failed:broken"}
		if got := kinds(rec, 0); !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("ClosesStartedChildrenInStartOrderWithTheError", func(t *testing.T) {
		// Given three started children, one already done
		rec := &recorder{}
		g := presenter.NewGroup(presenter.NewContext(context.Background(), rec), "k:")
		for _, name := range []string{"z", "m", "a"} {
			g.Start(name)
		}
		g.Done("m")
		errStop := errors.New("stopped")

		// When the group closes with an error
		g.Close(errStop)

		// Then the open children fail with the error, in start order
		var failed []string
		for _, e := range rec.events {
			if e.Kind == presenter.KindFailed {
				if !errors.Is(e.Err, errStop) {
					t.Errorf("unexpected error %v", e.Err)
				}
				failed = append(failed, e.Message)
			}
		}
		if !reflect.DeepEqual(failed, []string{"z", "a"}) {
			t.Errorf("unexpected failures %v", failed)
		}
	})
}
