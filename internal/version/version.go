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

// stamped is set at link time by release builds:
//
//	-ldflags "-X github.com/stone-age-io/helpdesk/internal/version.stamped=v0.1.0"
//
// It is empty for an ordinary `go build`, which is the common case and needs
// nothing: the toolchain already stamps the module version from VCS, and a
// developer build reporting its own commit is more useful than one reporting a
// number somebody typed.
//
// The override exists because two release paths cannot use VCS at all. The
// container build copies the source WITHOUT .git (see .dockerignore, which
// excludes it deliberately — a build context carrying the whole history is
// waste at best), so ReadBuildInfo there has nothing to read and would report
// "(devel)" on an image tagged v0.1.0. goreleaser cross-compiles from a clean
// tag and would mostly be fine, but it passes the same flag so that every
// artifact of a release agrees, however it was produced.
var stamped string

// Build is the version this binary reports: the link-time stamp when a release
// set one, otherwise the module version `go build` took from the checkout — a
// tag such as v0.1.0, a pseudo-version between tags, or "(devel)" with no VCS
// information at all.
func Build() string {
	if stamped != "" {
		return stamped
	}
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
