package version

import (
	"strings"
	"testing"
)

// Semver has one job and one failure mode: nats micro refuses a service version
// that is not strict semver, and losing discovery over an untagged developer
// build would be the wrong trade, so anything unparseable degrades to a
// placeholder rather than erroring.
func TestSemver(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"1.2", DevSemver},
		// What `go build` actually stamps: a pseudo-version from an untagged
		// checkout, and "(devel)" (or nothing) when it has no VCS info.
		{"v0.0.0-20261008040929-cdd0e877a05b+dirty", "0.0.0-20261008040929-cdd0e877a05b+dirty"},
		{Dev, DevSemver},
		{"", DevSemver},
		// The first tagged release, which is the case this package was written
		// in front of.
		{"v0.1.0", "0.1.0"},
	}
	for _, tc := range cases {
		if got := Semver(tc.in); got != tc.want {
			t.Errorf("Semver(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Build reads the toolchain's own stamp, so a test can only pin the contract
// every caller relies on: it is never empty, and it never carries whitespace
// that would corrupt a log line or a micro registration.
func TestBuildIsUsable(t *testing.T) {
	got := Build()
	if got == "" {
		t.Fatal("Build() is empty; callers print it unguarded")
	}
	if trimmed := strings.TrimSpace(got); trimmed != got {
		t.Errorf("Build() = %q, want no surrounding whitespace", got)
	}
	// Whatever it reports must survive the trip into service discovery.
	if s := Semver(got); !semverPattern.MatchString(s) {
		t.Errorf("Semver(Build()) = %q, which nats micro would refuse", s)
	}
}
