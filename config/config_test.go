package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// load runs Load with only the given HELPDESK_* variables and no config file.
func load(t *testing.T, env map[string]string) (*Config, error) {
	t.Helper()
	t.Chdir(t.TempDir()) // no ./helpdesk.yaml
	t.Setenv("HELPDESK_CONFIG", "")
	for k, v := range env {
		t.Setenv(k, v)
	}
	return Load()
}

func TestPlatformKeepsTheCredsFile(t *testing.T) {
	cfg, err := load(t, map[string]string{
		"HELPDESK_NATS_URLS":              "nats://hub:4222",
		"HELPDESK_NATS_CREDS_FILE":        "pb_creds/helpdesk.creds",
		"HELPDESK_PLATFORM_URL":           "https://platform.example.com",
		"HELPDESK_PLATFORM_IDENTITY":      "helpdesk@things.example.com",
		"HELPDESK_PLATFORM_PASSWORD":      "first-start-only",
		"HELPDESK_PLATFORM_SYNC_INTERVAL": "2m",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Platform
	if !p.Enabled() || p.Identity != "helpdesk@things.example.com" || p.Password != "first-start-only" || p.SyncInterval != 2*time.Minute {
		t.Fatalf("platform config = %+v", p)
	}
}

func TestPlatformDefaults(t *testing.T) {
	cfg, err := load(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Platform.Enabled() {
		t.Fatal("the keeper is on with no platform.url")
	}
	if cfg.Platform.SyncInterval != 5*time.Minute {
		t.Fatalf("sync_interval default = %s, want 5m", cfg.Platform.SyncInterval)
	}
}

func TestPlatformRefusals(t *testing.T) {
	base := map[string]string{
		"HELPDESK_NATS_URLS":         "nats://hub:4222",
		"HELPDESK_NATS_CREDS_FILE":   "pb_creds/helpdesk.creds",
		"HELPDESK_PLATFORM_URL":      "https://platform.example.com",
		"HELPDESK_PLATFORM_IDENTITY": "helpdesk@things.example.com",
	}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range base {
			m[a] = b
		}
		m[k] = v
		return m
	}
	for name, tc := range map[string]struct {
		env  map[string]string
		want string
	}{
		"no NATS":                  {with("HELPDESK_NATS_URLS", ""), "nats.urls is empty"},
		"creds file in data_dir":   {with("HELPDESK_NATS_CREDS_FILE", "pb_data/helpdesk.creds"), "nats.creds_file"},
		"creds file is data_dir":   {with("HELPDESK_NATS_CREDS_FILE", "pb_data"), "nats.creds_file"},
		"session file in data_dir": {with("HELPDESK_PLATFORM_SESSION_FILE", filepath.Join("pb_data", "s", "session.json")), "platform.session_file"},
		"zero interval":            {with("HELPDESK_PLATFORM_SYNC_INTERVAL", "0s"), "sync_interval"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := load(t, tc.env)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one naming %q", err, tc.want)
			}
		})
	}

	// A sibling whose name starts with the data dir's is not inside it.
	if _, err := load(t, with("HELPDESK_NATS_CREDS_FILE", "pb_data_creds/helpdesk.creds")); err != nil {
		t.Fatalf("pb_data_creds/ refused as inside pb_data/: %v", err)
	}
}
