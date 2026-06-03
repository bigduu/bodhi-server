# Bodhi Server · 菩提服务端

> Zenith 全家桶的「后端大脑」 — 负责账号、计费、密钥保险箱，以及把请求安全地转发到各家 AI 模型。
> The backend brain of the Zenith stack — it handles accounts, billing, the secret-key vault, and securely forwards requests to the major AI model providers.

---

## 1. 这是什么 · What is this

**中文：** 想象一个为团队服务的「AI 总管」。它替你保管每个人的登录账号、记录每一次用量与花费、把昂贵的 API 密钥锁进加密保险箱，并在你和 OpenAI、Anthropic、Google Gemini 等模型之间充当一个聪明的中转站 —— 谁能用哪个模型、每天能花多少钱，都由它说了算。

**English:** Picture an "AI concierge" for a whole team. It keeps everyone's login accounts, records every bit of usage and spend, locks expensive API keys away in an encrypted vault, and sits as a smart middleman between your users and the big model providers (OpenAI, Anthropic, Google Gemini, and more) — deciding who may use which model and how much each person is allowed to spend.

它是用 **Go** 写的单一可执行文件，数据存在 **PostgreSQL**，可以一条 `docker compose up` 跑起来。It is a single **Go** binary backed by **PostgreSQL**, and it boots with one `docker compose up`.

---

## 2. 核心能力一览 · Key Capabilities at a Glance

| 能力 Capability | 说明 What it does |
| --- | --- |
| 🔐 身份认证 Auth | 邮箱/用户名注册登录，签发 JWT 访问/刷新令牌；面向程序的 `bhi_sk_` API Key（仅存哈希） |
| 🗝️ 密钥保险箱 Credential Vault | 各家 provider 的密钥用 **AES-256-GCM** 加密落库，按用户或按用户组存放 |
| 🤖 LLM 代理 LLM Proxy | 统一入口转发到 OpenAI / Anthropic / Gemini / Azure OpenAI / 任意 OpenAI 兼容端点 |
| 💸 计费与配额 Billing & Quota | 逐次记录 token 与花费，账期报表、CSV 导出、余额管理、RPM/RPD/每日每月限额 |
| 👥 用户组 Groups | 共享密钥与配额，按组授权 |
| 🧭 模型路由 Model Routing | 模型注册表 + 多实例按优先级故障转移，自动跳过近期失败的实例 |
| 🛡️ 防护 Hardening | 登录暴力破解防护、IP 限流、全局速率限制、Key 级 IP 白名单与模型白名单 |
| 📋 治理 Governance | 审计日志、内容审查规则、Webhook 事件、数据保留策略、Prometheus 指标 |
| 🖥️ 内嵌前端 Embedded UI | 可把构建好的管理面板嵌入二进制（`web/dist`），无前端时退化为纯 API 模式 |

---

## 3. 架构 · Architecture

**中文：** 标准库 `net/http`（Go 1.22+ 的 `ServeMux` 路由）承载所有路由；三道认证闸门（JWT 用户、JWT 管理员、API Key 程序调用）守在不同前缀上；代理子系统负责解密密钥、解析模型路由、转发并按流式/非流式回传，同时计量 token、计费、扣配额。

**English:** Everything is served by the standard-library `net/http` (Go 1.22+ `ServeMux` routing). Three auth gates (JWT user, JWT admin, API-key machine) guard different path prefixes. The proxy subsystem decrypts the right credential, resolves model routing, forwards the call (streaming or not), and meters tokens, billing, and quota along the way.

```mermaid
flowchart TD
    subgraph clients[客户端 Clients]
      UI[管理面板 / Admin UI<br/>embedded web/dist]
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

    PROXY -->|加密密钥 AES-256-GCM| DB[(PostgreSQL · pgx)]
    SVC --> DB
    PROXY -->|转发 forward| LLM[OpenAI · Anthropic · Gemini<br/>Azure · OpenAI-compatible]
```

模块布局 · Module layout:

- `cmd/server/main.go` — 入口：加载配置、连库、跑迁移、启动后台任务、嵌入 `web/dist`、优雅关停。
- `api/router/router.go` — 全部路由与三道认证闸门（`withJWT` / `withAdmin` / `withAPIKey`）。
- `api/handler/` — 各业务 handler（auth、key、credential、admin、billing、group、model、instance、audit、content、webhook、retention、version、settings、health）。
- `api/middleware/` — CORS、RequestID。
- `internal/auth/` — JWT、API Key 生成与哈希、令牌吊销。
- `internal/proxy/` — 代理 handler、模型路由、SSE 流式；`proxy/providers/registry.go` 为 provider 注册表。
- `internal/crypto/` — AES-256-GCM 凭据加解密。
- `internal/{quota,pricing,billing}` 经由 models — 配额、定价、计费。
- `internal/{ratelimit,security,moderation,audit,webhook,retention,health,metrics,cache}` — 横切治理与防护。
- `internal/database/{postgres.go,schema.go}` — 连接池与自带迁移（启动即建表）。

---

## 4. 旗舰能力深挖 · Signature Deep-Dives

### 4.1 认证：JWT + API Key 双轨 · Dual-track Auth

**中文：** 人类用户走 JWT —— `internal/auth/jwt.go` 用 HS256 签发访问令牌（15 分钟）与刷新令牌（7 天），`Claims` 区分 `access` / `refresh` 类型，防止刷新令牌被当访问令牌用。注册可要求邀请码（`internal/models/invite_code.go`）。程序调用走 API Key —— `internal/auth/apikey.go` 生成 `bhi_sk_` 前缀的 base62 密钥，数据库**只存 SHA-256 哈希**，明文仅在创建时返回一次。Key 还能绑定允许的模型、provider 与 IP 白名单（在 `withAPIKey` 闸门中校验，见 `router.go`）。

**English:** Humans use JWT — `internal/auth/jwt.go` issues HS256 access tokens (15 min) and refresh tokens (7 days), with a typed `Claims.Type` so a refresh token can never masquerade as an access token. Registration can require an invite code. Machines use API keys — `internal/auth/apikey.go` mints `bhi_sk_`-prefixed base62 keys and stores **only the SHA-256 hash**; the plaintext is shown exactly once at creation. Keys can be scoped to allowed models, providers, and an IP whitelist, all enforced in the `withAPIKey` gate.

### 4.2 密钥保险箱 · Encrypted Credential Vault

**中文：** Provider 密钥从不明文落库。`internal/crypto/encryption.go` 用 **AES-256-GCM**（32 字节密钥，以 64 位十六进制串通过 `BODHI_ENCRYPTION_KEY` 传入，启动时强制校验长度）加密后存入 `provider_credentials`。密钥可按用户存放，也可按用户组共享（`group_credentials`），代理转发时即时解密并注入到上游请求头/查询参数。

**English:** Provider secrets are never stored in cleartext. `internal/crypto/encryption.go` uses **AES-256-GCM** (a 32-byte key supplied as 64 hex chars via `BODHI_ENCRYPTION_KEY`, length-validated at boot) before writing to `provider_credentials`. Credentials can live per-user or be shared per-group (`group_credentials`); the proxy decrypts on the fly and injects them into the upstream request.

### 4.3 LLM 代理与 Provider 注册表 · LLM Proxy & Provider Registry

**中文：** `internal/proxy/providers/registry.go` 内置 5 个 provider，每个实现 `Provider` 接口（`Name` / `BuildURL` / `InjectAuth`）：

- `openai` — Bearer 鉴权，路径透传。
- `anthropic` — `x-api-key` + `anthropic-version: 2023-06-01`。
- `gemini` — 把 model 拼进 `:generateContent` / `:streamGenerateContent`，密钥走查询参数。
- `azure-openai` — `/openai/deployments/{model}/chat/completions` + `api-key` 头。
- `openai-compatible` — 适配 Ollama / vLLM / LM Studio 等任意 OpenAI 兼容端点。

代理入口有四条路由（`router.go`）：`/proxy/openai/`、`/proxy/anthropic/`、`/proxy/gemini/`、以及通用 `/proxy/v1/{path...}`。`internal/proxy/handler.go` 维护带 TTL 的凭据/路由/配额缓存与连接复用的 `http.Client`，并通过 `internal/proxy/sse.go` 处理流式响应。`internal/proxy/router.go` 的 `ResolveModel` 从模型注册表查出多实例，按 `priority` 排序并跳过近期失败实例，实现故障转移。

**English:** `internal/proxy/providers/registry.go` ships 5 providers, each implementing the `Provider` interface (`Name` / `BuildURL` / `InjectAuth`): `openai`, `anthropic`, `gemini`, `azure-openai`, and `openai-compatible` (for Ollama / vLLM / LM Studio and friends). Four proxy entry routes exist: `/proxy/openai/`, `/proxy/anthropic/`, `/proxy/gemini/`, and a universal `/proxy/v1/{path...}`. The handler keeps TTL caches for credentials, routes, and quota plus a connection-pooled `http.Client`, and streams SSE responses. `ResolveModel` pulls multiple instances from the model registry, orders them by `priority`, and skips recently failed instances for automatic failover.

### 4.4 计费与配额 · Billing & Quota

**中文：** 每次代理调用都会计量并写入 `usage_tracking`；`internal/pricing/calculator.go` 按 `model_pricing` 算出花费，`internal/quota/checker.go` 维护分钟/日/月维度的请求数、token 数与花费计数（`user_usage_counters`），对照 `user_quotas` / `group_quotas` 的 RPM、RPD、每日/每月 token 与花费上限放行或拒绝。用户可查当前用量、月度报表、导出 CSV（`/api/v1/billing/*`）；管理员可加余额、看汇总与单用户用量。

**English:** Every proxied call is metered into `usage_tracking`. `internal/pricing/calculator.go` computes cost from `model_pricing`, and `internal/quota/checker.go` tracks per-minute/day/month request counts, tokens, and spend (`user_usage_counters`), enforcing the RPM / RPD / daily & monthly token & spend caps from `user_quotas` / `group_quotas`. Users can view current usage, monthly reports, and export CSV (`/api/v1/billing/*`); admins can top up balances and inspect aggregate and per-user usage.

---

## 5. 快速开始 · Quick Start / Development

### Docker（推荐 · recommended）

`docker-compose.yml` 会同时拉起 PostgreSQL 16 与 bodhi-server（构建自本目录 `Dockerfile`，监听 `8080`）。两个加密相关变量是必填的：

```bash
# 32 字节 = 64 位十六进制，作为凭据加密密钥 / credential encryption key (32 bytes = 64 hex)
export BODHI_ENCRYPTION_KEY=$(openssl rand -hex 32)
export BODHI_JWT_SECRET=$(openssl rand -hex 32)
export BODHI_DB_PASSWORD=change-me   # 可选 optional, default "bodhi"

docker compose up --build
```

启动后服务在 `http://localhost:8080`，健康检查 `GET /health`。数据库表由 `internal/database/schema.go` 在启动时自动迁移创建。
Service comes up at `http://localhost:8080`; health probe is `GET /health`. Tables are auto-migrated on boot.

### 本地裸跑 · Run locally with Go

> 需要 Go（`go.mod` 声明 `go 1.25.0`；`Dockerfile` 构建镜像用 `golang:1.23-alpine`）和一个可达的 PostgreSQL。

```bash
export BODHI_DB_URL="postgres://bodhi:bodhi@localhost:5432/bodhi?sslmode=disable"
export BODHI_ENCRYPTION_KEY=$(openssl rand -hex 32)
export BODHI_JWT_SECRET=$(openssl rand -hex 32)

go run ./cmd/server      # 启动服务 / start the server
go build -o bodhi-server ./cmd/server   # 构建二进制 / build the binary
go test ./...            # 运行测试 / run tests
```

> 若 `cmd/server/web/dist` 存在并被嵌入，会自动提供管理面板并对非 API 路径走 SPA 回退；否则以纯 API 模式运行。
> If an embedded `web/dist` is present, the admin SPA is served with fallback routing; otherwise it runs API-only.

### 环境变量 · Environment Variables

| 变量 Var | 默认 Default | 说明 |
| --- | --- | --- |
| `BODHI_JWT_SECRET` | — (必填 required) | JWT 签名密钥 / JWT signing secret |
| `BODHI_ENCRYPTION_KEY` | — (必填，64 hex) | 凭据 AES-256-GCM 密钥 / credential encryption key |
| `BODHI_DB_URL` | `postgres://bodhi:bodhi@localhost:5432/bodhi?sslmode=disable` | PostgreSQL 连接串 |
| `BODHI_PORT` | `8080` | 监听端口 / listen port |
| `BODHI_BIND` | `0.0.0.0` | 监听地址 / bind address |
| `BODHI_PROXY_TIMEOUT` | `300` (秒 s) | 上游代理超时 / upstream proxy timeout |
| `BODHI_MAX_BODY_MB` | `50` | 请求体上限 / max request body (MB) |
| `BODHI_CORS_ORIGINS` | (空 empty) | CORS 允许来源，逗号分隔 / comma-separated origins |
| `BODHI_RATE_LIMIT_RPM` | `60` | 全局速率限制 RPM |
| `BODHI_RATE_LIMIT_BURST` | `10` | 速率限制突发量 / burst |
| `BODHI_VERSION` | `dev` | 上报的服务版本 / reported version |
| `BODHI_LOG_JSON` | `false` | 设为 `true` 输出 JSON 日志 / JSON logs |

### 关键路由速查 · Key Routes

```
GET  /health                          健康检查（公开 public）
GET  /metrics                         Prometheus 指标（仅管理员 admin-only）
POST /api/v1/auth/register            注册（可需邀请码 invite-gated）
POST /api/v1/auth/login               登录
POST /api/v1/auth/refresh             刷新令牌
GET  /api/v1/auth/me                  当前用户
POST /api/v1/keys                     创建 API Key
POST /api/v1/credentials              保存 provider 密钥（加密）
GET  /api/v1/models                   可用模型列表
GET  /api/v1/billing/current          当前用量
/proxy/openai/  /proxy/anthropic/  /proxy/gemini/  /proxy/v1/{path...}   LLM 代理（API Key）
/api/v1/admin/...                     用户/配额/定价/模型/实例/组/审计/Webhook/保留策略（管理员）
```

---

## 6. 其余拼图 · The Rest of the Stack

`bodhi-server` 是 **Zenith** 单仓中的后端服务。Part of the **Zenith** monorepo:

- **[bodhi](../bodhi)** — 桌面 AI 产品外壳（Tauri shell），最终用户的产品界面。
- **[lotus](../lotus)** — React + Vite 前端 UI 层；通过 HTTP / SSE 与 bamboo 通信（不直接调用本服务）。
- **[bamboo](../bamboo)** — 本地优先的 Rust agent 执行引擎；可携带 API Key 走本服务的 `/proxy/*`。
- **bodhi-server**（本模块 this module）— Go 后端：认证 / 持久化 / 计费配额 / LLM 代理。
- **[pavilion](../pavilion)** — 官网与文档站。
- **[Zenith 根目录 root](../)** — 单仓入口、子模块指针与发布列车。

> 架构提示 Note：Zenith 各前端与运行时之间走 **HTTP**，bodhi/Tauri 仅是外壳。本服务即为统一的后端 API + LLM 网关。
