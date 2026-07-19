package notifications

import "fmt"

// TicketInfo is the ticket snapshot exposed to templates.
type TicketInfo struct {
	ID     string
	Number int
	Title  string
	Body   string
	Status string
	// OldStatus is set only for ticket.status_changed.
	OldStatus string
	Priority  string
	Source    string
	// Type is the ticket type (issue | install). Carried for the NATS event
	// envelope; the default email templates don't reference it.
	Type string
	// URL is the staff/portal-agnostic deep link (built from the PocketBase
	// application URL setting); empty when the app URL is unconfigured.
	URL string
}

// PersonInfo names one side of the conversation.
type PersonInfo struct {
	Name  string
	Email string
}

// CommentInfo rides ticket.commented payloads.
type CommentInfo struct {
	AuthorName string
	Body       string
	// ByStaff reports which side authored the comment.
	ByStaff bool
}

// VisitInfo rides visit.scheduled / visit.rescheduled / visit.canceled
// payloads. ScheduledAt is preformatted by the hook (PocketBase datetimes
// are strings); templates print it via formatTime, which passes strings
// through.
type VisitInfo struct {
	ScheduledAt string
	// OldScheduledAt is set only for visit.rescheduled.
	OldScheduledAt string
	// CompletedAt is set only for visit.completed.
	CompletedAt  string
	AssigneeName string
	Location     string
	Notes        string
}

// TicketContext is the render payload every helpdesk event type shares.
// Comment and Visit are populated only for their event types; the default
// templates for the other events never reference them.
type TicketContext struct {
	Ticket   TicketInfo
	Customer string // customer (company) name
	// CustomerID is the tickets.customer relation id — the always-present,
	// token-safe tenant token for the outbound NATS subject.
	CustomerID string
	// CustomerOrgID is customers.platform_org_id, empty for customers not
	// mapped to a platform org. Rides the NATS payload (never the subject,
	// since it's optional).
	CustomerOrgID string
	Requester     PersonInfo
	Assignee      PersonInfo
	Comment       *CommentInfo
	Visit         *VisitInfo

	// suppress* blank the corresponding provider email so "notify the other
	// side" events never email the person who triggered them. Set by the
	// hooks; the Name stays available to templates either way.
	suppressRequester bool
	suppressAssignee  bool

	// occurrenceKey seeds the Nats-Msg-Id for the outbound publish: stable
	// across a republish of the same save, distinct across events. Set by the
	// builders to the source record's id+updated (see buildTicketContext /
	// buildVisitContext and the comment hook).
	occurrenceKey string
}

// RequesterEmail implements RequesterEmailProvider. Empty for machine
// tickets (no requester) and when the requester authored the event.
func (c TicketContext) RequesterEmail() string {
	if c.suppressRequester {
		return ""
	}
	return c.Requester.Email
}

// AssigneeEmail implements AssigneeEmailProvider. Empty for unassigned
// tickets and when the assignee authored the event.
func (c TicketContext) AssigneeEmail() string {
	if c.suppressAssignee {
		return ""
	}
	return c.Assignee.Email
}

// PayloadSummary implements PayloadSummarizer for the send log.
func (c TicketContext) PayloadSummary() string {
	return fmt.Sprintf("#%d · %s", c.Ticket.Number, c.Customer)
}

// SampleContext returns a fully-populated TicketContext for previewing:
// every field the default templates reference is set, so any event type's
// template renders against it. Used by the test-send route and the render
// regression tests.
func SampleContext() TicketContext {
	return TicketContext{
		Ticket: TicketInfo{
			ID:        "sample1",
			Number:    42,
			Title:     "Pump fault on line 3",
			Body:      "Vibration sensor reports repeated overcurrent.",
			Status:    "in_progress",
			OldStatus: "open",
			Priority:  "high",
			Source:    "nats",
			Type:      "issue",
			URL:       "https://helpdesk.example.com/t/sample1",
		},
		Customer:      "Acme Corp",
		CustomerID:    "custacme00000001",
		CustomerOrgID: "org_acme000000001",
		Requester:     PersonInfo{Name: "Rita Requester", Email: "rita@acme.example"},
		Assignee:      PersonInfo{Name: "Sam Staff", Email: "sam@816tech.example"},
		Comment:       &CommentInfo{AuthorName: "Sam Staff", Body: "Heading out tomorrow with a replacement motor.", ByStaff: true},
		Visit: &VisitInfo{
			ScheduledAt:    "2026-07-14 14:00:00.000Z",
			OldScheduledAt: "2026-07-12 09:00:00.000Z",
			CompletedAt:    "2026-07-14 15:30:00.000Z",
			AssigneeName:   "Sam Staff",
			Location:       "Main St branch, rear entrance",
			Notes:          "Bring spare motor",
		},
	}
}
