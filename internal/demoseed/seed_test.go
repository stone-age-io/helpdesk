package demoseed_test

import (
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/activity"
	"github.com/stone-age-io/helpdesk/internal/demoseed"
	"github.com/stone-age-io/helpdesk/internal/projects"
	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
	"github.com/stone-age-io/helpdesk/internal/visits"
)

// A modest ticket count keeps the suite quick; the properties under test are
// independent of scale.
const testTickets = 60

// registerHooks binds the same record hooks cmd/helpdesk does, minus the
// notification hooks: the seeder depends on them: `tickets` assigns the
// sequential number, `projects` does the same for projects, `visits` enforces
// the scheduled-visit guard, and `activity` writes the audit trail. Seeding
// without them fails on the very first duplicate number, which is the point —
// the seed goes in through production behaviour, not around it.
//
// Notifications are deliberately left unbound here; every seeded save is marked
// notifications.Suppress regardless, and binding a notifier in tests would only
// add mail plumbing to assertions that are about data shape.
func registerHooks(app *pocketbase.PocketBase) {
	tickets.Register(app)
	projects.Register(app)
	visits.Register(app)
	activity.Register(app)
}

// seedOnce boots a hooked app and runs the seeder against it.
func seedOnce(t *testing.T) core.App {
	t.Helper()
	app := testutil.SetupApp(t)
	registerHooks(app)
	if _, err := demoseed.Run(app, demoseed.Options{Tickets: testTickets}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return app
}

func count(t *testing.T, app core.App, collection string) int {
	t.Helper()
	recs, err := app.FindAllRecords(collection)
	if err != nil {
		t.Fatalf("count %s: %v", collection, err)
	}
	return len(recs)
}

func TestSeedPopulatesEveryCollection(t *testing.T) {
	app := seedOnce(t)

	for _, c := range []struct {
		name string
		min  int
	}{
		{"customers", 8},
		{"location_types", 5},
		{"thing_types", 8},
		{"locations", 20},
		{"things", 30},
		{"staff", 10},
		{"users", 20},
		{"projects", 8},
		{"tickets", testTickets},
		{"ticket_comments", 20},
		{"visits", 10},
		{"time_entries", 20},
		{"ticket_events", 20},
	} {
		if got := count(t, app, c.name); got < c.min {
			t.Errorf("%s: got %d records, want at least %d", c.name, got, c.min)
		}
	}
}

// The reason this seeder is Go rather than an HTTP script: PocketBase's autodate
// overwrites `created` on save and ignores it on update, so an external client
// cannot produce a demo whose ages look real.
func TestTicketsAreBackdated(t *testing.T) {
	app := seedOnce(t)

	recs, err := app.FindAllRecords("tickets")
	if err != nil {
		t.Fatalf("load tickets: %v", err)
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	var old int
	for _, r := range recs {
		if r.GetDateTime("created").Time().Before(cutoff) {
			old++
		}
	}
	if old == 0 {
		t.Fatal("no ticket is older than 30 days — backdating did not take, and the " +
			"queue's Age column and the ticket-volume report will both read flat")
	}
}

// A comment must never predate the ticket it replies to, or the timeline reads
// out of order.
func TestCommentsFallAfterTheirTicket(t *testing.T) {
	app := seedOnce(t)

	comments, err := app.FindAllRecords("ticket_comments")
	if err != nil {
		t.Fatalf("load comments: %v", err)
	}
	if len(comments) == 0 {
		t.Fatal("expected seeded comments")
	}
	for _, c := range comments {
		ticket, err := app.FindRecordById("tickets", c.GetString("ticket"))
		if err != nil {
			t.Fatalf("load parent ticket: %v", err)
		}
		if c.GetDateTime("created").Time().Before(ticket.GetDateTime("created").Time()) {
			t.Fatalf("comment %s predates its ticket (%s < %s)",
				c.Id, c.GetString("created"), ticket.GetString("created"))
		}
	}
}

// Re-running must converge, not duplicate. This is the property that lets an
// operator top up a demo host without wiping it first.
func TestSeedIsIdempotent(t *testing.T) {
	app := seedOnce(t)

	tracked := []string{
		"customers", "locations", "things", "thing_types", "location_types",
		"staff", "users", "projects", "tickets", "ticket_comments",
		"visits", "time_entries", "ticket_events",
	}
	before := map[string]int{}
	for _, c := range tracked {
		before[c] = count(t, app, c)
	}

	if _, err := demoseed.Run(app, demoseed.Options{Tickets: testTickets}); err != nil {
		t.Fatalf("second run: %v", err)
	}

	for _, c := range tracked {
		if got := count(t, app, c); got != before[c] {
			t.Errorf("%s: %d records after re-run, want %d unchanged", c, got, before[c])
		}
	}
}

// A run that dies partway must heal rather than leaving a ticket permanently
// childless. Deleting a ticket's children simulates that, and the next run
// should put them back — the failure mode that the "only seed children when the
// ticket is new" shortcut cannot recover from.
func TestSeedHealsMissingChildren(t *testing.T) {
	app := seedOnce(t)

	ticket, err := app.FindFirstRecordByFilter("tickets", "dedupe_key = {:d}",
		dbx.Params{"d": "seed-nw-reader-offline"})
	if err != nil {
		t.Fatalf("find hero ticket: %v", err)
	}
	comments, err := app.FindAllRecords("ticket_comments",
		dbx.NewExp("ticket = {:t}", dbx.Params{"t": ticket.Id}))
	if err != nil {
		t.Fatalf("load comments: %v", err)
	}
	if len(comments) == 0 {
		t.Fatal("expected the hero ticket to have comments")
	}
	want := len(comments)
	for _, c := range comments {
		if err := app.Delete(c); err != nil {
			t.Fatalf("delete comment: %v", err)
		}
	}

	if _, err := demoseed.Run(app, demoseed.Options{Tickets: testTickets}); err != nil {
		t.Fatalf("healing run: %v", err)
	}

	after, err := app.FindAllRecords("ticket_comments",
		dbx.NewExp("ticket = {:t}", dbx.Params{"t": ticket.Id}))
	if err != nil {
		t.Fatalf("reload comments: %v", err)
	}
	if len(after) != want {
		t.Fatalf("comments after healing run = %d, want %d", len(after), want)
	}
}

// Same seed, same data — this is what makes the dedupe keys stable across runs
// and machines.
func TestGenerationIsDeterministic(t *testing.T) {
	titles := func() []string {
		app := testutil.SetupApp(t)
		registerHooks(app)
		if _, err := demoseed.Run(app, demoseed.Options{Tickets: 40, Seed: 99}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		recs, err := app.FindAllRecords("tickets")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		byKey := map[string]string{}
		for _, r := range recs {
			byKey[r.GetString("dedupe_key")] = r.GetString("title")
		}
		out := make([]string, 0, len(byKey))
		for k, v := range byKey {
			out = append(out, k+"="+v)
		}
		return out
	}

	a, b := titles(), titles()
	if len(a) != len(b) {
		t.Fatalf("run sizes differ: %d vs %d", len(a), len(b))
	}
	seen := map[string]bool{}
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			t.Fatalf("second run produced a different ticket: %q", v)
		}
	}
}

// The demo must exercise every status, source, and both types, or the queue
// filters and report breakdowns have nothing to show.
func TestSeedCoversTheFilterSurface(t *testing.T) {
	app := seedOnce(t)

	recs, err := app.FindAllRecords("tickets")
	if err != nil {
		t.Fatalf("load tickets: %v", err)
	}
	statuses, sources, types := map[string]int{}, map[string]int{}, map[string]int{}
	var withThing, withThingNote, withProject int
	for _, r := range recs {
		statuses[r.GetString("status")]++
		sources[r.GetString("source")]++
		types[r.GetString("type")]++
		if r.GetString("thing") != "" {
			withThing++
		}
		if r.GetString("thing_note") != "" {
			withThingNote++
		}
		if r.GetString("project") != "" {
			withProject++
		}
	}

	for _, s := range []string{"open", "in_progress", "waiting", "resolved", "closed"} {
		if statuses[s] == 0 {
			t.Errorf("no ticket with status %q", s)
		}
	}
	for _, s := range []string{"portal", "agent", "nats", "webhook", "email"} {
		if sources[s] == 0 {
			t.Errorf("no ticket with source %q", s)
		}
	}
	for _, ty := range []string{"reactive", "planned"} {
		if types[ty] == 0 {
			t.Errorf("no ticket with type %q", ty)
		}
	}
	if withThing == 0 {
		t.Error("no ticket names a thing — the whole point of the things layer")
	}
	if withThingNote == 0 {
		t.Error("no ticket carries a thing_note — the unmatched-code fallback is unrepresented")
	}
	if withProject == 0 {
		t.Error("no ticket is attached to a project")
	}
}

// Retired things and codeless things are both deliberate demo cases: one shows
// retire-not-delete, the other shows the catalog is a superset of the platform's.
func TestSeedIncludesThingEdgeCases(t *testing.T) {
	app := seedOnce(t)

	things, err := app.FindAllRecords("things")
	if err != nil {
		t.Fatalf("load things: %v", err)
	}
	var retired, codeless, typed, withMetadata int
	for _, th := range things {
		if th.GetBool("retired") {
			retired++
		}
		if th.GetString("code") == "" {
			codeless++
		}
		if th.GetString("type") != "" {
			typed++
		}
		if len(th.GetString("metadata")) > 2 { // "{}" or empty means nothing stored
			withMetadata++
		}
	}
	if retired == 0 {
		t.Error("no retired thing")
	}
	if codeless == 0 {
		t.Error("no codeless thing — the non-platform-gear superset case is unrepresented")
	}
	if typed == 0 {
		t.Error("no thing carries a type")
	}
	if withMetadata == 0 {
		t.Error("no thing carries metadata, so no typed metadata form is demonstrable")
	}
}

// Locations must show a hierarchy and at least one type schema, since both are
// what the 1824000000 shape exists to carry.
func TestSeedIncludesLocationHierarchyAndSchemas(t *testing.T) {
	app := seedOnce(t)

	locs, err := app.FindAllRecords("locations")
	if err != nil {
		t.Fatalf("load locations: %v", err)
	}
	var withParent int
	for _, l := range locs {
		if l.GetString("parent") != "" {
			withParent++
		}
	}
	if withParent == 0 {
		t.Error("no location has a parent — the hierarchy is unrepresented")
	}

	var schemas int
	for _, collection := range []string{"thing_types", "location_types"} {
		recs, err := app.FindAllRecords(collection)
		if err != nil {
			t.Fatalf("load %s: %v", collection, err)
		}
		for _, r := range recs {
			if len(r.GetString("metadata_schema")) > 2 {
				schemas++
			}
		}
	}
	if schemas == 0 {
		t.Error("no type carries a metadata_schema, so no typed metadata form is demonstrable")
	}
}

// Time entries drive the billable/write-off split in reports.
func TestSeedIncludesBillableAndNonBillableTime(t *testing.T) {
	app := seedOnce(t)

	entries, err := app.FindAllRecords("time_entries")
	if err != nil {
		t.Fatalf("load time entries: %v", err)
	}
	var billable, nonBillable int
	for _, e := range entries {
		if e.GetBool("non_billable") {
			nonBillable++
		} else {
			billable++
		}
	}
	if billable == 0 || nonBillable == 0 {
		t.Errorf("time split is billable=%d non_billable=%d; need both for the write-off report",
			billable, nonBillable)
	}
}

// Visits must cover all four lifecycle states so Dispatch has a needs-scheduling
// bucket, a day-grouped schedule, and a completed history.
func TestSeedCoversVisitStatuses(t *testing.T) {
	app := seedOnce(t)

	visits, err := app.FindAllRecords("visits")
	if err != nil {
		t.Fatalf("load visits: %v", err)
	}
	seen := map[string]int{}
	for _, v := range visits {
		seen[v.GetString("status")]++
	}
	for _, st := range []string{"requested", "scheduled", "completed"} {
		if seen[st] == 0 {
			t.Errorf("no visit with status %q", st)
		}
	}
}

// Every ticket gets its sequential number from the production create hook, which
// only holds because the subcommand binds the same hooks the server does.
func TestSeededTicketsGetNumbers(t *testing.T) {
	app := seedOnce(t)

	recs, err := app.FindAllRecords("tickets")
	if err != nil {
		t.Fatalf("load tickets: %v", err)
	}
	seen := map[int]bool{}
	for _, r := range recs {
		n := r.GetInt("number")
		if n == 0 {
			t.Fatalf("ticket %s has no number — the tickets create hook did not run", r.Id)
		}
		if seen[n] {
			t.Fatalf("duplicate ticket number %d", n)
		}
		seen[n] = true
	}
}
