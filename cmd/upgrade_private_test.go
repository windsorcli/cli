package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestUnconfirmedPrunes(t *testing.T) {
	t.Run("NilConfirmedMeansNoPlanWasShown", func(t *testing.T) {
		if got := unconfirmedPrunes([]string{"a"}, nil); got != nil {
			t.Errorf("Expected nil, got %v", got)
		}
	})

	t.Run("ReturnsNamesOutsideTheConfirmedSet", func(t *testing.T) {
		got := unconfirmedPrunes([]string{"a", "b", "c"}, map[string]bool{"a": true})
		if strings.Join(got, ",") != "b,c" {
			t.Errorf("Expected b,c, got %v", got)
		}
	})

	t.Run("EmptyConfirmedSetFlagsEveryName", func(t *testing.T) {
		got := unconfirmedPrunes([]string{"a"}, map[string]bool{})
		if len(got) != 1 {
			t.Errorf("Expected the name flagged, got %v", got)
		}
	})
}

func TestConfirmUpgrade(t *testing.T) {
	changes := sourceChanges{{Name: "core", From: "oci://x/core:v1", To: "oci://x/core:v2"}}

	t.Run("AcceptsYesInAnyCase", func(t *testing.T) {
		for _, answer := range []string{"y\n", "yes\n", "YES\n", "  Y  \n"} {
			if !confirmUpgrade(strings.NewReader(answer), &bytes.Buffer{}, "local", changes, nil, nil) {
				t.Errorf("Expected %q to confirm", answer)
			}
		}
	})

	t.Run("DeclinesAnythingElse", func(t *testing.T) {
		for _, answer := range []string{"\n", "n\n", "no\n", "maybe\n", ""} {
			if confirmUpgrade(strings.NewReader(answer), &bytes.Buffer{}, "local", changes, nil, nil) {
				t.Errorf("Expected %q to decline", answer)
			}
		}
	})

	t.Run("PlanStatesNoChangesAndNothingToPrune", func(t *testing.T) {
		var out bytes.Buffer
		confirmUpgrade(strings.NewReader("n\n"), &out, "local", nil, nil, nil)

		for _, want := range []string{"Upgrade plan for context local", "Sources: no changes", "Kustomizations to prune: none"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("Expected %q in the plan, got:\n%s", want, out.String())
			}
		}
	})

	t.Run("PlanReportsAFailedPruneListingAndStillAsks", func(t *testing.T) {
		var out bytes.Buffer
		confirmed := confirmUpgrade(strings.NewReader("y\n"), &out, "local", changes, nil, fmt.Errorf("cluster unreachable"))

		if !strings.Contains(out.String(), "Kustomizations to prune: unknown (cluster unreachable)") {
			t.Errorf("Expected the listing failure in the plan, got:\n%s", out.String())
		}
		if !confirmed {
			t.Error("Expected the operator's yes to count even when the prune listing failed")
		}
	})
}
