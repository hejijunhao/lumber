package compactor

import (
	"strings"
	"testing"
)

func TestEstimateTokensEmpty(t *testing.T) {
	if n := EstimateTokens(""); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestEstimateTokensSimple(t *testing.T) {
	n := EstimateTokens("hello world")
	// 2 words * 1.3 = 2.6 → ceil = 3
	if n != 3 {
		t.Fatalf("expected 3, got %d", n)
	}
}

func TestEstimateTokensSingleWord(t *testing.T) {
	n := EstimateTokens("hello")
	// 1 * 1.3 = 1.3 → ceil = 2
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

func TestEstimateTokensLong(t *testing.T) {
	words := make([]string, 500)
	for i := range words {
		words[i] = "word"
	}
	paragraph := strings.Join(words, " ")
	n := EstimateTokens(paragraph)
	// 500 * 1.3 = 650
	if n != 650 {
		t.Fatalf("expected 650, got %d", n)
	}
}

func TestEstimateTokensWhitespaceOnly(t *testing.T) {
	if n := EstimateTokens("   \t\n  "); n != 0 {
		t.Fatalf("expected 0 for whitespace-only, got %d", n)
	}
}
