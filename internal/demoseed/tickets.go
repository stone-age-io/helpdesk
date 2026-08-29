package demoseed

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/activity"
	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// seedTickets builds the complete fixture list FIRST, then writes it.
//
// The split is load-bearing, not tidiness. Writing is conditional — a record
// that already exists is skipped — so if writing consumed randomness, a re-run
// would advance the PRNG differently from the first run and every subsequent
// generated ticket would come out different, defeating the dedupe keys and
// duplicating children. Generation spends all the randomness (including
// resolving day offsets into absolute timestamps); writing spends none.
func (s *seeder) seedTickets() error {
	fixtures := make([]ticketFixture, 0, s.opts.Tickets)
	for _, t := range heroTickets {
		fixtures = append(fixtures, t)
	}
	for i := 0; i < s.opts.Tickets-len(heroTickets); i++ {
		fixtures = append(fixtures, s.generateTicket(i))
	}
	for i := range fixtures {
		s.resolveTimes(&fixtures[i])
	}

	for _, t := range fixtures {
		if err := s.writeTicket(t); err != nil {
			return fmt.Errorf("ticket %s: %w", t.Key, err)
		}
	}
	return nil
}

// resolveTimes converts a fixture's relative day offsets into absolute
// timestamps. Called for every fixture before any writing, so the PRNG sequence
// is identical whether a run creates records or finds them already there.
func (s *seeder) resolveTimes(f *ticketFixture) {
	f.CreatedAt = s.at(f.CreatedDays)
	for i := range f.Comments {
		at := s.at(f.Comments[i].DayOffset)
		// A reply can never predate the ticket it replies to.
		if at.Before(f.CreatedAt) {
			at = f.CreatedAt.Add(time.Hour)
		}
		f.Comments[i].At = at
	}
	for i := range f.Visits {
		f.Visits[i].ScheduledAt = s.at(f.Visits[i].ScheduledDays)
		if d := f.Visits[i].CompletedDays; d != nil {
			at := s.at(*d)
			f.Visits[i].CompletedAt = &at
		}
	}
}

// thingIndex is a per-customer view of the fixtures, used by the generator to
// pick a coherent (location, thing, type) triple rather than a random one.
type genTarget struct {
	customer, locationCode, thingKey, thingType string
}

func (s *seeder) targets() []genTarget {
	out := make([]genTarget, 0, len(things))
	for _, t := range things {
		if t.Retired {
			continue // a new ticket against decommissioned kit would read oddly
		}
		key := t.Code
		if key == "" {
			key = t.Name
		}
		loc := t.Location
		out = append(out, genTarget{customer: t.Customer, locationCode: loc, thingKey: key, thingType: t.Type})
	}
	return out
}

// generateTicket builds one plausible ticket. Everything is drawn from the
// seeded PRNG, so run N of a given seed always produces the same ticket N —
// which is what lets the dedupe key make re-runs idempotent.
func (s *seeder) generateTicket(i int) ticketFixture {
	targets := s.targets()
	tgt := targets[s.rng.Intn(len(targets))]

	// Weighted so the queue looks like a desk with history: most work is done,
	// a minority is live. `waiting` stays rare — it means blocked on a third party.
	status := s.weighted(map[string]int{
		"closed": 34, "resolved": 16, "open": 22, "in_progress": 20, "waiting": 8,
	})
	priority := s.weighted(map[string]int{"low": 22, "normal": 52, "high": 20, "urgent": 6})
	source := s.weighted(map[string]int{"portal": 38, "agent": 28, "nats": 14, "webhook": 10, "email": 10})
	ttype := s.weighted(map[string]int{"reactive": 82, "planned": 18})

	// Age: skewed recent, with a long tail so the aging buckets and the
	// ticket-volume report both have something to show.
	age := -s.weighted6040()

	locName := "the site"
	for _, l := range locations {
		if l.Code == tgt.locationCode {
			locName = l.Name
			break
		}
	}

	var title string
	if ttype == "planned" {
		title = fmt.Sprintf(s.pick(installTemplates), locName)
	} else {
		tpl := issueTemplates[tgt.thingType]
		if len(tpl) == 0 {
			tpl = issueTemplates[""]
		}
		title = fmt.Sprintf(s.pick(tpl), locName)
	}

	body := s.pick(bodyOpeners) + " " + s.pick(bodyDetails)

	// Requesters belong to a customer; machine sources have none.
	var requester string
	if source == "portal" || source == "email" {
		var pool []string
		for _, r := range requesters {
			if r.Customer == tgt.customer {
				pool = append(pool, r.Key)
			}
		}
		if len(pool) > 0 {
			requester = pool[s.rng.Intn(len(pool))]
		}
	}

	// Unassigned work is real, so leave some of it that way.
	assignee := ""
	if s.rng.Intn(100) < 82 {
		assignee = staffMembers[s.rng.Intn(len(staffMembers))].Key
	}

	category := s.categoryFor(tgt.thingType, ttype)

	f := ticketFixture{
		Key:         fmt.Sprintf("gen-%04d", i),
		Customer:    tgt.customer,
		Requester:   requester,
		Title:       title,
		Body:        body,
		Priority:    priority,
		Source:      source,
		Category:    category,
		Type:        ttype,
		Location:    tgt.locationCode,
		Assignee:    assignee,
		CreatedDays: age,
	}

	// Most tickets name a device; some only carry the free-text note, which is
	// what an unmatched intake code or a vague portal report actually leaves.
	switch {
	case s.rng.Intn(100) < 72:
		f.Thing = tgt.thingKey
	case s.rng.Intn(100) < 40:
		f.ThingNote = strings.ToUpper(tgt.thingKey) + "-?"
	}

	if source == "nats" {
		for _, c := range customers {
			if c.Key == tgt.customer && c.PlatformOrg != "" {
				f.OriginSubject = "helpdesk." + c.PlatformOrg + ".tickets.create"
			}
		}
	}
	if ttype == "planned" || s.rng.Intn(100) < 25 {
		f.EstimatedMinutes = (1 + s.rng.Intn(16)) * 30
	}
	if p := s.projectFor(tgt.customer, ttype); p != "" {
		f.Project = p
	}

	f.Transitions = transitionsTo(status)

	// Conversation, dispatch, and labor — only on tickets old enough to have
	// plausibly accumulated any.
	if age <= -1 {
		s.addGeneratedChildren(&f, status, age, assignee, requester)
	}
	dedupeChildren(&f)
	return f
}

// dedupeChildren drops children that collide on their own natural key. Comments
// are matched on (ticket, body) and time entries on (ticket, note, minutes), so
// two draws of the same phrase for one ticket would look like an already-written
// record and be skipped — leaving the run reporting "existing" rows on a fresh
// database. Cheaper to not generate the collision than to widen the key.
func dedupeChildren(f *ticketFixture) {
	seen := map[string]bool{}
	comments := f.Comments[:0]
	for _, c := range f.Comments {
		if seen[c.Body] {
			continue
		}
		seen[c.Body] = true
		comments = append(comments, c)
	}
	f.Comments = comments

	seenTime := map[string]bool{}
	entries := f.Time[:0]
	for _, e := range f.Time {
		key := fmt.Sprintf("%s|%d", e.Note, e.Minutes)
		if seenTime[key] {
			continue
		}
		seenTime[key] = true
		entries = append(entries, e)
	}
	f.Time = entries

	seenVisit := map[string]bool{}
	visits := f.Visits[:0]
	for _, v := range f.Visits {
		if seenVisit[v.Notes] {
			continue
		}
		seenVisit[v.Notes] = true
		visits = append(visits, v)
	}
	f.Visits = visits
}

// weighted6040 returns a day count skewed toward the recent past: roughly 60%
// of tickets inside a fortnight, the rest trailing back about six months.
func (s *seeder) weighted6040() int {
	switch s.weighted(map[string]int{"recent": 46, "mid": 32, "old": 22}) {
	case "recent":
		return s.rng.Intn(14)
	case "mid":
		return 14 + s.rng.Intn(46)
	default:
		return 60 + s.rng.Intn(120)
	}
}

// transitionsTo expands a target status into the intermediate steps a real
// ticket would have walked, so the audit timeline reads as a progression.
func transitionsTo(status string) []string {
	switch status {
	case "open":
		return nil
	case "in_progress":
		return []string{"in_progress"}
	case "waiting":
		return []string{"in_progress", "waiting"}
	case "resolved":
		return []string{"in_progress", "resolved"}
	case "closed":
		return []string{"in_progress", "resolved", "closed"}
	}
	return nil
}

func (s *seeder) categoryFor(thingType, ticketType string) string {
	byThing := map[string]string{
		"door-controller": "access-control", "badge-reader": "access-control", "gate": "access-control",
		"switch": "network", "ap": "network", "camera": "network",
		"kiosk": "kiosk",
		"plc":   "iot-device", "sensor": "iot-device", "rtu": "iot-device", "meter": "iot-device",
		"projector": "hardware", "printer": "hardware", "hvac": "hardware", "nurse-call": "hardware",
	}
	if c, ok := byThing[thingType]; ok {
		return c
	}
	if ticketType == "planned" {
		return "hardware"
	}
	return "other"
}

// projectFor attaches some of a customer's work to one of its projects, which
// is what gives the project rollups (crew, logged vs estimated) real numbers.
func (s *seeder) projectFor(customer, ticketType string) string {
	var pool []string
	for _, p := range projects {
		if p.Customer == customer && p.Status != "canceled" {
			pool = append(pool, p.Key)
		}
	}
	if len(pool) == 0 {
		return ""
	}
	odds := 18
	if ticketType == "planned" {
		odds = 62 // install work is what projects are mostly made of
	}
	if s.rng.Intn(100) >= odds {
		return ""
	}
	return pool[s.rng.Intn(len(pool))]
}

// terminal reports whether a ticket status is one the comment hooks treat as
// closed to further nudging (see tickets.markAwaitingRequester).
func terminal(status string) bool {
	return status == "resolved" || status == "closed"
}

func (s *seeder) addGeneratedChildren(f *ticketFixture, status string, age int, assignee, requester string) {
	worked := status != "open"

	if worked && assignee != "" {
		n := 1 + s.rng.Intn(2)
		for i := 0; i < n; i++ {
			off := age + 1 + s.rng.Intn(maxInt(1, -age))
			if off > 0 {
				off = 0
			}
			c := commentFixture{Author: assignee, Body: s.pick(staffReplies), DayOffset: off}
			// A public staff reply sometimes asks for something back, which is
			// what drives the portal's "needs your reply" prompt.
			//
			// Weighted by status, because an outstanding question only survives
			// on a live ticket: resolving or closing clears awaiting_requester
			// (see tickets.syncResolvedAt / markAwaitingRequester), so a request
			// drawn onto a terminal ticket produces no demo state at all. Half
			// the seeded tickets are terminal, and at flat odds that swallowed
			// nearly every request — six of eight customers opened the portal to
			// a "needs your reply" tile reading zero.
			askOdds := 15
			if !terminal(status) {
				askOdds = 60
			}
			if requester != "" && s.rng.Intn(100) < askOdds {
				c.RequestsReply = true
			}
			f.Comments = append(f.Comments, c)
		}
		if s.rng.Intn(100) < 22 {
			f.Comments = append(f.Comments, commentFixture{
				Author: assignee, Body: s.pick(internalNotes), Internal: true,
				DayOffset: age + 1,
			})
		}
	}
	// An ACTIVE ticket where staff asked a question and the customer has already
	// answered is a ticket staff should have moved on by now. So on those the
	// answer is withheld and the ball stays in the customer's court — otherwise
	// the closing reply clears awaiting_requester and the state never shows.
	pendingRequest := false
	if !terminal(status) {
		for _, c := range f.Comments {
			if c.RequestsReply {
				pendingRequest = true
			}
		}
	}
	if requester != "" && worked && !pendingRequest && s.rng.Intn(100) < 45 {
		off := age + 2
		if off > 0 {
			off = 0
		}
		f.Comments = append(f.Comments, commentFixture{
			Author: requester, Body: s.pick(requesterReplies), DayOffset: off,
		})
	}

	// On-site work: installs almost always, issues sometimes.
	visitOdds := 22
	if f.Type == "planned" {
		visitOdds = 70
	}
	if s.rng.Intn(100) < visitOdds {
		tech := s.fieldStaff()
		switch {
		case status == "closed" || status == "resolved":
			d := age + 1 + s.rng.Intn(maxInt(1, -age))
			if d > 0 {
				d = 0
			}
			f.Visits = append(f.Visits, visitFixture{
				Status: "completed", ScheduledDays: d, Assignee: tech,
				Notes: s.pick(visitNotes), CompletedDays: ptr(d),
			})
		case s.rng.Intn(100) < 55:
			f.Visits = append(f.Visits, visitFixture{
				Status: "scheduled", ScheduledDays: 1 + s.rng.Intn(10), Assignee: tech,
				Notes: s.pick(visitNotes),
			})
		default:
			f.Visits = append(f.Visits, visitFixture{Status: "requested", Notes: s.pick(visitNotes)})
		}
	}

	// Labor. Reports split billable from written-off, so seed some of each.
	if worked && assignee != "" {
		// Whoever actually went on site, if anyone did. Hours attributed to a
		// visit are logged by the technician who attended, not the desk agent
		// who owns the ticket — that is what makes the reports' "Field" column
		// and a technician's on-site utilization mean anything. Without this the
		// visit link was never set on any seeded entry and Field read "—" for
		// every technician in the demo.
		fieldTech := ""
		for _, v := range f.Visits {
			if v.Status == "completed" && v.Assignee != "" {
				fieldTech = v.Assignee
				break
			}
		}

		n := 1 + s.rng.Intn(3)
		for i := 0; i < n; i++ {
			d := age + s.rng.Intn(maxInt(1, -age+1))
			if d > 0 {
				d = 0
			}
			entry := timeFixture{
				Staff:       assignee,
				Minutes:     (1 + s.rng.Intn(12)) * 15,
				WorkDays:    d,
				Note:        s.pick(workNotes),
				NonBillable: s.rng.Intn(100) < 16,
			}
			// Most of the labor on a ticket that saw a completed visit is the
			// on-site work itself; the rest is desk time around it.
			if fieldTech != "" && s.rng.Intn(100) < 70 {
				entry.Staff = fieldTech
				entry.OnVisit = true
			}
			f.Time = append(f.Time, entry)
		}
	}
}

func (s *seeder) fieldStaff() string {
	var pool []string
	for _, m := range staffMembers {
		if m.Role == "field" {
			pool = append(pool, m.Key)
		}
	}
	return pool[s.rng.Intn(len(pool))]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ------------------------------------------------------------------ writing

// writeTicket is idempotent on dedupe_key and reconciles children individually,
// so a run that dies partway heals on the next one instead of leaving a ticket
// permanently childless.
func (s *seeder) writeTicket(t ticketFixture) error {
	dedupe := "seed-" + t.Key
	cust := s.customers[t.Customer]

	rec, created, err := s.ensure("tickets", "dedupe_key = {:d}", dbx.Params{"d": dedupe}, func(r *core.Record) {
		r.Set("customer", cust)
		r.Set("title", t.Title)
		r.Set("body", t.Body)
		r.Set("priority", t.Priority)
		r.Set("source", t.Source)
		r.Set("status", "open") // transitions below walk it to its real state
		r.Set("dedupe_key", dedupe)
		r.Set("location_note", t.LocationNote)
		r.Set("thing_note", t.ThingNote)
		if t.Type != "" {
			r.Set("type", t.Type)
		}
		if id := s.categories[t.Category]; id != "" {
			r.Set("category", id)
		}
		if id := s.projects[t.Project]; id != "" {
			r.Set("project", id)
		}
		if id := s.locations[t.Location]; id != "" {
			r.Set("location", id)
		}
		if id := s.things[t.Thing]; id != "" {
			r.Set("thing", id)
		}
		if id := s.staff[t.Assignee]; id != "" {
			r.Set("assignee", id)
		}
		if id := s.requesters[t.Requester]; id != "" {
			r.Set("requester", id)
		}
		if t.EstimatedMinutes > 0 {
			r.Set("estimated_minutes", t.EstimatedMinutes)
		}
		if t.OriginSubject != "" {
			r.Set("origin_subject", t.OriginSubject)
		}
	})
	if err != nil {
		return err
	}

	createdAt := t.CreatedAt
	if created {
		if err := s.backdate("tickets", rec.Id, createdAt); err != nil {
			return err
		}
	}

	if err := s.writeComments(rec, t); err != nil {
		return err
	}
	if err := s.writeVisits(rec, t); err != nil {
		return err
	}
	if err := s.writeTime(rec, t); err != nil {
		return err
	}

	// Transitions last, and only while the ticket is still at the created
	// default — otherwise a re-run walks it through the lifecycle again and
	// fabricates a second set of audit events every time.
	fresh, err := s.app.FindRecordById("tickets", rec.Id)
	if err != nil {
		return err
	}
	if fresh.GetString("status") == "open" && len(t.Transitions) > 0 {
		// Attribute changes to the assignee (or a fallback admin) so the
		// timeline names a person rather than showing a blank actor.
		actor := s.staff[t.Assignee]
		if actor == "" {
			actor = s.staff["maya"]
		}
		for _, st := range t.Transitions {
			// Re-fetch each time. The audit hook diffs against Record.Original(),
			// which is the snapshot taken when the record was LOADED — reusing one
			// object across saves makes every step report its old value as the
			// original "open", so a three-step lifecycle logs "open → in_progress",
			// "open → resolved", "open → closed" instead of a real progression.
			step, err := s.app.FindRecordById("tickets", rec.Id)
			if err != nil {
				return err
			}
			step.Set("status", st)
			activity.SetActor(step, "staff", actor)
			notifications.Suppress(step)
			if err := s.app.Save(step); err != nil {
				return fmt.Errorf("transition %s → %s: %w", t.Key, st, err)
			}
		}
		// The audit rows land with `created` = now; pull them back into the
		// ticket's own window so the timeline is chronological.
		if err := s.backdateEvents(rec.Id, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) writeComments(ticket *core.Record, t ticketFixture) error {
	// Chronological order, not array order. The generator appends a staff reply
	// first and the requester's answer second, but their DayOffsets are drawn
	// independently — the "answer" is frequently the earlier of the two. Saving
	// in array order therefore ran the comment hooks out of sequence: a reply
	// that predates the request still cleared awaiting_requester, so the portal's
	// "needs your reply" state came out near-empty on a seeded demo and
	// contradicted the timeline shown right beside it.
	//
	// Sorting here rather than at generation time on purpose: generation must
	// spend all of its randomness before any writing (see seed.go), and this
	// consumes none. Stable, so same-day comments keep the order they were
	// written in and the run stays reproducible.
	ordered := append([]commentFixture(nil), t.Comments...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].DayOffset < ordered[j].DayOffset })

	for _, c := range ordered {
		existing, _ := s.app.FindFirstRecordByFilter("ticket_comments",
			"ticket = {:t} && body = {:b}", dbx.Params{"t": ticket.Id, "b": c.Body})
		if existing != nil {
			s.res.mark("ticket_comments", false)
			continue
		}
		col, err := s.app.FindCollectionByNameOrId("ticket_comments")
		if err != nil {
			return err
		}
		rec := core.NewRecord(col)
		rec.Set("ticket", ticket.Id)
		rec.Set("body", c.Body)
		if id := s.staff[c.Author]; id != "" {
			rec.Set("author_staff", id)
			rec.Set("internal", c.Internal)
			rec.Set("requests_reply", c.RequestsReply)
		} else if id := s.requesters[c.Author]; id != "" {
			rec.Set("author_user", id)
		}
		notifications.Suppress(rec)
		if err := s.app.Save(rec); err != nil {
			return fmt.Errorf("comment on %s: %w", t.Key, err)
		}
		s.res.mark("ticket_comments", true)

		if err := s.backdate("ticket_comments", rec.Id, c.At); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) writeVisits(ticket *core.Record, t ticketFixture) error {
	for _, v := range t.Visits {
		existing, _ := s.app.FindFirstRecordByFilter("visits",
			"ticket = {:t} && notes = {:n}", dbx.Params{"t": ticket.Id, "n": v.Notes})
		if existing != nil {
			s.res.mark("visits", false)
			continue
		}
		col, err := s.app.FindCollectionByNameOrId("visits")
		if err != nil {
			return err
		}
		rec := core.NewRecord(col)
		rec.Set("ticket", ticket.Id)
		rec.Set("status", v.Status)
		rec.Set("notes", v.Notes)
		// A `requested` visit is the needs-scheduling case: the guard hook
		// requires a time AND a tech on a *scheduled* visit, so send neither here.
		if v.Status != "requested" {
			rec.Set("scheduled_at", v.ScheduledAt)
			if id := s.staff[v.Assignee]; id != "" {
				rec.Set("assignee", id)
			}
		}
		if v.CompletedAt != nil {
			rec.Set("completed_at", *v.CompletedAt)
		}
		notifications.Suppress(rec)
		if err := s.app.Save(rec); err != nil {
			return fmt.Errorf("visit on %s: %w", t.Key, err)
		}
		s.res.mark("visits", true)
	}
	return nil
}

func (s *seeder) writeTime(ticket *core.Record, t ticketFixture) error {
	for _, e := range t.Time {
		existing, _ := s.app.FindFirstRecordByFilter("time_entries",
			"ticket = {:t} && note = {:n} && minutes = {:m}",
			dbx.Params{"t": ticket.Id, "n": e.Note, "m": e.Minutes})
		if existing != nil {
			s.res.mark("time_entries", false)
			continue
		}
		col, err := s.app.FindCollectionByNameOrId("time_entries")
		if err != nil {
			return err
		}
		rec := core.NewRecord(col)
		rec.Set("ticket", ticket.Id)
		rec.Set("staff", s.staff[e.Staff])
		rec.Set("minutes", e.Minutes)
		rec.Set("work_date", s.dateOnly(e.WorkDays))
		rec.Set("note", e.Note)
		rec.Set("non_billable", e.NonBillable)
		// writeVisits ran first, so the completed visit exists by now.
		if e.OnVisit {
			if v, _ := s.app.FindFirstRecordByFilter("visits",
				"ticket = {:t} && status = 'completed'", dbx.Params{"t": ticket.Id}); v != nil {
				rec.Set("visit", v.Id)
			}
		}
		notifications.Suppress(rec)
		if err := s.app.Save(rec); err != nil {
			return fmt.Errorf("time on %s: %w", t.Key, err)
		}
		s.res.mark("time_entries", true)
	}
	return nil
}

// backdateEvents spreads a ticket's audit rows across its own lifetime. Without
// this every event on a six-month-old ticket carries today's timestamp, which
// makes the timeline read backwards.
func (s *seeder) backdateEvents(ticketID string, createdAt time.Time) error {
	events, err := s.app.FindAllRecords("ticket_events",
		dbx.NewExp("ticket = {:t}", dbx.Params{"t": ticketID}))
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	span := s.now.Sub(createdAt)
	step := span / time.Duration(len(events)+1)
	for i, e := range events {
		if err := s.backdate("ticket_events", e.Id, createdAt.Add(step*time.Duration(i+1))); err != nil {
			return err
		}
	}
	return nil
}
