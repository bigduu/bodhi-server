package converter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConvertOpenAIToAnthropic converts an OpenAI-format request body to Anthropic format.
func ConvertOpenAIToAnthropic(body []byte) ([]byte, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse openai request: %w", err)
	}

	// Extract system messages
	var systemParts []string
	var messages []OpenAIMessage
	for _, m := range req.Messages {
		if m.Role == "system" || m.Role == "developer" {
			text := extractText(m.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}
		} else {
			messages = append(messages, m)
		}
	}

	// Convert messages
	var anthropicMsgs []AnthropicMessage
	for _, m := range messages {
		content, err := convertToAnthropicContent(m)
		if err != nil {
			return nil, err
		}
		role := m.Role
		if role == "tool" {
			role = "user"
		}
		anthropicMsgs = append(anthropicMsgs, AnthropicMessage{Role: role, Content: content})
	}

	maxTokens := 4096
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	anthropicReq := AnthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Stream:    req.Stream,
		Messages:  anthropicMsgs,
	}

	if len(systemParts) > 0 {
		sysText := strings.Join(systemParts, "\n\n")
		sysContent, _ := json.Marshal([]AnthropicContentBlock{{Type: "text", Text: sysText}})
		anthropicReq.System = sysContent
	}

	// Convert tools
	for _, t := range req.Tools {
		anthropicReq.Tools = append(anthropicReq.Tools, AnthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	return json.Marshal(anthropicReq)
}

func convertToAnthropicContent(m OpenAIMessage) (json.RawMessage, error) {
	// Tool result message
	if m.Role == "tool" {
		text := extractText(m.Content)
		blocks := []AnthropicContentBlock{
			{Type: "tool_result", ToolUseID: m.ToolCallID, Content: text},
		}
		return json.Marshal(blocks)
	}

	// Assistant message with tool calls
	if len(m.ToolCalls) > 0 {
		var blocks []AnthropicContentBlock
		text := extractText(m.Content)
		if text != "" {
			blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: text})
		}
		for _, tc := range m.ToolCalls {
			blocks = append(blocks, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(tc.Function.Arguments),
			})
		}
		return json.Marshal(blocks)
	}

	// Simple text content
	text := extractText(m.Content)
	return json.Marshal(text)
}

// ConvertOpenAIToGemini converts an OpenAI-format request body to Gemini format.
func ConvertOpenAIToGemini(body []byte) ([]byte, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse openai request: %w", err)
	}

	// Extract system messages
	var systemParts []string
	var messages []OpenAIMessage
	for _, m := range req.Messages {
		if m.Role == "system" || m.Role == "developer" {
			text := extractText(m.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}
		} else {
			messages = append(messages, m)
		}
	}

	// Convert messages
	var contents []GeminiContent
	for _, m := range messages {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}

		var parts []GeminiPart

		if m.Role == "tool" {
			_ = extractText(m.Content)
			parts = append(parts, GeminiPart{
				FunctionResponse: &GeminiFuncResp{
					Name:     m.ToolCallID, // best effort
					Response: json.RawMessage(`{"result": "completed"}`),
				},
			})
		} else if len(m.ToolCalls) > 0 {
			text := extractText(m.Content)
			if text != "" {
				parts = append(parts, GeminiPart{Text: text})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, GeminiPart{
					FunctionCall: &GeminiFuncCall{
						Name: tc.Function.Name,
						Args: json.RawMessage(tc.Function.Arguments),
					},
				})
			}
		} else {
			text := extractText(m.Content)
			parts = append(parts, GeminiPart{Text: text})
		}

		contents = append(contents, GeminiContent{Role: role, Parts: parts})
	}

	geminiReq := GeminiRequest{Contents: contents}

	if len(systemParts) > 0 {
		sysText := strings.Join(systemParts, "\n\n")
		geminiReq.SystemInstruction = &GeminiContent{
			Parts: []GeminiPart{{Text: sysText}},
		}
	}

	// Convert tools
	if len(req.Tools) > 0 {
		var decls []GeminiFuncDecl
		for _, t := range req.Tools {
			decls = append(decls, GeminiFuncDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		geminiReq.Tools = []GeminiTools{{FunctionDeclarations: decls}}
	}

	// Generation config
	if req.MaxTokens != nil {
		geminiReq.GenerationConfig = map[string]interface{}{
			"maxOutputTokens": *req.MaxTokens,
		}
	}

	return json.Marshal(geminiReq)
}

func extractText(content json.RawMessage) string {
	if content == nil {
		return ""
	}
	// Try string
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	// Try array of content parts
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &parts) == nil {
		var texts []string
		for _, p := range parts {
			if p.Type == "text" || p.Type == "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "")
	}
	return string(content)
}
