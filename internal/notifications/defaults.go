// Package notifications renders and dispatches transactional emails for
// helpdesk ticket events. Templates are stored in the notification_templates
// collection and editable from the staff SPA; the compiled-in defaults below
// are the source of truth for the "Reset to defaults" affordance and the
// migration seeder. Lifted from the kiosk notifier subsystem with the domain
// touchpoints swapped: recipient classes are requester/assignee/all-staff,
// the staff collection replaces kiosk's admins, and the FuncMap verbs speak
// ticketing instead of inventory.
package notifications

// Event type identifiers. Each constant corresponds to one
// notification_templates row, one Defaults case, and one Recipients default.
const (
	EventTypeTicketCreated       = "ticket.created"
	EventTypeTicketAssigned      = "ticket.assigned"
	EventTypeTicketCommented     = "ticket.commented"
	EventTypeTicketStatusChanged = "ticket.status_changed"
	EventTypeVisitScheduled      = "visit.scheduled"
	EventTypeVisitRescheduled    = "visit.rescheduled"
	EventTypeVisitCanceled       = "visit.canceled"
	EventTypeVisitCompleted      = "visit.completed"
)

// Field references in the default templates must match
// notifications.TicketContext (see context.go).

const DefaultTicketCreatedSubject = `[#{{.Ticket.Number}}] {{.Ticket.Title}}`

const DefaultTicketCreatedBody = `A new ticket has been opened for {{.Customer}}.

Ticket:   #{{.Ticket.Number}} — {{.Ticket.Title}}
Status:   {{statusLabel .Ticket.Status}}
Priority: {{.Ticket.Priority}}
{{if .Requester.Name}}Requester: {{.Requester.Name}}
{{end}}{{if .Ticket.Body}}
{{.Ticket.Body}}
{{end}}{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

const DefaultTicketAssignedSubject = `[#{{.Ticket.Number}}] assigned to you — {{.Ticket.Title}}`

const DefaultTicketAssignedBody = `Hi {{.Assignee.Name}},

Ticket #{{.Ticket.Number}} for {{.Customer}} has been assigned to you.

Title:    {{.Ticket.Title}}
Status:   {{statusLabel .Ticket.Status}}
Priority: {{.Ticket.Priority}}
{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

const DefaultTicketCommentedSubject = `[#{{.Ticket.Number}}] new comment — {{.Ticket.Title}}`

const DefaultTicketCommentedBody = `{{.Comment.AuthorName}} commented on ticket #{{.Ticket.Number}} ({{.Customer}}):

{{.Comment.Body}}
{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

const DefaultTicketStatusChangedSubject = `[#{{.Ticket.Number}}] {{statusLabel .Ticket.Status}} — {{.Ticket.Title}}`

const DefaultTicketStatusChangedBody = `Ticket #{{.Ticket.Number}} for {{.Customer}} is now {{statusLabel .Ticket.Status}}{{if .Ticket.OldStatus}} (was {{statusLabel .Ticket.OldStatus}}){{end}}.

Title: {{.Ticket.Title}}
{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

const DefaultVisitScheduledSubject = `[#{{.Ticket.Number}}] site visit scheduled — {{formatTime .Visit.ScheduledAt}}`

const DefaultVisitScheduledBody = `A site visit has been scheduled for ticket #{{.Ticket.Number}} ({{.Customer}}).

When:       {{formatTime .Visit.ScheduledAt}}
Technician: {{.Visit.AssigneeName}}
{{if .Visit.Location}}Where:      {{.Visit.Location}}
{{end}}Ticket:     {{.Ticket.Title}}
{{if .Visit.Notes}}
Notes: {{.Visit.Notes}}
{{end}}{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

const DefaultVisitRescheduledSubject = `[#{{.Ticket.Number}}] site visit rescheduled — {{formatTime .Visit.ScheduledAt}}`

const DefaultVisitRescheduledBody = `The site visit for ticket #{{.Ticket.Number}} ({{.Customer}}) has been rescheduled.

New time:   {{formatTime .Visit.ScheduledAt}}
{{if .Visit.OldScheduledAt}}Was:        {{formatTime .Visit.OldScheduledAt}}
{{end}}Technician: {{.Visit.AssigneeName}}
{{if .Visit.Location}}Where:      {{.Visit.Location}}
{{end}}Ticket:     {{.Ticket.Title}}
{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

const DefaultVisitCanceledSubject = `[#{{.Ticket.Number}}] site visit canceled — was {{formatTime .Visit.ScheduledAt}}`

const DefaultVisitCanceledBody = `The site visit for ticket #{{.Ticket.Number}} ({{.Customer}}) scheduled for {{formatTime .Visit.ScheduledAt}} has been canceled.

Ticket: {{.Ticket.Title}}
{{if .Visit.Notes}}
Notes: {{.Visit.Notes}}
{{end}}{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

// The visit.completed template ships email-disabled (NATS-only by default —
// completion is already communicated by the ticket's status/comments, so it is
// noise in an inbox but a valuable "work done on site" signal on the wire).
// The subject/body are still provided so an operator who opts email back on
// gets a sensible message rather than a blank one.
const DefaultVisitCompletedSubject = `[#{{.Ticket.Number}}] site visit completed{{if .Visit.CompletedAt}} — {{formatTime .Visit.CompletedAt}}{{end}}`

const DefaultVisitCompletedBody = `The site visit for ticket #{{.Ticket.Number}} ({{.Customer}}) has been completed.

{{if .Visit.CompletedAt}}Completed:  {{formatTime .Visit.CompletedAt}}
{{end}}Technician: {{.Visit.AssigneeName}}
{{if .Visit.Location}}Where:      {{.Visit.Location}}
{{end}}Ticket:     {{.Ticket.Title}}
{{if .Visit.Notes}}
Notes: {{.Visit.Notes}}
{{end}}{{if .Ticket.URL}}
View the ticket: {{.Ticket.URL}}
{{end}}`

// Defaults returns the compiled-in default subject and body for the given
// event type. ok is false when the event type is unknown — callers (the
// migration seeder and the GET-defaults handler) treat that as "nothing to
// do" rather than an error.
func Defaults(eventType string) (subject, body string, ok bool) {
	switch eventType {
	case EventTypeTicketCreated:
		return DefaultTicketCreatedSubject, DefaultTicketCreatedBody, true
	case EventTypeTicketAssigned:
		return DefaultTicketAssignedSubject, DefaultTicketAssignedBody, true
	case EventTypeTicketCommented:
		return DefaultTicketCommentedSubject, DefaultTicketCommentedBody, true
	case EventTypeTicketStatusChanged:
		return DefaultTicketStatusChangedSubject, DefaultTicketStatusChangedBody, true
	case EventTypeVisitScheduled:
		return DefaultVisitScheduledSubject, DefaultVisitScheduledBody, true
	case EventTypeVisitRescheduled:
		return DefaultVisitRescheduledSubject, DefaultVisitRescheduledBody, true
	case EventTypeVisitCanceled:
		return DefaultVisitCanceledSubject, DefaultVisitCanceledBody, true
	case EventTypeVisitCompleted:
		return DefaultVisitCompletedSubject, DefaultVisitCompletedBody, true
	}
	return "", "", false
}

// DefaultName returns the human-readable label seeded for a template row.
func DefaultName(eventType string) string {
	switch eventType {
	case EventTypeTicketCreated:
		return "Ticket created"
	case EventTypeTicketAssigned:
		return "Ticket assigned"
	case EventTypeTicketCommented:
		return "New comment"
	case EventTypeTicketStatusChanged:
		return "Status changed"
	case EventTypeVisitScheduled:
		return "Site visit scheduled"
	case EventTypeVisitRescheduled:
		return "Site visit rescheduled"
	case EventTypeVisitCanceled:
		return "Site visit canceled"
	case EventTypeVisitCompleted:
		return "Site visit completed"
	}
	return eventType
}

// SeededEventTypes lists every event type the migration should seed on
// first run. Adding a new built-in template means appending here and to
// Defaults / DefaultName / DefaultRecipients.
func SeededEventTypes() []string {
	return []string{
		EventTypeTicketCreated,
		EventTypeTicketAssigned,
		EventTypeTicketCommented,
		EventTypeTicketStatusChanged,
		EventTypeVisitScheduled,
		EventTypeVisitRescheduled,
		EventTypeVisitCanceled,
		EventTypeVisitCompleted,
	}
}

// Recipients is the editable per-template audience descriptor stored in the
// recipients JSON column on notification_templates. The notifier resolves it
// to a concrete []mail.Address at send time:
//
//   - Requester: the ticket's requester, when the event's payload provides a
//     requester email. Machine tickets have none — resolves to nothing.
//   - Assignee:  the ticket's assigned staff member, when present.
//   - AllStaff:  every staff row with active=true.
//   - Extras:    free-form addresses (e.g., a shared ops mailbox).
//
// An empty/missing JSON column falls back to the event's compiled-in
// default. All classes false + empty Extras produces a no-op skip rather
// than an error.
type Recipients struct {
	Requester bool     `json:"requester"`
	Assignee  bool     `json:"assignee"`
	AllStaff  bool     `json:"all_staff"`
	Extras    []string `json:"extras"`
}

// DefaultRecipients returns the audience an event type ships with. Used by
// the migration seeder and by the notifier as the fallback when a row's
// recipients column is null/empty.
func DefaultRecipients(eventType string) Recipients {
	switch eventType {
	case EventTypeTicketCreated:
		// The requester gets their confirmation; the whole staff pool sees new
		// work arrive. Machine tickets (no requester) fan out to staff only.
		return Recipients{Requester: true, AllStaff: true, Extras: []string{}}
	case EventTypeTicketAssigned:
		return Recipients{Assignee: true, Extras: []string{}}
	case EventTypeTicketCommented:
		// Both classes are enabled; the payload blanks the author's side so a
		// staff comment mails the requester and a requester comment mails the
		// assignee — nobody is notified about their own comment.
		return Recipients{Requester: true, Assignee: true, Extras: []string{}}
	case EventTypeTicketStatusChanged:
		return Recipients{Requester: true, Extras: []string{}}
	case EventTypeVisitScheduled, EventTypeVisitRescheduled, EventTypeVisitCanceled:
		// Visit lifecycle events address whoever is affected on both sides:
		// the requester expecting the tech and the tech being dispatched.
		return Recipients{Requester: true, Assignee: true, Extras: []string{}}
	case EventTypeVisitCompleted:
		// NATS-only: nobody is emailed by default (the template also ships
		// email-disabled). Completion is communicated by the ticket's
		// status/comments; the wire event is for MSP-internal automation.
		return Recipients{Extras: []string{}}
	}
	// Conservative default for unrecognized event types: address nobody.
	// Operators must explicitly opt in to a recipient class.
	return Recipients{Extras: []string{}}
}

// RequesterEmailProvider is implemented by payloads whose recipient set can
// include the ticket's requester. TicketContext implements it, returning ""
// for machine tickets (no requester) and for events authored by the
// requester themselves.
type RequesterEmailProvider interface {
	RequesterEmail() string
}

// AssigneeEmailProvider is the assignee-side counterpart of
// RequesterEmailProvider.
type AssigneeEmailProvider interface {
	AssigneeEmail() string
}
