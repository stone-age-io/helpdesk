// Package demoseed populates a helpdesk instance with demo/showcase data.
//
// In-process against core.App rather than an HTTP client, which buys three
// things an external script could not have:
//
//   - `created` can be backdated. PocketBase's autodate field overwrites it on
//     every save and ignores it on update, so over HTTP every seeded ticket is
//     minutes old and the queue's Age column and the ticket-volume report are
//     both flat. A raw UPDATE through app.DB() fixes that; see backdate().
//   - It cannot drift from the schema. A migration that breaks the fixtures
//     breaks the build, and the seed is covered by the normal test harness.
//   - No auth dance. app.Save bypasses collection rules, so the seeder does not
//     have to log in as each comment author to satisfy the authorship pins on
//     ticket_comments and time_entries.
//
// Record hooks ARE bound in a subcommand (cmd/helpdesk registers them outside
// OnServe), which is deliberate: tickets get their sequential number and the
// audit trail fills in exactly as in production. The one thing that must be
// held back is mail, so every save is marked notifications.Suppress.
//
// Seeding is IDEMPOTENT. Everything is found-or-created by a natural key and
// generation is driven by a fixed-seed PRNG, so re-running converges rather than
// duplicating. Children reconcile individually rather than being gated on "was
// the parent new" — that gate leaves records permanently childless if a run dies
// partway.
package demoseed

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// Options controls the size of the generated bulk. The curated fixtures are
// always seeded in full; Tickets is a floor for the total, not a cap on them.
type Options struct {
	// Tickets is the total ticket count to converge on, curated ones included.
	Tickets int
	// Seed makes generation reproducible. Changing it produces a different but
	// equally deterministic data set; keeping it stable is what makes re-runs
	// idempotent, so there is rarely a reason to change it.
	Seed int64
	// Log receives progress lines. Nil discards them.
	Log func(string, ...any)
}

// Result reports what the run did.
type Result struct {
	Created map[string]int
	Matched map[string]int
}

func (r *Result) mark(collection string, created bool) {
	if created {
		r.Created[collection]++
		return
	}
	r.Matched[collection]++
}

// Summary renders the per-collection counts in a stable order.
func (r *Result) Summary() string {
	keys := make([]string, 0, len(r.Created))
	seen := map[string]bool{}
	for k := range r.Created {
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for k := range r.Matched {
		if !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-18s %4d created  %4d existing\n", k, r.Created[k], r.Matched[k])
	}
	return b.String()
}

// seeder carries the resolved id maps between phases.
type seeder struct {
	app  core.App
	opts Options
	rng  *rand.Rand
	res  *Result
	now  time.Time

	customers     map[string]string
	locationTypes map[string]string // "<customer>:<code>"
	thingTypes    map[string]string // "<customer>:<code>"
	locations     map[string]string // location code
	things        map[string]string // thing code, or name when codeless
	staff         map[string]string
	requesters    map[string]string
	categories    map[string]string // ticket_categories key
	projects      map[string]string
}

// Run seeds the app. Safe to call repeatedly.
func Run(app core.App, opts Options) (*Result, error) {
	if opts.Tickets <= 0 {
		opts.Tickets = 150
	}
	if opts.Seed == 0 {
		opts.Seed = 20260827
	}
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}

	s := &seeder{
		app: app, opts: opts,
		rng: rand.New(rand.NewSource(opts.Seed)),
		res: &Result{Created: map[string]int{}, Matched: map[string]int{}},
		now: time.Now().UTC(),

		customers: map[string]string{}, locationTypes: map[string]string{},
		thingTypes: map[string]string{}, locations: map[string]string{},
		things: map[string]string{}, staff: map[string]string{},
		requesters: map[string]string{}, categories: map[string]string{},
		projects: map[string]string{},
	}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"customers", s.seedCustomers},
		{"types", s.seedTypes},
		{"locations", s.seedLocations},
		{"things", s.seedThings},
		{"people", s.seedPeople},
		{"categories", s.loadCategories},
		{"projects", s.seedProjects},
		{"maintenance", s.seedMaintenancePlans},
		{"tickets", s.seedTickets},
	}
	for _, step := range steps {
		opts.Log("seeding %s…", step.name)
		if err := step.fn(); err != nil {
			return s.res, fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return s.res, nil
}

// ------------------------------------------------------------------ helpers

// ensure is the found-or-create primitive. `fill` only runs when the record is
// new, so hand-edits to demo data survive a re-run.
func (s *seeder) ensure(collection, filter string, params dbx.Params, fill func(*core.Record)) (*core.Record, bool, error) {
	existing, err := s.app.FindFirstRecordByFilter(collection, filter, params)
	if err == nil && existing != nil {
		s.res.mark(collection, false)
		return existing, false, nil
	}
	col, err := s.app.FindCollectionByNameOrId(collection)
	if err != nil {
		return nil, false, fmt.Errorf("find collection %s: %w", collection, err)
	}
	rec := core.NewRecord(col)
	fill(rec)
	// Demo data must never mail anyone, even on a host with SMTP configured.
	notifications.Suppress(rec)
	if err := s.app.Save(rec); err != nil {
		return nil, false, fmt.Errorf("save %s: %w", collection, err)
	}
	s.res.mark(collection, true)
	return rec, true, nil
}

// backdate rewrites a record's `created` column directly. PocketBase's autodate
// field stamps `created` on every save and ignores any value supplied by the
// caller, so this is the only way to get a demo whose ages look real. Writing
// the column behind the record API is safe here precisely because it is the one
// field the API refuses to let anyone set.
func (s *seeder) backdate(collection, id string, at time.Time) error {
	_, err := s.app.DB().NewQuery(
		fmt.Sprintf("UPDATE {{%s}} SET [[created]] = {:c} WHERE [[id]] = {:id}", collection),
	).Bind(dbx.Params{
		"c":  at.UTC().Format("2006-01-02 15:04:05.000Z"),
		"id": id,
	}).Execute()
	if err != nil {
		return fmt.Errorf("backdate %s %s: %w", collection, id, err)
	}
	return nil
}

func (s *seeder) day(offset int) time.Time   { return s.now.AddDate(0, 0, offset) }
func (s *seeder) dateOnly(offset int) string { return s.day(offset).Format("2006-01-02") }

// at returns a timestamp `offset` days from now at a plausible working hour,
// jittered so a generated batch doesn't stack on the same minute.
func (s *seeder) at(offset int) time.Time {
	d := s.day(offset)
	return time.Date(d.Year(), d.Month(), d.Day(),
		7+s.rng.Intn(11), s.rng.Intn(60), s.rng.Intn(60), 0, time.UTC)
}

func (s *seeder) pick(list []string) string { return list[s.rng.Intn(len(list))] }

// weighted picks a key by relative weight — the difference between a demo that
// looks like a working desk and one that looks uniformly random.
func (s *seeder) weighted(choices map[string]int) string {
	keys := make([]string, 0, len(choices))
	for k := range choices {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic iteration; map order is not
	total := 0
	for _, k := range keys {
		total += choices[k]
	}
	n := s.rng.Intn(total)
	for _, k := range keys {
		n -= choices[k]
		if n < 0 {
			return k
		}
	}
	return keys[len(keys)-1]
}

// ------------------------------------------------------------------ phases

func (s *seeder) seedCustomers() error {
	for _, c := range customers {
		rec, _, err := s.ensure("customers", "name = {:n}", dbx.Params{"n": c.Name}, func(r *core.Record) {
			r.Set("name", c.Name)
			// The fixture Key is already a lowercase single-word slug, so it is
			// the tenant code as-is. Set for every demo customer, not only the
			// platform-mapped ones: the code is what token 2 of the outbound NATS
			// subject carries, and a customer without one is skipped by the
			// publish channel — a showcase instance whose event stream is silent
			// for five of eight customers demonstrates the wrong thing.
			r.Set("code", c.Key)
			r.Set("active", true)
			r.Set("email_domain", c.Domain)
			r.Set("show_time_to_requester", c.ShowTime)
			if c.PlatformOrg != "" {
				r.Set("platform_org_id", c.PlatformOrg)
			}
		})
		if err != nil {
			return err
		}
		s.customers[c.Key] = rec.Id
	}
	return nil
}

func (s *seeder) seedTypes() error {
	load := func(collection string, fixtures []typeFixture, into map[string]string) error {
		for _, t := range fixtures {
			cust := s.customers[t.Customer]
			rec, _, err := s.ensure(collection, "customer = {:c} && code = {:k}",
				dbx.Params{"c": cust, "k": t.Code}, func(r *core.Record) {
					r.Set("customer", cust)
					r.Set("code", t.Code)
					r.Set("name", t.Name)
					r.Set("description", t.Description)
					if t.Schema != nil {
						r.Set("metadata_schema", t.Schema)
					}
				})
			if err != nil {
				return err
			}
			into[t.Customer+":"+t.Code] = rec.Id
		}
		return nil
	}
	if err := load("location_types", locationTypes, s.locationTypes); err != nil {
		return err
	}
	return load("thing_types", thingTypes, s.thingTypes)
}

func (s *seeder) seedLocations() error {
	// Pass 1: create without `parent`, since a parent may appear later in the
	// list. Same two-pass shape a platform export→seed needs, because exports
	// carry raw record ids rather than codes.
	for _, l := range locations {
		cust := s.customers[l.Customer]
		rec, _, err := s.ensure("locations", "customer = {:c} && code = {:k}",
			dbx.Params{"c": cust, "k": l.Code}, func(r *core.Record) {
				r.Set("customer", cust)
				r.Set("code", l.Code)
				r.Set("name", l.Name)
				r.Set("address", l.Address)
				r.Set("notes", l.Notes)
				r.Set("contact", l.Contact)
				r.Set("contact_phone", l.Phone)
				r.Set("lat", l.Lat)
				r.Set("lng", l.Lng)
				if id := s.locationTypes[l.Customer+":"+l.Type]; id != "" {
					r.Set("type", id)
				}
				if l.Metadata != nil {
					r.Set("metadata", l.Metadata)
				}
			})
		if err != nil {
			return err
		}
		s.locations[l.Code] = rec.Id
	}
	// Pass 2: wire parents now that every id is known.
	for _, l := range locations {
		if l.Parent == "" {
			continue
		}
		id, parentID := s.locations[l.Code], s.locations[l.Parent]
		if id == "" || parentID == "" {
			continue
		}
		rec, err := s.app.FindRecordById("locations", id)
		if err != nil {
			return err
		}
		if rec.GetString("parent") == parentID {
			continue
		}
		rec.Set("parent", parentID)
		notifications.Suppress(rec)
		if err := s.app.Save(rec); err != nil {
			return fmt.Errorf("set parent on %s: %w", l.Code, err)
		}
	}
	return nil
}

func (s *seeder) seedThings() error {
	for _, t := range things {
		cust := s.customers[t.Customer]
		// Codeless gear (the superset case) has only its name as a natural key.
		filter, params := "customer = {:c} && code = {:k}", dbx.Params{"c": cust, "k": t.Code}
		if t.Code == "" {
			filter, params = "customer = {:c} && name = {:n}", dbx.Params{"c": cust, "n": t.Name}
		}
		rec, _, err := s.ensure("things", filter, params, func(r *core.Record) {
			r.Set("customer", cust)
			r.Set("code", t.Code)
			r.Set("name", t.Name)
			r.Set("notes", t.Notes)
			r.Set("retired", t.Retired)
			if id := s.thingTypes[t.Customer+":"+t.Type]; id != "" {
				r.Set("type", id)
			}
			if id := s.locations[t.Location]; id != "" {
				r.Set("location", id)
			}
			if t.Metadata != nil {
				r.Set("metadata", t.Metadata)
			}
		})
		if err != nil {
			return err
		}
		key := t.Code
		if key == "" {
			key = t.Name
		}
		s.things[key] = rec.Id
	}
	return nil
}

func (s *seeder) seedPeople() error {
	for _, m := range staffMembers {
		rec, _, err := s.ensure("staff", "email = {:e}", dbx.Params{"e": m.Email}, func(r *core.Record) {
			r.Set("email", m.Email)
			r.Set("password", DemoPassword)
			r.Set("passwordConfirm", DemoPassword)
			r.Set("name", m.Name)
			r.Set("role", m.Role)
			r.Set("active", true)
			r.Set("verified", true)
		})
		if err != nil {
			return err
		}
		s.staff[m.Key] = rec.Id
	}
	for _, u := range requesters {
		rec, _, err := s.ensure("users", "email = {:e}", dbx.Params{"e": u.Email}, func(r *core.Record) {
			r.Set("email", u.Email)
			r.Set("password", DemoPassword)
			r.Set("passwordConfirm", DemoPassword)
			r.Set("name", u.Name)
			r.Set("customer", s.customers[u.Customer])
			r.Set("active", true)
			r.Set("verified", true)
			if u.Phone != "" {
				r.Set("phone", u.Phone)
			}
		})
		if err != nil {
			return err
		}
		s.requesters[u.Key] = rec.Id
	}
	return nil
}

// Categories are seeded by migration 1806000000 — look them up, don't create.
func (s *seeder) loadCategories() error {
	recs, err := s.app.FindAllRecords("ticket_categories")
	if err != nil {
		return err
	}
	for _, r := range recs {
		s.categories[r.GetString("key")] = r.Id
	}
	return nil
}

func (s *seeder) seedProjects() error {
	for _, p := range projects {
		cust := s.customers[p.Customer]
		rec, _, err := s.ensure("projects", "customer = {:c} && title = {:t}",
			dbx.Params{"c": cust, "t": p.Title}, func(r *core.Record) {
				r.Set("customer", cust)
				r.Set("title", p.Title)
				r.Set("description", p.Description)
				r.Set("status", p.Status)
				r.Set("start_date", s.dateOnly(p.StartDays))
				r.Set("target_date", s.dateOnly(p.TargetDays))
				if id := s.locations[p.Location]; id != "" {
					r.Set("location", id)
				}
				if id := s.staff[p.Lead]; id != "" {
					r.Set("lead", id)
				}
			})
		if err != nil {
			return err
		}
		s.projects[p.Key] = rec.Id
	}
	return nil
}

// seedMaintenancePlans fills the recurrence layer. The plans are seeded but
// deliberately NOT generated from — running the generator here would make the
// seeded ticket count non-deterministic and mint tickets whose ages the backdate
// pass never touches. `helpdesk maintenance-run` is one command away, and
// watching a plan produce its ticket is the more useful demo anyway.
func (s *seeder) seedMaintenancePlans() error {
	for _, p := range maintenancePlans {
		cust := s.customers[p.Customer]
		_, _, err := s.ensure("maintenance_plans", "customer = {:c} && title = {:t}",
			dbx.Params{"c": cust, "t": p.Title}, func(r *core.Record) {
				r.Set("customer", cust)
				r.Set("title", p.Title)
				r.Set("body", p.Body)
				r.Set("interval_days", p.IntervalDays)
				r.Set("lead_time_days", p.LeadTimeDays)
				r.Set("anchor", p.Anchor)
				r.Set("next_due", s.dateOnly(p.NextDueDays))
				r.Set("priority", p.Priority)
				r.Set("paused", p.Paused)
				if p.EstimatedMinutes > 0 {
					r.Set("estimated_minutes", p.EstimatedMinutes)
				}
				if id := s.things[p.Thing]; id != "" {
					r.Set("thing", id)
				}
				if id := s.locations[p.Location]; id != "" {
					r.Set("location", id)
				}
				if id := s.staff[p.Assignee]; id != "" {
					r.Set("assignee", id)
				}
				if id := s.categories[p.Category]; id != "" {
					r.Set("category", id)
				}
			})
		if err != nil {
			return err
		}
	}
	return nil
}
