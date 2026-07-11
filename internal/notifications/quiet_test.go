package notifications

import (
	"net/http/httptest"
	"testing"
)

func TestQuietRequested(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "TRUE": true, "yes": true,
		"0": false, "false": false, "off": false, "": false,
	}
	for header, want := range cases {
		req := httptest.NewRequest("PATCH", "/", nil)
		if header != "" {
			req.Header.Set("X-Helpdesk-Quiet", header)
		}
		if got := quietRequested(req); got != want {
			t.Errorf("quietRequested(%q) = %v, want %v", header, got, want)
		}
	}
}
