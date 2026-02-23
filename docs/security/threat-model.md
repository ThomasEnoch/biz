# Threat Model

Last updated: February 23, 2026

## Executive Summary
`biz` is a local, operator-run Go CLI for invoice and records automation backed by Notion. The highest-risk areas are:

- credential misuse (Notion token exposure),
- mutation integrity (wrong create/update/archive targets),
- unsafe rendering/dependency execution (HTML -> Chrome PDF pipeline),
- agent overreach (automated mutations without strict policy bounds).

The project is not an internet-exposed server, so remote unauthenticated attack paths are lower than local compromise and untrusted content ingestion risks.

## Scope
In scope:

- CLI orchestration and module loading (`cmd/biz/main.go`, `internal/command/*`)
- Invoice workflows (`internal/invoice/*`, `internal/modules/invoice/*`)
- Records workflows (`internal/records/*`, `internal/modules/records/*`)
- Notion adapters (`internal/invoice/notion/*`, `internal/records/notion/*`)
- Policy enforcement (`internal/policy/*`)
- Config and secret ingestion (`internal/platform/config/*`)
- Rendering and file output (`internal/invoice/render/*`, `internal/invoice/pdf/*`)
- Audit chain logging (`internal/audit/*`)

Out of scope:

- Notion SaaS internals
- OS hardening outside repository controls
- Third-party infrastructure not configured in this repo

## Assets

- Notion token and DB identifiers
- Confidential invoice/worklog/cost/client data
- Mutation integrity for record and invoice status changes
- Generated invoice artifacts (PDF/HTML previews)
- Idempotency and audit chain files

## Trust Boundaries

1. Local caller (human/agent) -> CLI process
2. Local config/env/filesystem -> runtime
3. Runtime -> Notion API (HTTPS, bearer token)
4. Notion payloads -> local domain/render pipeline
5. Render output -> local filesystem and optional Notion attachment

## Entry Points

- CLI commands and flags:
  - `invoice list/create/preview`
  - `records list/get/schema/create/update/archive`
- Config loading (`--config`, env vars)
- JSON payload inputs (`--data`, `--data-file`)
- Notion responses used for mapping, rendering, and mutation decisions

## Current Security Controls

- Typed error model and controlled exit codes (`internal/platform/errors`)
- Agent policy gates with command allowlist (`allowed_commands`)
- Invoice ID regex and list limit controls for agent mode
- Records mutation allowlists:
  - `records_allowed_collections`
  - `records_allowed_properties`
- Mutation friction:
  - `records archive --confirm`
  - `records --dry-run`
  - `records --if-last-edited` optimistic concurrency checks
  - schema-aware key validation (`records schema`, `--validate-schema`)
- Invoice mutation gate:
  - `allow_notion_mutations`
  - `require_mutation_confirm`
- Local file permission defaults for artifacts/idempotency (0700/0600 style paths)
- Tamper-evident audit chain with signatures (`internal/audit/writer.go`)
- Test + vet + CI pipeline

## Top Threats

### TM-001: Token exposure from local environment
Vector:

- token leaks through shell history, local config mishandling, logs, or process environment.

Impact:

- confidentiality and integrity loss across Notion resources.

Current controls:

- token presence checks, documentation guidance.

Gaps:

- no built-in keychain integration or token rotation workflow.

Recommended mitigations:

- prefer short-lived/least-privilege Notion credentials,
- isolate local config permissions,
- add operational token rotation guidance in docs.

### TM-002: Unsafe content through render pipeline
Vector:

- attacker-controlled Notion content is rendered into HTML then converted by Chrome.

Impact:

- potential artifact poisoning; in worst case, renderer exploit path.

Current controls:

- `html/template` usage and invoice validation.

Gaps:

- renderer still depends on external browser process and runtime flags.

Recommended mitigations:

- keep sandbox enabled in production,
- consider renderer isolation (container/user namespace),
- add explicit content safety guidance for high-risk fields.

### TM-003: Agent overreach for destructive mutations
Vector:

- autonomous agent executes broad records or invoice mutations.

Impact:

- accidental status corruption and business process integrity loss.

Current controls:

- agent command allowlist,
- records collection/property allowlists,
- archive confirmation, dry-run, optimistic concurrency flag.

Gaps:

- no per-environment role profile templates shipped by default.

Recommended mitigations:

- provide hardened example policies for read-only vs mutation roles,
- require explicit collection for all record mutations (already enforced in policy path).

### TM-004: Filesystem disclosure or tampering
Vector:

- outputs written to unsafe locations,
- idempotency/audit files tampered by local processes.

Impact:

- data leakage or workflow integrity issues.

Current controls:

- controlled file perms, output base-dir support, signed audit chain.

Gaps:

- idempotency store uses optional per-record HMAC signing (improved), but still remains a local JSON file and can be deleted/replaced.

Recommended mitigations:

- keep `invoice.idempotency_signing_key` enabled in production profiles,
- consider migrating to a locked local store (sqlite) for stronger anti-replacement guarantees,
- extend integrity checks at load/save boundaries.

### TM-005: External dependency/availability failures
Vector:

- Notion downtime, throttling, malformed responses.

Impact:

- operational unavailability and delayed billing flows.

Current controls:

- retry/backoff/timeouts, local fallback support for invoice data source.

Gaps:

- no circuit-breaker or queueing strategy.

Recommended mitigations:

- add lightweight retry budgets/circuit breaker for high-volume agent runs,
- surface fallback-use metrics in command output or audit.

## Risk Prioritization

- High: TM-001, TM-002, TM-003
- Medium: TM-004, TM-005

## Security Review Checklist (Current Architecture)

- Verify `agent_policy` in production configs:
  - strict `allowed_commands`
  - `records_allowed_collections` and `records_allowed_properties` populated
- Keep `invoice.allow_notion_mutations=false` unless explicitly needed
- Require `--dry-run` in automation runbooks before mutating commands
- Use `--if-last-edited` for high-value updates/archives
- Keep renderer sandboxing and runtime isolation strong
- Protect local config, idempotency, and audit paths with strict permissions

## Code Paths to Re-review on Major Changes

- `internal/modules/records/command_specs.go`
- `internal/policy/agent.go`
- `internal/invoice/workflow_create.go`
- `internal/invoice/notion/client.go`
- `internal/invoice/render/chromedp.go`
- `internal/platform/config/*`
- `internal/audit/writer.go`
