package proxy

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

func providerProtocol(r *http.Request, provider string) (inferenceProtocol, string) {
	switch provider {
	case "openai":
		if r.Method == http.MethodPost && r.URL.EscapedPath() == "/proxy/openai/v1/responses" {
			return openAIResponses, ""
		}
	case "anthropic":
		if isNativeAnthropicEndpoint(r) {
			return anthropicMessages, ""
		}
	case "gemini":
		if model, ok := nativeGeminiEndpoint(r); ok {
			return geminiContent, model
		}
	}
	return chatCompletions, ""
}

func isNativeAnthropicEndpoint(r *http.Request) bool {
	switch r.URL.EscapedPath() {
	case "/proxy/anthropic/v1/messages", "/proxy/anthropic/v1/messages/count_tokens":
		return r.Method == http.MethodPost
	case "/proxy/anthropic/v1/models":
		return r.Method == http.MethodGet
	}
	return false
}

func nativeGeminiEndpoint(r *http.Request) (model string, native bool) {
	path, ok := strings.CutPrefix(r.URL.Path, "/proxy/gemini/")
	if !ok {
		return "", false
	}
	version, resource, ok := strings.Cut(path, "/")
	if !ok || (version != "v1" && version != "v1beta") {
		return "", false
	}
	if r.Method == http.MethodGet && resource == "models" {
		return "", true
	}
	if r.Method != http.MethodPost {
		return "", false
	}
	name, ok := strings.CutPrefix(resource, "models/")
	model, action, found := strings.Cut(name, ":")
	if !ok || !found || model == "" || strings.Contains(model, "/") {
		return "", false
	}
	switch action {
	case "generateContent", "streamGenerateContent", "countTokens":
		return model, true
	}
	return "", false
}

func hasOpenAIMessages(body []byte) bool {
	var req struct {
		Messages json.RawMessage `json:"messages"`
		Contents json.RawMessage `json:"contents"`
	}
	return json.Unmarshal(body, &req) == nil && req.Messages != nil && req.Contents == nil
}

func extractGeminiText(body []byte) string {
	type content struct {
		Parts []struct {
			Text         string `json:"text"`
			FunctionCall *struct {
				Args any `json:"args"`
			} `json:"functionCall"`
			FunctionResponse *struct {
				Response any `json:"response"`
			} `json:"functionResponse"`
		} `json:"parts"`
	}
	var req struct {
		SystemInstruction content   `json:"systemInstruction"`
		SystemSnake       content   `json:"system_instruction"`
		Contents          []content `json:"contents"`
	}
	if json.Unmarshal(body, &req) != nil {
		return ""
	}
	var parts []string
	for _, item := range append([]content{req.SystemInstruction, req.SystemSnake}, req.Contents...) {
		for _, part := range item.Parts {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
			if part.FunctionResponse != nil {
				parts = appendJSONText(parts, part.FunctionResponse.Response)
			}
			if part.FunctionCall != nil {
				parts = appendJSONText(parts, part.FunctionCall.Args)
			}
		}
	}
	return strings.Join(parts, " ")
}

func extractResponsesText(body []byte) string {
	var req struct {
		Input        any    `json:"input"`
		Instructions string `json:"instructions"`
	}
	if json.Unmarshal(body, &req) != nil {
		return ""
	}
	var parts []string
	if req.Instructions != "" {
		parts = append(parts, req.Instructions)
	}
	switch input := req.Input.(type) {
	case string:
		parts = appendJSONText(parts, input)
	case []any:
		for _, item := range input {
			if obj, ok := item.(map[string]any); ok {
				parts = appendResponsesContent(parts, obj["content"])
				parts = appendJSONText(parts, obj["text"])
				parts = appendJSONText(parts, obj["arguments"])
				parts = appendJSONText(parts, obj["output"])
			}
		}
	}
	return strings.Join(parts, " ")
}

func appendResponsesContent(parts []string, content any) []string {
	switch value := content.(type) {
	case string:
		return appendJSONText(parts, value)
	case []any:
		for _, part := range value {
			if obj, ok := part.(map[string]any); ok {
				parts = appendJSONText(parts, obj["text"])
				parts = appendJSONText(parts, obj["refusal"])
			}
		}
	}
	return parts
}

func appendJSONText(parts []string, value any) []string {
	switch value := value.(type) {
	case string:
		if value != "" {
			parts = append(parts, value)
		}
	case []any:
		for _, item := range value {
			parts = appendJSONText(parts, item)
		}
	case map[string]any:
		keys := make([]string, 0, len(value))
		for name := range value {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			parts = appendJSONText(parts, value[name])
		}
	}
	return parts
}
