package dto

import "time"

type RegisterRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=64"`
	Password   string `json:"password" binding:"required,min=8"`
	Email      string `json:"email,omitempty"`
	InviteCode string `json:"invite_code,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type CreateKeyRequest struct {
	Name             string     `json:"name" binding:"required"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	AllowedModels    []string   `json:"allowed_models,omitempty"`
	AllowedProviders []string   `json:"allowed_providers,omitempty"`
	IPWhitelist      []string   `json:"ip_whitelist,omitempty"`
}

type RotateKeyRequest struct {
	Name      string     `json:"name" binding:"required"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreateCredentialRequest struct {
	Provider string `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	APIKey   string `json:"api_key" binding:"required"`
	BaseURL  string `json:"base_url,omitempty"`
}

type CreateVersionRequest struct {
	Version     string `json:"version" binding:"required"`
	Platform    string `json:"platform"`
	Changelog   string `json:"changelog"`
	DownloadURL string `json:"download_url,omitempty"`
	ForceUpdate bool   `json:"force_update"`
}

type UpdateVersionRequest struct {
	Changelog   *string `json:"changelog,omitempty"`
	DownloadURL *string `json:"download_url,omitempty"`
	ForceUpdate *bool   `json:"force_update,omitempty"`
	Platform    *string `json:"platform,omitempty"`
}
