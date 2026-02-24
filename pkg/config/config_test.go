package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromEnvPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
	  "agents": {"defaults": {"model": "openai/gpt-5.2"}},
	  "channels": {"telegram": {}},
	  "providers": {"opencode": {"base_url": "http://127.0.0.1:4096"}},
	  "heartbeat": {"enabled": true, "interval": 30},
	  "devices": {"enabled": false, "monitor_usb": true},
	  "gateway": {"host": "0.0.0.0", "port": 18790},
	  "logging": {"format": "json", "level": "debug", "add_source": true}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("MINICLAW_CONFIG", path)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.Logging.Format != "json" {
		t.Fatalf("logging.format = %q, want %q", cfg.Logging.Format, "json")
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("logging.level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if !cfg.Logging.AddSource {
		t.Fatal("logging.add_source = false, want true")
	}
}

func TestLoadConfigInvalidEnvPath(t *testing.T) {
	t.Setenv("MINICLAW_CONFIG", filepath.Join(t.TempDir(), "missing.json"))

	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error for missing config path")
	}
}
