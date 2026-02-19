package config

import "os"

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
}

// EngineConfig holds classification engine settings.
type EngineConfig struct {
	ModelPath           string
	VocabPath           string
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
		},
		Engine: EngineConfig{
			ModelPath:           getenv("LUMBER_MODEL_PATH", "models/model_quantized.onnx"),
			VocabPath:           getenv("LUMBER_VOCAB_PATH", "models/vocab.txt"),
			ConfidenceThreshold: 0.5,
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
