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
//   - visits create          → visit.scheduled
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
		ticket, err := e.App.FindRecordById("tickets", e.Record.GetString("ticket"))
		if err != nil {
			return e.Next()
		}
		ctx := buildTicketContext(e.App, ticket)
		visit := &VisitInfo{
			ScheduledAt: e.Record.GetString("scheduled_at"),
			Notes:       e.Record.GetString("notes"),
		}
		// The visit's technician is the assignee that matters for this event —
		// override the ticket's assignee so both the {{.Visit.AssigneeName}}
		// field and the assignee recipient class point at who shows up on site.
		if tech, err := e.App.FindRecordById("staff", e.Record.GetString("assignee")); err == nil {
			visit.AssigneeName = tech.GetString("name")
			ctx.Assignee = PersonInfo{Name: tech.GetString("name"), Email: tech.GetString("email")}
		}
		ctx.Visit = visit
		n.Send(EventTypeVisitScheduled, ctx)
		return e.Next()
	})
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
