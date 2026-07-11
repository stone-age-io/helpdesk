package notifications

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// RegisterRoutes binds the admin template-editor API under /api/helpdesk:
//
//	GET   /api/helpdesk/notifications                        — list templates
//	PATCH /api/helpdesk/notifications/{event_type}           — edit one
//	GET   /api/helpdesk/notifications/{event_type}/defaults  — compiled-in copy
//
// The send log is read by the SPA through the regular collection API (its
// list rule is admin-gated), so no route is needed for it.
func RegisterRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/helpdesk/notifications", listTemplates)
	e.Router.PATCH("/api/helpdesk/notifications/{event_type}", updateTemplate)
	e.Router.GET("/api/helpdesk/notifications/{event_type}/defaults", templateDefaults)
}

// templateDTO is the JSON shape returned to the staff SPA. Mirrors the
// collection columns; recipients is normalized so the SPA always renders
// concrete checkbox state.
type templateDTO struct {
	ID         string     `json:"id"`
	EventType  string     `json:"event_type"`
	Name       string     `json:"name"`
	Enabled    bool       `json:"enabled"`
	Subject    string     `json:"subject"`
	Body       string     `json:"body"`
	Updated    string     `json:"updated"`
	UpdatedBy  string     `json:"updated_by"`
	Recipients Recipients `json:"recipients"`
}

func toTemplateDTO(r *core.Record) templateDTO {
	eventType := r.GetString("event_type")
	recipients := DefaultRecipients(eventType)
	if raw := strings.TrimSpace(r.GetString("recipients")); raw != "" && raw != "null" {
		var parsed Recipients
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			recipients = parsed
		}
	}
	if recipients.Extras == nil {
		recipients.Extras = []string{}
	}
	return templateDTO{
		ID:         r.Id,
		EventType:  eventType,
		Name:       r.GetString("name"),
		Enabled:    r.GetBool("enabled"),
		Subject:    r.GetString("subject"),
		Body:       r.GetString("body"),
		Updated:    r.GetDateTime("updated").String(),
		UpdatedBy:  r.GetString("updated_by"),
		Recipients: recipients,
	}
}

// requireAdmin gates the editor routes to admin staff. Route-level guard
// (not a collection rule) because PATCH does template validation the
// collection API can't express.
func requireAdmin(re *core.RequestEvent) error {
	if re.Auth == nil ||
		re.Auth.Collection().Name != authz.StaffCollection ||
		re.Auth.GetString("role") != "admin" {
		return re.ForbiddenError("admin staff only", nil)
	}
	return nil
}

func listTemplates(re *core.RequestEvent) error {
	if err := requireAdmin(re); err != nil {
		return err
	}
	rows, err := re.App.FindRecordsByFilter(CollectionName, "", "event_type", 0, 0)
	if err != nil {
		return re.InternalServerError("load templates failed", err)
	}
	out := make([]templateDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTemplateDTO(r))
	}
	return re.JSON(http.StatusOK, map[string]any{"templates": out})
}

// updateTemplate patches subject/body/enabled/recipients on an existing
// template row identified by its event_type path segment. Both template
// strings must parse via text/template before saving — malformed input is
// rejected with a 400 carrying the parse error.
func updateTemplate(re *core.RequestEvent) error {
	if err := requireAdmin(re); err != nil {
		return err
	}
	eventType := re.Request.PathValue("event_type")
	if eventType == "" {
		return re.BadRequestError("event_type is required", nil)
	}

	var body struct {
		Subject    *string     `json:"subject,omitempty"`
		Body       *string     `json:"body,omitempty"`
		Enabled    *bool       `json:"enabled,omitempty"`
		Recipients *Recipients `json:"recipients,omitempty"`
	}
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("invalid request body", err)
	}
	if body.Subject == nil && body.Body == nil && body.Enabled == nil && body.Recipients == nil {
		return re.BadRequestError("at least one of subject, body, enabled, recipients is required", nil)
	}

	rec, err := re.App.FindFirstRecordByFilter(
		CollectionName,
		"event_type = {:t}",
		dbx.Params{"t": eventType},
	)
	if err != nil || rec == nil {
		return re.NotFoundError("template not found", nil)
	}

	newSubject := rec.GetString("subject")
	newBody := rec.GetString("body")
	if body.Subject != nil {
		newSubject = *body.Subject
	}
	if body.Body != nil {
		newBody = *body.Body
	}

	// Parse-validate whenever either text field is changing. Saving an
	// unchanged template would still pass; we only block the new bytes.
	if body.Subject != nil || body.Body != nil {
		if err := ValidateTemplates(newSubject, newBody); err != nil {
			return re.BadRequestError(err.Error(), nil)
		}
	}

	if body.Subject != nil {
		rec.Set("subject", newSubject)
	}
	if body.Body != nil {
		rec.Set("body", newBody)
	}
	if body.Enabled != nil {
		rec.Set("enabled", *body.Enabled)
	}
	if body.Recipients != nil {
		normalized, err := normalizeRecipients(*body.Recipients)
		if err != nil {
			return re.BadRequestError(err.Error(), nil)
		}
		raw, err := json.Marshal(normalized)
		if err != nil {
			return re.InternalServerError("encode recipients", err)
		}
		rec.Set("recipients", string(raw))
	}
	rec.Set("updated_by", re.Auth.Id)

	if err := re.App.Save(rec); err != nil {
		return re.InternalServerError("save failed", err)
	}
	return re.JSON(http.StatusOK, toTemplateDTO(rec))
}

// normalizeRecipients trims and validates the supplied spec. Empties are
// coerced to empty slices (never nil) so the persisted JSON shape is
// stable, and extras are validated as parseable mail addresses so the
// notifier never chokes on bad input at send time.
func normalizeRecipients(in Recipients) (Recipients, error) {
	out := Recipients{
		Requester: in.Requester,
		Assignee:  in.Assignee,
		AllStaff:  in.AllStaff,
		Extras:    []string{},
	}
	seen := map[string]bool{}
	for _, raw := range in.Extras {
		addr := strings.TrimSpace(raw)
		if addr == "" {
			continue
		}
		if _, err := mail.ParseAddress(addr); err != nil {
			return out, fmt.Errorf("invalid extras email %q: %v", addr, err)
		}
		key := strings.ToLower(addr)
		if seen[key] {
			continue
		}
		seen[key] = true
		out.Extras = append(out.Extras, addr)
	}
	return out, nil
}

// templateDefaults returns the compiled-in subject + body for the given
// event_type so the SPA's "Reset to defaults" button can refill its
// textareas without a server-side mutation. The admin still has to click
// Save to persist; this endpoint is read-only.
func templateDefaults(re *core.RequestEvent) error {
	if err := requireAdmin(re); err != nil {
		return err
	}
	eventType := re.Request.PathValue("event_type")
	if eventType == "" {
		return re.BadRequestError("event_type is required", nil)
	}
	subject, body, ok := Defaults(eventType)
	if !ok {
		return re.NotFoundError("no defaults for that event_type", nil)
	}
	return re.JSON(http.StatusOK, map[string]any{
		"event_type": eventType,
		"subject":    subject,
		"body":       body,
		"recipients": DefaultRecipients(eventType),
	})
}
