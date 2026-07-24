package inbound

import (
	"encoding/json"
	"testing"
)

// A representative Postmark inbound payload (trimmed to the fields we read).
const postmarkFixture = `{
  "MessageID": "pm-abc-123",
  "From": "rita@acme.example",
  "FromFull": { "Email": "rita@acme.example", "Name": "Rita Requester" },
  "Subject": "Re: [#42] printer on fire",
  "TextBody": "still broken\n\nOn Tue, staff wrote:\n> did you try restarting?",
  "StrippedTextReply": "still broken",
  "Headers": [
    { "Name": "X-Spam-Status", "Value": "No" },
    { "Name": "Authentication-Results", "Value": "mx.example; dkim=pass header.d=acme.example" },
    { "Name": "Message-ID", "Value": "<orig@acme.example>" }
  ]
}`

func TestPostmarkNormalize(t *testing.T) {
	var pm postmarkInbound
	if err := json.Unmarshal([]byte(postmarkFixture), &pm); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	n := pm.normalize()

	if n.MessageID != "pm-abc-123" {
		t.Errorf("MessageID: got %q", n.MessageID)
	}
	if n.From.Email != "rita@acme.example" || n.From.Name != "Rita Requester" {
		t.Errorf("From: got %+v", n.From)
	}
	// Body prefers the stripped reply (quoted history gone).
	if n.Body != "still broken" {
		t.Errorf("Body should be the stripped reply, got %q", n.Body)
	}
	if !n.DKIMPass {
		t.Error("DKIMPass should be true for dkim=pass")
	}
	if n.SpamFlag {
		t.Error("SpamFlag should be false for X-Spam-Status: No")
	}
	// The core derives the thread token from Subject; the adapter leaves it blank.
	if got := ParseTicketToken(n.Subject); got != "42" {
		t.Errorf("subject token: got %q want 42", got)
	}
}

func TestPostmarkNormalizeFallbacks(t *testing.T) {
	// No stripped reply, no top-level MessageID, an explicit DKIM failure.
	const raw = `{
      "From": "x@y.test",
      "Subject": "help",
      "TextBody": "full body here",
      "Headers": [
        { "Name": "Message-ID", "Value": "<fallback@y.test>" },
        { "Name": "Authentication-Results", "Value": "mx; dkim=fail" },
        { "Name": "X-Spam-Status", "Value": "Yes, score=9" }
      ]
    }`
	var pm postmarkInbound
	if err := json.Unmarshal([]byte(raw), &pm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	n := pm.normalize()

	if n.Body != "full body here" {
		t.Errorf("Body should fall back to TextBody, got %q", n.Body)
	}
	if n.MessageID != "<fallback@y.test>" {
		t.Errorf("MessageID should fall back to the header, got %q", n.MessageID)
	}
	if n.DKIMPass {
		t.Error("DKIMPass should be false on dkim=fail")
	}
	if !n.SpamFlag {
		t.Error("SpamFlag should be true on X-Spam-Status: Yes")
	}
}

func TestSecretMatches(t *testing.T) {
	if !secretMatches("s3cret", "s3cret") {
		t.Error("equal secrets should match")
	}
	if secretMatches("wrong", "s3cret") {
		t.Error("unequal secrets should not match")
	}
	if secretMatches("", "s3cret") {
		t.Error("empty provided should not match")
	}
}

func TestIPAllowlist(t *testing.T) {
	nets := parseAllowedNets([]string{"1.2.3.4", "10.0.0.0/8"})

	if !ipInAllowlist("1.2.3.4", nets) {
		t.Error("exact IP should be allowed")
	}
	if !ipInAllowlist("10.9.8.7", nets) {
		t.Error("CIDR member should be allowed")
	}
	if ipInAllowlist("8.8.8.8", nets) {
		t.Error("outside IP should be rejected")
	}
	// Empty allowlist ⇒ allow everything (secret is the gate).
	if !ipInAllowlist("8.8.8.8", nil) {
		t.Error("empty allowlist should allow all")
	}
}
