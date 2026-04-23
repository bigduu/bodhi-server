package converter

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SSEChunk struct {
	Event string
	Data  string
}

// StreamUsageTracker is implemented by stream converters that track token usage.
type StreamUsageTracker interface {
	Usage() (input, output int)
}

// OpenAIStreamUsageTracker tracks usage from OpenAI streaming chunks.
type OpenAIStreamUsageTracker struct {
	inputTokens  int
	outputTokens int
}

func NewOpenAIStreamUsageTracker() *OpenAIStreamUsageTracker {
	return &OpenAIStreamUsageTracker{}
}

func (t *OpenAIStreamUsageTracker) ParseChunk(data string) {
	var chunk struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(data), &chunk) == nil && chunk.Usage != nil {
		t.inputTokens = chunk.Usage.PromptTokens
		t.outputTokens = chunk.Usage.CompletionTokens
	}
}

func (t *OpenAIStreamUsageTracker) Usage() (input, output int) {
	return t.inputTokens, t.outputTokens
}

type AnthropicStreamConverter struct {
	toolCallIndex int
	contentIndex  int
	inputTokens   int
	outputTokens  int
}

func NewAnthropicStreamConverter() *AnthropicStreamConverter {
	return &AnthropicStreamConverter{}
}

func (c *AnthropicStreamConverter) Convert(event, data string) ([]SSEChunk, error) {
	if data == "" || data == `""` {
		return nil, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, nil
	}

	msgType := strings.Trim(string(raw["type"]), `"`)

	switch msgType {
	case "message_start", "message_stop", "content_block_stop", "ping":
		if msgType == "message_start" {
			c.extractInputTokens(raw)
		}
		return nil, nil

	case "content_block_start":
		return c.handleBlockStart(raw)

	case "content_block_delta":
		return c.handleBlockDelta(raw)

	case "message_delta":
		return c.handleMessageDelta(raw)
	}

	return nil, nil
}

func (c *AnthropicStreamConverter) handleBlockStart(raw map[string]json.RawMessage) ([]SSEChunk, error) {
	var block struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
	}
	if rawBlock, ok := raw["content_block"]; ok {
		json.Unmarshal(rawBlock, &block)
	}

	if block.Type == "tool_use" {
		idx := c.toolCallIndex
		c.toolCallIndex++
		c.contentIndex++
		return []SSEChunk{{
			Data: openAIStreamJSON(&OpenAIStreamChunk{
				Choices: []OpenAIChoice{{
					Index: 0,
					Delta: &OpenAIDelta{
						ToolCalls: []OpenAIToolCallDelta{{
							Index: idx,
							ID:    block.ID,
							Type:  "function",
							Function: &OpenAIFuncDelta{Name: block.Name},
						}},
					},
				}},
			}),
		}}, nil
	}

	if block.Type == "text" {
		c.contentIndex++
	}
	return nil, nil
}

func (c *AnthropicStreamConverter) handleBlockDelta(raw map[string]json.RawMessage) ([]SSEChunk, error) {
	var delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
		PartialJSON string `json:"partial_json"`
	}
	if rawDelta, ok := raw["delta"]; ok {
		json.Unmarshal(rawDelta, &delta)
	}

	switch delta.Type {
	case "text_delta":
		return []SSEChunk{{
			Data: openAIStreamJSON(&OpenAIStreamChunk{
				Choices: []OpenAIChoice{{
					Index: 0,
					Delta: &OpenAIDelta{Content: delta.Text},
				}},
			}),
		}}, nil

	case "input_json_delta":
		return []SSEChunk{{
			Data: openAIStreamJSON(&OpenAIStreamChunk{
				Choices: []OpenAIChoice{{
					Index: 0,
					Delta: &OpenAIDelta{
						ToolCalls: []OpenAIToolCallDelta{{
							Index:    c.toolCallIndex - 1,
							Function: &OpenAIFuncDelta{Arguments: delta.PartialJSON},
						}},
					},
				}},
			}),
		}}, nil
	}
	return nil, nil
}

func (c *AnthropicStreamConverter) handleMessageDelta(raw map[string]json.RawMessage) ([]SSEChunk, error) {
	var delta struct {
		StopReason string `json:"stop_reason"`
	}
	if rawDelta, ok := raw["delta"]; ok {
		json.Unmarshal(rawDelta, &delta)
	}

	// Extract output tokens from usage in message_delta
	if rawUsage, ok := raw["usage"]; ok {
		var usage struct {
			OutputTokens int `json:"output_tokens"`
		}
		if json.Unmarshal(rawUsage, &usage) == nil {
			c.outputTokens = usage.OutputTokens
		}
	}

	reason := mapStopReason(delta.StopReason)
	return []SSEChunk{{
		Data: openAIStreamJSON(&OpenAIStreamChunk{
			Choices: []OpenAIChoice{{
				Index:        0,
				FinishReason: &reason,
			}},
		}),
	}}, nil
}

func (c *AnthropicStreamConverter) extractInputTokens(raw map[string]json.RawMessage) {
	if rawMsg, ok := raw["message"]; ok {
		var msg struct {
			Usage struct {
				InputTokens int `json:"input_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(rawMsg, &msg) == nil {
			c.inputTokens = msg.Usage.InputTokens
		}
	}
}

func (c *AnthropicStreamConverter) Usage() (input, output int) {
	return c.inputTokens, c.outputTokens
}

type GeminiStreamConverter struct {
	toolCallIndex int
	inputTokens   int
	outputTokens  int
}

func NewGeminiStreamConverter() *GeminiStreamConverter {
	return &GeminiStreamConverter{}
}

func (c *GeminiStreamConverter) Convert(event, data string) ([]SSEChunk, error) {
	if data == "" || data == "[DONE]" {
		return nil, nil
	}

	var raw struct {
		Candidates []struct {
			Content *struct {
				Parts []struct {
					Text         string          `json:"text"`
					FunctionCall *GeminiFuncCall `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, nil
	}

	if raw.UsageMetadata != nil {
		c.inputTokens = raw.UsageMetadata.PromptTokenCount
		c.outputTokens = raw.UsageMetadata.CandidatesTokenCount
	}

	if len(raw.Candidates) == 0 {
		return nil, nil
	}

	cand := raw.Candidates[0]
	var chunks []SSEChunk

	if cand.Content != nil {
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				chunks = append(chunks, SSEChunk{
					Data: openAIStreamJSON(&OpenAIStreamChunk{
						Choices: []OpenAIChoice{{
							Index: 0,
							Delta: &OpenAIDelta{Content: part.Text},
						}},
					}),
				})
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				idx := c.toolCallIndex
				c.toolCallIndex++
				chunks = append(chunks, SSEChunk{
					Data: openAIStreamJSON(&OpenAIStreamChunk{
						Choices: []OpenAIChoice{{
							Index: 0,
							Delta: &OpenAIDelta{
								ToolCalls: []OpenAIToolCallDelta{{
									Index:    idx,
									ID:       fmt.Sprintf("call_%d", idx),
									Type:     "function",
									Function: &OpenAIFuncDelta{Name: part.FunctionCall.Name, Arguments: string(args)},
								}},
							},
						}},
					}),
				})
			}
		}
	}

	if cand.FinishReason != "" {
		reason := mapGeminiFinishReason(cand.FinishReason)
		chunks = append(chunks, SSEChunk{
			Data: openAIStreamJSON(&OpenAIStreamChunk{
				Choices: []OpenAIChoice{{
					Index:        0,
					FinishReason: &reason,
				}},
			}),
		})
	}

	return chunks, nil
}

func (c *GeminiStreamConverter) Usage() (input, output int) {
	return c.inputTokens, c.outputTokens
}

func openAIStreamJSON(chunk *OpenAIStreamChunk) string {
	b, _ := json.Marshal(chunk)
	return "data: " + string(b)
}

func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return reason
	}
}

func mapGeminiFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	default:
		return strings.ToLower(reason)
	}
}
