package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyProviderHeadersAcrossFormats(t *testing.T) {
	const chatSSE = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n"
	const responsesSSE = "event: response.completed\r\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":8,\"output_tokens\":3}}}\r\n\r\n"
	const geminiSSE = "data: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":3}}\n\n"
	for _, route := range []struct {
		name, provider, path, body, sse string
	}{
		{"chat completions", "openai", "/v1/chat/completions?trace=fixture", `{"model":"fixture-model","messages":[{"role":"user","content":"Hello"}],"stream":true}`, chatSSE},
		{"responses", "openai", "/v1/responses?trace=fixture", `{"model":"fixture-model","input":"Hello","stream":true}`, responsesSSE},
		{"gemini", "gemini", "/v1beta/models/fixture-model:streamGenerateContent?alt=sse&key=client-fixture-key", `{"contents":[{"parts":[{"text":"Hello"}]}],"generationConfig":{"thinkingConfig":{"thinkingLevel":"LOW"}}}`, geminiSSE},
	} {
		for _, status := range []int{http.StatusOK, http.StatusTooManyRequests} {
			t.Run(route.name+"/"+http.StatusText(status), func(t *testing.T) {
				body := route.sse
				contentType := "text/event-stream"
				if status == http.StatusTooManyRequests {
					body, contentType = `{"error":{"message":"fixture rate limit"}}`, "application/json"
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					bodyIn, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					if route.name == "chat completions" {
						var payload map[string]json.RawMessage
						if json.Unmarshal(bodyIn, &payload) != nil || string(payload["stream_options"]) != `{"include_usage":true}` {
							t.Errorf("expected Chat Completions usage options, got %s", bodyIn)
						}
					} else if string(bodyIn) != route.body {
						t.Errorf("native request was changed: %s", bodyIn)
					}
					if route.provider == "gemini" {
						if r.URL.Query().Get("alt") != "sse" || r.URL.Query().Get("key") != "upstream-fixture-key" || r.Header.Get("Authorization") != "" {
							t.Error("Gemini did not preserve SSE negotiation and use its own credential")
						}
					} else if r.URL.Query().Get("trace") != "fixture" || r.Header.Get("Authorization") != "Bearer upstream-fixture-key" {
						t.Error("OpenAI did not preserve query and use its own credential")
					}
					for name, expected := range map[string]string{
						"User-Agent": "Bamboo/fixture", "X-Request-ID": "caller-trace-fixture", "X-Provider-Trace": "trace-fixture",
						"X-Goog-Api-Key": "", "X-Api-Key": "", "Cookie": "", "X-Hop-Only": "", "X-User-ID": "", "Anthropic-Beta": "",
					} {
						if r.Header.Get(name) != expected {
							t.Errorf("unexpected request header %s", name)
						}
					}
					w.Header().Set("Content-Type", contentType)
					w.Header().Set("X-Request-ID", "req_fixture")
					w.Header().Set("X-Litellm-Call-Id", "call_fixture")
					w.Header().Set("X-Ratelimit-Remaining-Requests", "4")
					w.Header().Set("Connection", "X-Upstream-Hop")
					w.Header().Set("X-Upstream-Hop", "hop-only")
					w.Header().Set("Set-Cookie", "session=upstream-fixture")
					if status == http.StatusTooManyRequests {
						w.Header().Set("Retry-After", "9")
					}
					w.WriteHeader(status)
					_, _ = io.WriteString(w, body)
				}))
				defer upstream.Close()

				handler, userID, recorded := newProxyFixture(t, route.provider, upstream)
				request := httptest.NewRequest(http.MethodPost, "/proxy/"+route.provider+route.path, strings.NewReader(route.body))
				for name, value := range map[string]string{
					"User-Agent": "Bamboo/fixture", "X-Request-ID": "caller-trace-fixture", "X-Provider-Trace": "trace-fixture",
					"Authorization": "Bearer gateway-fixture-key", "X-Goog-Api-Key": "client-fixture-key", "X-Api-Key": "client-fixture-key",
					"Content-Type": "application/json", "Cookie": "session=client-fixture", "X-User-ID": userID,
					"X-Key-Allowed-Models": "{fixture-model}", "Connection": "X-Hop-Only", "X-Hop-Only": "hop-only", "Anthropic-Beta": "claude-only",
				} {
					request.Header.Set(name, value)
				}
				response := httptest.NewRecorder()
				handler.proxyProvider(response, request, route.provider)
				headers := response.Result().Header
				if response.Code != status || response.Flushed != (status == http.StatusOK) || headers.Get("Content-Type") != contentType ||
					headers.Get("X-Request-ID") != "req_fixture" || headers.Get("X-Litellm-Call-Id") != "call_fixture" ||
					headers.Get("X-Ratelimit-Remaining-Requests") != "4" || headers.Get("Connection") != "" ||
					headers.Get("X-Upstream-Hop") != "" || headers.Get("Set-Cookie") != "" {
					t.Errorf("unexpected response HTTP %d with headers %v", response.Code, headers)
				}
				if route.name != "chat completions" || status != http.StatusOK {
					if response.Body.String() != body {
						t.Errorf("response changed: %s", response.Body.String())
					}
				}
				if status == http.StatusOK {
					if len(recorded.usage) < 9 || recorded.usage[3].Value != "fixture-model" ||
						recorded.usage[5].Value != int64(8) || recorded.usage[6].Value != int64(3) {
						t.Errorf("usage was not recorded for the requested model: %v", recorded.usage)
					}
				} else if headers.Get("Retry-After") != "9" {
					t.Errorf("upstream rate limit hint missing: %v", headers)
				}
			})
		}
	}
}

func TestNativeInferenceTextExtraction(t *testing.T) {
	if text := extractGeminiText([]byte(`{"systemInstruction":{"parts":[{"text":"system"}]},"contents":[{"parts":[{"text":"message"},{"functionResponse":{"response":{"output":"tool result"}}}]}]}`)); text != "system message tool result" {
		t.Errorf("unexpected Gemini input: %q", text)
	}
	if text := extractResponsesText([]byte(`{"instructions":"system","input":[{"role":"user","content":[{"type":"input_text","text":"message"}]},{"type":"function_call_output","output":"tool result"}]}`)); text != "system message tool result" {
		t.Errorf("unexpected Responses input: %q", text)
	}
}
