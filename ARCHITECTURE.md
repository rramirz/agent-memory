# Architecture

How agent-memory works end to end.

## Overview

Centralized long-term memory for coding agents. Three pieces:

1. **memory-api** — Go HTTP server, stores memories in MongoDB, generates context markdown
2. **memory CLI** — runs on each workstation, talks to the API, writes `docs/ai/*.md` into repos
3. **opencode-agent-memory plugin** — MCP tools (`save_memory`, `search_memory`, `sync_memory`) wrapping the same API

```
workstation (home-mac / arrive-laptop / logicbroker-laptop)
  ├─ memory CLI ──────────────┐
  └─ opencode MCP plugin ─────┤  HTTPS + Bearer token
                              ▼
              https://memory.theramirez.casa  (kgateway HTTPRoute)
                              ▼
                    memory-api (Deployment, ns agent-memory)
                              ▼
                    MongoDB 4.4 (StatefulSet, ssd PVC 10Gi)
```

## Data model

Two collections: `memories` and `tokens`.

`tokens` (DB-backed bearer tokens, managed via admin API / web UI):

| Field | Notes |
|---|---|
| `name` | machine/app label |
| `token_hash` | hex SHA-256 of plaintext token — plaintext never stored, shown once at creation |
| `orgs` | orgs this token may access |
| `created_at`, `revoked_at` | revoked = `revoked_at` set; revoked tokens fail auth |

`memories` documents:

| Field | Notes |
|---|---|
| `org` | `arrive` \| `logicbroker` \| `personal` — hard isolation boundary, every query filters on it |
| `project`, `repo` | grouping within org |
| `workstation` | which machine wrote it — metadata only, NOT auth |
| `scope` | usually `repo` |
| `type` | `decision`, `session_summary`, `architecture`, `runbook`, `known_issue`, `task`, `preference`, `note` |
| `title`, `body` | the memory itself |
| `tags` | repeatable strings |
| `importance` | 1-10 |
| `status`, `source` | `active`; `manual` |
| `created_at`, `updated_at` | timestamps |

## API surface (cmd/api/main.go)

Plain `net/http` mux, port 8080, JSON logging via slog, graceful shutdown on SIGINT/SIGTERM.

| Route | What |
|---|---|
| `GET /v1/healthz` | liveness, no auth |
| `POST /v1/memories` | create memory (201) |
| `GET /v1/memories/search` | filter by `org` (required) + `q`/`project`/`repo`/`type`/`tag`/`limit` (max 100) |
| `PATCH /v1/memories/{id}` | partial update (title/body/tags/importance/type/project/repo/scope/status); org immutable |
| `DELETE /v1/memories/{id}` | soft delete (sets `status=deleted`) |
| `GET /v1/context` | returns generated markdown files for org/project/repo |
| `POST /v1/admin/tokens` | admin only — create DB token, returns plaintext once |
| `GET /v1/admin/tokens` | admin only — list tokens (no hashes/secrets) |
| `DELETE /v1/admin/tokens/{id}` | admin only — revoke (sets `revoked_at`) |

## Auth (internal/auth)

Bearer resolution order in `Authorizer`: admin token → env tokens → DB tokens.

- `ADMIN_TOKEN` env (optional). Constant-time compare. Grants `/v1/admin/*`, the `/ui` web console, and all-org access on memory endpoints. Unset = admin endpoints 503, UI shows "admin disabled".
- `MEMORY_TOKENS` env from k8s secret. One line per token: `token:org1,org2`. `#` lines are comments — used to label which machine owns each token.
- DB tokens: incoming bearer is SHA-256 hashed and looked up in the `tokens` collection (`revoked_at` must be unset). Created/revoked via admin API or web UI — no rollout restart needed.
- Request flow: extract `Authorization: Bearer <token>` → resolve orgs → reject 401 (no token) / 403 (token not scoped to requested org).
- Env path: one token per machine; revoke = delete its line + rollout restart. DB path: revoke instantly via UI.

## Web UI (internal/web)

Server-rendered admin console at `/ui`, embedded in the same binary (html/template + vendored HTMX via embed.FS, no CDN, no build step).

- Login: POST `/ui/login` with the admin token → HttpOnly cookie (`am_admin_token`, Path=/ui, SameSite=Lax, Secure behind TLS/X-Forwarded-Proto).
- Memories page: org switcher (org always scopes every query), text search, type/project/repo filters, HTMX partial swaps, inline edit (org immutable) and soft delete with confirm.
- Tokens page: list with active/revoked badges, create (name + org checkboxes) showing plaintext exactly once, revoke with confirm.
- UI handlers call `internal/db` directly; same validation as the API handlers.

## Context generation (internal/contextgen)

`GET /v1/context` builds four markdown files from memories, grouped by type:

| File | Types | Limit |
|---|---|---|
| `docs/ai/current-state.md` | note, task, session_summary, preference | 15 |
| `docs/ai/decisions.md` | decision | 50 |
| `docs/ai/architecture.md` | architecture, runbook | 30 |
| `docs/ai/known-issues.md` | known_issue | 30 |

`memory sync` fetches these and writes them into the repo so agents can read context locally without network calls.

## CLI flows (cmd/memory/main.go)

Config resolution: workstation `~/.agent-memory/config.yaml` (api_url, default_org, allowed_orgs, token_env → `MEMORY_TOKEN`) + repo `.agent-memory.yaml` (org, project, repo). CLI refuses orgs not in workstation `allowed_orgs` — client-side guard on top of server-side token scoping.

- `memory init --org X --project Y --repo Z` — writes `.agent-memory.yaml`, creates `docs/ai/`
- `memory add` — POST to API; on failure queues JSON into `~/.agent-memory/outbox/` (fail-open, never lose a memory)
- `memory flush` — replays outbox entries, deletes each on success
- `memory search "q"` — table output, scoped to repo's org/project/repo
- `memory sync` — pulls context files, writes `docs/ai/*.md`
- `memory status` — workstation, repo config, API reachability, outbox count

## Deployment (deploy/k8s/)

- Namespace `agent-memory`
- MongoDB: `mongo:4.4` (Proxmox VMs lack AVX; Mongo 5+ requires it), tcpSocket readiness probe, `ssd` StorageClass
- memory-api: image `ghcr.io/rramirz/agent-memory:latest`, built `--platform linux/amd64` (Mac is ARM, nodes amd64)
- Ingress: kgateway HTTPRoute `memory.theramirez.casa` → Service `memory-api:80` → pod 8080
- Secret `memory-api-secret`: `MONGO_URI`, `MONGO_DATABASE`, `MEMORY_TOKENS`, `ADMIN_TOKEN` (optional key) — never committed with real values

## Failure modes

- API down → CLI `add` queues to outbox, surfaces "queued"; flush later
- Mongo down → API fails startup (10s connect timeout) or requests error; health stays up only if connected at boot
- Bad token → 401/403, nothing written
- Org mismatch in outbox replay → entry skipped, kept on disk

## Deliberate non-goals

No vector search, no auto-injection, no multi-user accounts (single admin token for the UI). Memory writes are explicit (manual CLI/MCP calls or admin UI).
