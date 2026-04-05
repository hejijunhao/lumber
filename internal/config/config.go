package config

import (
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Version is the current Lumber release version.
// Set at build time via: go build -ldflags "-X github.com/kaminocorp/lumber/internal/config.Version=X.Y.Z"
var Version = "0.10.3"

// Config holds all Lumber configuration.
type Config struct {
	Connector       ConnectorConfig
	Engine          EngineConfig
	Output          OutputConfig
	LogLevel        string        // "debug", "info", "warn", "error"
	ShutdownTimeout time.Duration // max time to drain in-flight logs on shutdown
	Mode            string        // "stream" or "query"
	QueryFrom       time.Time     // query start time (RFC3339)
	QueryTo         time.Time     // query end time (RFC3339)
	QueryLimit      int           // max results; 0 = no limit
	ShowVersion     bool          // true when -version flag is set
	parseErrors     []string      // flag parse errors collected during LoadWithFlags
}

// ConnectorConfig holds connector-specific settings.
type ConnectorConfig struct {
	Provider string
	APIKey   string
	Endpoint string
	Extra    map[string]string
}

// EngineConfig holds classification engine settings.
type EngineConfig struct {
	ModelPath           string
	VocabPath           string
	ProjectionPath      string
	ConfidenceThreshold float64
	Verbosity           string        // "minimal", "standard", "full"
	DedupWindow         time.Duration // event dedup window; 0 disables
	MaxBufferSize       int           // max events buffered before force flush; 0 = unlimited
}

// OutputConfig holds output destination settings.
type OutputConfig struct {
	Format         string            // "stdout" for now
	Pretty         bool              // pretty-print JSON output
	FilePath       string            // NDJSON file output path; empty = disabled
	FileMaxSize    int64             // rotation size in bytes; 0 = no rotation
	WebhookURL     string            // POST endpoint; empty = disabled
	WebhookHeaders map[string]string // custom headers for webhook
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		LogLevel:        getenv("LUMBER_LOG_LEVEL", "info"),
		ShutdownTimeout: getenvDuration("LUMBER_SHUTDOWN_TIMEOUT", 10*time.Second),
		Mode:            getenv("LUMBER_MODE", "stream"),
		Connector: ConnectorConfig{
			Provider: getenv("LUMBER_CONNECTOR", ""),
			APIKey:   os.Getenv("LUMBER_API_KEY"),
			Endpoint: os.Getenv("LUMBER_ENDPOINT"),
			Extra:    loadConnectorExtra(),
		},
		Engine: EngineConfig{
			ModelPath:           getenv("LUMBER_MODEL_PATH", "models/model_quantized.onnx"),
			VocabPath:           getenv("LUMBER_VOCAB_PATH", "models/vocab.txt"),
			ProjectionPath:      getenv("LUMBER_PROJECTION_PATH", "models/2_Dense/model.safetensors"),
			ConfidenceThreshold: getenvFloat("LUMBER_CONFIDENCE_THRESHOLD", 0.5),
			Verbosity:           getenv("LUMBER_VERBOSITY", "standard"),
			DedupWindow:         getenvDuration("LUMBER_DEDUP_WINDOW", 5*time.Second),
			MaxBufferSize:       getenvInt("LUMBER_MAX_BUFFER_SIZE", 1000),
		},
		Output: OutputConfig{
			Format:      getenv("LUMBER_OUTPUT", "stdout"),
			Pretty:      getenvBool("LUMBER_OUTPUT_PRETTY", false),
			FilePath:    os.Getenv("LUMBER_OUTPUT_FILE"),
			FileMaxSize: int64(getenvInt("LUMBER_OUTPUT_FILE_MAX_SIZE", 0)),
			WebhookURL:  os.Getenv("LUMBER_WEBHOOK_URL"),
		},
	}
}

// LoadWithFlags loads config from env vars, then overlays CLI flags.
// Only explicitly-set flags override env var values.
func LoadWithFlags() Config {
	cfg := Load()

	showVersion := flag.Bool("version", false, "Print version and exit")
	mode := flag.String("mode", "", "Pipeline mode: stream or query")
	connFlag := flag.String("connector", "", "Connector: vercel, flyio, supabase, stdin, file")
	fileInput := flag.String("file", "", "Log file path (for file connector)")
	from := flag.String("from", "", "Query start time (RFC3339)")
	to := flag.String("to", "", "Query end time (RFC3339)")
	limit := flag.Int("limit", 0, "Query result limit")
	verbosity := flag.String("verbosity", "", "Verbosity: minimal, standard, full")
	pretty := flag.Bool("pretty", false, "Pretty-print JSON output")
	logLevel := flag.String("log-level", "", "Log level: debug, info, warn, error")
	outputFile := flag.String("output-file", "", "File path for NDJSON output")
	webhookURL := flag.String("webhook-url", "", "Webhook POST endpoint")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `lumber %s — log normalization pipeline

Usage:
  lumber                                Interactive setup wizard
  lumber -connector stdin               Classify piped logs
  lumber -connector file -file PATH     Classify a log file
  lumber -connector vercel              Stream from Vercel (requires LUMBER_API_KEY)
  cat app.log | lumber                  Auto-detect piped input

Flags:
`, Version)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environment variables:
  LUMBER_CONNECTOR      Log provider (vercel, flyio, supabase, stdin, file)
  LUMBER_API_KEY        Provider API key/token (cloud connectors only)
  LUMBER_FILE_PATH      Log file path (file connector)
  LUMBER_VERBOSITY      Output verbosity (minimal, standard, full)
  LUMBER_DEDUP_WINDOW   Dedup window duration (e.g. 5s, 0 to disable)
  LUMBER_LOG_LEVEL      Internal log level (debug, info, warn, error)

  See README for full configuration reference.
`)
	}

	flag.Parse()

	cfg.ShowVersion = *showVersion

	// Override only explicitly-set flags.
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "mode":
			cfg.Mode = *mode
		case "connector":
			cfg.Connector.Provider = *connFlag
		case "file":
			if cfg.Connector.Extra == nil {
				cfg.Connector.Extra = make(map[string]string)
			}
			cfg.Connector.Extra["file"] = *fileInput
		case "verbosity":
			cfg.Engine.Verbosity = *verbosity
		case "pretty":
			cfg.Output.Pretty = *pretty
		case "log-level":
			cfg.LogLevel = *logLevel
		case "from":
			if t, err := time.Parse(time.RFC3339, *from); err == nil {
				cfg.QueryFrom = t
			} else {
				cfg.parseErrors = append(cfg.parseErrors, fmt.Sprintf("-from: invalid RFC3339 time %q", *from))
			}
		case "to":
			if t, err := time.Parse(time.RFC3339, *to); err == nil {
				cfg.QueryTo = t
			} else {
				cfg.parseErrors = append(cfg.parseErrors, fmt.Sprintf("-to: invalid RFC3339 time %q", *to))
			}
		case "limit":
			cfg.QueryLimit = *limit
		case "output-file":
			cfg.Output.FilePath = *outputFile
		case "webhook-url":
			cfg.Output.WebhookURL = *webhookURL
		}
	})

	return cfg
}

// Validate checks the configuration for errors. Returns all errors found, not just the first.
func (c Config) Validate() error {
	var errs []string

	// API key required for cloud connectors only.
	localConnectors := map[string]bool{"stdin": true, "file": true, "": true}
	if c.Connector.Provider != "" && c.Connector.APIKey == "" && !localConnectors[c.Connector.Provider] {
		errs = append(errs, "LUMBER_API_KEY is required for cloud connectors")
	}

	// File connector requires a valid file path.
	if c.Connector.Provider == "file" {
		filePath := c.Connector.Extra["file"]
		if filePath == "" {
			errs = append(errs, "file path is required for file connector (-file flag or LUMBER_FILE_PATH)")
		} else if _, err := os.Stat(filePath); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("log file not found: %s", filePath))
		}
	}

	// Model files must exist on disk.
	for _, f := range []struct{ name, path string }{
		{"model", c.Engine.ModelPath},
		{"vocab", c.Engine.VocabPath},
		{"projection", c.Engine.ProjectionPath},
	} {
		if _, err := os.Stat(f.path); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("%s file not found: %s", f.name, f.path))
		}
	}

	// Confidence threshold in [0, 1].
	if c.Engine.ConfidenceThreshold < 0 || c.Engine.ConfidenceThreshold > 1 {
		errs = append(errs, fmt.Sprintf("confidence threshold must be 0-1, got %f", c.Engine.ConfidenceThreshold))
	}

	// Verbosity enum.
	switch c.Engine.Verbosity {
	case "minimal", "standard", "full":
	default:
		errs = append(errs, fmt.Sprintf("invalid verbosity %q (must be minimal|standard|full)", c.Engine.Verbosity))
	}

	// Log level enum.
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Sprintf("invalid log level %q (must be debug|info|warn|error)", c.LogLevel))
	}

	// Dedup window non-negative.
	if c.Engine.DedupWindow < 0 {
		errs = append(errs, fmt.Sprintf("dedup window must be non-negative, got %s", c.Engine.DedupWindow))
	}

	// Mode enum.
	switch c.Mode {
	case "stream", "query":
	default:
		errs = append(errs, fmt.Sprintf("invalid mode %q (must be stream or query)", c.Mode))
	}

	// Flag parse errors from LoadWithFlags.
	errs = append(errs, c.parseErrors...)

	// Query mode requires from/to time range.
	if c.Mode == "query" {
		if c.QueryFrom.IsZero() {
			errs = append(errs, "-from is required in query mode (RFC3339 format, e.g. 2026-02-24T00:00:00Z)")
		}
		if c.QueryTo.IsZero() {
			errs = append(errs, "-to is required in query mode (RFC3339 format, e.g. 2026-02-24T01:00:00Z)")
		}
		if !c.QueryFrom.IsZero() && !c.QueryTo.IsZero() && !c.QueryFrom.Before(c.QueryTo) {
			errs = append(errs, fmt.Sprintf("-from (%s) must be before -to (%s)", c.QueryFrom.Format(time.RFC3339), c.QueryTo.Format(time.RFC3339)))
		}
	}

	// Webhook URL must be a valid HTTP(S) URL with a host.
	if c.Output.WebhookURL != "" {
		u, err := url.ParseRequestURI(c.Output.WebhookURL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid webhook URL %q: %s", c.Output.WebhookURL, err))
		} else if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Sprintf("invalid webhook URL %q (must be a valid http:// or https:// URL with a host)", c.Output.WebhookURL))
		}
	}

	// File output parent directory must exist if set.
	if c.Output.FilePath != "" {
		dir := filepath.Dir(c.Output.FilePath)
		if dir != "." && dir != "" {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("output file directory does not exist: %s", dir))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadConnectorExtra reads provider-specific env vars into an Extra map.
func loadConnectorExtra() map[string]string {
	vars := []struct {
		envVar   string
		extraKey string
	}{
		{"LUMBER_VERCEL_PROJECT_ID", "project_id"},
		{"LUMBER_VERCEL_TEAM_ID", "team_id"},
		{"LUMBER_FLY_APP_NAME", "app_name"},
		{"LUMBER_SUPABASE_PROJECT_REF", "project_ref"},
		{"LUMBER_SUPABASE_TABLES", "tables"},
		{"LUMBER_POLL_INTERVAL", "poll_interval"},
		{"LUMBER_FILE_PATH", "file"},
	}

	var m map[string]string
	for _, v := range vars {
		if val := os.Getenv(v.envVar); val != "" {
			if m == nil {
				m = make(map[string]string)
			}
			m[v.extraKey] = val
		}
	}
	return m
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return strings.EqualFold(v, "true") || v == "1"
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if v == "0" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	return d
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	return n
}

func getenvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		slog.Warn("invalid env var value, using default", "key", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	return f
}
