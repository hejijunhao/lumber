# Phase 6, Section 4: README Overhaul — Completion Notes

**Completed:** 2026-02-24
**Phase:** 6 (Beta Validation & Polish)
**Depends on:** Sections 1–3 (documents the version flag, edge case behavior, and expanded corpus)

## Summary

Full rewrite of README.md to reflect the current state of Lumber at v0.5.0-beta. The old README was written during Phase 1 scaffolding and had stale taxonomy labels, missing configuration, no CLI documentation, and an outdated status checklist. A new user can now go from zero to running pipeline by following the README alone.

## What Changed

### Modified Files

| File | What |
|------|------|
| `README.md` | Full content rewrite (~200 lines) |

### Sections Added

| Section | What's new |
|---------|-----------|
| **CLI Flags** | All 9 flags documented with 3 usage examples |
| **Connectors** | Vercel, Fly.io, Supabase — setup instructions with required env vars |
| **Configuration (expanded)** | 4 tables: core, engine, operational, provider-specific (21 env vars total) |

### Sections Updated

| Section | Before | After |
|---------|--------|-------|
| **How It Works** | "Vercel, AWS, etc." | "Vercel, Fly.io, Supabase" — reflects actual implementations |
| **Quickstart** | Stream only | Stream + query mode + version check |
| **Taxonomy** | 8 categories with old labels (APPLICATION, SECURITY, incoming_request, etc.) | 8 categories with current 42 leaves (ACCESS, PERFORMANCE, etc.) |
| **Embedding Model** | "Embedding dimension: 384" | "Output dimension: 1024 (384-dim transformer + learned projection)" |
| **Project Structure** | Missing logging/, testdata/, dedup/, httpclient/, connector sub-packages | Full tree with all packages |
| **Status** | 3 checked + 5 unchecked (stale) | 13 checked + 3 unchecked (accurate) |
| **Nav links** | Quickstart, How It Works, Taxonomy, Changelog | Added Connectors link |

### Key Corrections

| Item | Old (wrong) | New (correct) |
|------|-------------|---------------|
| Taxonomy roots | ERROR, REQUEST, DEPLOY, SYSTEM, SECURITY, DATA, SCHEDULED, APPLICATION | ERROR, REQUEST, DEPLOY, SYSTEM, ACCESS, PERFORMANCE, DATA, SCHEDULED |
| ERROR leaves | 5 (runtime_exception, connection_failure, timeout, auth_failure, validation_error) | 9 (+ authorization_failure, out_of_memory, rate_limited, dependency_error) |
| REQUEST leaves | incoming_request, outgoing_request, response | success, client_error, server_error, redirect, slow_request |
| SYSTEM leaves | startup, shutdown, health_check, resource_limit, scaling | health_check, scaling_event, resource_alert, process_lifecycle, config_change |
| Embedding dim | 384 | 1024 (after projection) |
| Go version | 1.23+ | 1.23+ (still correct — go.mod says 1.24.0, README states minimum) |

## Cross-Check Verification

All data points in the README were verified against source code:

- 21 env vars match `internal/config/config.go` `Load()` and `loadConnectorExtra()`
- 9 CLI flags match `LoadWithFlags()` flag registrations
- 42 taxonomy leaves match `internal/engine/taxonomy/default.go`
- 8 root names match
- All defaults (5s dedup, 0.5 threshold, 10s shutdown, 1000 buffer) match
- Project structure matches actual directory layout
- Version constant matches `internal/config/config.go`

## Verification

```
go build ./cmd/lumber   # compiles
go test ./...            # all tests pass
```

README review checklist:
- [x] Every env var in README exists in config.go
- [x] Every CLI flag in README exists in LoadWithFlags()
- [x] Taxonomy table matches default.go
- [x] Project structure matches `ls` output
- [x] Quickstart instructions are copy-pasteable
- [x] Status checklist reflects actual completion state
