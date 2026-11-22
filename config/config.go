// Package config loads the helpdesk configuration: a YAML file (path from
// $HELPDESK_CONFIG, default ./helpdesk.yaml) with HELPDESK_-prefixed
// environment overrides. A missing file is fine — defaults plus env cover
// containerized deployments.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	// DataDir is PocketBase's data directory (SQLite, uploads).
	DataDir string

	// BrandingDir is an optional operator branding overlay: a host directory
	// whose files (theme.css, logo.svg, branding.json) the helpdesk serves under
	// /branding/*, overriding the embedded app name, logo, and DaisyUI theme
	// without a rebuild. Empty (the default) = embedded defaults only.
	BrandingDir string

	// AutoCloseResolvedDays promotes tickets left `resolved` (untouched) this
	// many days to `closed` via a daily cron. 0 disables it (tickets then only
	// close when staff close them by hand). Default 7 — the grace window in
	// which a requester reply reopens the ticket before it's finalized.
	AutoCloseResolvedDays int

	NATS      NATSConfig
	Platform  PlatformConfig
	Inbound   InboundConfig
	Readiness ReadinessConfig
	Metrics   MetricsConfig
}

// MetricsConfig controls the Prometheus exposition at GET /metrics.
//
// Unlike readiness, this one can be turned off: an install may reasonably not
// want the port exposed at all, and removing the route is clearer than serving
// one that 401s.
type MetricsConfig struct {
	// Enabled registers the route. Default true.
	Enabled bool

	// Token, when set, is required as `Authorization: Bearer <token>` or as the
	// password half of Basic auth. Empty leaves the endpoint open, which is the
	// upstream default and is why no series carries a customer label.
	Token string
}

// ReadinessConfig paces the background prober behind GET /api/ready. There is
// deliberately no `enabled` knob: readiness is a contract with whatever is
// orchestrating this process, so it is not something an install turns off.
type ReadinessConfig struct {
	// IntervalSeconds is how often the checks run. The endpoint always serves
	// the last result, so this is the staleness of the answer, not the cost of
	// a probe.
	IntervalSeconds int

	// TimeoutSeconds bounds a single check. One slow check must not hold up
	// the others or stall the loop.
	TimeoutSeconds int
}

// Interval is the probe period as a duration.
func (r ReadinessConfig) Interval() time.Duration {
	return time.Duration(r.IntervalSeconds) * time.Second
}

// Timeout is the per-check bound as a duration.
func (r ReadinessConfig) Timeout() time.Duration {
	return time.Duration(r.TimeoutSeconds) * time.Second
}

// InboundConfig authenticates the email-provider webhook (Postmark to start).
// Empty Secret disables email ingestion — the app serves without it, exactly
// as it does without NATS. There is deliberately no reply-domain or
// default-customer knob: replies thread on the [#N] subject token (the operator
// sets the PocketBase sender address to the forwarded intake mailbox), and mail
// that can't be attributed to a known tenant is rejected, not funneled to a
// catch-all. See docs/email-ingestion.md.
type InboundConfig struct {
	// Secret is the Basic-auth password the provider presents on the webhook
	// URL. Empty ⇒ the email route is not registered.
	Secret string
	// AllowedIPs optionally restricts the webhook to the provider's published
	// egress ranges. Empty ⇒ no IP restriction (secret alone gates it).
	AllowedIPs []string
	// ReplyTo is a stored-only escape hatch for installs whose outbound sender
	// address differs from the intake mailbox. Unwired in v1 (threading works
	// off the sender address); documented for when a deployment needs it.
	ReplyTo string
}

// Enabled reports whether email ingestion should be wired.
func (i InboundConfig) Enabled() bool { return i.Secret != "" }

// NATSConfig connects the helpdesk to the platform operator's hub account.
// Auth is a Stone-Age.io-issued .creds file for a nats_user scoped to
// `helpdesk.>`. Disabled (no URLs) is valid: the app serves without NATS and
// tickets arrive only via portal/agent/webhook.
type NATSConfig struct {
	URLs      []string
	CredsFile string
	// Stream is the JetStream stream the helpdesk creates and owns in the hub
	// account to durably capture inbound ticket events.
	Stream string
	// Durable is the consumer name; stable so restarts resume where they left off.
	Durable string
	// NotifyStream is the JetStream stream the helpdesk creates and owns for
	// OUTBOUND notification events (helpdesk.*.evt.>). Separate from Stream
	// because its subjects must not overlap the ingest stream's and it carries
	// a dedupe window for Nats-Msg-Id.
	NotifyStream string
	// Code is the code of this helpdesk's Thing on the platform (ADR 0008 rule
	// 4). It goes into the NATS service discovery metadata, which is how the
	// console joins the running process to its Thing. Empty skips discovery
	// registration; everything else works.
	Code string
}

// Enabled reports whether a NATS connection should be attempted.
func (n NATSConfig) Enabled() bool { return len(n.URLs) > 0 }

// PlatformConfig keeps nats.creds_file current from the platform, as the
// helpdesk's Thing (ADR 0009 in platform-docs). The kit's credskeeper signs in,
// writes the creds file, and rewrites it after a Regenerate or a Revoke, so a
// rotated credential reaches the helpdesk without anyone copying a file.
// Without a URL nothing starts, and nats.creds_file is a file you provide.
type PlatformConfig struct {
	// URL is the platform's base URL. https:// unless AllowInsecure.
	URL string
	// Identity is the helpdesk Thing's login email.
	Identity string
	// Password is the Thing's password. Needed only until the first sync has
	// saved a session; then remove it (set it by env, not in the file).
	Password string
	// SessionFile defaults to platform-session.json beside nats.creds_file.
	SessionFile string
	// SyncInterval is how often the platform is asked for a change. Default 5m.
	SyncInterval time.Duration
	// AllowInsecure lets URL be http://. Development only: the first sync
	// sends the password and receives a private key.
	AllowInsecure bool
}

// Enabled reports whether the creds file is kept by the platform.
func (p PlatformConfig) Enabled() bool { return p.URL != "" }

func Load() (*Config, error) {
	v := viper.New()
	v.SetDefault("data_dir", "pb_data")
	v.SetDefault("branding.dir", "")
	v.SetDefault("auto_close_resolved_days", 7)
	v.SetDefault("nats.urls", []string{})
	v.SetDefault("nats.creds_file", "")
	v.SetDefault("nats.stream", "HELPDESK_EVENTS")
	v.SetDefault("nats.durable", "helpdesk-ingest")
	v.SetDefault("nats.notify_stream", "HELPDESK_NOTIFICATIONS")
	v.SetDefault("nats.code", "")
	v.SetDefault("platform.url", "")
	v.SetDefault("platform.identity", "")
	v.SetDefault("platform.password", "")
	v.SetDefault("platform.session_file", "")
	v.SetDefault("platform.sync_interval", "5m")
	v.SetDefault("platform.allow_insecure", false)
	v.SetDefault("inbound.secret", "")
	v.SetDefault("inbound.allowed_ips", []string{})
	v.SetDefault("inbound.reply_to", "")
	// 30s is the platform's cadence. The endpoint serves the cached result, so
	// this is how stale an answer may be, not what a probe costs.
	v.SetDefault("readiness.interval_seconds", 30)
	v.SetDefault("readiness.timeout_seconds", 10)
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.token", "")

	v.SetEnvPrefix("HELPDESK")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path := os.Getenv("HELPDESK_CONFIG"); path != "" {
		v.SetConfigFile(path)
	} else {
		// No SetConfigType here: with a type set, viper also matches the
		// extensionless file "helpdesk" — the binary itself when cwd is the
		// deploy dir. Name-only search matches helpdesk.yaml and friends.
		v.SetConfigName("helpdesk")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/helpdesk/")
	}
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfg := &Config{
		DataDir:               v.GetString("data_dir"),
		BrandingDir:           v.GetString("branding.dir"),
		AutoCloseResolvedDays: v.GetInt("auto_close_resolved_days"),
		NATS: NATSConfig{
			URLs:         v.GetStringSlice("nats.urls"),
			CredsFile:    v.GetString("nats.creds_file"),
			Stream:       v.GetString("nats.stream"),
			Durable:      v.GetString("nats.durable"),
			NotifyStream: v.GetString("nats.notify_stream"),
			Code:         v.GetString("nats.code"),
		},
		Platform: PlatformConfig{
			URL:           v.GetString("platform.url"),
			Identity:      v.GetString("platform.identity"),
			Password:      v.GetString("platform.password"),
			SessionFile:   v.GetString("platform.session_file"),
			SyncInterval:  v.GetDuration("platform.sync_interval"),
			AllowInsecure: v.GetBool("platform.allow_insecure"),
		},
		Inbound: InboundConfig{
			Secret:     v.GetString("inbound.secret"),
			AllowedIPs: v.GetStringSlice("inbound.allowed_ips"),
			ReplyTo:    v.GetString("inbound.reply_to"),
		},
		Readiness: ReadinessConfig{
			IntervalSeconds: v.GetInt("readiness.interval_seconds"),
			TimeoutSeconds:  v.GetInt("readiness.timeout_seconds"),
		},
		Metrics: MetricsConfig{
			Enabled: v.GetBool("metrics.enabled"),
			Token:   v.GetString("metrics.token"),
		},
	}

	if cfg.NATS.Enabled() && cfg.NATS.CredsFile == "" {
		return nil, fmt.Errorf("nats.urls is set but nats.creds_file is empty — the helpdesk authenticates with a hub-account creds file")
	}
	if cfg.Platform.Enabled() {
		if !cfg.NATS.Enabled() {
			return nil, fmt.Errorf("platform.url is set but nats.urls is empty — the platform keeps a creds file only for a NATS connection")
		}
		if cfg.Platform.SyncInterval <= 0 {
			return nil, fmt.Errorf("platform.sync_interval must be a positive duration such as 5m")
		}
		// PocketBase backups archive all of data_dir. The creds file, the
		// session and a device-held seed must not ride along: a restored
		// backup would give a second host this helpdesk's identity.
		for key, path := range map[string]string{
			"nats.creds_file":       cfg.NATS.CredsFile,
			"platform.session_file": cfg.Platform.SessionFile,
		} {
			if path != "" && within(cfg.DataDir, path) {
				return nil, fmt.Errorf("%s (%s) is inside data_dir (%s), which PocketBase backups archive; keep the platform's credentials in a directory beside it, such as pb_creds/", key, path, cfg.DataDir)
			}
		}
	}
	return cfg, nil
}

// within reports whether path is dir or under it.
func within(dir, path string) bool {
	d, err1 := filepath.Abs(dir)
	p, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(d, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
