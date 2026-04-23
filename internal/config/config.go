package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	RateLimit RateLimitConfig
}

type ServerConfig struct {
	Port         int
	Bind         string
	ProxyTimeout time.Duration
	MaxBodyBytes int64
	CORSOrigins  []string
	Version      string
}

type DatabaseConfig struct {
	URL string
}

type AuthConfig struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	EncryptionKey   string
}

type RateLimitConfig struct {
	RPM   int
	Burst int
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnvInt("BODHI_PORT", 8080),
			Bind:         getEnv("BODHI_BIND", "0.0.0.0"),
			ProxyTimeout: time.Duration(getEnvInt("BODHI_PROXY_TIMEOUT", 300)) * time.Second,
			MaxBodyBytes: int64(getEnvInt("BODHI_MAX_BODY_MB", 50)) * 1024 * 1024,
			CORSOrigins:  parseCSV(getEnv("BODHI_CORS_ORIGINS", "")),
			Version:      getEnv("BODHI_VERSION", "dev"),
		},
		Database: DatabaseConfig{
			URL: getEnv("BODHI_DB_URL", "postgres://bodhi:bodhi@localhost:5432/bodhi?sslmode=disable"),
		},
		Auth: AuthConfig{
			JWTSecret:       getEnv("BODHI_JWT_SECRET", ""),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			EncryptionKey:   getEnv("BODHI_ENCRYPTION_KEY", ""),
		},
		RateLimit: RateLimitConfig{
			RPM:   getEnvInt("BODHI_RATE_LIMIT_RPM", 60),
			Burst: getEnvInt("BODHI_RATE_LIMIT_BURST", 10),
		},
	}

	if cfg.Auth.JWTSecret == "" {
		return nil, fmt.Errorf("BODHI_JWT_SECRET is required")
	}
	if cfg.Auth.EncryptionKey == "" {
		return nil, fmt.Errorf("BODHI_ENCRYPTION_KEY is required")
	}
	if len(cfg.Auth.EncryptionKey) != 64 {
		return nil, fmt.Errorf("BODHI_ENCRYPTION_KEY must be 64 hex characters (32 bytes)")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
