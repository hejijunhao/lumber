package lumber

import (
	"os"
	"sync"
	"testing"
	"time"
)

const testModelDir = "../../models"

func skipWithoutModel(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(testModelDir + "/model_quantized.onnx"); os.IsNotExist(err) {
		t.Skip("ONNX model not available, skipping integration test")
	}
}

func TestNewWithModelDir(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()
}

func TestNewBadPathReturnsError(t *testing.T) {
	_, err := New(WithModelDir("/nonexistent/path"))
	if err == nil {
		t.Fatal("expected error for bad model path, got nil")
	}
}

func TestClassifyKnownLogLine(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	event, err := l.Classify("ERROR [2026-02-28] UserService — connection refused (host=db-primary, port=5432)")
	if err != nil {
		t.Fatalf("Classify() error: %v", err)
	}

	if event.Type != "ERROR" {
		t.Errorf("Type = %q, want ERROR", event.Type)
	}
	if event.Category != "connection_failure" {
		t.Errorf("Category = %q, want connection_failure", event.Category)
	}
	if event.Severity != "error" {
		t.Errorf("Severity = %q, want error", event.Severity)
	}
	if event.Confidence <= 0 {
		t.Errorf("Confidence = %f, want > 0", event.Confidence)
	}
	if event.Summary == "" {
		t.Error("Summary is empty")
	}
}

func TestClassifyBatchMatchesIndividual(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	texts := []string{
		"ERROR: connection refused to db-primary:5432",
		"GET /api/users 200 OK 12ms",
		"Build succeeded in 45s",
	}

	batch, err := l.ClassifyBatch(texts)
	if err != nil {
		t.Fatalf("ClassifyBatch() error: %v", err)
	}

	if len(batch) != len(texts) {
		t.Fatalf("ClassifyBatch returned %d events, want %d", len(batch), len(texts))
	}

	for i, text := range texts {
		individual, err := l.Classify(text)
		if err != nil {
			t.Fatalf("Classify(%d) error: %v", i, err)
		}
		if batch[i].Type != individual.Type || batch[i].Category != individual.Category {
			t.Errorf("text[%d]: batch=%s.%s individual=%s.%s",
				i, batch[i].Type, batch[i].Category, individual.Type, individual.Category)
		}
	}
}

func TestClassifyEmptyInput(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	event, err := l.Classify("")
	if err != nil {
		t.Fatalf("Classify() error: %v", err)
	}

	if event.Type != "UNCLASSIFIED" {
		t.Errorf("Type = %q, want UNCLASSIFIED", event.Type)
	}
}

func TestClassifyWhitespaceInput(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	event, err := l.Classify("   \t\n  ")
	if err != nil {
		t.Fatalf("Classify() error: %v", err)
	}

	if event.Type != "UNCLASSIFIED" {
		t.Errorf("Type = %q, want UNCLASSIFIED", event.Type)
	}
}

func TestConcurrentClassify(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := l.Classify("ERROR: connection timeout after 30s")
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Classify() error: %v", err)
	}
}

func TestOptionsDefaults(t *testing.T) {
	o := defaultOptions()
	if o.confidenceThreshold != 0.5 {
		t.Errorf("default confidence threshold = %f, want 0.5", o.confidenceThreshold)
	}
	if o.verbosity != "standard" {
		t.Errorf("default verbosity = %q, want standard", o.verbosity)
	}
}

func TestResolvePathsExplicit(t *testing.T) {
	o := options{
		modelPath:      "/a/model.onnx",
		vocabPath:      "/a/vocab.txt",
		projectionPath: "/a/proj.safetensors",
	}
	m, v, p := resolvePaths(o)
	if m != "/a/model.onnx" || v != "/a/vocab.txt" || p != "/a/proj.safetensors" {
		t.Errorf("explicit paths not preserved: got %s, %s, %s", m, v, p)
	}
}

func TestResolvePathsFromDir(t *testing.T) {
	o := options{modelDir: "/data/models"}
	m, v, p := resolvePaths(o)
	if m != "/data/models/model_quantized.onnx" {
		t.Errorf("model path = %q", m)
	}
	if v != "/data/models/vocab.txt" {
		t.Errorf("vocab path = %q", v)
	}
	if p != "/data/models/2_Dense/model.safetensors" {
		t.Errorf("projection path = %q", p)
	}
}

func TestResolvePathsDefaultDir(t *testing.T) {
	o := options{}
	m, _, _ := resolvePaths(o)
	if m != "models/model_quantized.onnx" {
		t.Errorf("default model path = %q, want models/model_quantized.onnx", m)
	}
}

func TestClassifyLogPreservesTimestamp(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	event, err := l.ClassifyLog(Log{
		Text:      "ERROR: connection refused to db-primary:5432",
		Timestamp: ts,
		Source:    "vercel",
	})
	if err != nil {
		t.Fatalf("ClassifyLog() error: %v", err)
	}

	if !event.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", event.Timestamp, ts)
	}
	if event.Type != "ERROR" {
		t.Errorf("Type = %q, want ERROR", event.Type)
	}
}

func TestClassifyLogZeroTimestampDefaultsToNow(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	before := time.Now()
	event, err := l.ClassifyLog(Log{
		Text: "GET /api/users 200 OK",
	})
	after := time.Now()
	if err != nil {
		t.Fatalf("ClassifyLog() error: %v", err)
	}

	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Errorf("zero Timestamp not defaulted to now: got %v, expected between %v and %v",
			event.Timestamp, before, after)
	}
}

func TestClassifyLogs(t *testing.T) {
	skipWithoutModel(t)

	l, err := New(WithModelDir(testModelDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer l.Close()

	logs := []Log{
		{Text: "ERROR: connection refused", Source: "vercel"},
		{Text: "GET /health 200 OK", Source: "flyio"},
	}

	events, err := l.ClassifyLogs(logs)
	if err != nil {
		t.Fatalf("ClassifyLogs() error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Type != "ERROR" {
		t.Errorf("events[0].Type = %q, want ERROR", events[0].Type)
	}
}
