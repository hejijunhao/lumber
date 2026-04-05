package lumber

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests verify the thin wrappers in pkg/lumber delegate correctly
// to internal/download. Core download logic is tested exhaustively in
// internal/download/download_test.go.

func TestDefaultCacheDir_Wrapper(t *testing.T) {
	t.Setenv("LUMBER_CACHE_DIR", "/tmp/lumber-wrapper-test")
	dir, err := defaultCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/tmp/lumber-wrapper-test" {
		t.Errorf("expected /tmp/lumber-wrapper-test, got %s", dir)
	}
}

func TestFileValid_Wrapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	h := sha256.Sum256([]byte("hello"))
	goodHash := hex.EncodeToString(h[:])
	if !fileValid(path, goodHash) {
		t.Error("expected true for matching checksum via wrapper")
	}
}

func TestDownloadFile_Wrapper(t *testing.T) {
	content := []byte("wrapper test")
	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "model.bin")

	if err := downloadFile(srv.URL+"/model.bin", dest, hash); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q", got)
	}
}

func TestOrtPlatform_Wrapper(t *testing.T) {
	_, _, err := ortPlatform()
	switch runtime.GOOS + "-" + runtime.GOARCH {
	case "linux-amd64", "linux-arm64", "darwin-arm64":
		if err != nil {
			t.Errorf("unexpected error on supported platform: %v", err)
		}
	default:
		if err == nil {
			t.Error("expected error for unsupported platform")
		}
	}
}

func TestAtomicWriteFromReader_Wrapper(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	r := strings.NewReader("atomic wrapper test")
	if err := atomicWriteFromReader(dest, r); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "atomic wrapper test" {
		t.Errorf("got %q", got)
	}
}
