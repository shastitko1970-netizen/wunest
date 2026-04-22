package wuapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ChatCompletionRequest is the subset of the OpenAI-compatible payload that
// WuApi accepts. Additional fields survive via Extra (preserved verbatim).
type ChatCompletionRequest struct {
	Model            string           `json:"model"`
	Messages         []map[string]any `json:"messages"`
	Temperature      *float64         `json:"temperature,omitempty"`
	TopP             *float64         `json:"top_p,omitempty"`
	MaxTokens        *int             `json:"max_tokens,omitempty"`
	FrequencyPenalty *float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64         `json:"presence_penalty,omitempty"`
	Stream           bool             `json:"stream"`
	// Extra holds unknown/user-supplied fields so the caller can pass through
	// model-specific knobs (tool_choice, response_format, etc.) without this
	// struct growing every release.
	Extra map[string]any `json:"-"`
}

// MarshalJSON merges the named fields with Extra so unknown keys ride along.
func (r ChatCompletionRequest) MarshalJSON() ([]byte, error) {
	// Build an ordered map by encoding the struct, decoding into a map, then
	// merging Extra on top.
	type alias ChatCompletionRequest
	namedJSON, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	named := map[string]any{}
	if err := json.Unmarshal(namedJSON, &named); err != nil {
		return nil, err
	}
	for k, v := range r.Extra {
		if _, exists := named[k]; exists {
			continue // named fields win over Extra
		}
		named[k] = v
	}
	return json.Marshal(named)
}

// ChatCompletionsStream POSTs to WuApi /v1/chat/completions with streaming
// enabled and returns the raw response body (SSE stream) for the caller to
// pipe to an HTTP client.
//
// It is the caller's responsibility to Close the returned body.
func (c *Client) ChatCompletionsStream(
	ctx context.Context,
	apiKey string,
	req ChatCompletionRequest,
) (io.ReadCloser, *http.Response, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	// NOTE: no per-request timeout here. SSE streams can legitimately run for
	// minutes (reasoning models especially). Cancellation is driven by the
	// context passed in from the handler.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.url("/v1/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("do request: %w", err)
	}

	// Pass non-2xx through so the caller can surface upstream errors.
	// The body is intentionally not closed here — caller reads it.
	return resp.Body, resp, nil
}
