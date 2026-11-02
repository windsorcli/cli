package cmd

import (
	"testing"
)

func TestAnnotatedVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"taggedRelease", "0.9.0", "0.9.0"},
		{"taggedReleaseCandidate", "0.9.0-rc.1", "0.9.0-rc.1"},
		{"goreleaserSnapshot", "0.0.0-SNAPSHOT-abc1234", "0.0.0-SNAPSHOT-abc1234 (nightly build)"},
		{"lowercaseSnapshot", "1.2.3-snapshot-deadbee", "1.2.3-snapshot-deadbee (nightly build)"},
		{"devBuildUnannotated", "dev", "dev"},
		{"emptyVersion", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := annotatedVersion(tc.in); got != tc.want {
				t.Errorf("annotatedVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
