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
//
// Field choice: we expose everything the OpenAI / Anthropic / DeepSeek /
// OpenRouter interfaces accept. Providers that don't know a field (e.g.
// vanilla OpenAI has no top_k) just ignore it. WuApi itself is permissive.
type ChatCompletionRequest struct {
	Model             string           `json:"model"`
	Messages          []map[string]any `json:"messages"`
	Temperature       *float64         `json:"temperature,omitempty"`
	TopP              *float64         `json:"top_p,omitempty"`
	TopK              *int             `json:"top_k,omitempty"`
	MinP              *float64         `json:"min_p,omitempty"`
	MaxTokens         *int             `json:"max_tokens,omitempty"`
	FrequencyPenalty  *float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64         `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float64         `json:"repetition_penalty,omitempty"`
	Seed              *int             `json:"seed,omitempty"`
	Stop              []string         `json:"stop,omitempty"`
	Stream            bool             `json:"stream"`
	// StreamOptions — OAI-compat knob we set to force providers (OpenAI in
	// particular) to emit a final usage chunk during streaming. Without
	// this OpenAI native DOES NOT include prompt/completion tokens in the
	// stream and our per-message token accounting ends up at zero.
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	// Extra holds unknown/user-supplied fields so the caller can pass through
	// model-specific knobs (tool_choice, response_format, reasoning_effort,
	// thinking, etc.) without this struct growing every release.
	Extra map[string]any `json:"-"`
}

// StreamOptions carries OAI-compat stream_options fields. Only include_usage
// is in scope today; room to grow when provider docs add more (e.g.
// token-level logprobs).
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
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
	// Opt-in to usage in stream — OpenAI native and most OAI-compat
	// providers honour this. Without it we silently get 0-token messages.
	if req.StreamOptions == nil {
		req.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

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
