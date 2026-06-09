# agent-memory

## What this is

Go API + CLI for centralized agent memory. Stores memories in MongoDB. Generates context markdown files for OpenCode agents.

Full system explanation: `ARCHITECTURE.md` (components, data model, auth, flows, deploy, failure modes). Keep it updated when architecture changes.

## Repos

- API + CLI: `github.com/rramirz/agent-memory`
- OpenCode plugin: `github.com/rramirz/opencode-agent-memory`

## Architecture

```
cmd/api/     → HTTP server (port 8080)
cmd/memory/  → CLI (save/search/sync/flush/status)
internal/
  auth/       bearer token store, org scoping
  config/     API env config + workstation/repo YAML config
  db/         MongoDB connection, indexes, CRUD
  contextgen/ generates docs/ai/*.md from memories
  handlers/   HTTP handlers (health, memories, context)
  models/     Memory struct + request/response types
  outbox/     local fail-safe write queue (~/.agent-memory/outbox/)
deploy/k8s/  namespace, secret, mongodb StatefulSet, memory-api Deployment, HTTPRoute
```

## Cluster deployment

- Namespace: `agent-memory`
- MongoDB: StatefulSet, single replica, `ssd` StorageClass, 10Gi PVC
- memory-api: Deployment, image `ghcr.io/rramirz/agent-memory:latest`
- Ingress: kgateway HTTPRoute → `memory.theramirez.casa`
- Secret: `memory-api-secret` — never commit real values, create via kubectl

## Orgs

Three isolated orgs: `arrive`, `logicbroker`, `personal`. Every memory, every query, every token must include org. Never mix.

## Token format

`MEMORY_TOKENS` env, one per line: `token:org1,org2`. Lines starting with `#` are comments (auth.go parser skips them).

## Machine access management

No machine registry, no UI. Machine access = one bearer token per machine, mapped via comments in the secret:

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
Tools: `save_memory`, `search_memory`, `sync_memory` — all manual, all fail-open.

## Safety

- Never commit `deploy/k8s/secret.yaml` with real values
- MongoDB queries always filter by `org`
- Tokens scoped to allowed orgs only
- Outbox writes go to `~/.agent-memory/outbox/` — flush with `memory flush`
