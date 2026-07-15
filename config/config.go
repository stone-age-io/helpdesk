// Package config loads the helpdesk configuration: a YAML file (path from
// $HELPDESK_CONFIG, default ./helpdesk.yaml) with HELPDESK_-prefixed
// environment overrides. A missing file is fine — defaults plus env cover
// containerized deployments.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	// DataDir is PocketBase's data directory (SQLite, uploads).
	DataDir string

	NATS NATSConfig
}

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
	// OUTBOUND notification events (helpdesk.*.events.>). Separate from Stream
	// because its subjects must not overlap the ingest stream's and it carries
	// a dedupe window for Nats-Msg-Id.
	NotifyStream string
}

// Enabled reports whether a NATS connection should be attempted.
func (n NATSConfig) Enabled() bool { return len(n.URLs) > 0 }

func Load() (*Config, error) {
	v := viper.New()
	v.SetDefault("data_dir", "pb_data")
	v.SetDefault("nats.urls", []string{})
	v.SetDefault("nats.creds_file", "")
	v.SetDefault("nats.stream", "HELPDESK_EVENTS")
	v.SetDefault("nats.durable", "helpdesk-ingest")
	v.SetDefault("nats.notify_stream", "HELPDESK_NOTIFICATIONS")

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
		DataDir: v.GetString("data_dir"),
		NATS: NATSConfig{
			URLs:         v.GetStringSlice("nats.urls"),
			CredsFile:    v.GetString("nats.creds_file"),
			Stream:       v.GetString("nats.stream"),
			Durable:      v.GetString("nats.durable"),
			NotifyStream: v.GetString("nats.notify_stream"),
		},
	}

	if cfg.NATS.Enabled() && cfg.NATS.CredsFile == "" {
		return nil, fmt.Errorf("nats.urls is set but nats.creds_file is empty — the helpdesk authenticates with a hub-account creds file")
	}
	return cfg, nil
}
