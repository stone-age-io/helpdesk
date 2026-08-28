package migrations_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/authz"
	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
)

// These exercise 1825000000: the portal create rule trading the `location` and
// `thing` :isset guards for tenant-scoped relation hops.
//
// Unlike every other rule test in this package these run the rule for real,
// over HTTP. A string assertion cannot distinguish `:isset = false` from `= ''`,
// and that distinction is the whole migration — see the table in
// 1825000000_portal_site_device.go.

func TestPortalSiteDeviceCreateRuleIntact(t *testing.T) {
	app := testutil.SetupApp(t)

	ticketsCol, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	if ticketsCol.CreateRule == nil {
		t.Fatal("tickets create rule is nil")
	}
	rule := *ticketsCol.CreateRule

	// The five triage fields stay staff-only. Losing one of these is a silent
	// portal privilege escalation.
	for _, clause := range []string{
		authz.StaffRule,
		authz.RequesterRule,
		"@request.body.customer = @request.auth.customer",
		"@request.body.requester = @request.auth.id",
		"@request.body.assignee:isset = false",
		"@request.body.category:isset = false",
		"@request.body.project:isset = false",
		"@request.body.type:isset = false",
		"@request.body.estimated_minutes:isset = false",
		"@request.body.source = 'portal'",
	} {
		if !strings.Contains(rule, clause) {
			t.Errorf("tickets create rule missing %q", clause)
		}
	}

	// The two that changed: no longer :isset-guarded, now tenant-hopped.
	for _, field := range []string{"location", "thing"} {
		if strings.Contains(rule, "@request.body."+field+":isset = false") {
			t.Errorf("%s should no longer be :isset-guarded — requesters set it now", field)
		}
		hop := "@request.body." + field + ".customer = @request.auth.customer"
		if !strings.Contains(rule, hop) {
			t.Errorf("tickets create rule missing tenant hop %q", hop)
		}
		// The empty-value term must be present AND must precede the hop; see the
		// migration comment. Without it every ticket filed without a site is
		// rejected, because an untouched picker submits "" rather than nothing.
		empty := "@request.body." + field + " = ''"
		ei, hi := strings.Index(rule, empty), strings.Index(rule, hop)
		if ei < 0 {
			t.Errorf("tickets create rule missing empty-value term %q", empty)
		} else if ei > hi {
			t.Errorf("%s empty-value term must come before the hop", field)
		}
	}

	// Still unguarded — the free-text fallbacks are how a requester names gear
	// that isn't in the catalog, which is most of it.
	for _, clause := range []string{"thing_note:isset", "location_note:isset"} {
		if strings.Contains(rule, clause) {
			t.Errorf("tickets create rule should not guard %q", clause)
		}
	}
}

// portalFixture builds a requester with one site and one device of their own,
// plus a second customer's site and device to attack with.
type portalFixture struct {
	mux        http.Handler
	token      string
	customer   string
	requester  string
	myLocation string
	myThing    string
	theirLocID string
	theirThing string
}

func newPortalFixture(t *testing.T) portalFixture {
	t.Helper()

	app := testutil.SetupApp(t)
	// Without the create hook every ticket gets number 0 and the second create
	// fails on the unique index — which looks exactly like a rule rejection.
	tickets.Register(app)

	mine := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	theirs := seed(t, app, "customers", map[string]any{"name": "Globex", "active": true})

	myLoc := seed(t, app, "locations", map[string]any{"customer": mine.Id, "name": "Acme HQ"})
	theirLoc := seed(t, app, "locations", map[string]any{"customer": theirs.Id, "name": "Globex HQ"})
	myThing := seed(t, app, "things", map[string]any{
		"customer": mine.Id, "name": "North Door Reader", "location": myLoc.Id,
	})
	theirThing := seed(t, app, "things", map[string]any{
		"customer": theirs.Id, "name": "Globex Reader", "location": theirLoc.Id,
	})

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	u := core.NewRecord(users)
	u.Set("email", "requester@acme.test")
	u.Set("password", "password12345")
	u.Set("customer", mine.Id)
	u.Set("active", true)
	if err := app.Save(u); err != nil {
		t.Fatalf("save requester: %v", err)
	}

	return portalFixture{
		mux:        testutil.Handler(t, app),
		token:      testutil.AuthToken(t, u),
		customer:   mine.Id,
		requester:  u.Id,
		myLocation: myLoc.Id,
		myThing:    myThing.Id,
		theirLocID: theirLoc.Id,
		theirThing: theirThing.Id,
	}
}

// post files a ticket as the requester and returns the HTTP status.
func (f portalFixture) post(t *testing.T, extra map[string]any) int {
	t.Helper()

	body := map[string]any{
		"customer":  f.customer,
		"requester": f.requester,
		"title":     "Badge reader is dead",
		"source":    "portal",
	}
	for k, v := range extra {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest("POST", "/api/collections/tickets/records", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", f.token)
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec.Code
}

// TestPortalCanSetOwnSiteAndDevice is the feature: the requester names where
// the problem is and what it's on.
func TestPortalCanSetOwnSiteAndDevice(t *testing.T) {
	f := newPortalFixture(t)

	if code := f.post(t, map[string]any{"location": f.myLocation}); code != http.StatusOK {
		t.Errorf("own location: got %d, want 200", code)
	}
	if code := f.post(t, map[string]any{"thing": f.myThing}); code != http.StatusOK {
		t.Errorf("own thing: got %d, want 200", code)
	}
	if code := f.post(t, map[string]any{"location": f.myLocation, "thing": f.myThing}); code != http.StatusOK {
		t.Errorf("own location+thing: got %d, want 200", code)
	}
}

// TestPortalEmptySiteAndDeviceStillFiles is the regression this migration's
// comment is mostly about. An untouched picker submits "", not nothing; the
// `:isset = false` form of the guard rejects it, which would have broken the
// ordinary "I don't know / don't care" ticket — the majority of them.
func TestPortalEmptySiteAndDeviceStillFiles(t *testing.T) {
	f := newPortalFixture(t)

	cases := map[string]map[string]any{
		"both absent":    {},
		"both empty":     {"location": "", "thing": ""},
		"empty location": {"location": "", "thing": f.myThing},
		"empty thing":    {"location": f.myLocation, "thing": ""},
		"null location":  {"location": nil, "thing": nil},
		"note fallbacks": {"location_note": "north door", "thing_note": "the beige reader"},
	}
	for name, body := range cases {
		if code := f.post(t, body); code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", name, code)
		}
	}
}

// TestPortalCannotSetOtherTenantSiteOrDevice is why the :isset guards could not
// simply be deleted. PocketBase validates that a relation id EXISTS, not that
// it is yours — without the hop these all succeed, cross-tenant, and the ticket
// detail then expands and renders the other customer's names.
func TestPortalCannotSetOtherTenantSiteOrDevice(t *testing.T) {
	f := newPortalFixture(t)

	cases := map[string]map[string]any{
		"their location":        {"location": f.theirLocID},
		"their thing":           {"thing": f.theirThing},
		"their location + mine": {"location": f.theirLocID, "thing": f.myThing},
		"mine + their thing":    {"location": f.myLocation, "thing": f.theirThing},
		// The multi-relation wire shape: PocketBase accepts an array for a
		// maxSelect=1 relation, and the hop must resolve through it too.
		"their location boxed": {"location": []string{f.theirLocID}},
		"their thing boxed":    {"thing": []string{f.theirThing}},
		"nonexistent id":       {"location": "zzzzzzzzzzzzzzz"},
	}
	for name, body := range cases {
		if code := f.post(t, body); code == http.StatusOK {
			t.Errorf("%s: got 200, want rejection — cross-tenant write", name)
		}
	}
}

// TestPortalStillCannotTriage pins that opening up location/thing did not open
// up the triage fields alongside them.
func TestPortalStillCannotTriage(t *testing.T) {
	f := newPortalFixture(t)

	for _, field := range []string{"category", "project", "type", "estimated_minutes", "assignee"} {
		var value any = "anything"
		if field == "estimated_minutes" {
			value = 30
		}
		if code := f.post(t, map[string]any{field: value}); code == http.StatusOK {
			t.Errorf("requester set %s: got 200, want rejection", field)
		}
	}
}
