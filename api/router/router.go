package router

import (
	"database/sql"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/bigduu/bodhi-server/api/handler"
	"github.com/bigduu/bodhi-server/api/middleware"
	"github.com/bigduu/bodhi-server/internal/auth"
	"github.com/bigduu/bodhi-server/internal/config"
	"github.com/bigduu/bodhi-server/internal/metrics"
	"github.com/bigduu/bodhi-server/internal/moderation"
	"github.com/bigduu/bodhi-server/internal/proxy"
	"github.com/bigduu/bodhi-server/internal/ratelimit"
	"github.com/bigduu/bodhi-server/internal/retention"
	"github.com/bigduu/bodhi-server/internal/security"
	"github.com/bigduu/bodhi-server/internal/webhook"
)

func Setup(db *sql.DB, cfg *config.Config, staticFiles fs.FS) http.Handler {
	mux := http.NewServeMux()

	cors := middleware.NewCORS(cfg.Server.CORSOrigins)
	handler.SetServerVersion(cfg.Server.Version)

	loginGuard := security.NewLoginGuard(5, 15*time.Minute)
	authHandler := handler.NewAuthHandler(db, cfg, loginGuard)
	ipLimiter := security.NewIPLimiter(20, 5)
	ipLimiter.StartCleanup()
	keyHandler := handler.NewKeyHandler(db)
	credHandler := handler.NewCredentialHandler(db, cfg)
	webhookDispatcher := webhook.NewDispatcher(db, 128)
	contentFilter := moderation.NewFilter(db)
	proxyHandler := proxy.NewHandler(db, cfg, webhookDispatcher, contentFilter)
	adminHandler := handler.NewAdminHandler(db)
	settingsHandler := handler.NewSettingsHandler(db)
	modelHandler := handler.NewModelHandler(db)
	instanceHandler := handler.NewInstanceHandler(db)
	billingHandler := handler.NewBillingHandler(db)
	groupHandler := handler.NewGroupHandler(db, cfg)
	auditHandler := handler.NewAuditHandler(db)
	contentHandler := handler.NewContentHandler(db)
	webhookHandler := handler.NewWebhookHandler(db, webhookDispatcher)
	retentionPurger := retention.NewPurger(db)
	retentionHandler := handler.NewRetentionHandler(db, retentionPurger)
	versionHandler := handler.NewVersionHandler(db)

	// Health
	mux.HandleFunc("GET /health", handler.Health)

	// Prometheus metrics (admin-only; must not be publicly exposed)
	mux.HandleFunc("GET /metrics", withAdmin(db, cfg, metrics.ServeHTTP))

	// Version check (public, no auth)
	mux.HandleFunc("GET /api/v1/version/latest", versionHandler.CheckUpdate)

	// Auth (public)
	mux.HandleFunc("POST /api/v1/auth/register", ipLimiter.Middleware(authHandler.Register))
	mux.HandleFunc("POST /api/v1/auth/login", ipLimiter.Middleware(authHandler.Login))
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("GET /api/v1/auth/me", withJWT(cfg, authHandler.Me))

	// API Keys (authenticated)
	mux.HandleFunc("POST /api/v1/keys", withJWT(cfg, keyHandler.Create))
	mux.HandleFunc("GET /api/v1/keys", withJWT(cfg, keyHandler.List))
	mux.HandleFunc("DELETE /api/v1/keys/{id}", withJWT(cfg, func(w http.ResponseWriter, r *http.Request) {
		keyHandler.Delete(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/v1/keys/{id}/rotate", withJWT(cfg, func(w http.ResponseWriter, r *http.Request) {
		keyHandler.Rotate(w, r, r.PathValue("id"))
	}))

	// Credentials (authenticated)
	mux.HandleFunc("POST /api/v1/credentials", withJWT(cfg, credHandler.Create))
	mux.HandleFunc("GET /api/v1/credentials", withJWT(cfg, credHandler.List))
	mux.HandleFunc("PUT /api/v1/credentials/{provider}", withJWT(cfg, func(w http.ResponseWriter, r *http.Request) {
		credHandler.Update(w, r, r.PathValue("provider"))
	}))
	mux.HandleFunc("DELETE /api/v1/credentials/{provider}", withJWT(cfg, func(w http.ResponseWriter, r *http.Request) {
		credHandler.Delete(w, r, r.PathValue("provider"))
	}))

	// Admin (admin only)
	mux.HandleFunc("GET /api/v1/admin/users", withAdmin(db, cfg, adminHandler.ListUsers))
	mux.HandleFunc("GET /api/v1/admin/users/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.GetUser(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/users/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.UpdateUser(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.DeleteUser(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /api/v1/admin/users/{id}/keys", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.ListUserKeys(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}/keys/{keyId}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.DeleteUserKey(w, r, r.PathValue("id"), r.PathValue("keyId"))
	}))
	mux.HandleFunc("GET /api/v1/admin/users/{id}/credentials", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.ListUserCredentials(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}/credentials/{provider}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.DeleteUserCredential(w, r, r.PathValue("id"), r.PathValue("provider"))
	}))
	mux.HandleFunc("GET /api/v1/admin/usage/summary", withAdmin(db, cfg, adminHandler.UsageSummary))
	mux.HandleFunc("GET /api/v1/admin/usage/users/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.UserUsage(w, r, r.PathValue("id"))
	}))

	// Quota management
	mux.HandleFunc("GET /api/v1/admin/users/{id}/quota", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.GetQuota(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/users/{id}/quota", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		adminHandler.SetQuota(w, r, r.PathValue("id"))
	}))

	// Metrics
	mux.HandleFunc("GET /api/v1/admin/metrics/latency", withAdmin(db, cfg, adminHandler.LatencyStats))
	mux.HandleFunc("GET /api/v1/admin/metrics/tokens", withAdmin(db, cfg, adminHandler.TokenUsage))
	mux.HandleFunc("GET /api/v1/admin/metrics/errors", withAdmin(db, cfg, adminHandler.ErrorLogs))

	// Pricing
	mux.HandleFunc("GET /api/v1/admin/pricing", withAdmin(db, cfg, adminHandler.ListPricing))
	mux.HandleFunc("PUT /api/v1/admin/pricing", withAdmin(db, cfg, adminHandler.UpdatePricing))

	// Settings (admin)
	mux.HandleFunc("GET /api/v1/admin/settings", withAdmin(db, cfg, settingsHandler.ListSettings))
	mux.HandleFunc("PUT /api/v1/admin/settings/{key}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		settingsHandler.UpdateSetting(w, r, r.PathValue("key"))
	}))
	mux.HandleFunc("POST /api/v1/admin/invites", withAdmin(db, cfg, settingsHandler.CreateInvite))
	mux.HandleFunc("GET /api/v1/admin/invites", withAdmin(db, cfg, settingsHandler.ListInvites))
	mux.HandleFunc("DELETE /api/v1/admin/invites/{code}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		settingsHandler.DeleteInvite(w, r, r.PathValue("code"))
	}))

	// Model Registry (admin)
	mux.HandleFunc("GET /api/v1/admin/models", withAdmin(db, cfg, modelHandler.ListModels))
	mux.HandleFunc("POST /api/v1/admin/models", withAdmin(db, cfg, modelHandler.CreateModel))
	mux.HandleFunc("PUT /api/v1/admin/models/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		modelHandler.UpdateModel(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/models/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		modelHandler.DeleteModel(w, r, r.PathValue("id"))
	}))

	// Public model list (for users)
	mux.HandleFunc("GET /api/v1/models", withJWT(cfg, modelHandler.ListActiveModels))

	// LLM Proxy (API key auth)
	mux.HandleFunc("/proxy/openai/", withAPIKey(db, proxyHandler.ProxyOpenAI))
	mux.HandleFunc("/proxy/anthropic/", withAPIKey(db, proxyHandler.ProxyAnthropic))
	mux.HandleFunc("/proxy/gemini/", withAPIKey(db, proxyHandler.ProxyGemini))
	mux.HandleFunc("/proxy/v1/{path...}", withAPIKey(db, proxyHandler.ProxyUniversal))

	// Billing (authenticated)
	mux.HandleFunc("GET /api/v1/billing/current", withJWT(cfg, billingHandler.CurrentUsage))
	mux.HandleFunc("GET /api/v1/billing/reports", withJWT(cfg, func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-ID")
		billingHandler.ListReports(w, r, userID)
	}))
	mux.HandleFunc("GET /api/v1/billing/reports/{year}/{month}", withJWT(cfg, func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-ID")
		billingHandler.MonthlyReport(w, r, userID)
	}))
	mux.HandleFunc("GET /api/v1/billing/reports/{year}/{month}/csv", withJWT(cfg, func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-ID")
		billingHandler.ExportCSV(w, r, userID)
	}))
	mux.HandleFunc("POST /api/v1/admin/billing/balance", withAdmin(db, cfg, billingHandler.AddBalance))

	// Groups (admin)
	mux.HandleFunc("POST /api/v1/admin/groups", withAdmin(db, cfg, groupHandler.CreateGroup))
	mux.HandleFunc("GET /api/v1/admin/groups", withAdmin(db, cfg, groupHandler.ListGroups))
	mux.HandleFunc("GET /api/v1/admin/groups/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.GetGroup(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/groups/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.UpdateGroup(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/groups/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.DeleteGroup(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /api/v1/admin/groups/{id}/members", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.ListMembers(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/v1/admin/groups/{id}/members", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.AddMember(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/groups/{id}/members/{userId}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.RemoveMember(w, r, r.PathValue("id"), r.PathValue("userId"))
	}))
	mux.HandleFunc("GET /api/v1/admin/groups/{id}/credentials", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.ListCredentials(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/groups/{id}/credentials/{provider}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.SetCredential(w, r, r.PathValue("id"), r.PathValue("provider"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/groups/{id}/credentials/{provider}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.DeleteCredential(w, r, r.PathValue("id"), r.PathValue("provider"))
	}))
	mux.HandleFunc("GET /api/v1/admin/groups/{id}/quota", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.GetQuota(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/groups/{id}/quota", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		groupHandler.SetQuota(w, r, r.PathValue("id"))
	}))

	// Provider Instances (admin)
	mux.HandleFunc("GET /api/v1/admin/instances", withAdmin(db, cfg, instanceHandler.ListInstances))
	mux.HandleFunc("POST /api/v1/admin/instances", withAdmin(db, cfg, instanceHandler.CreateInstance))
	mux.HandleFunc("PUT /api/v1/admin/instances/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		instanceHandler.UpdateInstance(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/instances/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		instanceHandler.DeleteInstance(w, r, r.PathValue("id"))
	}))

	// Audit Log (admin)
	mux.HandleFunc("GET /api/v1/admin/audit", withAdmin(db, cfg, auditHandler.ListLogs))

	// Content Rules (admin)
	mux.HandleFunc("GET /api/v1/admin/content-rules", withAdmin(db, cfg, contentHandler.ListRules))
	mux.HandleFunc("POST /api/v1/admin/content-rules", withAdmin(db, cfg, contentHandler.CreateRule))
	mux.HandleFunc("DELETE /api/v1/admin/content-rules/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		contentHandler.DeleteRule(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/content-rules/{id}/toggle", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		contentHandler.ToggleRule(w, r, r.PathValue("id"))
	}))

	// Webhooks (admin)
	mux.HandleFunc("GET /api/v1/admin/webhooks", withAdmin(db, cfg, webhookHandler.List))
	mux.HandleFunc("POST /api/v1/admin/webhooks", withAdmin(db, cfg, webhookHandler.Create))
	mux.HandleFunc("DELETE /api/v1/admin/webhooks/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		webhookHandler.Delete(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/webhooks/{id}/toggle", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		webhookHandler.Toggle(w, r, r.PathValue("id"))
	}))

	// Retention policies (admin)
	mux.HandleFunc("GET /api/v1/admin/retention", withAdmin(db, cfg, retentionHandler.ListPolicies))
	mux.HandleFunc("PUT /api/v1/admin/retention/{table}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		retentionHandler.UpdatePolicy(w, r, r.PathValue("table"))
	}))
	mux.HandleFunc("POST /api/v1/admin/retention/purge", withAdmin(db, cfg, retentionHandler.TriggerPurge))

	// Version management (admin)
	mux.HandleFunc("GET /api/v1/admin/versions", withAdmin(db, cfg, versionHandler.ListVersions))
	mux.HandleFunc("POST /api/v1/admin/versions", withAdmin(db, cfg, versionHandler.CreateVersion))
	mux.HandleFunc("GET /api/v1/admin/versions/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		versionHandler.GetVersion(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("PUT /api/v1/admin/versions/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		versionHandler.UpdateVersion(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/v1/admin/versions/{id}/publish", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		versionHandler.PublishVersion(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/v1/admin/versions/{id}/unpublish", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		versionHandler.UnpublishVersion(w, r, r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/v1/admin/versions/{id}", withAdmin(db, cfg, func(w http.ResponseWriter, r *http.Request) {
		versionHandler.DeleteVersion(w, r, r.PathValue("id"))
	}))

	fileServer := http.FileServer(http.FS(staticFiles))

	rl := ratelimit.New(cfg.RateLimit.RPM, cfg.RateLimit.Burst)
	retentionPurger.Start(1 * time.Hour)

	handler := middleware.RequestID(cors.Handler(rl.Middleware(mux)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/proxy/") ||
			r.URL.Path == "/health" ||
			r.URL.Path == "/metrics" {
			handler.ServeHTTP(w, r)
			return
		}

		staticR := r.Clone(r.Context())
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(staticFiles, path); err != nil {
				if strings.HasPrefix(r.URL.Path, "/assets/") {
					http.NotFound(w, r)
					return
				}
				staticR.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(w, staticR)
	})
}

func withJWT(cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := auth.ExtractBearer(r)
		if tokenStr == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}
		claims, err := auth.ValidateToken(tokenStr, cfg.Auth.JWTSecret, auth.AccessToken)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-User-ID", claims.Subject)
		r.Header.Set("X-Username", claims.Username)
		next(w, r)
	}
}

func withAdmin(db *sql.DB, cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := auth.ExtractBearer(r)
		if tokenStr == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}
		claims, err := auth.ValidateToken(tokenStr, cfg.Auth.JWTSecret, auth.AccessToken)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		var isAdmin bool
		err = db.QueryRowContext(r.Context(),
			`SELECT is_admin FROM users WHERE id = $1::uuid AND is_active = true`,
			claims.Subject,
		).Scan(&isAdmin)
		if err != nil || !isAdmin {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		r.Header.Set("X-User-ID", claims.Subject)
		r.Header.Set("X-Username", claims.Username)
		next(w, r)
	}
}

func withAPIKey(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := auth.ExtractBearer(r)
		if key == "" || !auth.IsBodhiAPIKey(key) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		keyHash := auth.HashAPIKey(key)
		var userID, apiKeyID string
		var isActive bool
		var allowedModels, allowedProviders, ipWhitelist string
		err := db.QueryRowContext(r.Context(),
			`SELECT u.id::text, ak.id::text, ak.is_active,
			        COALESCE(ak.allowed_models::text, '{}'), COALESCE(ak.allowed_providers::text, '{}'), COALESCE(ak.ip_whitelist::text, '{}')
			 FROM api_keys ak
			 JOIN users u ON ak.user_id = u.id
			 WHERE ak.key_hash = $1 AND u.is_active = true`,
			keyHash,
		).Scan(&userID, &apiKeyID, &isActive, &allowedModels, &allowedProviders, &ipWhitelist)
		if err != nil || !isActive {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// IP whitelist check
		ipList := parsePgTextArray(ipWhitelist)
		if len(ipList) > 0 {
			clientIP := r.Header.Get("X-Real-IP")
			if clientIP == "" {
				clientIP, _, _ = net.SplitHostPort(r.RemoteAddr)
			}
			if !matchIPWhitelist(clientIP, ipList) {
				http.Error(w, `{"error":"ip not allowed"}`, http.StatusForbidden)
				return
			}
		}

		db.ExecContext(r.Context(), `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1::uuid`, apiKeyID)
		r.Header.Set("X-User-ID", userID)
		r.Header.Set("X-API-Key-ID", apiKeyID)
		r.Header.Set("X-Key-Allowed-Models", allowedModels)
		r.Header.Set("X-Key-Allowed-Providers", allowedProviders)
		next(w, r)
	}
}

func parsePgTextArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "{}" || s == "" {
		return nil
	}
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, strings.Trim(p, `"`))
		}
	}
	return result
}

func matchIPWhitelist(clientIP string, whitelist []string) bool {
	ip := net.ParseIP(clientIP)
	for _, entry := range whitelist {
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err == nil && cidr.Contains(ip) {
				return true
			}
		} else {
			if entry == clientIP {
				return true
			}
		}
	}
	return false
}

// NOTE: Token extraction is centralized in internal/auth.ExtractBearer.
//
// The withJWT / withAdmin / withAPIKey gates below intentionally remain inline
// rather than delegating to auth.JWTAuthMiddleware / auth.APIKeyAuthMiddleware:
// those middlewares propagate identity via request context, whereas every
// downstream handler in this codebase reads identity from request headers
// (X-User-ID, X-Username, X-API-Key-ID, etc.). The inline withAPIKey also
// enforces additional behavior the auth middleware lacks (IP whitelist,
// allowed_models / allowed_providers propagation). Consolidating those would
// change auth behavior, so it is deferred; only the duplicated bearer-token
// extraction has been unified here.
