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

One collection: memories. Each document:

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
| `GET /v1/context` | returns generated markdown files for org/project/repo |

## Auth (internal/auth)

- `MEMORY_TOKENS` env from k8s secret. One line per token: `token:org1,org2`. `#` lines are comments — used to label which machine owns each token.
- Request flow: extract `Authorization: Bearer <token>` → look up token's orgs → reject 401 (no token) / 403 (token not scoped to requested org).
- One token per machine. Revoke machine = delete its line + rollout restart. No machine registry, no sessions, no users.

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
- Secret `memory-api-secret`: `MONGO_URI`, `MONGO_DATABASE`, `MEMORY_TOKENS` — never committed with real values

## Failure modes

- API down → CLI `add` queues to outbox, surfaces "queued"; flush later
- Mongo down → API fails startup (10s connect timeout) or requests error; health stays up only if connected at boot
- Bad token → 401/403, nothing written
- Org mismatch in outbox replay → entry skipped, kept on disk

## Deliberate non-goals (v1)

No web UI, no vector search, no auto-injection, no machine registry, no token self-service. Memory writes are explicit (manual CLI/MCP calls).
