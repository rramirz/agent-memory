# agent-memory

Centralized memory backend for coding agents. Go API + CLI + Kubernetes deployment.

## What it does

Stores long-term agent memory (decisions, architecture notes, session summaries, known issues) in MongoDB. Generates small curated markdown files that OpenCode agents can read locally.

## Structure

```
cmd/api/        HTTP API server
cmd/memory/     CLI tool
internal/       packages (auth, config, db, contextgen, handlers, models, outbox)
deploy/
  Dockerfile
  k8s/          namespace, secret, mongodb, memory-api, ingress
docs/ai/        generated context files (gitignored content)
```

## Quick start

```bash
# 1. Dependencies
task tidy

# 2. Run locally (set env vars first)
export MONGO_URI=mongodb://localhost:27017
export MONGO_DATABASE=agent_memory
export MEMORY_TOKENS="mytoken:personal"
task dev

# 3. Install CLI
task install:cli
```

## Workstation config

Create `~/.agent-memory/config.yaml`:

```yaml
workstation: home-mac
default_org: personal
allowed_orgs:
  - personal
api_url: https://memory.theramirez.casa
token_env: MEMORY_TOKEN
```

## Repo config

In each repo root run:

```bash
memory init --org personal --project my-app --repo my-repo
```

Creates `.agent-memory.yaml` and `docs/ai/`.

## CLI usage

```bash
memory status
memory add --type decision --title "Use HTMX" --body "Prefer server-rendered HTMX over React." --tag htmx --importance 8
memory search "htmx"
memory sync
memory flush
```

## Token format

`MEMORY_TOKENS` env var, one token per line:

```
logicbroker-laptop-token:logicbroker
arrive-laptop-token:arrive
home-mac-token:personal
```

## Deploy

```bash
# 1. Create real secret (do not commit)
kubectl -n agent-memory create secret generic memory-api-secret \
  --from-literal=MONGO_URI='mongodb://mongodb.agent-memory.svc.cluster.local:27017' \
  --from-literal=MONGO_DATABASE='agent_memory' \
  --from-literal=MEMORY_TOKENS='your-token:personal'

# 2. Apply manifests
task k8s:apply

# 3. Check status
task k8s:status
```

Exposes `https://memory.theramirez.casa` via kgateway `home` Gateway.

## Install CLI on any machine

```bash
go install github.com/rramirz/agent-memory/cmd/memory@latest
```

## Non-goals v1

No web UI. No vector search. No MCP server. No auto-injection. See `opencode-agent-memory` plugin for OpenCode integration.
