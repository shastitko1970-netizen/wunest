package chats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/outboundproxy"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// openChatStream opens a streaming chat-completions request against
// whichever upstream the chat is configured for.
//
//   - If upstream.BaseURL is empty → route through the WuApi proxy (the
//     default path: uses the user's WuApi key, WuApi handles provider
//     translation + billing). Goes direct; no outbound proxy pool needed
//     (WuApi is on the same box).
//   - If upstream.BaseURL is set → call that URL directly through the
//     handler's outbound proxy pool (required for OpenAI/Anthropic from
//     geo-blocked server IPs; no-op for other providers when pool is nil).
//
// The caller owns the returned body (must Close). Non-2xx responses are
// passed through so the caller can surface upstream errors with body text.
func (h *Handler) openChatStream(ctx context.Context, up upstream, req wuapi.ChatCompletionRequest) (io.ReadCloser, *http.Response, error) {
	if up.BaseURL == "" {
		// Default path: WuApi handles everything.
		return h.WuApi.ChatCompletionsStream(ctx, up.APIKey, req)
	}
	return directChatStream(ctx, h.ProxyPool, up, req)
}

// directChatStream POSTs to a user-supplied OpenAI-compatible endpoint
// with streaming enabled. The request is shaped per-provider BEFORE
// serialising (top_k stripped for OpenAI, reasoning keys filtered to the
// one the provider recognises, etc.); auth headers are chosen per-
// provider.
//
// When `pool` is non-nil, traffic routes through one of its HTTP proxies
// (with round-robin + dead-cooldown). Without a pool, a fresh Transport
// based on Go's defaults is used — effectively identical to the old
// package-level `directHTTPClient` but built per-call so the pool wiring
// only adds one indirection.
func directChatStream(ctx context.Context, pool *outboundproxy.Pool, up upstream, req wuapi.ChatCompletionRequest) (io.ReadCloser, *http.Response, error) {
	req = PrepareRequestForProvider(up.Provider, req)
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(up.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	// WuNest identifies itself so providers can tell us apart in their logs;
	// useful if someone reports unexpected traffic from a user's own key.
	httpReq.Header.Set("User-Agent", "WuNest/0.1 (+https://nest.wusphere.ru)")
	for k, v := range DirectCallHeaders(up.Provider, up.APIKey) {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Transport: pool.Transport()}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("do request: %w", err)
	}
	return resp.Body, resp, nil
}
