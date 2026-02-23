package compactor

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// --- truncate tests ---

func TestTruncateRuneSafety(t *testing.T) {
	// CJK characters are 3 bytes each in UTF-8.
	input := strings.Repeat("日本語", 100) // 300 runes, 900 bytes
	result := truncate(input, 10)

	if !utf8.ValidString(result) {
		t.Fatal("truncated string is not valid UTF-8")
	}
	// 10 runes + "..."
	if utf8.RuneCountInString(result) != 13 {
		t.Fatalf("expected 13 runes (10 + ...), got %d", utf8.RuneCountInString(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Fatal("expected ... suffix")
	}
}

func TestTruncateEmoji(t *testing.T) {
	// Emoji are 4 bytes each.
	input := strings.Repeat("🔥", 50) // 50 runes, 200 bytes
	result := truncate(input, 5)

	if !utf8.ValidString(result) {
		t.Fatal("truncated string is not valid UTF-8")
	}
	if utf8.RuneCountInString(result) != 8 { // 5 + "..."
		t.Fatalf("expected 8 runes, got %d", utf8.RuneCountInString(result))
	}
}

func TestTruncateASCII(t *testing.T) {
	input := "hello world this is a test"
	result := truncate(input, 11)
	if result != "hello world..." {
		t.Fatalf("expected 'hello world...', got %q", result)
	}
}

func TestTruncateShortInput(t *testing.T) {
	input := "short"
	result := truncate(input, 100)
	if result != input {
		t.Fatalf("expected unchanged input, got %q", result)
	}
}

func TestTruncateExactLength(t *testing.T) {
	input := "exact"
	result := truncate(input, 5)
	if result != input {
		t.Fatalf("expected unchanged input, got %q", result)
	}
}

// --- summarize tests ---

func TestSummarizeFirstLine(t *testing.T) {
	input := "ERROR: connection refused\n\tat com.example.Main.connect(Main.java:42)\n\tat com.example.Main.run(Main.java:10)"
	result := summarize(input)
	if result != "ERROR: connection refused" {
		t.Fatalf("expected first line, got %q", result)
	}
}

func TestSummarizeWordBoundary(t *testing.T) {
	// Build a line with words that exceeds 120 runes.
	words := make([]string, 30)
	for i := range words {
		words[i] = "word"
	}
	input := strings.Join(words, " ") // "word word word..." = 30*4 + 29 = 149 chars

	result := summarize(input)
	if !strings.HasSuffix(result, "...") {
		t.Fatal("expected ... suffix")
	}
	// Should be cut at a word boundary — no partial words.
	withoutSuffix := strings.TrimSuffix(result, "...")
	if strings.HasSuffix(withoutSuffix, " ") {
		t.Fatal("trailing space before ...")
	}
	if utf8.RuneCountInString(withoutSuffix) > 120 {
		t.Fatalf("result exceeds 120 runes before ...: %d", utf8.RuneCountInString(withoutSuffix))
	}
}

func TestSummarizeShortInput(t *testing.T) {
	input := "short log"
	result := summarize(input)
	if result != input {
		t.Fatalf("expected unchanged input, got %q", result)
	}
}

func TestSummarizeMultibyteFirstLine(t *testing.T) {
	input := "エラー: 接続が拒否されました\nスタックトレース"
	result := summarize(input)
	if result != "エラー: 接続が拒否されました" {
		t.Fatalf("expected first line, got %q", result)
	}
	if !utf8.ValidString(result) {
		t.Fatal("result is not valid UTF-8")
	}
}

// --- stack trace tests ---

func TestStackTraceJava(t *testing.T) {
	lines := []string{
		"java.lang.NullPointerException: null",
	}
	// Add 30 Java frames.
	for i := 0; i < 30; i++ {
		lines = append(lines, "\tat com.example.Class.method(Class.java:"+strings.Repeat("1", i%3+1)+")")
	}
	input := strings.Join(lines, "\n")

	result := truncateStackTrace(input, 5)

	resultLines := strings.Split(result, "\n")
	// Should have: 1 header + 5 first frames + 1 omission + 2 last frames = 9
	if len(resultLines) != 9 {
		t.Fatalf("expected 9 lines, got %d:\n%s", len(resultLines), result)
	}
	// Header preserved.
	if resultLines[0] != "java.lang.NullPointerException: null" {
		t.Fatalf("header not preserved: %q", resultLines[0])
	}
	// Omission message present.
	if !strings.Contains(result, "(23 frames omitted)") {
		t.Fatalf("expected omission message, got:\n%s", result)
	}
}

func TestStackTraceGo(t *testing.T) {
	lines := []string{
		"goroutine 1 [running]:",
		"main.main()",
	}
	for i := 0; i < 20; i++ {
		lines = append(lines, "\t/app/main.go:"+strings.Repeat("1", i%3+1))
	}
	input := strings.Join(lines, "\n")

	result := truncateStackTrace(input, 5)
	if !strings.Contains(result, "frames omitted") {
		t.Fatalf("expected omission message, got:\n%s", result)
	}
}

func TestStackTraceNone(t *testing.T) {
	input := "ERROR: connection refused to host=db-primary port=5432"
	result := truncateStackTrace(input, 5)
	if result != input {
		t.Fatalf("expected unchanged input for non-trace log, got %q", result)
	}
}

func TestStackTraceShort(t *testing.T) {
	// A trace with fewer frames than maxFrames + tail — should not be truncated.
	lines := []string{
		"java.lang.RuntimeException: fail",
		"\tat com.example.A.a(A.java:1)",
		"\tat com.example.B.b(B.java:2)",
		"\tat com.example.C.c(C.java:3)",
	}
	input := strings.Join(lines, "\n")
	result := truncateStackTrace(input, 5) // 3 frames < 5+2
	if result != input {
		t.Fatalf("expected unchanged short trace, got:\n%s", result)
	}
}

// --- stripFields tests ---

func TestStripFieldsJSON(t *testing.T) {
	input := `{"level":"error","msg":"timeout","trace_id":"abc123","request_id":"req456","service":"api"}`
	result := stripFields(input, defaultStripFields)

	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := m["trace_id"]; ok {
		t.Fatal("trace_id should have been stripped")
	}
	if _, ok := m["request_id"]; ok {
		t.Fatal("request_id should have been stripped")
	}
	if m["level"] != "error" {
		t.Fatal("non-stripped field should be preserved")
	}
	if m["msg"] != "timeout" {
		t.Fatal("non-stripped field should be preserved")
	}
	if m["service"] != "api" {
		t.Fatal("non-stripped field should be preserved")
	}
}

func TestStripFieldsNonJSON(t *testing.T) {
	input := "ERROR connection refused trace_id=abc123"
	result := stripFields(input, defaultStripFields)
	if result != input {
		t.Fatalf("expected unchanged non-JSON input, got %q", result)
	}
}

func TestStripFieldsFullVerbosity(t *testing.T) {
	input := `{"trace_id":"abc","msg":"test"}`
	cmp := New(Full)
	compacted, _ := cmp.Compact(input, "ERROR")
	// Full verbosity should preserve everything.
	var m map[string]any
	if err := json.Unmarshal([]byte(compacted), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := m["trace_id"]; !ok {
		t.Fatal("Full verbosity should preserve trace_id")
	}
}

func TestStripFieldsCustomList(t *testing.T) {
	input := `{"custom_field":"val","msg":"test","trace_id":"abc"}`
	cmp := New(Minimal, WithStripFields([]string{"custom_field"}))
	compacted, _ := cmp.Compact(input, "REQUEST")

	var m map[string]any
	if err := json.Unmarshal([]byte(compacted), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if _, ok := m["custom_field"]; ok {
		t.Fatal("custom_field should have been stripped")
	}
	// trace_id NOT in custom list, should be preserved.
	if _, ok := m["trace_id"]; !ok {
		t.Fatal("trace_id should be preserved with custom strip list")
	}
}

func TestStripFieldsNoMatch(t *testing.T) {
	input := `{"level":"info","msg":"ok"}`
	result := stripFields(input, defaultStripFields)
	// No fields to strip — should return original.
	if result != input {
		t.Fatalf("expected unchanged input when no fields match, got %q", result)
	}
}

// --- Compact integration tests ---

func TestCompactMinimal(t *testing.T) {
	cmp := New(Minimal)
	input := `{"level":"error","msg":"connection timeout","trace_id":"abc","span_id":"def","service":"api"}`
	compacted, summary := cmp.Compact(input, "ERROR")

	// trace_id and span_id should be stripped.
	if strings.Contains(compacted, "trace_id") {
		t.Fatal("trace_id should be stripped at Minimal")
	}
	if strings.Contains(compacted, "span_id") {
		t.Fatal("span_id should be stripped at Minimal")
	}
	// Should be valid UTF-8.
	if !utf8.ValidString(compacted) {
		t.Fatal("compacted is not valid UTF-8")
	}
	// Summary should be the first line.
	if summary == "" {
		t.Fatal("summary should not be empty")
	}
}

func TestCompactStandard(t *testing.T) {
	cmp := New(Standard)
	// A long log that should be truncated at 2000 runes.
	input := `{"level":"error","msg":"` + strings.Repeat("x", 3000) + `","trace_id":"abc"}`
	compacted, _ := cmp.Compact(input, "ERROR")

	if !strings.HasSuffix(compacted, "...") {
		t.Fatal("expected truncation at Standard for long log")
	}
	if utf8.RuneCountInString(compacted) > 2003 { // 2000 + "..."
		t.Fatalf("compacted too long: %d runes", utf8.RuneCountInString(compacted))
	}
}

func TestCompactFull(t *testing.T) {
	cmp := New(Full)
	input := `{"level":"error","msg":"timeout","trace_id":"abc123","span_id":"def456"}`
	compacted, _ := cmp.Compact(input, "ERROR")

	// Full preserves everything.
	if compacted != input {
		t.Fatalf("Full should preserve input unchanged, got %q", compacted)
	}
}
