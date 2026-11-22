// Package streams owns the helpdesk's two JetStream streams in the hub account
// and the publisher for outbound events. The connection itself comes from the
// kit's natsx; what is here is the helpdesk's own stream configuration.
//
// The helpdesk's NATS identity is a hub-account nats_user minted by the
// platform, scoped to `helpdesk.>`.
package streams

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stone-age-io/kit/natsx"
)

const (
	opTimeout = 10 * time.Second
	maxAge    = 7 * 24 * time.Hour

	// notifyDedupeWindow is the Duplicates window for the outbound stream: a
	// Nats-Msg-Id republished within it is collapsed. Kept short: this only
	// guards accidental double-publish of the same event, not long-horizon
	// idempotency (which the inbound side does with a DB dedupe_key, held for
	// as long as its ticket stays open).
	notifyDedupeWindow = 2 * time.Minute
)

// EnsureIngest creates or updates the inbox stream for ticket commands. The
// helpdesk owns it in the hub account, deliberately not the platform's job.
// CreateOrUpdate, so a change of subjects reaches an existing deployment at
// startup.
func EnsureIngest(ctx context.Context, js jetstream.JetStream, name string, subjs []string) (jetstream.Stream, error) {
	return ensure(ctx, js, jetstream.StreamConfig{
		Name:        name,
		Description: "helpdesk inbound ticket commands (hub-side, org-scoped subjects)",
		Subjects:    subjs,
		Retention:   jetstream.LimitsPolicy,
		MaxAge:      maxAge,
		Storage:     jetstream.FileStorage,
	})
}

// EnsureNotify creates or updates the outbound event stream. Its subjects must
// stay disjoint from the ingest stream's (helpdesk.*.evt.> vs
// helpdesk.*.cmd.tickets.>), and it carries a Duplicates window so the
// Nats-Msg-Id header dedupes republished events.
func EnsureNotify(ctx context.Context, js jetstream.JetStream, name string, subjs []string) (jetstream.Stream, error) {
	return ensure(ctx, js, jetstream.StreamConfig{
		Name:        name,
		Description: "helpdesk outbound notification events (hub-side)",
		Subjects:    subjs,
		Retention:   jetstream.LimitsPolicy,
		MaxAge:      maxAge,
		Storage:     jetstream.FileStorage,
		Duplicates:  notifyDedupeWindow,
	})
}

func ensure(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	stream, err := js.CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("ensure stream %q: %w", cfg.Name, err)
	}
	slog.Info("jetstream stream ready", "stream", cfg.Name, "subjects", cfg.Subjects)
	return stream, nil
}

// Publisher publishes outbound event envelopes through the kit's
// natsx.PublishEvent, which sets Nats-Msg-Id and refuses an event without an
// RFC 3339 "ts". It satisfies notifications.Publisher structurally, keeping
// the notifications package free of any NATS import.
type Publisher struct {
	js jetstream.JetStream
}

// NewPublisher wraps a JetStream context for outbound publishing.
func NewPublisher(js jetstream.JetStream) *Publisher {
	return &Publisher{js: js}
}

// Publish sends the already-marshalled envelope on subject and blocks on the
// JetStream ack. A publish to a subject no stream captures, or without publish
// permission, returns an error the caller logs: the notifier treats it as a
// best-effort failure, never fatal.
func (p *Publisher) Publish(ctx context.Context, subject string, data []byte, msgID string) error {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	_, err := natsx.PublishEvent(ctx, p.js, subject, msgID, json.RawMessage(data))
	return err
}
