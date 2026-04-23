package providers

import (
	"net/http"
)

type Provider interface {
	Name() string
	BuildURL(baseURL, model, path string, streaming bool) string
	InjectAuth(req *http.Request, apiKey string)
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	r := &Registry{providers: make(map[string]Provider)}
	r.Register(&OpenAIProvider{})
	r.Register(&AnthropicProvider{})
	r.Register(&GeminiProvider{})
	r.Register(&AzureOpenAIProvider{})
	r.Register(&OpenAICompatProvider{})
	return r
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) Provider {
	return r.providers[name]
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for k := range r.providers {
		names = append(names, k)
	}
	return names
}

// OpenAI

type OpenAIProvider struct{}

func (p *OpenAIProvider) Name() string { return "openai" }
func (p *OpenAIProvider) BuildURL(baseURL, _, path string, _ bool) string {
	return baseURL + path
}
func (p *OpenAIProvider) InjectAuth(req *http.Request, apiKey string) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

// Anthropic

type AnthropicProvider struct{}

func (p *AnthropicProvider) Name() string { return "anthropic" }
func (p *AnthropicProvider) BuildURL(baseURL, _, path string, _ bool) string {
	return baseURL + path
}
func (p *AnthropicProvider) InjectAuth(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

// Gemini

type GeminiProvider struct{}

func (p *GeminiProvider) Name() string { return "gemini" }
func (p *GeminiProvider) BuildURL(baseURL, model, _ string, streaming bool) string {
	if streaming {
		return baseURL + "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"
	}
	return baseURL + "/v1beta/models/" + model + ":generateContent"
}
func (p *GeminiProvider) InjectAuth(req *http.Request, apiKey string) {
	q := req.URL.Query()
	q.Set("key", apiKey)
	req.URL.RawQuery = q.Encode()
}

// AzureOpenAI

type AzureOpenAIProvider struct{}

func (p *AzureOpenAIProvider) Name() string { return "azure-openai" }
func (p *AzureOpenAIProvider) BuildURL(baseURL, model, _ string, _ bool) string {
	return baseURL + "/openai/deployments/" + model + "/chat/completions?api-version=2024-02-15-preview"
}
func (p *AzureOpenAIProvider) InjectAuth(req *http.Request, apiKey string) {
	req.Header.Set("api-key", apiKey)
}

// OpenAICompat — generic OpenAI-compatible (Ollama, vLLM, LM Studio, etc.)

type OpenAICompatProvider struct{}

func (p *OpenAICompatProvider) Name() string { return "openai-compatible" }
func (p *OpenAICompatProvider) BuildURL(baseURL, _, path string, _ bool) string {
	return baseURL + path
}
func (p *OpenAICompatProvider) InjectAuth(req *http.Request, apiKey string) {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}
