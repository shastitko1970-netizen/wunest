// Package wuapi is an HTTP client for the sibling WuApi service.
//
// WuApi (api.wusphere.ru) owns user identity, API-key issuance, and the
// upstream LLM proxy. WuNest calls it for two things:
//
//  1. Resolve a session cookie into a user profile (GET /api/me)
//  2. Forward chat-completion requests as the authenticated user
//     (POST /v1/chat/completions — streamed)
//
// Routing strategy:
//
//   - If InternalURL is configured AND reachable, requests go through it
//     (container-to-container, skips nginx). Lower latency.
//   - If InternalURL connection is refused (typical scenario: WuApi's
//     blue/green deploy has just swapped ports while our .env still points
//     at the old color), the request is transparently retried against
//     BaseURL, and subsequent requests use BaseURL until a cooldown passes.
//     This keeps WuNest working across WuApi deploys without a deploy of
//     its own.
//   - If InternalURL is empty, every request goes to BaseURL.
package wuapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Config struct {
	BaseURL     string        // https://api.wusphere.ru
	InternalURL string        // optional: http://127.0.0.1:8080 (blue) or :8081 (green)
	Timeout     time.Duration // per-request timeout (ignored for streaming)
}

type Client struct {
	cfg  Config
	http *http.Client

	// InternalURL circuit breaker. When a dial to InternalURL fails with
	// "connection refused", we flip useBase to true and skip InternalURL
	// for cooldown. Mostly serves blue/green deploys where the old
	// InternalURL port is dead for a few minutes until someone updates the
	// env.
	breakerMu    sync.Mutex
	useBase      bool
	breakerUntil time.Time
}

const breakerCooldown = 5 * time.Minute

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

	resp, err := c.doGET(ctx, "/api/me", sessionKey)
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

// doGET issues a short-timeout GET and, if the InternalURL attempt fails
// with a connection-refused-style error, transparently retries against the
// public BaseURL.
func (c *Client) doGET(ctx context.Context, path, bearerToken string) (*http.Response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)

	urls := c.candidates()
	var lastErr error

	for i, base := range urls {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+path, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("build request: %w", err)
		}
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}

		resp, err := c.http.Do(req)
		if err == nil {
			// Wrap body so cancel fires when caller closes it.
			resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
			return resp, nil
		}
		lastErr = err

		if !isDialRefused(err) {
			cancel()
			return nil, err
		}

		// First candidate failed with refuse. Trip breaker and try BaseURL.
		if i == 0 && len(urls) > 1 {
			c.tripBreaker(base)
			continue
		}
	}
	cancel()
	return nil, lastErr
}

// candidates returns the ordered list of base URLs to try.
func (c *Client) candidates() []string {
	if c.cfg.InternalURL != "" && !c.breakerOpen() {
		if c.cfg.InternalURL != c.cfg.BaseURL {
			return []string{c.cfg.InternalURL, c.cfg.BaseURL}
		}
	}
	return []string{c.cfg.BaseURL}
}

func (c *Client) breakerOpen() bool {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if !c.useBase {
		return false
	}
	if time.Now().After(c.breakerUntil) {
		c.useBase = false
		return false
	}
	return true
}

func (c *Client) tripBreaker(badURL string) {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if !c.useBase {
		slog.Warn("wuapi internal URL unreachable, failing over to base",
			"internal", badURL,
			"base", c.cfg.BaseURL,
			"cooldown", breakerCooldown,
		)
	}
	c.useBase = true
	c.breakerUntil = time.Now().Add(breakerCooldown)
}

// isDialRefused reports whether the error looks like "couldn't open a TCP
// connection" — as opposed to a timeout, a TLS failure, or a 4xx/5xx.
func isDialRefused(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Op == "dial" {
			return true
		}
	}
	// Fallback: string-match for platforms where the error tree doesn't
	// surface *net.OpError (rare, but defensive).
	return strings.Contains(err.Error(), "connection refused")
}

// url is kept for back-compat with callers (stream.go, models.go). It
// returns the *preferred* base; it doesn't know about the breaker. New
// code should route through doGET / doStream helpers that honour it.
func (c *Client) url(path string) string {
	if c.cfg.InternalURL != "" && !c.breakerOpen() {
		return c.cfg.InternalURL + path
	}
	return c.cfg.BaseURL + path
}
