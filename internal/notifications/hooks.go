package notifications

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// RegisterHooks binds the record hooks that turn ticket activity into
// notification events:
//
//   - tickets create        → ticket.created
//   - tickets update        → ticket.status_changed (status diff),
//     ticket.assigned (assignee diff, when newly set/changed)
//   - ticket_comments create → ticket.commented (public comments only)
//   - visits create          → visit.scheduled (only when created scheduled;
//     a `requested` visit has no time or tech to announce yet)
//   - visits update          → visit.scheduled (became scheduled),
//     visit.rescheduled (time moved while scheduled),
//     visit.canceled (was scheduled — canceling a bare request is silent)
//
// All fire from After*Success hooks, so an email never precedes its commit.
// The notifier itself is async + nil-safe; hooks never fail the write.
func RegisterHooks(app core.App, n *Notifier) {
	app.OnRecordAfterCreateSuccess("tickets").BindFunc(func(e *core.RecordEvent) error {
		n.Send(EventTypeTicketCreated, buildTicketContext(e.App, e.Record))
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("tickets").BindFunc(func(e *core.RecordEvent) error {
		orig := e.Record.Original()
		if old, now := orig.GetString("status"), e.Record.GetString("status"); old != now {
			ctx := buildTicketContext(e.App, e.Record)
			ctx.Ticket.OldStatus = old
			n.Send(EventTypeTicketStatusChanged, ctx)
		}
		if old, now := orig.GetString("assignee"), e.Record.GetString("assignee"); now != "" && old != now {
			n.Send(EventTypeTicketAssigned, buildTicketContext(e.App, e.Record))
		}
		return e.Next()
	})

	app.OnRecordAfterCreateSuccess("ticket_comments").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool("internal") {
			return e.Next() // staff-only working notes never leave the app
		}
		ticket, err := e.App.FindRecordById("tickets", e.Record.GetString("ticket"))
		if err != nil {
			return e.Next()
		}
		ctx := buildTicketContext(e.App, ticket)
		comment := &CommentInfo{Body: e.Record.GetString("body")}
		if staffID := e.Record.GetString("author_staff"); staffID != "" {
			comment.ByStaff = true
			if author, err := e.App.FindRecordById("staff", staffID); err == nil {
				comment.AuthorName = author.GetString("name")
			}
			// A staff comment notifies the requester side only — the plan's
			// "other side" rule, and staff already work in the queue.
			ctx.suppressAssignee = true
		} else {
			if author, err := e.App.FindRecordById("users", e.Record.GetString("author_user")); err == nil {
				comment.AuthorName = author.GetString("name")
			}
			ctx.suppressRequester = true
		}
		ctx.Comment = comment
		n.Send(EventTypeTicketCommented, ctx)
		return e.Next()
	})

	app.OnRecordAfterCreateSuccess("visits").BindFunc(func(e *core.RecordEvent) error {
		// The guard hook (internal/visits) runs pre-save, so status is final
		// here. A visit created directly as scheduled announces itself; a
		// `requested` one waits for the dispatcher.
		if e.Record.GetString("status") != "scheduled" {
			return e.Next()
		}
		if ctx, ok := buildVisitContext(e.App, e.Record); ok {
			n.Send(EventTypeVisitScheduled, ctx)
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("visits").BindFunc(func(e *core.RecordEvent) error {
		orig := e.Record.Original()
		old, now := orig.GetString("status"), e.Record.GetString("status")
		ctx, ok := buildVisitContext(e.App, e.Record)
		if !ok {
			return e.Next()
		}
		switch {
		case now == "scheduled" && old != "scheduled":
			// Covers requested→scheduled even when the time arrives in the
			// same update — that's a scheduling, not a reschedule.
			n.Send(EventTypeVisitScheduled, ctx)
		case now == "scheduled" && orig.GetString("scheduled_at") != e.Record.GetString("scheduled_at"):
			ctx.Visit.OldScheduledAt = orig.GetString("scheduled_at")
			n.Send(EventTypeVisitRescheduled, ctx)
		case now == "canceled" && old == "scheduled":
			n.Send(EventTypeVisitCanceled, ctx)
		}
		// Everything else is silent: completion is communicated by the
		// ticket's status/comments, and a tech swap without a time change is
		// an accepted gap (nobody is emailed).
		return e.Next()
	})
}

// buildVisitContext assembles the render payload for one visit event. ok is
// false when the parent ticket is gone (cascade races) — the caller skips
// the send.
func buildVisitContext(app core.App, visit *core.Record) (TicketContext, bool) {
	ticket, err := app.FindRecordById("tickets", visit.GetString("ticket"))
	if err != nil {
		return TicketContext{}, false
	}
	ctx := buildTicketContext(app, ticket)
	ctx.Visit = &VisitInfo{
		ScheduledAt: visit.GetString("scheduled_at"),
		Location:    visit.GetString("location"),
		Notes:       visit.GetString("notes"),
	}
	// The visit's technician is the assignee that matters for these events —
	// override the ticket's assignee so both the {{.Visit.AssigneeName}}
	// field and the assignee recipient class point at who shows up on site.
	// An unassigned (requested) visit leaves the ticket assignee in place;
	// requested visits never send anyway.
	if tech, err := app.FindRecordById("staff", visit.GetString("assignee")); err == nil {
		ctx.Visit.AssigneeName = tech.GetString("name")
		ctx.Assignee = PersonInfo{Name: tech.GetString("name"), Email: tech.GetString("email")}
	}
	return ctx, true
}

// buildTicketContext assembles the shared render payload for one ticket.
// Dangling relations resolve to zero values — a machine ticket without a
// requester simply renders (and mails) nothing for that side.
func buildTicketContext(app core.App, ticket *core.Record) TicketContext {
	ctx := TicketContext{
		Ticket: TicketInfo{
			ID:       ticket.Id,
			Number:   ticket.GetInt("number"),
			Title:    ticket.GetString("title"),
			Body:     ticket.GetString("body"),
			Status:   ticket.GetString("status"),
			Priority: ticket.GetString("priority"),
			Source:   ticket.GetString("source"),
			URL:      ticketURL(app, ticket.Id),
		},
	}
	if customer, err := app.FindRecordById("customers", ticket.GetString("customer")); err == nil {
		ctx.Customer = customer.GetString("name")
	}
	if id := ticket.GetString("requester"); id != "" {
		if requester, err := app.FindRecordById("users", id); err == nil {
			ctx.Requester = PersonInfo{Name: requester.GetString("name"), Email: requester.GetString("email")}
		}
	}
	if id := ticket.GetString("assignee"); id != "" {
		if assignee, err := app.FindRecordById("staff", id); err == nil {
			ctx.Assignee = PersonInfo{Name: assignee.GetString("name"), Email: assignee.GetString("email")}
		}
	}
	return ctx
}

// ticketURL builds the role-neutral deep link (/t/{id} — the SPA router
// forwards it to the staff or portal detail view based on who is logged
// in). Empty when the PocketBase application URL isn't configured, which
// the default templates tolerate.
func ticketURL(app core.App, id string) string {
	base := strings.TrimRight(app.Settings().Meta.AppURL, "/")
	if base == "" {
		return ""
	}
	return base + "/t/" + id
}
