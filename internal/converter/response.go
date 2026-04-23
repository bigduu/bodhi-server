package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ConvertAnthropicResponseToOpenAI converts a non-streaming Anthropic response to OpenAI format.
func ConvertAnthropicResponseToOpenAI(body []byte) ([]byte, error) {
	var ar AnthropicResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("parse anthropic response: %w", err)
	}

	resp := OpenAIChatResponse{
		ID:     ar.ID,
		Object: "chat.completion",
		Model:  ar.Model,
	}

	choice := struct {
		Index         int    `json:"index"`
		FinishReason  string `json:"finish_reason"`
		Message       *struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	}{
		Index:        0,
		FinishReason: mapStopReason(ar.StopReason),
		Message: &struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		}{Role: "assistant"},
	}

	var texts []string
	var toolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	for _, block := range ar.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		} else if block.Type == "tool_use" {
			tc := struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			}{
				ID:   block.ID,
				Type: "function",
			}
			tc.Function.Name = block.Name
			tc.Function.Arguments = string(block.Input)
			toolCalls = append(toolCalls, tc)
		}
	}
	choice.Message.Content = strings.Join(texts, "")
	if len(toolCalls) > 0 {
		choice.Message.ToolCalls = toolCalls
	}
	resp.Choices = append(resp.Choices, choice)

	if ar.Usage != nil {
		resp.Usage = &struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		}
	}

	resp.Created = time.Now().Unix()
	return json.Marshal(resp)
}

// ConvertGeminiResponseToOpenAI converts a non-streaming Gemini response to OpenAI format.
func ConvertGeminiResponseToOpenAI(body []byte) ([]byte, error) {
	var gr GeminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("parse gemini response: %w", err)
	}

	resp := OpenAIChatResponse{
		ID:     fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object: "chat.completion",
	}

	if len(gr.Candidates) > 0 {
		cand := gr.Candidates[0]
		choice := struct {
			Index         int    `json:"index"`
			FinishReason  string `json:"finish_reason"`
			Message       *struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		}{
			Index:        0,
			FinishReason: mapGeminiFinishReason(cand.FinishReason),
			Message: &struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			}{Role: "assistant"},
		}

		if cand.Content != nil {
			var texts []string
			var toolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			}
			for i, part := range cand.Content.Parts {
				if part.Text != "" {
					texts = append(texts, part.Text)
				}
				if part.FunctionCall != nil {
					args, _ := json.Marshal(part.FunctionCall.Args)
					tc := struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					}{
						ID:   fmt.Sprintf("call_%d", i),
						Type: "function",
					}
					tc.Function.Name = part.FunctionCall.Name
					tc.Function.Arguments = string(args)
					toolCalls = append(toolCalls, tc)
				}
			}
			choice.Message.Content = strings.Join(texts, "")
			if len(toolCalls) > 0 {
				choice.Message.ToolCalls = toolCalls
			}
		}
		resp.Choices = append(resp.Choices, choice)
	}

	if gr.UsageMetadata != nil {
		resp.Usage = &struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.TotalTokenCount,
		}
	}

	resp.Created = time.Now().Unix()
	return json.Marshal(resp)
}

// ExtractNonStreamingUsage parses token usage from a non-streaming response body.
func ExtractNonStreamingUsage(provider string, body []byte) (input, output int) {
	switch provider {
	case "openai":
		var r OpenAIChatResponse
		if json.Unmarshal(body, &r) == nil && r.Usage != nil {
			return r.Usage.PromptTokens, r.Usage.CompletionTokens
		}
	case "anthropic":
		var r AnthropicResponse
		if json.Unmarshal(body, &r) == nil && r.Usage != nil {
			return r.Usage.InputTokens, r.Usage.OutputTokens
		}
	case "gemini":
		var r GeminiResponse
		if json.Unmarshal(body, &r) == nil && r.UsageMetadata != nil {
			return r.UsageMetadata.PromptTokenCount, r.UsageMetadata.CandidatesTokenCount
		}
	}
	return 0, 0
}

// InjectStreamOptions adds stream_options.include_usage=true to OpenAI request body.
func InjectStreamOptions(body []byte) []byte {
	var req map[string]json.RawMessage
	if json.Unmarshal(body, &req) != nil {
		return body
	}
	// Only inject if stream=true
	if stream, ok := req["stream"]; ok {
		var s bool
		if json.Unmarshal(stream, &s) != nil || !s {
			return body
		}
	}
	req["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}
