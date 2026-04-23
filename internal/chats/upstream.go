package chats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// directHTTPClient is the shared transport for BYOK direct-provider calls.
// Reused so connections pool across turns; long-running streams rely on
// the request context for cancellation rather than a per-request timeout.
var directHTTPClient = &http.Client{Transport: http.DefaultTransport}

// openChatStream opens a streaming chat-completions request against
// whichever upstream the chat is configured for.
//
//   - If upstream.BaseURL is empty → route through the WuApi proxy (the
//     default path: uses the user's WuApi key, WuApi handles provider
//     translation + billing).
//   - If upstream.BaseURL is set → call that URL directly. This is the
//     BYOK hot path: the request body is still OpenAI-compatible (we
//     don't rewrite it), just sent with the user's own Bearer token
//     against e.g. https://api.openai.com/v1/chat/completions.
//
// The caller owns the returned body (must Close). Non-2xx responses are
// passed through so the caller can surface upstream errors with body text.
func (h *Handler) openChatStream(ctx context.Context, up upstream, req wuapi.ChatCompletionRequest) (io.ReadCloser, *http.Response, error) {
	if up.BaseURL == "" {
		// Default path: WuApi handles everything.
		return h.WuApi.ChatCompletionsStream(ctx, up.APIKey, req)
	}
	return directChatStream(ctx, up, req)
}

// directChatStream POSTs to a user-supplied OpenAI-compatible endpoint
// with streaming enabled. Identical wire shape to WuApi's client — we
// just change the URL and auth header.
func directChatStream(ctx context.Context, up upstream, req wuapi.ChatCompletionRequest) (io.ReadCloser, *http.Response, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := up.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+up.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	// WuNest identifies itself so providers can tell us apart in their logs;
	// useful if someone reports unexpected traffic from a user's own key.
	httpReq.Header.Set("User-Agent", "WuNest/0.1 (+https://nest.wusphere.ru)")

	resp, err := directHTTPClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("do request: %w", err)
	}
	return resp.Body, resp, nil
}
