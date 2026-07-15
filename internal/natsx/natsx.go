// Package natsx is a small NATS connection helper, adapted (trimmed, not
// imported) from access-control's natsx: connection lifecycle with
// creds-file auth, plus a convenience helper to ensure the inbox stream
// exists. It deliberately knows nothing about consumers or projection.
//
// The helpdesk's NATS identity is a hub-account nats_user minted by the
// platform, scoped to `sub helpdesk.>` — it can read its inbox subjects and
// nothing else in the operator account.
package natsx

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	opTimeout          = 10 * time.Second
	reconnectJitterMin = 100 * time.Millisecond
	reconnectJitterMax = 1 * time.Second
	drainTimeout       = 10 * time.Second
)

// Conn bundles the core NATS connection and a JetStream context.
type Conn struct {
	NC *nats.Conn
	JS jetstream.JetStream
}

// Connect establishes the NATS connection and JetStream context using
// creds-file auth. Reconnects forever with jitter — the helpdesk serves
// portal/webhook traffic regardless of broker health, and the durable
// consumer resumes where it left off on reconnect.
func Connect(urls []string, credsFile string) (*Conn, error) {
	opts := []nats.Option{
		nats.MaxReconnects(-1),
		nats.ReconnectJitter(reconnectJitterMin, reconnectJitterMax),
		nats.DrainTimeout(drainTimeout),
		nats.UserCredentials(credsFile),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("nats disconnected", "err", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Info("nats reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			slog.Error("nats connection permanently closed", "err", nc.LastError())
		}),
	}

	urlString := strings.Join(urls, ",")
	nc, err := nats.Connect(urlString, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS (urls=%v): %w", urls, err)
	}
	slog.Info("nats connected", "url", nc.ConnectedUrl())

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}
	return &Conn{NC: nc, JS: js}, nil
}

// EnsureStream returns the named JetStream stream, creating it if it does
// not exist. The helpdesk owns its inbox stream in the hub account —
// deliberately not the platform's job.
func (c *Conn) EnsureStream(ctx context.Context, name string, subjs []string) (jetstream.Stream, error) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	stream, err := c.JS.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        name,
		Description: "helpdesk inbound ticket events (hub-side, org-scoped subjects)",
		Subjects:    subjs,
		Retention:   jetstream.LimitsPolicy,
		MaxAge:      7 * 24 * time.Hour,
		Storage:     jetstream.FileStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure stream %q: %w", name, err)
	}
	slog.Info("jetstream stream ready", "stream", name, "subjects", subjs)
	return stream, nil
}

// notifyDedupeWindow is the JetStream Duplicates window for the outbound
// notification stream: a Nats-Msg-Id republished within this window is
// collapsed. Kept short — this only guards accidental double-publish of the
// same event, not long-horizon idempotency (which the inbound side does with
// a DB dedupe_key).
const notifyDedupeWindow = 2 * time.Minute

// EnsureNotifyStream returns the outbound notification stream, creating it if
// absent. Separate from EnsureStream because its subjects must stay disjoint
// from the ingest stream (helpdesk.*.events.> vs helpdesk.*.tickets.>) and it
// carries a Duplicates window so the Nats-Msg-Id header (see Publisher) dedupes
// republished events. The helpdesk owns this stream, symmetric with the inbox.
func (c *Conn) EnsureNotifyStream(ctx context.Context, name string, subjs []string) (jetstream.Stream, error) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	stream, err := c.JS.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        name,
		Description: "helpdesk outbound notification events (hub-side)",
		Subjects:    subjs,
		Retention:   jetstream.LimitsPolicy,
		MaxAge:      7 * 24 * time.Hour,
		Storage:     jetstream.FileStorage,
		Duplicates:  notifyDedupeWindow,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure notify stream %q: %w", name, err)
	}
	slog.Info("jetstream notify stream ready", "stream", name, "subjects", subjs)
	return stream, nil
}

// Publisher publishes JSON event envelopes to a JetStream subject. It is a
// dumb pipe: the caller builds the subject and the Nats-Msg-Id; the Publisher
// only marshals the wire message and waits for the JetStream ack. It satisfies
// notifications.Publisher structurally, keeping the notifications package free
// of any NATS import.
type Publisher struct {
	js jetstream.JetStream
}

// NewPublisher wraps a JetStream context for outbound publishing.
func NewPublisher(js jetstream.JetStream) *Publisher {
	return &Publisher{js: js}
}

// Publish sends data on subject with the given Nats-Msg-Id (for stream-side
// dedupe) and blocks on the JetStream ack. A publish to a subject no stream
// captures, or without publish permission, returns an error the caller logs —
// the notifier treats it as a best-effort failure, never fatal.
func (p *Publisher) Publish(ctx context.Context, subject string, data []byte, msgID string) error {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	if msgID != "" {
		msg.Header.Set(jetstream.MsgIDHeader, msgID)
	}
	if _, err := p.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("publish %q: %w", subject, err)
	}
	return nil
}

// Close drains and closes the NATS connection.
func (c *Conn) Close() error {
	if c == nil || c.NC == nil {
		return nil
	}
	if err := c.NC.Drain(); err != nil {
		return fmt.Errorf("drain NATS connection: %w", err)
	}
	return nil
}
