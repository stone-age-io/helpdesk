package notifications

import "fmt"

// TicketInfo is the ticket snapshot exposed to templates.
type TicketInfo struct {
	ID       string
	Number   int
	Title    string
	Body     string
	Status   string
	// OldStatus is set only for ticket.status_changed.
	OldStatus string
	Priority  string
	Source    string
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

// VisitInfo rides visit.scheduled payloads. ScheduledAt is preformatted by
// the hook (PocketBase datetimes are strings); templates print it via
// formatTime, which passes strings through.
type VisitInfo struct {
	ScheduledAt  string
	AssigneeName string
	Notes        string
}

// TicketContext is the render payload every helpdesk event type shares.
// Comment and Visit are populated only for their event types; the default
// templates for the other events never reference them.
type TicketContext struct {
	Ticket    TicketInfo
	Customer  string // customer (company) name
	Requester PersonInfo
	Assignee  PersonInfo
	Comment   *CommentInfo
	Visit     *VisitInfo

	// suppress* blank the corresponding provider email so "notify the other
	// side" events never email the person who triggered them. Set by the
	// hooks; the Name stays available to templates either way.
	suppressRequester bool
	suppressAssignee  bool
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
