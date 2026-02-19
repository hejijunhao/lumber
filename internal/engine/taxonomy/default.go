package taxonomy

import "github.com/crimson-sun/lumber/internal/model"

// DefaultRoots returns the built-in taxonomy tree that ships with Lumber.
// These labels will be expanded as the taxonomy matures.
func DefaultRoots() []*model.TaxonomyNode {
	return []*model.TaxonomyNode{
		{
			Name: "ERROR",
			Desc: "Application errors, exceptions, and failures",
			Children: []*model.TaxonomyNode{
				{Name: "runtime_exception", Desc: "Unhandled runtime exception or panic", Severity: "error"},
				{Name: "connection_failure", Desc: "Network or database connection failure", Severity: "error"},
				{Name: "timeout", Desc: "Request or operation timeout", Severity: "error"},
				{Name: "auth_failure", Desc: "Authentication or authorization failure", Severity: "error"},
				{Name: "validation_error", Desc: "Input validation or schema error", Severity: "warning"},
			},
		},
		{
			Name: "REQUEST",
			Desc: "HTTP requests and API calls",
			Children: []*model.TaxonomyNode{
				{Name: "incoming_request", Desc: "Incoming HTTP request log", Severity: "info"},
				{Name: "outgoing_request", Desc: "Outgoing HTTP or API call", Severity: "info"},
				{Name: "response", Desc: "HTTP response with status code", Severity: "info"},
			},
		},
		{
			Name: "DEPLOY",
			Desc: "Deployment and build events",
			Children: []*model.TaxonomyNode{
				{Name: "build_started", Desc: "Build process started", Severity: "info"},
				{Name: "build_succeeded", Desc: "Build completed successfully", Severity: "info"},
				{Name: "build_failed", Desc: "Build failed with errors", Severity: "error"},
				{Name: "deploy_started", Desc: "Deployment initiated", Severity: "info"},
				{Name: "deploy_succeeded", Desc: "Deployment completed successfully", Severity: "info"},
				{Name: "deploy_failed", Desc: "Deployment failed", Severity: "error"},
			},
		},
		{
			Name: "SYSTEM",
			Desc: "Infrastructure and system-level events",
			Children: []*model.TaxonomyNode{
				{Name: "startup", Desc: "Service or process startup", Severity: "info"},
				{Name: "shutdown", Desc: "Service or process shutdown", Severity: "info"},
				{Name: "health_check", Desc: "Health check or liveness probe", Severity: "info"},
				{Name: "resource_limit", Desc: "Memory, CPU, or disk limit reached", Severity: "warning"},
				{Name: "scaling", Desc: "Auto-scaling event", Severity: "info"},
			},
		},
		{
			Name: "SECURITY",
			Desc: "Security-related events",
			Children: []*model.TaxonomyNode{
				{Name: "login_success", Desc: "Successful user login", Severity: "info"},
				{Name: "login_failure", Desc: "Failed login attempt", Severity: "warning"},
				{Name: "rate_limited", Desc: "Request rate limited or throttled", Severity: "warning"},
				{Name: "suspicious_activity", Desc: "Suspicious or anomalous activity detected", Severity: "warning"},
			},
		},
		{
			Name: "DATA",
			Desc: "Database and data operations",
			Children: []*model.TaxonomyNode{
				{Name: "query", Desc: "Database query execution", Severity: "info"},
				{Name: "migration", Desc: "Database schema migration", Severity: "info"},
				{Name: "cache_hit", Desc: "Cache hit", Severity: "debug"},
				{Name: "cache_miss", Desc: "Cache miss", Severity: "info"},
			},
		},
		{
			Name: "SCHEDULED",
			Desc: "Cron jobs and scheduled tasks",
			Children: []*model.TaxonomyNode{
				{Name: "cron_started", Desc: "Scheduled job started", Severity: "info"},
				{Name: "cron_completed", Desc: "Scheduled job completed", Severity: "info"},
				{Name: "cron_failed", Desc: "Scheduled job failed", Severity: "error"},
			},
		},
		{
			Name: "APPLICATION",
			Desc: "General application-level log messages",
			Children: []*model.TaxonomyNode{
				{Name: "info", Desc: "Informational application message", Severity: "info"},
				{Name: "warning", Desc: "Application warning", Severity: "warning"},
				{Name: "debug", Desc: "Debug-level trace message", Severity: "debug"},
			},
		},
	}
}
