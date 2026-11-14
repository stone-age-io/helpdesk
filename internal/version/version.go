// Package version answers "what build is this" for the one binary, in the two
// forms that ask: a string for a person reading `helpdesk version`, and a
// strict semver string for NATS service discovery.
//
// It is one package rather than a helper inside internal/natsx, where this
// logic started, because what the binary is has nothing to do with how it talks
// to NATS — discovery is simply the first caller that needed it, and an
// operator asking a binary its version should not go through the NATS client to
// get an answer.
package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Dev is what an untagged build reports, and what Semver falls back to.
const (
	Dev       = "(devel)"
	DevSemver = "0.0.0-dev"
)

// Build is the module version `go build` stamped from the checkout: a tag such
// as v0.1.0, a pseudo-version between tags, or "(devel)" when the build carried
// no VCS information. There are no ldflags to keep in sync — the toolchain
// already knows this, and a hand-stamped version is one more thing to forget on
// a release.
func Build() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return Dev
}

// semverPattern is the regular expression semver.org publishes, which is also
// the one nats micro validates a service version against.
var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// Semver renders a build version in the form nats micro accepts: the leading
// "v" is dropped, and anything still not semver (a bare "(devel)", most often)
// reports as 0.0.0-dev rather than refusing to register. Losing discovery
// because a developer build has no tag would be the wrong trade.
func Semver(v string) string {
	v = strings.TrimPrefix(v, "v")
	if semverPattern.MatchString(v) {
		return v
	}
	return DevSemver
}
