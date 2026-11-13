package proxy

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bigduu/bodhi-server/internal/cache"
	"github.com/bigduu/bodhi-server/internal/config"
	"github.com/bigduu/bodhi-server/internal/crypto"
	"github.com/bigduu/bodhi-server/internal/models"
)

type usageRecorderDB struct {
	usage []driver.NamedValue
}

func (db *usageRecorderDB) Open(string) (driver.Conn, error)             { return db, nil }
func (db *usageRecorderDB) Connect(context.Context) (driver.Conn, error) { return db, nil }
func (db *usageRecorderDB) Driver() driver.Driver                        { return db }
func (db *usageRecorderDB) Prepare(string) (driver.Stmt, error)          { return nil, driver.ErrSkip }
func (db *usageRecorderDB) Begin() (driver.Tx, error)                    { return nil, driver.ErrSkip }
func (db *usageRecorderDB) Close() error                                 { return nil }

func (db *usageRecorderDB) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO usage_tracking") {
		db.usage = append([]driver.NamedValue(nil), args...)
	}
	return driver.RowsAffected(1), nil
}

func TestProxyAnthropicPreservesNativeMessagesAndHeaders(t *testing.T) {
	const requestBody = `{"model":"claude-test","stream":true,"max_tokens":64,"system":[{"type":"text","text":"Fixture","cache_control":{"type":"ephemeral","ttl":"1h"}}],"thinking":{"type":"adaptive"},"tools":[{"name":"weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}],"messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`
	streamBody := "event: message_start\r\ndata: {\"type\":\"message_start\",\r\ndata: \"message\":{\"usage\":{\"input_tokens\":12}}}\r\n\r\n" +
		":" + strings.Repeat("x", 64*1024) + "\r\n\r\nevent: message_delta\r\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\r\n\r\n" +
		"event: message_stop\r\ndata: {\"type\":\"message_stop\"}\r\n\r\n"
	const errorBody = `{"type":"error","error":{"type":"rate_limit_error","message":"fixture"}}`
	const messageBody = `{"type":"message","role":"assistant","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":9,"output_tokens":4}}`

	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		response    string
		input       int64
		output      int64
	}{
		{"SSE", http.StatusOK, "text/event-stream", streamBody, 12, 3},
		{"JSON", http.StatusOK, "application/json", messageBody, 9, 4},
		{"rate limit", http.StatusTooManyRequests, "application/json", errorBody, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type capturedRequest struct {
				header http.Header
				uri    string
				body   string
			}
			captured := make(chan capturedRequest, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				captured <- capturedRequest{r.Header.Clone(), r.URL.RequestURI(), string(body)}
				w.Header().Set("Request-Id", "req_fixture")
				w.Header().Set("Anthropic-Ratelimit-Requests-Remaining", "42")
				w.Header().Set("X-Litellm-Call-Id", "call_fixture")
				w.Header().Set("Set-Cookie", "session=upstream-private")
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("Connection", "X-Upstream-Hop")
				w.Header().Set("X-Upstream-Hop", "hop-only")
				if tc.status == http.StatusTooManyRequests {
					w.Header().Set("Retry-After", "7")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer upstream.Close()

			recorded := &usageRecorderDB{}
			db := sql.OpenDB(recorded)
			defer db.Close()
			cfg := &config.Config{}
			cfg.Auth.EncryptionKey = strings.Repeat("a", 64)
			const userID = "29fd5c19-a8c7-4f88-9a7e-26b55c36c6b8"
			key, err := crypto.Encrypt("upstream-fixture-key", cfg.Auth.EncryptionKey)
			if err != nil {
				t.Fatal(err)
			}
			handler := &Handler{db: db, cfg: cfg, client: upstream.Client(), credCache: cache.NewTTLCache(time.Minute)}
			cfg.Server.MaxBodyBytes = 4096
			handler.credCache.Set("cred:"+userID+":anthropic", &models.ProviderCredential{
				EncryptedAPIKey: key, BaseURL: sql.NullString{String: upstream.URL, Valid: true},
			})

			request := httptest.NewRequest(http.MethodPost, "/proxy/anthropic/v1/messages?beta=true", strings.NewReader(requestBody))
			request.Header.Set("Authorization", "Bearer gateway-fixture-key")
			request.Header.Set("X-Api-Key", "caller-provider-key")
			request.Header.Set("X-User-Id", userID)
			request.Header.Set("X-Api-Key-Id", "gateway-key-id")
			request.Header.Set("X-Key-Allowed-Providers", "{anthropic}")
			request.Header.Set("User-Agent", "Fixture/2.0")
			request.Header.Set("Anthropic-Version", "2026-10-21")
			request.Header.Add("Anthropic-Beta", "feature-one")
			request.Header.Add("Anthropic-Beta", "feature-two")
			request.Header.Set("X-Custom-Provider", "configured")
			request.Header.Set("Connection", "X-Hop-Only")
			request.Header.Set("X-Hop-Only", "hop-only")
			request.Header.Set("Cookie", "session=client-private")
			request.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			handler.ProxyAnthropic(result, request)

			select {
			case sent := <-captured:
				if sent.uri != "/v1/messages?beta=true" || sent.body != requestBody {
					t.Errorf("upstream path or body changed: %s %s", sent.uri, sent.body)
				}
				for name, want := range map[string]string{
					"User-Agent": "Fixture/2.0", "Anthropic-Version": "2026-10-21", "X-Api-Key": "upstream-fixture-key",
					"X-Custom-Provider": "configured",
				} {
					if got := sent.header.Get(name); got != want {
						t.Errorf("upstream %s = %q, want %q", name, got, want)
					}
				}
				if beta := strings.Join(sent.header.Values("Anthropic-Beta"), ","); beta != "feature-one,feature-two" {
					t.Errorf("upstream beta values changed: %q", beta)
				}
				for _, name := range []string{"Authorization", "X-User-Id", "X-Api-Key-Id", "X-Key-Allowed-Providers", "Cookie", "Connection", "X-Hop-Only"} {
					if sent.header.Get(name) != "" {
						t.Errorf("client header reached upstream: %s", name)
					}
				}
			default:
				t.Fatal("no upstream request was made")
			}

			headers := result.Result().Header
			if result.Code != tc.status || headers.Get("Content-Type") != tc.contentType || headers.Get("Request-Id") != "req_fixture" ||
				headers.Get("Anthropic-Ratelimit-Requests-Remaining") != "42" ||
				headers.Get("X-Litellm-Call-Id") != "call_fixture" {
				t.Errorf("unexpected downstream response: HTTP %d %v", result.Code, headers)
			}
			for _, name := range []string{"Set-Cookie", "Connection", "X-Upstream-Hop"} {
				if headers.Get(name) != "" {
					t.Errorf("upstream header reached caller: %s", name)
				}
			}
			if result.Body.String() != tc.response || result.Flushed != (tc.contentType == "text/event-stream") {
				t.Errorf("native response changed: %q", result.Body.String())
			}
			if len(recorded.usage) < 9 || recorded.usage[5].Value != tc.input || recorded.usage[6].Value != tc.output {
				t.Errorf("native usage was not recorded: %v", recorded.usage)
			}
			if tc.status == http.StatusTooManyRequests && headers.Get("Retry-After") != "7" {
				t.Errorf("rate limit hint missing: %v", headers)
			}
		})
	}
}

func TestProxyAnthropicRejectsNonInferenceEndpoint(t *testing.T) {
	result := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/proxy/anthropic/v1/messages/batches", nil)
	(&Handler{}).ProxyAnthropic(result, request)
	if result.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", result.Code, http.StatusNotFound)
	}
}
