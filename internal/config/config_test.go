package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear all connector-related env vars.
	for _, key := range []string{
		"LUMBER_CONNECTOR", "LUMBER_API_KEY", "LUMBER_ENDPOINT",
		"LUMBER_VERCEL_PROJECT_ID", "LUMBER_VERCEL_TEAM_ID",
		"LUMBER_FLY_APP_NAME", "LUMBER_SUPABASE_PROJECT_REF",
		"LUMBER_SUPABASE_TABLES", "LUMBER_POLL_INTERVAL",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Connector.Provider != "vercel" {
		t.Fatalf("expected default provider 'vercel', got %q", cfg.Connector.Provider)
	}
	if cfg.Connector.APIKey != "" {
		t.Fatalf("expected empty APIKey, got %q", cfg.Connector.APIKey)
	}
	if cfg.Connector.Extra != nil {
		t.Fatalf("expected nil Extra when no provider vars set, got %v", cfg.Connector.Extra)
	}
}

func TestLoad_ConnectorExtra(t *testing.T) {
	os.Setenv("LUMBER_VERCEL_PROJECT_ID", "proj_123")
	defer os.Unsetenv("LUMBER_VERCEL_PROJECT_ID")

	// Clear others to avoid interference.
	for _, key := range []string{
		"LUMBER_VERCEL_TEAM_ID", "LUMBER_FLY_APP_NAME",
		"LUMBER_SUPABASE_PROJECT_REF", "LUMBER_SUPABASE_TABLES",
		"LUMBER_POLL_INTERVAL",
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
