# biz

`biz` is an agent-friendly business automation CLI in Go.

It starts with invoice automation (Notion-backed), with modular domain design to expand into broader business ops.

## OSS Baseline

This repository is prepared for public open-source use:
- secret-safe defaults (`config.example.yaml`)
- local secret file ignored (`config.yaml`)
- typed error handling and stable JSON envelope
- module-level architecture guides in `docs/modules/`

## Core Features
- Cobra-based CLI with machine-first `--json` output
- Composable module architecture (`internal/modules/*`)
- Notion-backed invoice source using `Invoices -> Worklogs/Costs` relations
- Optional generic records module (`records list/get/schema/create/update/archive`) for Notion pages/databases
- HTML template rendering + PDF generation
- Idempotent invoice create behavior

## Quick Start

1. Create local config:
```bash
cp config.example.yaml config.yaml
```

2. Set Notion credentials in `config.yaml` or env vars:
- `BIZ_NOTION_TOKEN`
- `BIZ_NOTION_INVOICE_DB_ID`

3. Build and test:
```bash
go test ./...
go build -o ./bin/biz ./cmd/biz
```

4. Run:
```bash
./bin/biz doctor --json
./bin/biz invoice list --json
./bin/biz invoice preview <invoice_page_id> --json
./bin/biz invoice create <invoice_page_id> --json
```

## Command Matrix

Command availability depends on `modules.enabled` in your config.

Always available:
- `biz doctor`
- `biz help`
- `biz completion`

Loaded when `modules.enabled` includes `invoice`:
- `biz invoice list [status]`
  - flags: `--status`, `--limit`, `--cursor`
- `biz invoice create <invoice_id>`
  - flags: `--out`, `--source`, `--source-file`, `--upload-notion`, `--confirm`
- `biz invoice preview <invoice_id>`
  - flags: `--format`

Loaded when `modules.enabled` includes `records`:
- `biz records list <collection-or-db-id>`
  - flags: `--limit`, `--cursor`
- `biz records get <page-id>`
- `biz records schema <collection-or-db-id>`
- `biz records create <collection-or-db-id>`
  - flags: `--data` or `--data-file`, `--validate-schema`, `--dry-run`
- `biz records update <page-id>`
  - flags: `--collection`, `--data` or `--data-file`, `--validate-schema`, `--if-last-edited`, `--dry-run`
- `biz records archive <page-id>`
  - flags: `--collection`, `--confirm`, `--if-last-edited`, `--dry-run`

Global flags on all commands:
- `--config`
- `--profile`
- `--actor`
- `--json`
- `--trace-id`

Enable records module example:
```yaml
modules:
  enabled: [invoice, records]
```

Notes:
- `--config` is optional; `biz` auto-loads `./config.yaml` or `$HOME/.config/biz/config.yaml`.
- Use `biz inv ...` as a short alias for `biz invoice ...`.
- Notion mutations are policy-disabled by default (`invoice.allow_notion_mutations: false`).
- Use `--actor agent` for agent-invoked runs; this activates `agent_policy` controls.
- Commands are loaded from `modules.enabled` in config (for example `modules.enabled: [invoice, records]`).

## Quality Gates

```bash
go test ./...
go vet ./...
docker build -t biz:local .
```

## Configuration

See:
- `config.example.yaml`
- `docs/notion-worklogs-setup.md`

Local-only:
- `config.yaml` (ignored by git)

## Mutation Safety

- `--upload-notion` is guarded by policy and confirmation:
  - set `invoice.allow_notion_mutations: true`
  - keep `invoice.require_mutation_confirm: true`
  - run with `--confirm` on create

Example:
```bash
./bin/biz invoice create <invoice_page_id> --upload-notion --confirm
```

## Agent Policy

`agent_policy` is enforced only when `--actor agent` is used.

Example:
```bash
./bin/biz --actor agent invoice list --limit 20 --json
```

For records mutations, use strict allowlists:

```yaml
agent_policy:
  enabled: true
  allowed_commands: [invoice.list, invoice.preview, records.list, records.get, records.create, records.update, records.archive]
  records_allowed_collections: [invoices]
  records_allowed_properties: [Status, Notes]
```

Optional collection aliases for records:

```yaml
notion:
  collections:
    invoices: "<NOTION_INVOICE_DB_ID>"
    clients: "<NOTION_CLIENTS_DB_ID>"
```

Records mutation safety features:
- `--dry-run` for create/update/archive preview
- `--if-last-edited <RFC3339>` optimistic concurrency check for update/archive
- `--data-file` to load mutation payloads from JSON files

Invoice idempotency hardening:
- optional `invoice.idempotency_signing_key` enables HMAC signing/verification of idempotency records
- if enabled, tampered idempotency records fail validation during `invoice create`

## Tamper-Evident Audit Log

Enable signed, hash-chained JSONL audit logging:

```yaml
audit:
  enabled: true
  path: audit/biz-audit.log
  signing_key: "replace-with-strong-secret"
  strict: true
```

Every audited event records actor, command, args, exit code, result code, `prev_hash`, `hash`, and HMAC `signature`.

## Project Structure

- `cmd/biz`: application entrypoint
- `internal/command`: shared command framework and command spec builder
- `internal/modules`: pluggable command modules (`invoice`, `records`, runtime `tax`)
- `internal/invoice`: invoice domain and workflows
- `internal/invoice/notion`: Notion adapter
- `internal/records`: generic record CRUD domain and Notion adapter
- `internal/tax`: tax policy logic
- `internal/platform`: config/errors/output/logging utilities
- `docs/modules`: module-level guides

## Docs

- Notion setup and worklog import:
  - `docs/notion-worklogs-setup.md`
- Security:
  - `docs/security/threat-model.md`
- Module guides:
  - `docs/modules/cli.md`
  - `docs/modules/invoice.md`
  - `docs/modules/notion.md`
  - `docs/modules/records.md`
  - `docs/modules/tax.md`
  - `docs/modules/platform.md`

## Security

Read `SECURITY.md` before running in production.

## Contributing

See `CONTRIBUTING.md`.

## License

MIT. See `LICENSE`.
