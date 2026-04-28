// Package config loads runtime configuration from environment variables.
//
// In development, .env is auto-loaded from the working directory.
// In production, env comes from docker-compose or systemd.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Env      string
	LogLevel string

	HTTPPort      string
	PublicBaseURL string

	DatabaseURL string
	RedisURL    string

	WuApiBaseURL     string
	WuApiInternalURL string
	// WuNestAPISecret is the shared secret WuNest sends to WuApi as
	// `?app=wunest&secret=...` when fetching the gold catalog (M55.2).
	// Server-side only — never exposed to the SPA. WuApi compares
	// constant-time and includes `:lite` eco-mode variants in the
	// response when matched. Empty → no eco-mode for this deploy.
	WuNestAPISecret string

	SessionCookieDomain string
	SessionCookieName   string

	SecretsKey []byte // AES-GCM key for BYOK encryption (32 bytes)
	CSRFSecret []byte // HMAC key for CSRF tokens

	// MinIO object storage (avatars, message attachments, backgrounds).
	// When MinIOEndpoint is empty the storage layer runs in "disabled"
	// mode and Put* calls return ErrDisabled — used on dev laptops
	// without a MinIO running.
	MinIOEndpoint      string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOUseSSL        bool
	MinIOPublicBaseURL string // origin where /images/* is served

	// OutboundProxies is a comma- or newline-separated list of HTTP proxies
	// used for direct BYOK calls to OpenAI / Anthropic (which geo-block our
	// Selectel IP). Format: `host:port:user:pass` per entry, or a fully
	// qualified URL (http://user:pass@host:port). Empty → no proxy.
	OutboundProxies string
}

// Load reads env vars (falling back to .env if present) and returns the config.
// Returns an error if any required variable is missing or malformed.
func Load() (*Config, error) {
	// Best-effort .env load; missing file is OK in production.
	_ = godotenv.Load()

	cfg := &Config{
		Env:      envOr("ENV", "development"),
		LogLevel: envOr("LOG_LEVEL", "info"),

		HTTPPort:      envOr("HTTP_PORT", "9090"),
		PublicBaseURL: envOr("PUBLIC_BASE_URL", "http://localhost:9090"),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),

		WuApiBaseURL:     envOr("WUAPI_BASE_URL", "https://api.wusphere.ru"),
		WuApiInternalURL: os.Getenv("WUAPI_INTERNAL_URL"),
		WuNestAPISecret:  os.Getenv("WUNEST_API_SECRET"),

		SessionCookieDomain: envOr("SESSION_COOKIE_DOMAIN", ".wusphere.ru"),
		SessionCookieName:   envOr("SESSION_COOKIE_NAME", "wu_session"),

		MinIOEndpoint:      os.Getenv("MINIO_ENDPOINT"),
		MinIOAccessKey:     os.Getenv("MINIO_ACCESS_KEY"),
		MinIOSecretKey:     os.Getenv("MINIO_SECRET_KEY"),
		MinIOUseSSL:        strings.EqualFold(os.Getenv("MINIO_USE_SSL"), "true"),
		MinIOPublicBaseURL: envOr("MINIO_PUBLIC_BASE_URL", ""),

		OutboundProxies: os.Getenv("NEST_OUTBOUND_PROXIES"),
	}

	// Default MinIOPublicBaseURL to PublicBaseURL so the common same-
	// origin case (nginx on nest.wusphere.ru proxies /images/*) needs no
	// extra env var. Only set explicitly when serving from a different
	// host.
	if cfg.MinIOPublicBaseURL == "" {
		cfg.MinIOPublicBaseURL = cfg.PublicBaseURL
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}

	secretsKey, err := decodeB64Key(os.Getenv("SECRETS_KEY"), 32)
	if err != nil {
		return nil, fmt.Errorf("SECRETS_KEY: %w", err)
	}
	cfg.SecretsKey = secretsKey

	csrfKey, err := decodeB64Key(os.Getenv("CSRF_SECRET"), 32)
	if err != nil {
		return nil, fmt.Errorf("CSRF_SECRET: %w", err)
	}
	cfg.CSRFSecret = csrfKey

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// IsProduction reports whether the service is running in the prod environment.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func decodeB64Key(s string, wantLen int) ([]byte, error) {
	if s == "" {
		return nil, errors.New("empty")
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// Allow URL-safe base64 too.
		raw, err = base64.URLEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("not valid base64: %w", err)
		}
	}
	if len(raw) != wantLen {
		return nil, fmt.Errorf("expected %d bytes, got %d", wantLen, len(raw))
	}
	return raw, nil
}
