# agent-memory

## What this is

Go API + CLI for centralized agent memory. Stores memories in MongoDB. Generates context markdown files for OpenCode agents. Embedded admin web UI at `/ui` (added June 11 2026) for managing memories + API tokens.

Full system explanation: `ARCHITECTURE.md` (components, data model, auth, flows, deploy, failure modes). Keep it updated when architecture changes.

## Repos

- API + CLI: `github.com/rramirz/agent-memory`
- OpenCode plugin: `github.com/rramirz/opencode-agent-memory`

## Architecture

```
cmd/api/     → HTTP server (port 8080): /v1 API + /ui admin console
cmd/memory/  → CLI (add/import/search/sync/flush/status)
internal/
  auth/       Authorizer: admin token → env tokens → DB tokens (SHA-256 lookup)
  config/     API env config + workstation/repo YAML config
  db/         MongoDB connection, indexes, CRUD (memories + tokens collections)
  contextgen/ generates docs/ai/*.md from memories
  handlers/   HTTP handlers (health, memories, context, admin tokens)
  models/     Memory + Token structs, request/response types
  outbox/     local fail-safe write queue (~/.agent-memory/outbox/)
  web/        embedded HTMX admin UI (templates + static via embed.FS)
deploy/k8s/  namespace, secret, mongodb StatefulSet, memory-api Deployment, HTTPRoute
```

## Web UI + admin token

- `/ui` on the same binary: login with `ADMIN_TOKEN` (HttpOnly cookie), memories browse/search/edit/soft-delete per org, token create/revoke.
- `ADMIN_TOKEN` env from `memory-api-secret` (key optional in Deployment manifest). Unset = admin API 503 + UI disabled.
- Admin token on home-mac: `~/.agent-memory/admin-token` (chmod 600); source of truth is the k8s secret.
- Admin endpoints: `POST/GET /v1/admin/tokens`, `DELETE /v1/admin/tokens/{id}`. Memory edit: `PATCH/DELETE /v1/memories/{id}` (soft delete, org immutable).

## Cluster deployment

- Namespace: `agent-memory`
- MongoDB: StatefulSet, single replica, `ssd` StorageClass, 10Gi PVC
- memory-api: Deployment, image `ghcr.io/rramirz/agent-memory:latest`
- Ingress: kgateway HTTPRoute → `memory.theramirez.casa`
- Secret: `memory-api-secret` — never commit real values, create via kubectl

## Orgs

Three isolated orgs: `arrive`, `logicbroker`, `personal`. Every memory, every query, every token must include org. Never mix.

## Core namespace (shared personality)

`org = "core"` is a reserved cross-org namespace for foundational agent "personality" (preferences, conventions, rules) that every workstation shares. Readable by any recognized token; writable only by a token granted `core` (`MEMORY_TOKENS` line `<token>:personal,core`). Never put org secrets or machine-specific paths in core.

- Write path: the local `/reflect` OpenCode skill (`~/.config/opencode/skill/reflect/SKILL.md`) mines recent sessions and proposes universal signals, writes approved memories via `save_memory core=true`, and records progress with `reflection-marker` session summaries in the core namespace.
- Read path: `GET /v1/context` always emits `docs/ai/core.md` from `org=core` (any requested org), so `memory sync` / `sync_memory` pull it everywhere.
- MCP: `save_memory` / `search_memory` accept `core: true` to target the namespace.
- Go seams: `models.OrgCore` + `models.IsWritableOrg` (create), `Authorizer.CanReadOrg` + `Authorizer.Recognized` (core readable by any known token), `db.GetCoreMemories`, `contextgen` core.md. `core` is NOT in `models.ValidOrgs`.
- Enable writes: grant `core` to the home-mac token in `memory-api-secret`, then `kubectl -n agent-memory rollout restart deploy/memory-api`. Reads need no change.

## Token format

`MEMORY_TOKENS` env, one per line: `token:org1,org2`. Lines starting with `#` are comments (auth.go parser skips them).

## Machine access management

Two paths since June 11 2026:

1. **Web UI / admin API (preferred)**: create + revoke DB-backed tokens at `https://memory.theramirez.casa/ui` → Tokens. Hashed in Mongo, plaintext shown once, instant revoke, no restart.
2. **Env tokens (legacy, still active)**: one bearer token per machine in the secret, mapped via comments:

```
# home-mac
<token>:personal
# arrive-laptop
<token>:arrive
# logicbroker-laptop
<token>:logicbroker
```

Live since June 9 2026: per-machine tokens for `home-mac` (personal), `arrive-laptop` (arrive), `logicbroker-laptop` (logicbroker). All scoping verified (200 own org, 403 others). `workstation` field on memories is metadata only, not auth.

Add machine: `openssl rand -hex 24`, add commented line to secret, rollout restart.
Revoke machine: remove its line, rollout restart.

```bash
# read current tokens
kubectl -n agent-memory get secret memory-api-secret -o jsonpath='{.data.MEMORY_TOKENS}' | base64 -d
# rebuild secret (preserve MONGO_URI/MONGO_DATABASE), then:
kubectl -n agent-memory rollout restart deploy/memory-api
```

Laptop setup: get token from secret, set as `MEMORY_TOKEN` env on that machine, create `~/.agent-memory/config.yaml` with matching `default_org`.

## CLI install

```bash
go install github.com/rramirz/agent-memory/cmd/memory@latest
```

Workstation config: `~/.agent-memory/config.yaml`
Repo config: `.agent-memory.yaml`

## OpenCode plugin

Install: `npm install -g github:rramirz/opencode-agent-memory`
Tools: `save_memory`, `search_memory` (both accept `core: true` to target the shared core namespace), `sync_memory` — all manual, all fail-open.

## Safety

- Never commit `deploy/k8s/secret.yaml` with real values
- MongoDB queries always filter by `org`
- Tokens scoped to allowed orgs only
- Outbox writes go to `~/.agent-memory/outbox/` — flush with `memory flush`
