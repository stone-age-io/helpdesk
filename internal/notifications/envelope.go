package notifications

// This file defines the outbound NATS event contract — the JSON envelope
// published to helpdesk.{customerId}.events.{event_type} when a template has
// publish_nats enabled. It is deliberately a SEPARATE set of wire structs from
// TicketContext (the render payload): the wire contract is versioned and
// consumed by machines, so it must not drift silently when the internal render
// shape is refactored. docs/protocol.md documents it as the sibling of the
// inbound contract.

const (
	// EnvelopeSchema names the contract so consumers can route/validate.
	EnvelopeSchema = "helpdesk.event"
	// EnvelopeVersion is bumped on any breaking change to the wire shape.
	EnvelopeVersion = 1
)

// EventEnvelope is the top-level published document. Optional blocks are
// present only for the events that carry them (Change for status_changed,
// Comment for commented, Visit for the visit.* events).
type EventEnvelope struct {
	Schema     string       `json:"schema"`
	Version    int          `json:"version"`
	EventType  string       `json:"event_type"`
	OccurredAt string       `json:"occurred_at"` // RFC3339 UTC, stamped at publish
	Customer   CustomerWire `json:"customer"`
	Ticket     TicketWire   `json:"ticket"`
	Change     *ChangeWire  `json:"change,omitempty"`
	Comment    *CommentWire `json:"comment,omitempty"`
	Visit      *VisitWire   `json:"visit,omitempty"`
}

// CustomerWire identifies the tenant. ID is always set; PlatformOrgID is
// omitted for customers not mapped to a platform org.
type CustomerWire struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PlatformOrgID string `json:"platform_org_id,omitempty"`
}

// TicketWire is the ticket snapshot. Assignee reflects the ticket's assignee —
// or, for visit events, the dispatched technician (buildVisitContext overrides
// it). The consumer is MSP-internal, so staff identity is fine to include.
type TicketWire struct {
	ID       string      `json:"id"`
	Number   int         `json:"number"`
	Title    string      `json:"title"`
	Status   string      `json:"status"`
	Priority string      `json:"priority"`
	Type     string      `json:"type,omitempty"`
	Source   string      `json:"source"`
	URL      string      `json:"url,omitempty"`
	Assignee *PersonWire `json:"assignee,omitempty"`
}

// PersonWire names one party.
type PersonWire struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// ChangeWire records a workflow-field transition (status_changed).
type ChangeWire struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// CommentWire rides ticket.commented.
type CommentWire struct {
	AuthorName string `json:"author_name"`
	Body       string `json:"body"`
	ByStaff    bool   `json:"by_staff"`
}

// VisitWire rides the visit.* events. Fields are omitted when empty so a
// requested-then-scheduled visit doesn't publish blank timestamps.
type VisitWire struct {
	ScheduledAt    string `json:"scheduled_at,omitempty"`
	OldScheduledAt string `json:"old_scheduled_at,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	Location       string `json:"location,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

// toEnvelope projects a render context into the wire contract for eventType.
// occurredAt is stamped by the caller (publish) so this stays testable.
func (c TicketContext) toEnvelope(eventType, occurredAt string) EventEnvelope {
	env := EventEnvelope{
		Schema:     EnvelopeSchema,
		Version:    EnvelopeVersion,
		EventType:  eventType,
		OccurredAt: occurredAt,
		Customer: CustomerWire{
			ID:            c.CustomerID,
			Name:          c.Customer,
			PlatformOrgID: c.CustomerOrgID,
		},
		Ticket: TicketWire{
			ID:       c.Ticket.ID,
			Number:   c.Ticket.Number,
			Title:    c.Ticket.Title,
			Status:   c.Ticket.Status,
			Priority: c.Ticket.Priority,
			Type:     c.Ticket.Type,
			Source:   c.Ticket.Source,
			URL:      c.Ticket.URL,
		},
	}
	if c.Assignee.Name != "" {
		env.Ticket.Assignee = &PersonWire{Name: c.Assignee.Name, Email: c.Assignee.Email}
	}
	if eventType == EventTypeTicketStatusChanged && c.Ticket.OldStatus != "" {
		env.Change = &ChangeWire{Field: "status", From: c.Ticket.OldStatus, To: c.Ticket.Status}
	}
	if c.Comment != nil {
		env.Comment = &CommentWire{
			AuthorName: c.Comment.AuthorName,
			Body:       c.Comment.Body,
			ByStaff:    c.Comment.ByStaff,
		}
	}
	if c.Visit != nil {
		env.Visit = &VisitWire{
			ScheduledAt:    c.Visit.ScheduledAt,
			OldScheduledAt: c.Visit.OldScheduledAt,
			AssigneeName:   c.Visit.AssigneeName,
			Location:       c.Visit.Location,
			Notes:          c.Visit.Notes,
		}
	}
	return env
}
