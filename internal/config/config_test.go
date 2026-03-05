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

func TestLoadNewFilterFlags(t *testing.T) {
	cfg, err := Load([]string{
		"--filter-routing-key", "order.*",
		"--filter-event", "publish",
		"--warn-unrouted",
		"--show-body-bytes", "256",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.FilterRoutingKey != "order.*" {
		t.Fatalf("unexpected FilterRoutingKey: %q", cfg.FilterRoutingKey)
	}
	if cfg.FilterEvent != "publish" {
		t.Fatalf("unexpected FilterEvent: %q", cfg.FilterEvent)
	}
	if !cfg.WarnUnrouted {
		t.Fatalf("expected WarnUnrouted to be true")
	}
	if cfg.ShowBodyBytes != 256 {
		t.Fatalf("unexpected ShowBodyBytes: %d", cfg.ShowBodyBytes)
	}
}

func TestLoadValidatesFilterEvent(t *testing.T) {
	_, err := Load([]string{"--filter-event", "invalid"})
	if err == nil {
		t.Fatal("expected validation error for invalid filter-event")
	}
}

func TestLoadNewFormatConstants(t *testing.T) {
	for _, format := range []string{FormatSummary, FormatMermaid} {
		cfg, err := Load([]string{"--output", format})
		if err != nil {
			t.Fatalf("Load() with format=%q error = %v", format, err)
		}
		if cfg.OutputFormat != format {
			t.Fatalf("expected output format %q, got %q", format, cfg.OutputFormat)
		}
	}
}

func TestLoadRequiresMetricsAddrForPrometheus(t *testing.T) {
	_, err := Load([]string{"--output", "prometheus"})
	if err == nil {
		t.Fatal("expected error when --output prometheus without --metrics-addr")
	}
}

func TestLoadRequiresStatsdAddrForStatsd(t *testing.T) {
	_, err := Load([]string{"--output", "statsd"})
	if err == nil {
		t.Fatal("expected error when --output statsd without --statsd-addr")
	}
}

func TestLoadRequiresWebAddrForWeb(t *testing.T) {
	_, err := Load([]string{"--output", "web"})
	if err == nil {
		t.Fatal("expected error when --output web without --web-addr")
	}
}

func TestLoadPrometheusWithAddr(t *testing.T) {
	cfg, err := Load([]string{"--output", "prometheus", "--metrics-addr", ":9090"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MetricsAddr != ":9090" {
		t.Fatalf("unexpected MetricsAddr: %q", cfg.MetricsAddr)
	}
}
