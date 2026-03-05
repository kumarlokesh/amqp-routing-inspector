package config

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	FormatCLI        = "cli"
	FormatJSON       = "json"
	FormatDOT        = "dot"
	FormatSummary    = "summary"
	FormatMermaid    = "mermaid"
	FormatPrometheus = "prometheus"
	FormatStatsd     = "statsd"
	FormatWeb        = "web"
)

// ErrHelpRequested is returned when -h/--help is requested.
var ErrHelpRequested = errors.New("help requested")

// Config holds runtime settings for the inspector.
type Config struct {
	ConfigPath  string `yaml:"-"`
	ShowVersion bool   `yaml:"-"`

	RabbitMQURL      string        `yaml:"rabbitmq_url"`
	FirehoseExchange string        `yaml:"firehose_exchange"`
	QueueName        string        `yaml:"queue_name"`
	ConsumerTag      string        `yaml:"consumer_tag"`
	OutputFormat     string        `yaml:"output_format"`
	FilterExchange   string        `yaml:"filter_exchange"`
	FilterQueue      string        `yaml:"filter_queue"`
	FilterRoutingKey string        `yaml:"filter_routing_key"`
	FilterEvent      string        `yaml:"filter_event"`
	MaxEvents        int           `yaml:"max_events"`
	Prefetch         int           `yaml:"prefetch"`
	ReconnectInitial time.Duration `yaml:"reconnect_initial"`
	ReconnectMax     time.Duration `yaml:"reconnect_max"`
	GraphName        string        `yaml:"graph_name"`
	WarnUnrouted     bool          `yaml:"warn_unrouted"`
	ShowBodyBytes    int           `yaml:"show_body_bytes"`
	MetricsAddr      string        `yaml:"metrics_addr"`
	StatsdAddr       string        `yaml:"statsd_addr"`
	WebAddr          string        `yaml:"web_addr"`
}

// Default returns a production-sensible configuration baseline.
func Default() Config {
	return Config{
		RabbitMQURL:      "amqp://guest:guest@localhost:5672/",
		FirehoseExchange: "amq.rabbitmq.trace",
		QueueName:        "",
		ConsumerTag:      "amqp-routing-inspector",
		OutputFormat:     FormatCLI,
		FilterExchange:   "",
		FilterQueue:      "",
		FilterRoutingKey: "",
		FilterEvent:      "",
		MaxEvents:        0,
		Prefetch:         200,
		ReconnectInitial: 1 * time.Second,
		ReconnectMax:     30 * time.Second,
		GraphName:        "routing",
		WarnUnrouted:     false,
		ShowBodyBytes:    0,
		MetricsAddr:      "",
		StatsdAddr:       "",
		WebAddr:          "",
	}
}

// Load resolves configuration using this precedence (low to high):
// defaults -> config file -> env vars -> CLI flags.
func Load(args []string) (Config, error) {
	cfg := Default()

	configPath, err := scanConfigPath(args)
	if err != nil {
		return Config{}, err
	}

	if configPath != "" {
		if err := loadFromFile(configPath, &cfg); err != nil {
			return Config{}, err
		}
		cfg.ConfigPath = configPath
	}

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}

	if err := parseFlags(args, &cfg); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cfg, ErrHelpRequested
		}
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Validate checks runtime constraints and normalizes a few fields.
func (c *Config) Validate() error {
	c.OutputFormat = strings.ToLower(strings.TrimSpace(c.OutputFormat))
	c.GraphName = strings.TrimSpace(c.GraphName)
	c.RabbitMQURL = strings.TrimSpace(c.RabbitMQURL)
	c.FirehoseExchange = strings.TrimSpace(c.FirehoseExchange)
	c.FilterExchange = strings.TrimSpace(c.FilterExchange)
	c.FilterQueue = strings.TrimSpace(c.FilterQueue)
	c.FilterRoutingKey = strings.TrimSpace(c.FilterRoutingKey)
	c.FilterEvent = strings.ToLower(strings.TrimSpace(c.FilterEvent))
	c.MetricsAddr = strings.TrimSpace(c.MetricsAddr)
	c.StatsdAddr = strings.TrimSpace(c.StatsdAddr)
	c.WebAddr = strings.TrimSpace(c.WebAddr)

	if c.RabbitMQURL == "" {
		return errors.New("rabbitmq-url must not be empty")
	}
	if c.FirehoseExchange == "" {
		return errors.New("firehose-exchange must not be empty")
	}
	if c.GraphName == "" {
		c.GraphName = "routing"
	}

	switch c.OutputFormat {
	case FormatCLI, FormatJSON, FormatDOT, FormatSummary, FormatMermaid, FormatPrometheus, FormatStatsd, FormatWeb:
	default:
		return fmt.Errorf("unsupported output format %q (allowed: cli, json, dot, summary, mermaid, prometheus, statsd, web)", c.OutputFormat)
	}

	if c.FilterEvent != "" && c.FilterEvent != "publish" && c.FilterEvent != "deliver" {
		return fmt.Errorf("filter-event must be 'publish' or 'deliver', got %q", c.FilterEvent)
	}

	if c.Prefetch <= 0 {
		return errors.New("prefetch must be greater than 0")
	}
	if c.MaxEvents < 0 {
		return errors.New("max-events must be >= 0")
	}
	if c.ShowBodyBytes < 0 {
		return errors.New("show-body-bytes must be >= 0")
	}
	if c.ReconnectInitial <= 0 {
		return errors.New("reconnect-initial must be greater than 0")
	}
	if c.ReconnectMax <= 0 {
		return errors.New("reconnect-max must be greater than 0")
	}
	if c.ReconnectMax < c.ReconnectInitial {
		return errors.New("reconnect-max must be >= reconnect-initial")
	}

	if c.OutputFormat == FormatPrometheus && c.MetricsAddr == "" {
		return errors.New("--metrics-addr is required when --output prometheus")
	}
	if c.OutputFormat == FormatStatsd && c.StatsdAddr == "" {
		return errors.New("--statsd-addr is required when --output statsd")
	}
	if c.OutputFormat == FormatWeb && c.WebAddr == "" {
		return errors.New("--web-addr is required when --output web")
	}

	return nil
}

func parseFlags(args []string, cfg *Config) error {
	fs, configPath := newFlagSet(cfg, io.Discard)

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfg.ConfigPath = strings.TrimSpace(*configPath)
	return nil
}

// Usage returns the full CLI usage/help text.
func Usage() string {
	tmp := Default()

	var builder strings.Builder
	builder.WriteString("Usage: amqp-routing-inspector [flags]\n\n")

	fs, _ := newFlagSet(&tmp, &builder)
	fs.PrintDefaults()

	builder.WriteString("\nEnvironment variables (override config file defaults):\n")
	builder.WriteString("  AMQP_INSPECTOR_RABBITMQ_URL\n")
	builder.WriteString("  AMQP_INSPECTOR_FIREHOSE_EXCHANGE\n")
	builder.WriteString("  AMQP_INSPECTOR_QUEUE_NAME\n")
	builder.WriteString("  AMQP_INSPECTOR_CONSUMER_TAG\n")
	builder.WriteString("  AMQP_INSPECTOR_OUTPUT_FORMAT\n")
	builder.WriteString("  AMQP_INSPECTOR_FILTER_EXCHANGE\n")
	builder.WriteString("  AMQP_INSPECTOR_FILTER_QUEUE\n")
	builder.WriteString("  AMQP_INSPECTOR_FILTER_ROUTING_KEY\n")
	builder.WriteString("  AMQP_INSPECTOR_FILTER_EVENT\n")
	builder.WriteString("  AMQP_INSPECTOR_MAX_EVENTS\n")
	builder.WriteString("  AMQP_INSPECTOR_PREFETCH\n")
	builder.WriteString("  AMQP_INSPECTOR_RECONNECT_INITIAL\n")
	builder.WriteString("  AMQP_INSPECTOR_RECONNECT_MAX\n")
	builder.WriteString("  AMQP_INSPECTOR_GRAPH_NAME\n")
	builder.WriteString("  AMQP_INSPECTOR_WARN_UNROUTED\n")
	builder.WriteString("  AMQP_INSPECTOR_SHOW_BODY_BYTES\n")
	builder.WriteString("  AMQP_INSPECTOR_METRICS_ADDR\n")
	builder.WriteString("  AMQP_INSPECTOR_STATSD_ADDR\n")
	builder.WriteString("  AMQP_INSPECTOR_WEB_ADDR\n")

	return builder.String()
}

func newFlagSet(cfg *Config, output io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet("amqp-routing-inspector", flag.ContinueOnError)
	if output == nil {
		output = io.Discard
	}
	fs.SetOutput(output)

	configPath := fs.String("config", cfg.ConfigPath, "Path to YAML configuration file")
	fs.StringVar(configPath, "c", cfg.ConfigPath, "Path to YAML configuration file")

	fs.BoolVar(&cfg.ShowVersion, "version", cfg.ShowVersion, "Print version and exit")
	fs.StringVar(&cfg.RabbitMQURL, "rabbitmq-url", cfg.RabbitMQURL, "AMQP URL, e.g. amqp://guest:guest@localhost:5672/")
	fs.StringVar(&cfg.FirehoseExchange, "firehose-exchange", cfg.FirehoseExchange, "RabbitMQ firehose exchange")
	fs.StringVar(&cfg.QueueName, "queue-name", cfg.QueueName, "Queue name used for consuming firehose (empty means auto-generated ephemeral queue)")
	fs.StringVar(&cfg.ConsumerTag, "consumer-tag", cfg.ConsumerTag, "Consumer tag")
	fs.StringVar(&cfg.OutputFormat, "output", cfg.OutputFormat, "Output format: cli|json|dot|summary|mermaid|prometheus|statsd|web")
	fs.StringVar(&cfg.FilterExchange, "filter-exchange", cfg.FilterExchange, "Only include events from this exchange (supports glob: orders.*, ?rders)")
	fs.StringVar(&cfg.FilterQueue, "filter-queue", cfg.FilterQueue, "Only include events routed to this queue (supports glob)")
	fs.StringVar(&cfg.FilterRoutingKey, "filter-routing-key", cfg.FilterRoutingKey, "Only include events with this routing key (supports AMQP wildcards: * and #)")
	fs.StringVar(&cfg.FilterEvent, "filter-event", cfg.FilterEvent, "Only include events of this type: publish|deliver")
	fs.IntVar(&cfg.MaxEvents, "max-events", cfg.MaxEvents, "Stop after N matched events (0 means unlimited)")
	fs.IntVar(&cfg.Prefetch, "prefetch", cfg.Prefetch, "AMQP prefetch count")
	fs.DurationVar(&cfg.ReconnectInitial, "reconnect-initial", cfg.ReconnectInitial, "Initial reconnect backoff")
	fs.DurationVar(&cfg.ReconnectMax, "reconnect-max", cfg.ReconnectMax, "Maximum reconnect backoff")
	fs.StringVar(&cfg.GraphName, "graph-name", cfg.GraphName, "Graph name used for DOT/Mermaid output")
	fs.BoolVar(&cfg.WarnUnrouted, "warn-unrouted", cfg.WarnUnrouted, "Log a warning for publish events with no queue destinations")
	fs.IntVar(&cfg.ShowBodyBytes, "show-body-bytes", cfg.ShowBodyBytes, "Include first N bytes of message body in output (0 disables)")
	fs.StringVar(&cfg.MetricsAddr, "metrics-addr", cfg.MetricsAddr, "HTTP address for Prometheus metrics endpoint (required for --output prometheus)")
	fs.StringVar(&cfg.StatsdAddr, "statsd-addr", cfg.StatsdAddr, "UDP address for StatsD metrics (required for --output statsd)")
	fs.StringVar(&cfg.WebAddr, "web-addr", cfg.WebAddr, "HTTP address for live routing graph web UI (required for --output web)")

	return fs, configPath
}

func loadFromFile(path string, cfg *Config) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %q: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("decode config file %q: %w", path, err)
	}

	return nil
}

func scanConfigPath(args []string) (string, error) {
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		switch {
		case a == "-config" || a == "--config" || a == "-c":
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", a)
			}
			return strings.TrimSpace(args[i+1]), nil
		case strings.HasPrefix(a, "-config="):
			return strings.TrimSpace(strings.TrimPrefix(a, "-config=")), nil
		case strings.HasPrefix(a, "--config="):
			return strings.TrimSpace(strings.TrimPrefix(a, "--config=")), nil
		case strings.HasPrefix(a, "-c="):
			return strings.TrimSpace(strings.TrimPrefix(a, "-c=")), nil
		}
	}
	return "", nil
}

func applyEnv(cfg *Config) error {
	applyEnvString("AMQP_INSPECTOR_RABBITMQ_URL", &cfg.RabbitMQURL)
	applyEnvString("AMQP_INSPECTOR_FIREHOSE_EXCHANGE", &cfg.FirehoseExchange)
	applyEnvString("AMQP_INSPECTOR_QUEUE_NAME", &cfg.QueueName)
	applyEnvString("AMQP_INSPECTOR_CONSUMER_TAG", &cfg.ConsumerTag)
	applyEnvString("AMQP_INSPECTOR_OUTPUT_FORMAT", &cfg.OutputFormat)
	applyEnvString("AMQP_INSPECTOR_FILTER_EXCHANGE", &cfg.FilterExchange)
	applyEnvString("AMQP_INSPECTOR_FILTER_QUEUE", &cfg.FilterQueue)
	applyEnvString("AMQP_INSPECTOR_FILTER_ROUTING_KEY", &cfg.FilterRoutingKey)
	applyEnvString("AMQP_INSPECTOR_FILTER_EVENT", &cfg.FilterEvent)
	applyEnvString("AMQP_INSPECTOR_GRAPH_NAME", &cfg.GraphName)
	applyEnvString("AMQP_INSPECTOR_METRICS_ADDR", &cfg.MetricsAddr)
	applyEnvString("AMQP_INSPECTOR_STATSD_ADDR", &cfg.StatsdAddr)
	applyEnvString("AMQP_INSPECTOR_WEB_ADDR", &cfg.WebAddr)

	if err := applyEnvInt("AMQP_INSPECTOR_MAX_EVENTS", &cfg.MaxEvents); err != nil {
		return err
	}
	if err := applyEnvInt("AMQP_INSPECTOR_PREFETCH", &cfg.Prefetch); err != nil {
		return err
	}
	if err := applyEnvInt("AMQP_INSPECTOR_SHOW_BODY_BYTES", &cfg.ShowBodyBytes); err != nil {
		return err
	}
	if err := applyEnvDuration("AMQP_INSPECTOR_RECONNECT_INITIAL", &cfg.ReconnectInitial); err != nil {
		return err
	}
	if err := applyEnvDuration("AMQP_INSPECTOR_RECONNECT_MAX", &cfg.ReconnectMax); err != nil {
		return err
	}
	if err := applyEnvBool("AMQP_INSPECTOR_WARN_UNROUTED", &cfg.WarnUnrouted); err != nil {
		return err
	}

	return nil
}

func applyEnvString(name string, out *string) {
	if value, ok := os.LookupEnv(name); ok {
		value = strings.TrimSpace(value)
		if value != "" {
			*out = value
		}
	}
}

func applyEnvInt(name string, out *int) error {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("invalid integer in %s: %w", name, err)
	}
	*out = parsed
	return nil
}

func applyEnvDuration(name string, out *time.Duration) error {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("invalid duration in %s: %w", name, err)
	}
	*out = parsed
	return nil
}

func applyEnvBool(name string, out *bool) error {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("invalid boolean in %s: %w", name, err)
	}
	*out = parsed
	return nil
}
