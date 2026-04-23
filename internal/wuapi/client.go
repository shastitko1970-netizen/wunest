// Package wuapi is an HTTP client for the sibling WuApi service.
//
// WuApi (api.wusphere.ru) owns user identity, API-key issuance, and the
// upstream LLM proxy. WuNest calls it for three things:
//
//  1. Resolve a session cookie into a user profile (GET /api/me)
//  2. Forward chat-completion requests as the authenticated user
//     (POST /v1/chat/completions — streamed)
//  3. Pass-through stats / gold-transaction queries for the Cabinet view
//
// Routing strategy:
//
//   - If InternalURL is configured AND reachable, requests go through it
//     (container-to-container, skips nginx). Lower latency.
//   - If InternalURL connection is refused (typical scenario: WuApi's
//     blue/green deploy has just swapped ports while our .env still points
//     at the old color), the request is transparently retried against
//     BaseURL, and subsequent requests use BaseURL until a cooldown passes.
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

// MeResponse mirrors WuApi's userJSON struct (see auth.go).
//
// NOTE: WuApi returns camelCase JSON keys, not snake_case. Tags must match.
// If you see zero-valued fields here in practice, the first thing to verify
// is that the JSON tag still matches WuApi's wire format.
type MeResponse struct {
	ID              int64      `json:"id"`
	Username        string     `json:"username"`
	FirstName       string     `json:"firstName"`
	Tier            string     `json:"tier"`
	TierExpiresAt   *time.Time `json:"tierExpiresAt"`
	APIKey          string     `json:"apiKey"`
	GoldBalanceNano int64      `json:"goldBalanceNano"`
	ReferralCount   int        `json:"referralCount"`
	CreatedAt       time.Time  `json:"createdAt"`
	UsedToday       int64      `json:"usedToday"`
	DailyLimit      int        `json:"dailyLimit"`
	// WuNest beta gate. FALSE by default; flips TRUE once the user
	// redeems an access code on WuApi. SPA hides the in-app lock screen
	// when this is true.
	NestAccessGranted bool `json:"nestAccessGranted,omitempty"`
	// Blocked isn't exposed by /api/me today — if WuApi ever adds it here
	// as `blocked`, update the tag.
	Blocked bool `json:"blocked,omitempty"`
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

// Proxy issues a GET to an arbitrary path on WuApi with the caller's
// Bearer token and returns the raw body for pipe-through. Used by stats /
// gold-transactions handlers. Caller must Close the body.
func (c *Client) Proxy(ctx context.Context, path, sessionKey string) (io.ReadCloser, *http.Response, error) {
	resp, err := c.doGET(ctx, path, sessionKey)
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp, nil
}

// ProxyPOST issues a POST to an arbitrary path on WuApi, forwarding the
// caller's body verbatim. Used for mutating endpoints we don't need to
// inspect — e.g. /api/me/nest-access/redeem, where WuApi validates the
// code and flips the flag. Caller must Close the returned body.
func (c *Client) ProxyPOST(ctx context.Context, path, sessionKey string, body io.Reader) (io.ReadCloser, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), body)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionKey != "" {
		req.Header.Set("Authorization", "Bearer "+sessionKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("do request: %w", err)
	}
	return resp.Body, resp, nil
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
