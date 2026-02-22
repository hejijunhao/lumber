package testdata

import (
	"testing"
)

func TestLoadCorpus(t *testing.T) {
	entries, err := LoadCorpus()
	if err != nil {
		t.Fatalf("LoadCorpus() error: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("corpus is empty")
	}
	t.Logf("Total entries: %d", len(entries))

	// Every entry must have all required fields.
	for i, e := range entries {
		if e.Raw == "" {
			t.Errorf("entry[%d] has empty raw", i)
		}
		if e.ExpectedType == "" {
			t.Errorf("entry[%d] has empty expected_type", i)
		}
		if e.ExpectedCategory == "" {
			t.Errorf("entry[%d] has empty expected_category", i)
		}
		if e.ExpectedSeverity == "" {
			t.Errorf("entry[%d] has empty expected_severity", i)
		}
	}
}

func TestCorpusCoverage(t *testing.T) {
	entries, err := LoadCorpus()
	if err != nil {
		t.Fatalf("LoadCorpus() error: %v", err)
	}

	// All 42 taxonomy leaves.
	allLeaves := map[string]bool{
		"ERROR.connection_failure": false, "ERROR.auth_failure": false, "ERROR.authorization_failure": false,
		"ERROR.timeout": false, "ERROR.runtime_exception": false, "ERROR.validation_error": false,
		"ERROR.out_of_memory": false, "ERROR.rate_limited": false, "ERROR.dependency_error": false,
		"REQUEST.success": false, "REQUEST.client_error": false, "REQUEST.server_error": false,
		"REQUEST.redirect": false, "REQUEST.slow_request": false,
		"DEPLOY.build_started": false, "DEPLOY.build_succeeded": false, "DEPLOY.build_failed": false,
		"DEPLOY.deploy_started": false, "DEPLOY.deploy_succeeded": false, "DEPLOY.deploy_failed": false,
		"DEPLOY.rollback": false,
		"SYSTEM.health_check": false, "SYSTEM.scaling_event": false, "SYSTEM.resource_alert": false,
		"SYSTEM.process_lifecycle": false, "SYSTEM.config_change": false,
		"ACCESS.login_success": false, "ACCESS.login_failure": false, "ACCESS.session_expired": false,
		"ACCESS.permission_change": false, "ACCESS.api_key_event": false,
		"PERFORMANCE.latency_spike": false, "PERFORMANCE.throughput_drop": false, "PERFORMANCE.queue_backlog": false,
		"PERFORMANCE.cache_event": false, "PERFORMANCE.db_slow_query": false,
		"DATA.query_executed": false, "DATA.migration": false, "DATA.replication": false,
		"SCHEDULED.cron_started": false, "SCHEDULED.cron_completed": false, "SCHEDULED.cron_failed": false,
	}

	catCounts := map[string]int{}
	for _, e := range entries {
		key := e.ExpectedType + "." + e.ExpectedCategory
		catCounts[key]++
		allLeaves[key] = true
	}

	// Check coverage.
	for leaf, covered := range allLeaves {
		if !covered {
			t.Errorf("taxonomy leaf %q has no corpus entries", leaf)
		}
	}

	// Check minimum 2 entries per leaf.
	for leaf, count := range catCounts {
		if count < 2 {
			t.Errorf("leaf %q has only %d entry (want >= 2)", leaf, count)
		}
	}

	t.Logf("Coverage: %d leaves, %d total entries", len(allLeaves), len(entries))
}

func TestCorpusSeverityValues(t *testing.T) {
	entries, err := LoadCorpus()
	if err != nil {
		t.Fatalf("LoadCorpus() error: %v", err)
	}

	valid := map[string]bool{"error": true, "warning": true, "info": true, "debug": true}
	for i, e := range entries {
		if !valid[e.ExpectedSeverity] {
			t.Errorf("entry[%d] (%s) has invalid severity %q", i, e.Description, e.ExpectedSeverity)
		}
	}
}
