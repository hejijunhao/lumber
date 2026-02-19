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
				{Name: "runtime_exception", Desc: "Unhandled runtime exception or panic"},
				{Name: "connection_failure", Desc: "Network or database connection failure"},
				{Name: "timeout", Desc: "Request or operation timeout"},
				{Name: "auth_failure", Desc: "Authentication or authorization failure"},
				{Name: "validation_error", Desc: "Input validation or schema error"},
			},
		},
		{
			Name: "REQUEST",
			Desc: "HTTP requests and API calls",
			Children: []*model.TaxonomyNode{
				{Name: "incoming_request", Desc: "Incoming HTTP request log"},
				{Name: "outgoing_request", Desc: "Outgoing HTTP or API call"},
				{Name: "response", Desc: "HTTP response with status code"},
			},
		},
		{
			Name: "DEPLOY",
			Desc: "Deployment and build events",
			Children: []*model.TaxonomyNode{
				{Name: "build_started", Desc: "Build process started"},
				{Name: "build_succeeded", Desc: "Build completed successfully"},
				{Name: "build_failed", Desc: "Build failed with errors"},
				{Name: "deploy_started", Desc: "Deployment initiated"},
				{Name: "deploy_succeeded", Desc: "Deployment completed successfully"},
				{Name: "deploy_failed", Desc: "Deployment failed"},
			},
		},
		{
			Name: "SYSTEM",
			Desc: "Infrastructure and system-level events",
			Children: []*model.TaxonomyNode{
				{Name: "startup", Desc: "Service or process startup"},
				{Name: "shutdown", Desc: "Service or process shutdown"},
				{Name: "health_check", Desc: "Health check or liveness probe"},
				{Name: "resource_limit", Desc: "Memory, CPU, or disk limit reached"},
				{Name: "scaling", Desc: "Auto-scaling event"},
			},
		},
		{
			Name: "SECURITY",
			Desc: "Security-related events",
			Children: []*model.TaxonomyNode{
				{Name: "login_success", Desc: "Successful user login"},
				{Name: "login_failure", Desc: "Failed login attempt"},
				{Name: "rate_limited", Desc: "Request rate limited or throttled"},
				{Name: "suspicious_activity", Desc: "Suspicious or anomalous activity detected"},
			},
		},
		{
			Name: "DATA",
			Desc: "Database and data operations",
			Children: []*model.TaxonomyNode{
				{Name: "query", Desc: "Database query execution"},
				{Name: "migration", Desc: "Database schema migration"},
				{Name: "cache_hit", Desc: "Cache hit"},
				{Name: "cache_miss", Desc: "Cache miss"},
			},
		},
		{
			Name: "SCHEDULED",
			Desc: "Cron jobs and scheduled tasks",
			Children: []*model.TaxonomyNode{
				{Name: "cron_started", Desc: "Scheduled job started"},
				{Name: "cron_completed", Desc: "Scheduled job completed"},
				{Name: "cron_failed", Desc: "Scheduled job failed"},
			},
		},
		{
			Name: "APPLICATION",
			Desc: "General application-level log messages",
			Children: []*model.TaxonomyNode{
				{Name: "info", Desc: "Informational application message"},
				{Name: "warning", Desc: "Application warning"},
				{Name: "debug", Desc: "Debug-level trace message"},
			},
		},
	}
}
