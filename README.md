# Bodhi Server

> 📖 中文版请看 **[README.zh-CN.md](./README.zh-CN.md)**

> An account, billing, quota, secret-vault, and LLM-gateway service for the Zenith ecosystem.

---

## What is this

Use bodhi-server when several users need a shared model gateway, centrally stored
provider credentials, and per-user usage and quota controls. It supplies a Go API
and browser administration interface. It is an **optional hosted service** in
Zenith; to use an agent on your own computer, start with
[Bodhi](https://github.com/bigduu/Bodhi-AI) or [Bamboo](https://github.com/bigduu/Bamboo-agent).

The service needs PostgreSQL, a JWT signing secret, and a credential-encryption key.
Docker Compose builds the admin interface and Go binary, then starts the service.
Real model calls also require provider credentials and configuration; a healthy
server alone does not establish that a provider request succeeds.

As of 2026-10-03, the public GitHub Releases page has no releases. Capabilities
below describe inspected source, and the quickstart builds that source. They do
not establish a released hosted product or end-to-end validation of every provider.
See [audit notes](./docs/readme-audit.md).

---

## Key Capabilities at a Glance

| Capability | What it does |
| --- | --- |
| 🔐 Auth | Email/username register & login, issues JWT access/refresh tokens; machine-facing `bhi_sk_` API keys (hash-only storage) |
| 🗝️ Credential Vault | Each provider's secret is encrypted with **AES-256-GCM** before storage, per-user or per-group |
| 🤖 LLM Proxy | Unified entry point forwarding to OpenAI / Anthropic / Gemini / Azure OpenAI / any OpenAI-compatible endpoint |
| 💸 Billing & Quota | Per-call token & spend metering, billing-period reports, CSV export, balance management, RPM/RPD/daily & monthly caps |
| 👥 Groups | Shared credentials and quotas, group-based authorization |
| 🧭 Model Routing | Model registry + multi-instance priority failover, automatically skipping recently failed instances |
| 🛡️ Hardening | Login brute-force protection, IP rate limiting, global rate limit, key-level IP whitelist and model whitelist |
| 📋 Governance | Audit logs, content moderation rules, webhook events, data retention policies, Prometheus metrics |
| 🖥️ Embedded UI | The admin panel is built first and embedded into the Go binary from `cmd/server/web/dist` |

---

## Architecture

Everything is served by the standard-library `net/http` (Go 1.22+ `ServeMux` routing). Three auth gates (JWT user, JWT admin, API-key machine) guard different path prefixes. The proxy subsystem decrypts the right credential, resolves model routing, forwards the call (streaming or not), and meters tokens, billing, and quota along the way.

```mermaid
flowchart TD
    subgraph clients[Clients]
      UI[Admin UI<br/>embedded web/dist]
      Agent[bamboo Agent / SDK]
    end

    UI -->|JWT: /api/v1/*| GW
    Agent -->|API Key: /proxy/*| GW

    subgraph bodhi[bodhi-server · Go]
      GW[net/http ServeMux<br/>CORS · RequestID · RateLimit]
      AUTH[Auth Gates<br/>withJWT / withAdmin / withAPIKey]
      PROXY[LLM Proxy<br/>decrypt → route → forward → meter]
      SVC[Services<br/>billing · quota · groups · audit · webhooks]
      GW --> AUTH --> SVC
      AUTH --> PROXY
    end

    PROXY -->|encrypted credential AES-256-GCM| DB[(PostgreSQL · pgx)]
    SVC --> DB
    PROXY -->|forward| LLM[OpenAI · Anthropic · Gemini<br/>Azure · OpenAI-compatible]
```

Module layout:

- `cmd/server/main.go` — entry point: load config, connect to DB, run migrations, start background tasks, embed `web/dist`, graceful shutdown.
- `api/router/router.go` — all routes and the three auth gates (`withJWT` / `withAdmin` / `withAPIKey`).
- `api/handler/` — business handlers (auth, key, credential, admin, billing, group, model, instance, audit, content, webhook, retention, version, settings, health).
- `api/middleware/` — CORS, RequestID.
- `internal/auth/` — JWT, API key generation & hashing, token revocation.
- `internal/proxy/` — proxy handler, model routing, SSE streaming; `proxy/providers/registry.go` is the provider registry.
- `internal/crypto/` — AES-256-GCM credential encryption/decryption.
- `internal/{quota,pricing,billing}` via models — quota, pricing, billing.
- `internal/{ratelimit,security,moderation,audit,webhook,retention,health,metrics,cache}` — cross-cutting governance and hardening.
- `internal/database/{postgres.go,schema.go}` — connection pool and built-in migrations (tables created at boot).

---

## Signature Deep-Dives

### Dual-track Auth: JWT + API Key

Humans use JWT — `internal/auth/jwt.go` issues HS256 access tokens (15 min) and refresh tokens (7 days), with a typed `Claims.Type` so a refresh token can never masquerade as an access token. Registration can require an invite code (`internal/models/invite_code.go`). Machines use API keys — `internal/auth/apikey.go` mints `bhi_sk_`-prefixed base62 keys and stores **only the SHA-256 hash**; the plaintext is shown exactly once at creation. Keys can be scoped to allowed models, providers, and an IP whitelist, all enforced in the `withAPIKey` gate (see `router.go`).

### Encrypted Credential Vault

Provider secrets are never stored in cleartext. `internal/crypto/encryption.go` uses **AES-256-GCM** (a 32-byte key supplied as 64 hex chars via `BODHI_ENCRYPTION_KEY`, length-validated to exactly 64 hex chars at startup in `internal/config/config.go`) before writing to `provider_credentials`. Credentials can live per-user or be shared per-group (`group_credentials`); the proxy decrypts on the fly and injects them into the upstream request.

### LLM Proxy & Provider Registry

`internal/proxy/providers/registry.go` ships 5 providers, each implementing the `Provider` interface (`Name` / `BuildURL` / `InjectAuth`):

- `openai` — Bearer auth, path pass-through.
- `anthropic` — `x-api-key` + `anthropic-version: 2023-06-01`.
- `gemini` — splices the model into `:generateContent` / `:streamGenerateContent`, key via query parameter.
- `azure-openai` — `/openai/deployments/{model}/chat/completions` + `api-key` header.
- `openai-compatible` — adapts to any OpenAI-compatible endpoint such as Ollama / vLLM / LM Studio.

Four proxy entry routes exist (`router.go`): `/proxy/openai/`, `/proxy/anthropic/`, `/proxy/gemini/`, and a universal `/proxy/v1/{path...}`. `internal/proxy/handler.go` keeps TTL caches for credentials, routes, and quota plus a connection-pooled `http.Client`, and streams SSE responses via `internal/proxy/sse.go`. `ResolveModel` in `internal/proxy/router.go` pulls multiple instances from the model registry, orders them by `priority`, and skips recently failed instances for automatic failover.

### Billing & Quota

Every proxied call is metered into `usage_tracking`. `internal/pricing/calculator.go` computes cost from `model_pricing`, and `internal/quota/checker.go` tracks per-minute/day/month request counts, tokens, and spend (`user_usage_counters`), enforcing the RPM / RPD / daily & monthly token & spend caps from `user_quotas` / `group_quotas`. Users can view current usage, monthly reports, and export CSV (`/api/v1/billing/*`); admins can top up balances and inspect aggregate and per-user usage.

---

## Quick Start / Development

### Docker: build from source

`docker-compose.yml` brings up PostgreSQL 16 and bodhi-server on port `8080`. The Dockerfile builds the React admin panel with Node.js 22, then compiles and embeds it with the Go 1.25 toolchain. The two encryption-related variables are required:

```bash
git clone https://github.com/bigduu/bodhi-server.git
cd bodhi-server

# credential encryption key (32 bytes = 64 hex)
export BODHI_ENCRYPTION_KEY=$(openssl rand -hex 32)
export BODHI_JWT_SECRET=$(openssl rand -hex 32)
export BODHI_DB_PASSWORD=$(openssl rand -hex 24)

docker compose up --build
```

Service comes up at `http://localhost:8080`; health probe is `GET /health`. Tables are auto-migrated on boot by `internal/database/schema.go`.

Keep these generated secrets stable for the same database: changing the encryption
key prevents decryption of existing provider credentials. The Compose file exposes
ports 8080 and 5432 on the host; use an isolated development environment for this
quickstart. A production deployment needs its own network and TLS configuration.

### Run locally with Go

> Requires Go 1.25, Node.js/npm, and a reachable PostgreSQL.

```bash
export BODHI_DB_URL="postgres://bodhi:bodhi@localhost:5432/bodhi?sslmode=disable"
export BODHI_ENCRYPTION_KEY=$(openssl rand -hex 32)
export BODHI_JWT_SECRET=$(openssl rand -hex 32)

(cd cmd/server/web && npm ci && npm run build)
go run ./cmd/server      # start the server
go build -o bodhi-server ./cmd/server   # build the binary
go test ./...            # run tests
```

The Go build embeds the generated admin assets. Browser routes fall back to the embedded `index.html`; `/health`, `/metrics`, `/api/*`, and `/proxy/*` remain server routes.

### Environment Variables

| Var | Default | Description |
| --- | --- | --- |
| `BODHI_JWT_SECRET` | — (required) | JWT signing secret |
| `BODHI_ENCRYPTION_KEY` | — (required, 64 hex) | credential AES-256-GCM key |
| `BODHI_DB_URL` | `postgres://bodhi:bodhi@localhost:5432/bodhi?sslmode=disable` | PostgreSQL connection string |
| `BODHI_PORT` | `8080` | listen port |
| `BODHI_BIND` | `0.0.0.0` | bind address |
| `BODHI_PROXY_TIMEOUT` | `300` (s) | upstream proxy timeout |
| `BODHI_MAX_BODY_MB` | `50` | max request body (MB) |
| `BODHI_CORS_ORIGINS` | (empty) | comma-separated allowed CORS origins |
| `BODHI_RATE_LIMIT_RPM` | `60` | global rate limit RPM |
| `BODHI_RATE_LIMIT_BURST` | `10` | rate limit burst |
| `BODHI_VERSION` | `dev` | reported service version |
| `BODHI_LOG_JSON` | `false` | set to `true` for JSON logs |

### Key Routes

```
GET  /health                          health check (public)
GET  /metrics                         Prometheus metrics (admin-only)
POST /api/v1/auth/register            register (invite-gated)
POST /api/v1/auth/login               login
POST /api/v1/auth/refresh             refresh token
GET  /api/v1/auth/me                  current user
POST /api/v1/keys                     create API key
POST /api/v1/credentials              save provider credential (encrypted)
GET  /api/v1/models                   available model list
GET  /api/v1/billing/current          current usage
/proxy/openai/  /proxy/anthropic/  /proxy/gemini/  /proxy/v1/{path...}   LLM proxy (API key)
/api/v1/admin/...                     users/quota/pricing/models/instances/groups/audit/webhooks/retention (admin)
```

---

## The Rest of the Stack

`bodhi-server` is a separate hosted service in the **Zenith** ecosystem. The
core local desktop path does not require it:

- **[Bodhi](https://github.com/bigduu/Bodhi-AI)** — the Tauri desktop shell; starts its owned Bamboo sidecar, waits for its health endpoint, then opens the Lotus Next UI served by Bamboo. External-server reuse is limited to the explicitly selected legacy rollback path.
- **[Lotus Next](https://github.com/bigduu/lotus-next)** — the React + Vite frontend served by Bamboo; uses HTTP APIs plus the shared `/v2/stream` WebSocket, with legacy SSE fallback when the first WebSocket connection cannot be established.
- **[Bamboo](https://github.com/bigduu/Bamboo-agent)** — the local-first Rust agent runtime and Lotus Next host; when configured, it can carry an API key through this service's `/proxy/*` gateway.
- **bodhi-server** (this module) — Go backend: auth / persistence / billing & quota / LLM proxy.
- **[Pavilion](https://github.com/bigduu/Pavilion)** — marketing site and documentation.
- **[Zenith](https://github.com/bigduu/Zenith)** — repository index, submodule pointers, and release trains.

> `bodhi-server` is not Bamboo's local API server or the host for Lotus Next. It is
> the optional account, billing, quota, and provider gateway described in this
> README.
