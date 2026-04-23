package dto

import "time"

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         UserResponse `json:"user"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email,omitempty"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key,omitempty"`
	KeyPrefix string     `json:"key_prefix"`
	KeySuffix string     `json:"key_suffix"`
	IsActive  bool       `json:"is_active"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type CredentialResponse struct {
	Provider  string    `json:"provider"`
	BaseURL   string    `json:"base_url,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type VersionResponse struct {
	ID          string     `json:"id"`
	Version     string     `json:"version"`
	Platform    string     `json:"platform"`
	Changelog   string     `json:"changelog"`
	DownloadURL string     `json:"download_url,omitempty"`
	IsLatest    bool       `json:"is_latest"`
	ForceUpdate bool       `json:"force_update"`
	IsDraft     bool       `json:"is_draft,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
