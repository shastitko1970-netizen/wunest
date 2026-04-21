// Package wuapi is an HTTP client for the sibling WuApi service.
//
// WuApi (api.wusphere.ru) owns user identity, API-key issuance, and the
// upstream LLM proxy. WuNest calls it for two things:
//
//  1. Resolve a session cookie into a user profile (GET /api/me)
//  2. Forward chat-completion requests as the authenticated user
//     (POST /v1/chat/completions — streamed)
//
// The client prefers InternalURL when set (direct container-to-container
// communication, skips nginx). Falls back to BaseURL (the public HTTPS URL).
package wuapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Config struct {
	BaseURL     string        // https://api.wusphere.ru
	InternalURL string        // optional: http://wuapi-blue:8080
	Timeout     time.Duration // per-request timeout (ignored for streaming)
}

type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			// No top-level timeout here — individual calls set their own via ctx.
			// This lets /v1/chat/completions stream for minutes without getting cut.
			Transport: http.DefaultTransport,
		},
	}
}

// MeResponse is the payload returned by WuApi's GET /api/me.
//
// Only the fields WuNest actually needs are modelled here; the rest are
// ignored. If WuApi adds required fields in the future, update this struct.
type MeResponse struct {
	ID              int64   `json:"id"`
	TelegramID      *int64  `json:"telegram_id,omitempty"`
	Username        string  `json:"username,omitempty"`
	FirstName       string  `json:"first_name,omitempty"`
	APIKey          string  `json:"api_key"`
	Tier            string  `json:"tier"`
	TierExpiresAt   *string `json:"tier_expires_at,omitempty"`
	GoldBalanceNano int64   `json:"gold_balance_nano"`
	DailyLimit      int     `json:"daily_limit,omitempty"`
	UsedToday       int     `json:"used_today,omitempty"`
	Blocked         bool    `json:"blocked,omitempty"`
}

// Me resolves a session cookie (the wu-API-key that WuApi stores in the
// `wu_session` cookie) into a full user profile. Returns ErrUnauthorized if
// the key is unknown or expired.
func (c *Client) Me(ctx context.Context, sessionKey string) (*MeResponse, error) {
	if sessionKey == "" {
		return nil, ErrUnauthorized
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.url("/api/me"), nil)
	if err != nil {
		return nil, fmt.Errorf("build me request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+sessionKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call me: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var me MeResponse
		if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
			return nil, fmt.Errorf("decode me: %w", err)
		}
		return &me, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("wuapi /api/me returned %d: %s", resp.StatusCode, string(body))
	}
}

// ErrUnauthorized is returned when WuApi rejects the session key.
var ErrUnauthorized = errors.New("wuapi: unauthorized")

func (c *Client) url(path string) string {
	base := c.cfg.BaseURL
	if c.cfg.InternalURL != "" {
		base = c.cfg.InternalURL
	}
	return base + path
}
