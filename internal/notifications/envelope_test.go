package notifications

import "testing"

// TestSampleEnvelopePerEventBlocks locks the per-event shaping the reference
// drawer relies on: each optional block appears only for the events that
// actually carry it, so the sample never implies (say) a comment on a
// ticket.created event.
func TestSampleEnvelopePerEventBlocks(t *testing.T) {
	cases := []struct {
		et        string
		change    bool
		comment   bool
		visit     bool
		oldSched  bool
		completed bool
	}{
		{EventTypeTicketCreated, false, false, false, false, false},
		{EventTypeTicketAssigned, false, false, false, false, false},
		{EventTypeTicketStatusChanged, true, false, false, false, false},
		{EventTypeTicketCommented, false, true, false, false, false},
		{EventTypeVisitScheduled, false, false, true, false, false},
		{EventTypeVisitRescheduled, false, false, true, true, false},
		{EventTypeVisitCanceled, false, false, true, false, false},
		{EventTypeVisitCompleted, false, false, true, false, true},
	}
	for _, c := range cases {
		subject, env, ok := SampleEnvelope(c.et)
		if !ok {
			t.Fatalf("%s: SampleEnvelope not ok", c.et)
		}
		if want := "helpdesk.<customer>.events." + c.et; subject != want {
			t.Errorf("%s: subject = %q, want %q", c.et, subject, want)
		}
		if env.Schema != EnvelopeSchema || env.Version != EnvelopeVersion || env.EventType != c.et {
			t.Errorf("%s: header = %q/%d/%q", c.et, env.Schema, env.Version, env.EventType)
		}
		if got := env.Change != nil; got != c.change {
			t.Errorf("%s: change present = %v, want %v", c.et, got, c.change)
		}
		if got := env.Comment != nil; got != c.comment {
			t.Errorf("%s: comment present = %v, want %v", c.et, got, c.comment)
		}
		if got := env.Visit != nil; got != c.visit {
			t.Errorf("%s: visit present = %v, want %v", c.et, got, c.visit)
		}
		if c.visit {
			if got := env.Visit.OldScheduledAt != ""; got != c.oldSched {
				t.Errorf("%s: old_scheduled_at present = %v, want %v", c.et, got, c.oldSched)
			}
			if got := env.Visit.CompletedAt != ""; got != c.completed {
				t.Errorf("%s: completed_at present = %v, want %v", c.et, got, c.completed)
			}
		}
	}

	if _, _, ok := SampleEnvelope("bogus.event"); ok {
		t.Error("SampleEnvelope should be !ok for an unknown event type")
	}
}
