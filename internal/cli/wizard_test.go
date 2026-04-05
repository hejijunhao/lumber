package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kaminocorp/lumber/internal/config"
)

func TestModelsReady_AllPresent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"model.onnx", "vocab.txt", "proj.safetensors"} {
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644)
	}
	// Also create a fake ORT library so the check passes.
	// On macOS this is libonnxruntime.dylib, on Linux libonnxruntime.so.
	ortName := "libonnxruntime.so"
	if ext := ortLibNameForTest(); ext != "" {
		ortName = ext
	}
	os.WriteFile(filepath.Join(dir, ortName), []byte("x"), 0o644)

	cfg := config.Config{
		Engine: config.EngineConfig{
			ModelPath:      filepath.Join(dir, "model.onnx"),
			VocabPath:      filepath.Join(dir, "vocab.txt"),
			ProjectionPath: filepath.Join(dir, "proj.safetensors"),
		},
	}

	if !ModelsReady(cfg) {
		t.Error("expected ModelsReady=true when all files present")
	}
}

func TestModelsReady_MissingModel(t *testing.T) {
	dir := t.TempDir()
	// Only create vocab and projection, not the model.
	os.WriteFile(filepath.Join(dir, "vocab.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "proj.safetensors"), []byte("x"), 0o644)

	cfg := config.Config{
		Engine: config.EngineConfig{
			ModelPath:      filepath.Join(dir, "model.onnx"),
			VocabPath:      filepath.Join(dir, "vocab.txt"),
			ProjectionPath: filepath.Join(dir, "proj.safetensors"),
		},
	}

	if ModelsReady(cfg) {
		t.Error("expected ModelsReady=false when model file missing")
	}
}

func TestModelsReady_AllMissing(t *testing.T) {
	cfg := config.Config{
		Engine: config.EngineConfig{
			ModelPath:      "/nonexistent/model.onnx",
			VocabPath:      "/nonexistent/vocab.txt",
			ProjectionPath: "/nonexistent/proj.safetensors",
		},
	}

	if ModelsReady(cfg) {
		t.Error("expected ModelsReady=false when all files missing")
	}
}

func TestBuildSummary_StdoutOnly(t *testing.T) {
	cfg := &config.Config{
		Connector: config.ConnectorConfig{Provider: "stdin"},
		Mode:      "stream",
		Engine:    config.EngineConfig{Verbosity: "standard"},
	}
	s := buildSummary(cfg)
	if s == "" {
		t.Fatal("expected non-empty summary")
	}
	assertContains(t, s, "stdin")
	assertContains(t, s, "stream")
	assertContains(t, s, "standard")
	assertContains(t, s, "stdout")
}

func TestBuildSummary_WithFileAndWebhook(t *testing.T) {
	cfg := &config.Config{
		Connector: config.ConnectorConfig{Provider: "vercel"},
		Mode:      "query",
		Engine:    config.EngineConfig{Verbosity: "minimal"},
		Output: config.OutputConfig{
			FilePath:   "out.ndjson",
			WebhookURL: "https://example.com/hook",
		},
	}
	s := buildSummary(cfg)
	assertContains(t, s, "vercel")
	assertContains(t, s, "query")
	assertContains(t, s, "minimal")
	assertContains(t, s, "out.ndjson")
	assertContains(t, s, "webhook")
}

func TestBuildSummary_FileConnector(t *testing.T) {
	cfg := &config.Config{
		Connector: config.ConnectorConfig{Provider: "file"},
		Mode:      "stream",
		Engine:    config.EngineConfig{Verbosity: "full"},
	}
	s := buildSummary(cfg)
	assertContains(t, s, "file")
	assertContains(t, s, "full")
}

// --- helpers ---

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected summary to contain %q, got:\n%s", substr, s)
	}
}

// ortLibNameForTest returns the ORT library filename for the current platform.
func ortLibNameForTest() string {
	// Import cycle prevention: we can't import internal/download here
	// (it would create cli -> download -> cli). Use runtime.GOOS directly.
	if runtime.GOOS == "darwin" {
		return "libonnxruntime.dylib"
	}
	return "libonnxruntime.so"
}
