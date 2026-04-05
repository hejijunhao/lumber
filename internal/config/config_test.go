package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear all connector-related env vars.
	for _, key := range []string{
		"LUMBER_CONNECTOR", "LUMBER_API_KEY", "LUMBER_ENDPOINT",
		"LUMBER_VERCEL_PROJECT_ID", "LUMBER_VERCEL_TEAM_ID",
		"LUMBER_FLY_APP_NAME", "LUMBER_SUPABASE_PROJECT_REF",
		"LUMBER_SUPABASE_TABLES", "LUMBER_POLL_INTERVAL",
		"LUMBER_FILE_PATH",
		"LUMBER_OUTPUT_PRETTY", "LUMBER_DEDUP_WINDOW",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Connector.Provider != "" {
		t.Fatalf("expected default provider '' (empty, triggers wizard), got %q", cfg.Connector.Provider)
	}
	if cfg.Connector.APIKey != "" {
		t.Fatalf("expected empty APIKey, got %q", cfg.Connector.APIKey)
	}
	if cfg.Connector.Extra != nil {
		t.Fatalf("expected nil Extra when no provider vars set, got %v", cfg.Connector.Extra)
	}
	if cfg.Output.Pretty {
		t.Fatal("expected default Pretty=false")
	}
	if cfg.Engine.DedupWindow != 5*time.Second {
		t.Fatalf("expected default DedupWindow=5s, got %v", cfg.Engine.DedupWindow)
	}
}

func TestLoad_ConnectorExtra(t *testing.T) {
	os.Setenv("LUMBER_VERCEL_PROJECT_ID", "proj_123")
	defer os.Unsetenv("LUMBER_VERCEL_PROJECT_ID")

	// Clear others to avoid interference.
	for _, key := range []string{
		"LUMBER_VERCEL_TEAM_ID", "LUMBER_FLY_APP_NAME",
		"LUMBER_SUPABASE_PROJECT_REF", "LUMBER_SUPABASE_TABLES",
		"LUMBER_POLL_INTERVAL", "LUMBER_FILE_PATH",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Connector.Extra == nil {
		t.Fatal("expected non-nil Extra")
	}
	if cfg.Connector.Extra["project_id"] != "proj_123" {
		t.Fatalf("expected project_id 'proj_123', got %q", cfg.Connector.Extra["project_id"])
	}
	if len(cfg.Connector.Extra) != 1 {
		t.Fatalf("expected 1 Extra entry, got %d: %v", len(cfg.Connector.Extra), cfg.Connector.Extra)
	}
}

func TestLoad_EmptyExtraOmitted(t *testing.T) {
	// Set to empty string — should not appear in Extra.
	os.Setenv("LUMBER_VERCEL_PROJECT_ID", "")
	os.Setenv("LUMBER_FLY_APP_NAME", "")
	defer os.Unsetenv("LUMBER_VERCEL_PROJECT_ID")
	defer os.Unsetenv("LUMBER_FLY_APP_NAME")

	for _, key := range []string{
		"LUMBER_VERCEL_TEAM_ID", "LUMBER_SUPABASE_PROJECT_REF",
		"LUMBER_SUPABASE_TABLES", "LUMBER_POLL_INTERVAL",
		"LUMBER_FILE_PATH",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Connector.Extra != nil {
		t.Fatalf("expected nil Extra when all vars are empty, got %v", cfg.Connector.Extra)
	}
}

func TestLoad_MultipleProviders(t *testing.T) {
	os.Setenv("LUMBER_VERCEL_PROJECT_ID", "proj_v")
	os.Setenv("LUMBER_VERCEL_TEAM_ID", "team_v")
	os.Setenv("LUMBER_FLY_APP_NAME", "my-fly-app")
	os.Setenv("LUMBER_SUPABASE_PROJECT_REF", "ref_s")
	os.Setenv("LUMBER_SUPABASE_TABLES", "edge_logs,auth_logs")
	os.Setenv("LUMBER_POLL_INTERVAL", "10s")
	os.Unsetenv("LUMBER_FILE_PATH") // clear to avoid interference
	defer func() {
		for _, key := range []string{
			"LUMBER_VERCEL_PROJECT_ID", "LUMBER_VERCEL_TEAM_ID",
			"LUMBER_FLY_APP_NAME", "LUMBER_SUPABASE_PROJECT_REF",
			"LUMBER_SUPABASE_TABLES", "LUMBER_POLL_INTERVAL",
		} {
			os.Unsetenv(key)
		}
	}()

	cfg := Load()

	expected := map[string]string{
		"project_id":  "proj_v",
		"team_id":     "team_v",
		"app_name":    "my-fly-app",
		"project_ref": "ref_s",
		"tables":      "edge_logs,auth_logs",
		"poll_interval": "10s",
	}

	if len(cfg.Connector.Extra) != len(expected) {
		t.Fatalf("expected %d Extra entries, got %d: %v", len(expected), len(cfg.Connector.Extra), cfg.Connector.Extra)
	}
	for k, want := range expected {
		if got := cfg.Connector.Extra[k]; got != want {
			t.Fatalf("Extra[%q]: expected %q, got %q", k, want, got)
		}
	}
}

func TestLoad_PrettyDefault(t *testing.T) {
	os.Unsetenv("LUMBER_OUTPUT_PRETTY")
	cfg := Load()
	if cfg.Output.Pretty {
		t.Fatal("expected default Pretty=false")
	}
}

func TestLoad_PrettyEnv(t *testing.T) {
	os.Setenv("LUMBER_OUTPUT_PRETTY", "true")
	defer os.Unsetenv("LUMBER_OUTPUT_PRETTY")

	cfg := Load()
	if !cfg.Output.Pretty {
		t.Fatal("expected Pretty=true when LUMBER_OUTPUT_PRETTY=true")
	}
}

func TestLoad_DedupWindowDefault(t *testing.T) {
	os.Unsetenv("LUMBER_DEDUP_WINDOW")
	cfg := Load()
	if cfg.Engine.DedupWindow != 5*time.Second {
		t.Fatalf("expected default DedupWindow=5s, got %v", cfg.Engine.DedupWindow)
	}
}

func TestLoad_DedupWindowEnv(t *testing.T) {
	os.Setenv("LUMBER_DEDUP_WINDOW", "10s")
	defer os.Unsetenv("LUMBER_DEDUP_WINDOW")

	cfg := Load()
	if cfg.Engine.DedupWindow != 10*time.Second {
		t.Fatalf("expected DedupWindow=10s, got %v", cfg.Engine.DedupWindow)
	}
}

func TestLoad_DedupWindowDisabled(t *testing.T) {
	os.Setenv("LUMBER_DEDUP_WINDOW", "0")
	defer os.Unsetenv("LUMBER_DEDUP_WINDOW")

	cfg := Load()
	if cfg.Engine.DedupWindow != 0 {
		t.Fatalf("expected DedupWindow=0 (disabled), got %v", cfg.Engine.DedupWindow)
	}
}

// --- Validation tests ---

// validConfig returns a Config with real temp files so file-existence checks pass.
func validConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"model.onnx", "vocab.txt", "proj.safetensors"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return Config{
		Mode:      "stream",
		LogLevel:  "info",
		Connector: ConnectorConfig{Provider: "vercel", APIKey: "tok_123"},
		Engine: EngineConfig{
			ModelPath:           filepath.Join(dir, "model.onnx"),
			VocabPath:           filepath.Join(dir, "vocab.txt"),
			ProjectionPath:      filepath.Join(dir, "proj.safetensors"),
			ConfidenceThreshold: 0.5,
			Verbosity:           "standard",
			DedupWindow:         5 * time.Second,
		},
		Output: OutputConfig{Format: "stdout"},
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validConfig(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil error for valid config, got: %v", err)
	}
}

func TestValidate_BadConfidenceThreshold(t *testing.T) {
	cfg := validConfig(t)
	cfg.Engine.ConfidenceThreshold = 1.5
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for confidence threshold 1.5")
	}
	if !strings.Contains(err.Error(), "confidence") {
		t.Fatalf("expected error to mention 'confidence', got: %v", err)
	}
}

func TestValidate_BadVerbosity(t *testing.T) {
	cfg := validConfig(t)
	cfg.Engine.Verbosity = "verbose"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid verbosity")
	}
	if !strings.Contains(err.Error(), "verbosity") {
		t.Fatalf("expected error to mention 'verbosity', got: %v", err)
	}
}

func TestValidate_NegativeDedupWindow(t *testing.T) {
	cfg := validConfig(t)
	cfg.Engine.DedupWindow = -1 * time.Second
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative dedup window")
	}
	if !strings.Contains(err.Error(), "dedup") {
		t.Fatalf("expected error to mention 'dedup', got: %v", err)
	}
}

func TestValidate_MissingModelFile(t *testing.T) {
	cfg := validConfig(t)
	cfg.Engine.ModelPath = "/nonexistent/model.onnx"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing model file")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected error to mention 'model', got: %v", err)
	}
}

func TestValidate_MissingAPIKey(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.APIKey = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing API key with cloud provider set")
	}
	if !strings.Contains(err.Error(), "LUMBER_API_KEY") {
		t.Fatalf("expected error to mention 'LUMBER_API_KEY', got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.APIKey = ""
	cfg.Engine.ConfidenceThreshold = -0.1
	cfg.Engine.Verbosity = "loud"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for multiple bad fields")
	}
	msg := err.Error()
	for _, want := range []string{"LUMBER_API_KEY", "confidence", "verbosity"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to mention %q, got: %v", want, msg)
		}
	}
}

// --- getenvInt tests ---

func TestGetenvInt(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		set      bool
		fallback int
		want     int
	}{
		{"empty uses fallback", "", false, 1000, 1000},
		{"valid int", "500", true, 1000, 500},
		{"zero", "0", true, 1000, 0},
		{"invalid falls back", "abc", true, 1000, 1000},
		{"negative", "-1", true, 1000, -1},
	}

	const key = "LUMBER_TEST_GETENVINT"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				os.Setenv(key, tt.envVal)
				defer os.Unsetenv(key)
			} else {
				os.Unsetenv(key)
			}
			got := getenvInt(key, tt.fallback)
			if got != tt.want {
				t.Errorf("getenvInt(%q, %d) = %d, want %d", tt.envVal, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestLoad_MaxBufferSizeDefault(t *testing.T) {
	os.Unsetenv("LUMBER_MAX_BUFFER_SIZE")
	cfg := Load()
	if cfg.Engine.MaxBufferSize != 1000 {
		t.Fatalf("expected default MaxBufferSize=1000, got %d", cfg.Engine.MaxBufferSize)
	}
}

func TestLoad_MaxBufferSizeEnv(t *testing.T) {
	os.Setenv("LUMBER_MAX_BUFFER_SIZE", "500")
	defer os.Unsetenv("LUMBER_MAX_BUFFER_SIZE")
	cfg := Load()
	if cfg.Engine.MaxBufferSize != 500 {
		t.Fatalf("expected MaxBufferSize=500, got %d", cfg.Engine.MaxBufferSize)
	}
}

// --- shutdown timeout tests ---

func TestLoad_ShutdownTimeoutDefault(t *testing.T) {
	os.Unsetenv("LUMBER_SHUTDOWN_TIMEOUT")
	cfg := Load()
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("expected default ShutdownTimeout=10s, got %v", cfg.ShutdownTimeout)
	}
}

func TestLoad_ShutdownTimeoutEnv(t *testing.T) {
	os.Setenv("LUMBER_SHUTDOWN_TIMEOUT", "5s")
	defer os.Unsetenv("LUMBER_SHUTDOWN_TIMEOUT")
	cfg := Load()
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Fatalf("expected ShutdownTimeout=5s, got %v", cfg.ShutdownTimeout)
	}
}

// --- mode tests ---

func TestLoad_ModeDefault(t *testing.T) {
	os.Unsetenv("LUMBER_MODE")
	cfg := Load()
	if cfg.Mode != "stream" {
		t.Fatalf("expected default Mode='stream', got %q", cfg.Mode)
	}
}

func TestLoad_ModeEnv(t *testing.T) {
	os.Setenv("LUMBER_MODE", "query")
	defer os.Unsetenv("LUMBER_MODE")
	cfg := Load()
	if cfg.Mode != "query" {
		t.Fatalf("expected Mode='query', got %q", cfg.Mode)
	}
}

func TestValidate_BadMode(t *testing.T) {
	cfg := validConfig(t)
	cfg.Mode = "replay"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Fatalf("expected error to mention 'mode', got: %v", err)
	}
}

// --- version tests ---

func TestLoad_ShowVersionDefault(t *testing.T) {
	cfg := Load()
	if cfg.ShowVersion {
		t.Fatal("expected default ShowVersion=false")
	}
}

func TestVersion_IsSet(t *testing.T) {
	if Version == "" {
		t.Fatal("expected non-empty Version constant")
	}
}

func TestValidate_StreamModeValid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Mode = "stream"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil error for mode='stream', got: %v", err)
	}
}

func TestValidate_QueryModeValid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Mode = "query"
	cfg.QueryFrom = time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)
	cfg.QueryTo = time.Date(2026, 2, 24, 1, 0, 0, 0, time.UTC)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil error for mode='query' with from/to set, got: %v", err)
	}
}

func TestValidate_QueryModeMissingFrom(t *testing.T) {
	cfg := validConfig(t)
	cfg.Mode = "query"
	cfg.QueryTo = time.Date(2026, 2, 24, 1, 0, 0, 0, time.UTC)
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for query mode without -from")
	}
	if !strings.Contains(err.Error(), "-from") {
		t.Fatalf("expected error to mention '-from', got: %v", err)
	}
}

func TestValidate_QueryModeMissingTo(t *testing.T) {
	cfg := validConfig(t)
	cfg.Mode = "query"
	cfg.QueryFrom = time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for query mode without -to")
	}
	if !strings.Contains(err.Error(), "-to") {
		t.Fatalf("expected error to mention '-to', got: %v", err)
	}
}

func TestValidate_QueryModeMissingBoth(t *testing.T) {
	cfg := validConfig(t)
	cfg.Mode = "query"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for query mode without -from and -to")
	}
	msg := err.Error()
	if !strings.Contains(msg, "-from") || !strings.Contains(msg, "-to") {
		t.Fatalf("expected error to mention both '-from' and '-to', got: %v", err)
	}
}

func TestValidate_QueryModeReversedRange(t *testing.T) {
	cfg := validConfig(t)
	cfg.Mode = "query"
	cfg.QueryFrom = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cfg.QueryTo = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) // before From
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for reversed query time range")
	}
	if !strings.Contains(err.Error(), "must be before") {
		t.Fatalf("expected error to mention ordering, got: %v", err)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := validConfig(t)
	cfg.LogLevel = "verbose"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("expected error to mention log level, got: %v", err)
	}
}

func TestValidate_ParseErrors(t *testing.T) {
	cfg := validConfig(t)
	cfg.parseErrors = []string{"-from: invalid RFC3339 time \"yesterday\""}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for parse errors")
	}
	if !strings.Contains(err.Error(), "invalid RFC3339") {
		t.Fatalf("expected error to mention parse error, got: %v", err)
	}
}

// --- output config tests ---

func TestValidate_WebhookURLInvalid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Output.WebhookURL = "not-a-url"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid webhook URL")
	}
	if !strings.Contains(err.Error(), "webhook URL") {
		t.Fatalf("expected error to mention 'webhook URL', got: %v", err)
	}
}

func TestValidate_WebhookURLValid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Output.WebhookURL = "https://hooks.example.com/lumber"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for valid webhook URL, got: %v", err)
	}
}

func TestValidate_FileOutputBadDir(t *testing.T) {
	cfg := validConfig(t)
	cfg.Output.FilePath = "/nonexistent/dir/output.jsonl"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for non-existent file output directory")
	}
	if !strings.Contains(err.Error(), "output file directory") {
		t.Fatalf("expected error to mention 'output file directory', got: %v", err)
	}
}

func TestValidate_FileOutputValid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Output.FilePath = filepath.Join(t.TempDir(), "output.jsonl")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for valid file output, got: %v", err)
	}
}

func TestLoad_OutputFileEnv(t *testing.T) {
	os.Setenv("LUMBER_OUTPUT_FILE", "/tmp/lumber.jsonl")
	defer os.Unsetenv("LUMBER_OUTPUT_FILE")

	cfg := Load()
	if cfg.Output.FilePath != "/tmp/lumber.jsonl" {
		t.Fatalf("expected FilePath=/tmp/lumber.jsonl, got %q", cfg.Output.FilePath)
	}
}

func TestLoad_WebhookURLEnv(t *testing.T) {
	os.Setenv("LUMBER_WEBHOOK_URL", "https://hooks.example.com")
	defer os.Unsetenv("LUMBER_WEBHOOK_URL")

	cfg := Load()
	if cfg.Output.WebhookURL != "https://hooks.example.com" {
		t.Fatalf("expected WebhookURL=https://hooks.example.com, got %q", cfg.Output.WebhookURL)
	}
}

// --- Section 5: local connector validation tests ---

func TestValidate_StdinSkipsAPIKey(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.Provider = "stdin"
	cfg.Connector.APIKey = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for stdin without API key, got: %v", err)
	}
}

func TestValidate_FileConnectorRequiresFilePath(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.Provider = "file"
	cfg.Connector.APIKey = ""
	cfg.Connector.Extra = map[string]string{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for file connector without file path")
	}
	if !strings.Contains(err.Error(), "file path is required") {
		t.Fatalf("expected error to mention 'file path is required', got: %v", err)
	}
}

func TestValidate_FileConnectorValidatesFileExists(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.Provider = "file"
	cfg.Connector.APIKey = ""
	cfg.Connector.Extra = map[string]string{"file": "/nonexistent/app.log"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for file connector with nonexistent file")
	}
	if !strings.Contains(err.Error(), "log file not found") {
		t.Fatalf("expected error to mention 'log file not found', got: %v", err)
	}
}

func TestValidate_FileConnectorValid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.Provider = "file"
	cfg.Connector.APIKey = ""

	// Create a temp log file.
	logFile := filepath.Join(t.TempDir(), "app.log")
	os.WriteFile(logFile, []byte("test log line"), 0o644)
	cfg.Connector.Extra = map[string]string{"file": logFile}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for valid file connector, got: %v", err)
	}
}

func TestValidate_CloudConnectorStillRequiresAPIKey(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.Provider = "vercel"
	cfg.Connector.APIKey = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for cloud connector without API key")
	}
	if !strings.Contains(err.Error(), "LUMBER_API_KEY") {
		t.Fatalf("expected error to mention 'LUMBER_API_KEY', got: %v", err)
	}
}

func TestValidate_EmptyProviderSkipsAPIKey(t *testing.T) {
	cfg := validConfig(t)
	cfg.Connector.Provider = ""
	cfg.Connector.APIKey = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for empty provider without API key, got: %v", err)
	}
}

func TestLoad_FilePathEnv(t *testing.T) {
	os.Setenv("LUMBER_FILE_PATH", "/var/log/app.log")
	defer os.Unsetenv("LUMBER_FILE_PATH")

	// Clear other extra vars to isolate.
	for _, key := range []string{
		"LUMBER_VERCEL_PROJECT_ID", "LUMBER_VERCEL_TEAM_ID",
		"LUMBER_FLY_APP_NAME", "LUMBER_SUPABASE_PROJECT_REF",
		"LUMBER_SUPABASE_TABLES", "LUMBER_POLL_INTERVAL",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()
	if cfg.Connector.Extra == nil {
		t.Fatal("expected non-nil Extra with LUMBER_FILE_PATH set")
	}
	if cfg.Connector.Extra["file"] != "/var/log/app.log" {
		t.Fatalf("expected Extra[\"file\"]=/var/log/app.log, got %q", cfg.Connector.Extra["file"])
	}
}
