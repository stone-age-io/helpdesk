package metrics

import (
	"encoding/base64"
	"testing"
)

// The two forms every common scraper sends, and nothing else. The username
// half of Basic is ignored on purpose: a scraper's basic_auth block always has
// one, it is not a second secret, and demanding a particular value would only
// produce 401s nobody can diagnose.
func TestAuthorized(t *testing.T) {
	basic := func(user, pass string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	}
	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"bearer", "Bearer s3cret", true},
		{"bearer is case-insensitive on the scheme", "bearer s3cret", true},
		{"basic with any username", basic("prometheus", "s3cret"), true},
		{"basic with an empty username", basic("", "s3cret"), true},
		{"basic with a colon in the password", basic("x", "s3cret"), true},
		{"wrong bearer token", "Bearer nope", false},
		{"wrong basic password", basic("x", "nope"), false},
		{"empty header", "", false},
		{"no scheme", "s3cret", false},
		{"unknown scheme", "Digest s3cret", false},
		{"token in the username half is not accepted", basic("s3cret", "nope"), false},
		{"undecodable basic payload", "Basic !!!not-base64!!!", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := authorized(tc.header, "s3cret"); got != tc.want {
				t.Errorf("authorized(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

// A password that merely shares a prefix must not pass. The comparison is
// constant-time, so this also pins that it is a full-length check.
func TestAuthorizedRejectsPrefix(t *testing.T) {
	if authorized("Bearer s3", "s3cret") {
		t.Error("a prefix of the token was accepted")
	}
	if authorized("Bearer s3cretlonger", "s3cret") {
		t.Error("a superstring of the token was accepted")
	}
}
