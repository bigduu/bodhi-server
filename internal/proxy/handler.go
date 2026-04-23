package proxy

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bigduu/bodhi-server/internal/cache"
	"github.com/bigduu/bodhi-server/internal/config"
	"github.com/bigduu/bodhi-server/internal/converter"
	"github.com/bigduu/bodhi-server/internal/crypto"
	"github.com/bigduu/bodhi-server/internal/metrics"
	"github.com/bigduu/bodhi-server/internal/models"
	"github.com/bigduu/bodhi-server/internal/moderation"
	"github.com/bigduu/bodhi-server/internal/pricing"
	"github.com/bigduu/bodhi-server/internal/quota"
)

type Dispatcher interface {
	Dispatch(event string, data interface{})
}

type Handler struct {
	db         *sql.DB
	cfg        *config.Config
	client     *http.Client
	dispatcher Dispatcher
	filter     *moderation.Filter
	credCache  *cache.TTLCache
	routeCache *cache.TTLCache
	quotaCache *cache.TTLCache
}

func NewHandler(db *sql.DB, cfg *config.Config, dispatcher Dispatcher, filter *moderation.Filter) *Handler {
	credCache := cache.NewTTLCache(60 * time.Second)
	routeCache := cache.NewTTLCache(30 * time.Second)
	quotaCache := cache.NewTTLCache(30 * time.Second)
	pricingCache := cache.NewTTLCache(5 * time.Minute)

	credCache.StartCleanup()
	routeCache.StartCleanup()
	quotaCache.StartCleanup()
	pricingCache.StartCleanup()

	pricing.InitCache(pricingCache)
	quota.InitCache(quotaCache)
	cache.RegisterCaches(credCache, routeCache, quotaCache)

	return &Handler{
		db:         db,
		cfg:        cfg,
		dispatcher: dispatcher,
		filter:     filter,
		credCache:  credCache,
		routeCache: routeCache,
		quotaCache: quotaCache,
		client: &http.Client{
			Timeout: h.cfg.Server.ProxyTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (h *Handler) ProxyOpenAI(w http.ResponseWriter, r *http.Request) {
	h.proxyProvider(w, r, "openai")
}

func (h *Handler) ProxyAnthropic(w http.ResponseWriter, r *http.Request) {
	h.proxyProvider(w, r, "anthropic")
}

func (h *Handler) ProxyGemini(w http.ResponseWriter, r *http.Request) {
	h.proxyProvider(w, r, "gemini")
}

// ProxyUniversal resolves the model from the request body and auto-routes through the model registry.
func (h *Handler) ProxyUniversal(w http.ResponseWriter, r *http.Request) {
	h.proxyWithRouting(w, r)
}

func (h *Handler) proxyWithRouting(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	userID := r.Header.Get("X-User-ID")
	apiKeyID := r.Header.Get("X-API-Key-ID")

	body, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.Server.MaxBodyBytes))
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	model := extractModel(body)
	if model == "" {
		http.Error(w, `{"error":"model field required"}`, http.StatusBadRequest)
		return
	}

	// Content moderation: check input
	if h.filter != nil {
		inputText := extractMessagesText(body)
		if inputText != "" {
			if result := h.filter.CheckInput(r.Context(), inputText); result != nil {
				if result.Action == "block" {
					slog.Warn("content blocked", "rule", result.Rule, "user_id", userID)
					http.Error(w, `{"error":"request blocked by content policy: `+result.Rule+`"}`, http.StatusForbidden)
					return
				}
			}
		}
	}

	// API key restriction check
	if !checkKeyRestrictions(w, r, model, "") {
		return
	}

	// Quota check
	if quotaErr := quota.CheckLimit(r.Context(), h.db, userID, model); quotaErr != nil {
		quota.IncrementRequest(r.Context(), h.db, userID)
		http.Error(w, `{"error":"`+quotaErr.Error()+`"}`, http.StatusTooManyRequests)
		return
	}
	quota.IncrementRequest(r.Context(), h.db, userID)

	// Try model registry routing (with cache)
	routeKey := "route:" + model
	var targets []ProviderTarget
	if v, ok := h.routeCache.Get(routeKey); ok {
		targets = v.([]ProviderTarget)
	} else {
		router := NewProviderRouter(h.db)
		targets, _ = router.ResolveModel(r.Context(), model)
		if targets == nil {
			targets = []ProviderTarget{}
		}
		h.routeCache.Set(routeKey, targets)
	}

	if len(targets) > 0 {
		// Try each target in priority order with fallback
		for i, target := range targets {
			provider := target.ProviderType

			cred, credErr := h.resolveCredential(r, userID, provider)
			if credErr != nil {
				continue // User has no credential for this provider, try next
			}

			realKey, decErr := crypto.Decrypt(cred.EncryptedAPIKey, h.cfg.Auth.EncryptionKey)
			if decErr != nil {
				continue
			}

			// Convert body for this provider
			needsConversion := provider != "openai"
			convertedBody := body
			if needsConversion {
				convertedBody, err = h.convertRequest(body, provider)
				if err != nil {
					continue
				}
			}

			isStreaming := false
			var payload map[string]interface{}
			if json.Unmarshal(body, &payload) == nil {
				if s, ok := payload["stream"].(bool); ok {
					isStreaming = s
				}
			}

			if provider == "openai" && isStreaming {
				convertedBody = converter.InjectStreamOptions(convertedBody)
			}

			// Build URL — use instance base_url or credential base_url
			baseURL := target.BaseURL
			if baseURL == "" {
				baseURL = providerDefaultBaseURL(provider)
			}
			if cred.BaseURL.Valid && cred.BaseURL.String != "" && baseURL == providerDefaultBaseURL(provider) {
				baseURL = cred.BaseURL.String
			}
			baseURL = strings.TrimRight(baseURL, "/")
			targetPath := "/v1/chat/completions"
			if needsConversion {
				switch provider {
				case "anthropic":
					targetPath = "/v1/messages"
				case "gemini":
					if isStreaming {
						targetPath = "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"
					} else {
						targetPath = "/v1beta/models/" + model + ":generateContent"
					}
				}
			}
			targetURL := baseURL + targetPath

			// Apply per-instance timeout override if set
			reqCtx := r.Context()
			if target.TimeoutSeconds > 0 {
				var cancel context.CancelFunc
				reqCtx, cancel = context.WithTimeout(reqCtx, time.Duration(target.TimeoutSeconds)*time.Second)
				defer cancel()
			}

			proxyReq, reqErr := http.NewRequestWithContext(reqCtx, r.Method, targetURL, strings.NewReader(string(convertedBody)))
			if reqErr != nil {
				continue
			}

			for k, vv := range r.Header {
				if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "x-api-key") {
					continue
				}
				for _, v := range vv {
					proxyReq.Header.Add(k, v)
				}
			}
			h.injectAuth(provider, proxyReq, realKey, targetURL)

			resp, respErr := h.client.Do(proxyReq)
			if respErr != nil {
				router.RecordFailure(r.Context(), target.InstanceID, respErr.Error())
				if i == len(targets)-1 {
					h.recordUsage(userID, apiKeyID, provider, model, r.URL.Path, start, 0, 0, 502, respErr.Error())
					http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
					return
				}
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				respBody, _ := io.ReadAll(resp.Body)
				if resp.StatusCode >= 500 {
					router.RecordFailure(r.Context(), target.InstanceID, string(respBody[:min(len(respBody), 500)]))
				}
				for k, vv := range resp.Header {
					for _, v := range vv { w.Header().Add(k, v) }
				}
				w.WriteHeader(resp.StatusCode)
				w.Write(respBody)
				h.recordUsage(userID, apiKeyID, provider, model, r.URL.Path, start, 0, 0, resp.StatusCode, "")
				return
			}

			var inputTokens, outputTokens int
			if isStreaming {
				inputTokens, outputTokens = h.handleStreaming(w, resp, provider, needsConversion)
			} else {
				inputTokens, outputTokens, _ = h.handleNonStreaming(w, resp, provider, needsConversion)
			}

			costCents := pricing.CalculateCost(r.Context(), h.db, provider, model, inputTokens, outputTokens)
			quota.AddTokenUsage(r.Context(), h.db, userID, int64(inputTokens+outputTokens), costCents)
			models.DeductBalance(r.Context(), h.db, userID, costCents)
			models.CheckBalanceAlert(r.Context(), h.db, userID, h.dispatcher)
			h.recordUsage(userID, apiKeyID, provider, model, r.URL.Path, start, inputTokens, outputTokens, resp.StatusCode, "")
			return
		}

		// All targets exhausted
		http.Error(w, `{"error":"no available provider for model `+model+`"}`, http.StatusBadGateway)
		return
	}

	// No model in registry — fall back to auto-detect from model name
	provider := detectProvider(model)
	h.proxyProvider(w, r, provider)
}

func (h *Handler) proxyProvider(w http.ResponseWriter, r *http.Request, provider string) {
	start := time.Now()
	userID := r.Header.Get("X-User-ID")
	apiKeyID := r.Header.Get("X-API-Key-ID")

	cred, err := h.resolveCredential(r, userID, provider)
	if err != nil {
		if err == models.ErrCredentialNotFound {
			http.Error(w, `{"error":"no credential configured for `+provider+`"}`, http.StatusForbidden)
			return
		}
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	realKey, err := crypto.Decrypt(cred.EncryptedAPIKey, h.cfg.Auth.EncryptionKey)
	if err != nil {
		slog.Error("failed to decrypt credential", "user_id", userID, "provider", provider, "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.Server.MaxBodyBytes))
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	model := extractModel(body)

	// Content moderation: check input
	if h.filter != nil {
		inputText := extractMessagesText(body)
		if inputText != "" {
			if result := h.filter.CheckInput(r.Context(), inputText); result != nil {
				if result.Action == "block" {
					slog.Warn("content blocked", "rule", result.Rule, "user_id", userID)
					http.Error(w, `{"error":"request blocked by content policy: `+result.Rule+`"}`, http.StatusForbidden)
					return
				}
			}
		}
	}

	// API key restriction check
	if !checkKeyRestrictions(w, r, model, provider) {
		return
	}

	// Quota check
	if quotaErr := quota.CheckLimit(r.Context(), h.db, userID, model); quotaErr != nil {
		quota.IncrementRequest(r.Context(), h.db, userID)
		http.Error(w, `{"error":"`+quotaErr.Error()+`"}`, http.StatusTooManyRequests)
		return
	}
	quota.IncrementRequest(r.Context(), h.db, userID)

	// Cache check for non-streaming cacheable requests
	isStreamingRequest := false
	var payloadCheck map[string]interface{}
	if json.Unmarshal(body, &payloadCheck) == nil {
		if s, ok := payloadCheck["stream"].(bool); ok {
			isStreamingRequest = s
		}
	}
	if !isStreamingRequest && cache.IsCacheable(body) {
		cacheKey := cache.ComputeKey(model, body)
		if entry, _ := cache.Get(r.Context(), h.db, cacheKey); entry != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(entry.ResponseBody)
			metrics.IncCacheHit()
			h.recordUsage(userID, apiKeyID, provider, model, r.URL.Path, start, entry.InputTokens, entry.OutputTokens, 200, "")
			return
		}
		metrics.IncCacheMiss()
	}

	needsConversion := provider != "openai"
	var convertedBody []byte = body

	if needsConversion {
		convertedBody, err = h.convertRequest(body, provider)
		if err != nil {
			slog.Error("failed to convert request", "provider", provider, "error", err)
			http.Error(w, `{"error":"request conversion failed"}`, http.StatusBadRequest)
			return
		}
	}

	isStreaming := false
	if provider == "gemini" {
		isStreaming = strings.Contains(r.URL.Path, "streamGenerateContent")
	} else {
		var payload map[string]interface{}
		if json.Unmarshal(body, &payload) == nil {
			if s, ok := payload["stream"].(bool); ok {
				isStreaming = s
			}
		}
	}

	// For OpenAI streaming, inject stream_options to get usage in final chunk
	if provider == "openai" && isStreaming {
		convertedBody = converter.InjectStreamOptions(convertedBody)
	}

	targetURL := h.buildTargetURL(r, provider, cred)
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, strings.NewReader(string(convertedBody)))
	if err != nil {
		http.Error(w, `{"error":"failed to create request"}`, http.StatusInternalServerError)
		return
	}

	for k, vv := range r.Header {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "x-api-key") {
			continue
		}
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	h.injectAuth(provider, proxyReq, realKey, targetURL)

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		slog.Error("proxy request failed", "error", err)
		h.recordUsage(userID, apiKeyID, provider, model, r.URL.Path, start, 0, 0, 502, err.Error())
		http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Error responses: pass through and record
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		h.recordUsage(userID, apiKeyID, provider, model, r.URL.Path, start, 0, 0, resp.StatusCode, "")
		return
	}

	// Successful response paths
	var inputTokens, outputTokens int

	if isStreaming {
		inputTokens, outputTokens = h.handleStreaming(w, resp, provider, needsConversion)
	} else {
		var clientBody []byte
		inputTokens, outputTokens, clientBody = h.handleNonStreaming(w, resp, provider, needsConversion)

		// Store in cache if cacheable
		if !isStreamingRequest && cache.IsCacheable(body) && resp.StatusCode == 200 {
			cacheKey := cache.ComputeKey(model, body)
			costCents := pricing.CalculateCost(r.Context(), h.db, provider, model, inputTokens, outputTokens)
			cache.Set(r.Context(), h.db, cacheKey, model, clientBody, inputTokens, outputTokens, costCents, time.Hour)
		}
	}

	// Record usage after response is fully processed
	costCents := pricing.CalculateCost(r.Context(), h.db, provider, model, inputTokens, outputTokens)
	quota.AddTokenUsage(r.Context(), h.db, userID, int64(inputTokens+outputTokens), costCents)
	models.DeductBalance(r.Context(), h.db, userID, costCents)
	models.CheckBalanceAlert(r.Context(), h.db, userID, h.dispatcher)
	h.recordUsage(userID, apiKeyID, provider, model, r.URL.Path, start, inputTokens, outputTokens, resp.StatusCode, "")
}

// handleNonStreaming buffers the response, extracts usage, converts if needed, and sends to client.
func (h *Handler) handleNonStreaming(w http.ResponseWriter, resp *http.Response, provider string, needsConversion bool) (input, output int, clientBody []byte) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read response body", "error", err)
		return 0, 0, nil
	}

	// Extract usage from the raw provider response
	input, output = converter.ExtractNonStreamingUsage(provider, respBody)

	// Convert response back to OpenAI format if needed
	clientBody = respBody
	if needsConversion {
		converted, err := h.convertResponse(respBody, provider)
		if err != nil {
			slog.Error("failed to convert response", "provider", provider, "error", err)
		} else {
			clientBody = converted
		}
	}

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(clientBody)
	return input, output, clientBody
}

// handleStreaming processes a streaming response, converting SSE events if needed.
func (h *Handler) handleStreaming(w http.ResponseWriter, resp *http.Response, provider string, needsConversion bool) (input, output int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		StreamSSE(w, resp)
		return 0, 0
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if needsConversion {
		return h.streamWithConversion(w, resp, provider, flusher)
	}
	return h.streamOpenAI(w, resp, flusher)
}

// streamOpenAI passes through OpenAI SSE events while tracking usage from the final chunk.
func (h *Handler) streamOpenAI(w http.ResponseWriter, resp *http.Response, flusher http.Flusher) (input, output int) {
	tracker := converter.NewOpenAIStreamUsageTracker()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				w.Write([]byte("data: [DONE]\n\n"))
				flusher.Flush()
				break
			}
			// Track usage from this chunk
			tracker.ParseChunk(data)
			w.Write([]byte(line + "\n\n"))
			flusher.Flush()
		} else {
			w.Write([]byte(line + "\n"))
		}
	}

	return tracker.Usage()
}

// streamWithConversion converts SSE events from Anthropic/Gemini to OpenAI format while tracking usage.
func (h *Handler) streamWithConversion(w http.ResponseWriter, resp *http.Response, provider string, flusher http.Flusher) (input, output int) {
	var conv interface {
		Convert(event, data string) ([]converter.SSEChunk, error)
	}

	switch provider {
	case "anthropic":
		conv = converter.NewAnthropicStreamConverter()
	case "gemini":
		conv = converter.NewGeminiStreamConverter()
	default:
		io.Copy(w, resp.Body)
		return 0, 0
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			chunks, err := conv.Convert(currentEvent, data)
			if err == nil && len(chunks) > 0 {
				for _, chunk := range chunks {
					if chunk.Event != "" {
						w.Write([]byte("event: " + chunk.Event + "\n"))
					}
					w.Write([]byte(chunk.Data + "\n\n"))
				}
				flusher.Flush()
			}
			currentEvent = ""
		}
	}

	w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()

	// Extract usage from the converter if it implements StreamUsageTracker
	if tracker, ok := conv.(converter.StreamUsageTracker); ok {
		return tracker.Usage()
	}
	return 0, 0
}

func (h *Handler) convertRequest(body []byte, targetProvider string) ([]byte, error) {
	switch targetProvider {
	case "anthropic":
		return converter.ConvertOpenAIToAnthropic(body)
	case "gemini":
		return converter.ConvertOpenAIToGemini(body)
	}
	return body, nil
}

func (h *Handler) convertResponse(body []byte, provider string) ([]byte, error) {
	switch provider {
	case "anthropic":
		return converter.ConvertAnthropicResponseToOpenAI(body)
	case "gemini":
		return converter.ConvertGeminiResponseToOpenAI(body)
	}
	return body, nil
}

func (h *Handler) resolveCredential(r *http.Request, userID, provider string) (*models.ProviderCredential, error) {
	// Try cache first
	credKey := "cred:" + userID + ":" + provider
	if v, ok := h.credCache.Get(credKey); ok {
		return v.(*models.ProviderCredential), nil
	}

	cred, err := models.GetCredential(r.Context(), h.db, userID, provider)
	if err == nil {
		h.credCache.Set(credKey, cred)
		return cred, nil
	}
	if err != models.ErrCredentialNotFound {
		return nil, err
	}

	// Fallback: check group credentials
	groupsKey := "groups:" + userID
	var groups []*models.Group
	if v, ok := h.credCache.Get(groupsKey); ok {
		groups = v.([]*models.Group)
	} else {
		groups, _ = models.GetUserGroups(r.Context(), h.db, userID)
		if groups != nil {
			h.credCache.Set(groupsKey, groups)
		}
	}
	if gErr != nil || len(groups) == 0 {
		return nil, models.ErrCredentialNotFound
	}

	for _, g := range groups {
		gc, gcErr := models.GetGroupCredential(r.Context(), h.db, g.ID, provider)
		if gcErr != nil {
			continue
		}
		// Convert group credential to ProviderCredential shape
		return &models.ProviderCredential{
			ID:              gc.ID,
			UserID:          userID,
			Provider:        gc.Provider,
			EncryptedAPIKey: gc.EncryptedAPIKey,
			BaseURL:         gc.BaseURL,
			IsActive:        gc.IsActive,
		}, nil
	}
	return nil, models.ErrCredentialNotFound
}

func extractModel(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &req) == nil {
		return req.Model
	}
	return ""
}

func extractMessagesText(body []byte) string {
	var req struct {
		Messages []struct {
			Content interface{} `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &req) != nil {
		return ""
	}
	var parts []string
	for _, m := range req.Messages {
		switch v := m.Content.(type) {
		case string:
			parts = append(parts, v)
		case []interface{}:
			for _, item := range v {
				if obj, ok := item.(map[string]interface{}); ok {
					if t, ok := obj["type"].(string); ok && t == "text" {
						if text, ok := obj["text"].(string); ok {
							parts = append(parts, text)
						}
					}
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func checkKeyRestrictions(w http.ResponseWriter, r *http.Request, model, provider string) bool {
	allowedModels := r.Header.Get("X-Key-Allowed-Models")
	if allowedModels != "" && allowedModels != "{}" {
		models := parseKeyRestrictionList(allowedModels)
		if len(models) > 0 && !matchWithWildcard(model, models) {
			http.Error(w, `{"error":"model not allowed for this API key"}`, http.StatusForbidden)
			return false
		}
	}

	allowedProviders := r.Header.Get("X-Key-Allowed-Providers")
	if allowedProviders != "" && allowedProviders != "{}" && provider != "" {
		providers := parseKeyRestrictionList(allowedProviders)
		if len(providers) > 0 && !contains(providers, provider) {
			http.Error(w, `{"error":"provider not allowed for this API key"}`, http.StatusForbidden)
			return false
		}
	}

	return true
}

func parseKeyRestrictionList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return nil
	}
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

func matchWithWildcard(s string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(s, p[:len(p)-1]) {
				return true
			}
		} else if s == p {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// detectProvider guesses the provider from the model name as a fallback.
func detectProvider(model string) string {
	switch {
	case strings.HasPrefix(model, "gpt-"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return "openai"
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "gemini-"):
		return "gemini"
	default:
		return "openai"
	}
}

func (h *Handler) buildTargetURL(r *http.Request, provider string, cred *models.ProviderCredential) string {
	baseURL := providerDefaultBaseURL(provider)
	if cred.BaseURL.Valid && cred.BaseURL.String != "" {
		baseURL = cred.BaseURL.String
	}
	baseURL = strings.TrimRight(baseURL, "/")

	switch provider {
	case "openai":
		return baseURL + strings.TrimPrefix(r.URL.Path, "/proxy/openai")
	case "anthropic":
		return baseURL + strings.TrimPrefix(r.URL.Path, "/proxy/anthropic")
	case "gemini":
		return baseURL + strings.TrimPrefix(r.URL.Path, "/proxy/gemini")
	}
	return baseURL + r.URL.Path
}

func (h *Handler) injectAuth(provider string, req *http.Request, apiKey, targetURL string) {
	switch provider {
	case "openai":
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case "anthropic":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "gemini":
		q := req.URL.Query()
		q.Set("key", apiKey)
		req.URL.RawQuery = q.Encode()
	case "azure-openai":
		req.Header.Set("api-key", apiKey)
	case "openai-compatible":
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
}

func providerDefaultBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com"
	case "anthropic":
		return "https://api.anthropic.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com"
	}
	return ""
}

func (h *Handler) recordUsage(userID, apiKeyID, provider, model, endpoint string, start time.Time, inputTokens, outputTokens, statusCode int, errMsg string) {
	costCents := pricing.CalculateCost(context.Background(), h.db, provider, model, inputTokens, outputTokens)
	isError := statusCode >= 400
	record := &models.UsageRecord{
		UserID:       userID,
		Provider:     provider,
		Model:        sql.NullString{String: model, Valid: model != ""},
		Endpoint:     sql.NullString{String: endpoint, Valid: endpoint != ""},
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		DurationMs:   int(time.Since(start).Milliseconds()),
		StatusCode:   statusCode,
		CostCents:    sql.NullInt64{Int64: int64(costCents), Valid: true},
		IsError:      isError,
	}
	if apiKeyID != "" {
		record.APIKeyID = sql.NullString{String: apiKeyID, Valid: true}
	}
	if errMsg != "" {
		record.ErrorMessage = sql.NullString{String: errMsg, Valid: true}
	}
	if err := models.RecordUsage(context.Background(), h.db, record); err != nil {
		slog.Error("failed to record usage", "error", err)
	}

	metrics.IncProxyRequests(provider, model, statusCode)
	metrics.IncProxyTokens(provider, model, inputTokens, outputTokens)
	metrics.ObserveDuration(provider, int(time.Since(start).Milliseconds()))
}
