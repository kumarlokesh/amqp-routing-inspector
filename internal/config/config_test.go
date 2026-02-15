package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("AMQP_INSPECTOR_RABBITMQ_URL", "")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RabbitMQURL != "amqp://guest:guest@localhost:5672/" {
		t.Fatalf("unexpected RabbitMQURL: %q", cfg.RabbitMQURL)
	}
	if cfg.OutputFormat != FormatCLI {
		t.Fatalf("unexpected OutputFormat: %q", cfg.OutputFormat)
	}
	if cfg.Prefetch <= 0 {
		t.Fatalf("prefetch should be > 0, got %d", cfg.Prefetch)
	}
}

func TestLoadConfigFileAndFlagOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`rabbitmq_url: amqp://user:pass@localhost:5678/
output_format: json
max_events: 25
prefetch: 20
reconnect_initial: 2s
reconnect_max: 10s
graph_name: inspector
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Load([]string{"--config", path, "--output", "dot", "--max-events", "10"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OutputFormat != FormatDOT {
		t.Fatalf("expected output format dot, got %q", cfg.OutputFormat)
	}
	if cfg.MaxEvents != 10 {
		t.Fatalf("expected max-events 10, got %d", cfg.MaxEvents)
	}
	if cfg.RabbitMQURL != "amqp://user:pass@localhost:5678/" {
		t.Fatalf("unexpected RabbitMQURL: %q", cfg.RabbitMQURL)
	}
	if cfg.ConfigPath != path {
		t.Fatalf("expected config path %q, got %q", path, cfg.ConfigPath)
	}
}

func TestLoadUnknownConfigFieldFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(`unknown_field: true
`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	if _, err := Load([]string{"--config", path}); err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

func TestLoadUsesEnvironment(t *testing.T) {
	t.Setenv("AMQP_INSPECTOR_OUTPUT_FORMAT", "json")
	t.Setenv("AMQP_INSPECTOR_PREFETCH", "300")
	t.Setenv("AMQP_INSPECTOR_RECONNECT_INITIAL", "3s")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OutputFormat != FormatJSON {
		t.Fatalf("expected json output from env, got %q", cfg.OutputFormat)
	}
	if cfg.Prefetch != 300 {
		t.Fatalf("expected prefetch 300 from env, got %d", cfg.Prefetch)
	}
	if cfg.ReconnectInitial != 3*time.Second {
		t.Fatalf("expected reconnect initial 3s, got %s", cfg.ReconnectInitial)
	}
}

func TestLoadValidationError(t *testing.T) {
	_, err := Load([]string{"--output", "xml"})
	if err == nil {
		t.Fatalf("expected invalid format error")
	}
}
