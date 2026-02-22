package config

import (
	"os"
	"strconv"
)

// Config holds all Lumber configuration.
type Config struct {
	Connector ConnectorConfig
	Engine    EngineConfig
	Output    OutputConfig
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
	Verbosity           string // "minimal", "standard", "full"
}

// OutputConfig holds output destination settings.
type OutputConfig struct {
	Format string // "stdout" for now
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		Connector: ConnectorConfig{
			Provider: getenv("LUMBER_CONNECTOR", "vercel"),
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
		},
		Output: OutputConfig{
			Format: getenv("LUMBER_OUTPUT", "stdout"),
		},
	}
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

func getenvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
