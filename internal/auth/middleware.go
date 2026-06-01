package auth

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

type contextKey string

const (
	ContextKeyUserID   contextKey = "user_id"
	ContextKeyUsername contextKey = "username"
	ContextKeyAPIKeyID contextKey = "api_key_id"
)

func JWTAuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := ValidateToken(tokenStr, jwtSecret, AccessToken)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.Subject)
			ctx = context.WithValue(ctx, ContextKeyUsername, claims.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func APIKeyAuthMiddleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := extractBearerToken(r)
			if key == "" || !IsBodhiAPIKey(key) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			keyHash := HashAPIKey(key)

			var userID, apiKeyID string
			var isActive bool
			err := db.QueryRowContext(r.Context(),
				`SELECT ak.user_id, ak.id::text, ak.is_active
				 FROM api_keys ak
				 JOIN users u ON ak.user_id = u.id::text
				 WHERE ak.key_hash = $1 AND u.is_active = true`,
				keyHash,
			).Scan(&userID, &apiKeyID, &isActive)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if !isActive {
				http.Error(w, `{"error":"api key revoked"}`, http.StatusUnauthorized)
				return
			}

			db.ExecContext(r.Context(),
				`UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`,
				apiKeyID,
			)

			ctx := context.WithValue(r.Context(), ContextKeyUserID, userID)
			ctx = context.WithValue(ctx, ContextKeyAPIKeyID, apiKeyID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return auth
}

// ExtractBearer returns the token from an "Authorization: Bearer <token>"
// header, or "" if the header is missing or not a bearer token. Unlike
// extractBearerToken it does NOT fall back to the raw header value.
func ExtractBearer(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return v
	}
	return ""
}

func UsernameFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyUsername).(string); ok {
		return v
	}
	return ""
}

func APIKeyIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyAPIKeyID).(string); ok {
		return v
	}
	return ""
}
