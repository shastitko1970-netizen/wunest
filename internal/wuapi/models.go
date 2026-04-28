package wuapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GetModels returns the response body of WuApi's GET /v1/models. Body is
// passed through unmodified — the caller decides whether to decode or proxy.
//
// A short timeout is applied (this is a small JSON payload, not a stream).
func (c *Client) GetModels(ctx context.Context, apiKey string) (io.ReadCloser, *http.Response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.url("/v1/models"), nil)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("do request: %w", err)
	}

	// Body is closed by caller; they'll also cancel the context (we wrap the
	// body so cancel is called when the body is closed).
	return &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}, resp, nil
}

// GetGoldCatalog returns WuApi's gold-catalog response (M55.2). When
// `wunestSecret` is non-empty the request includes the WuNest-private
// query parameters that expand the response with `:lite` eco-mode
// variants. Body is streamed back to the caller verbatim.
//
// Catalog is small and public-ish, so we set a short request timeout
// and don't attach an Authorization header — the secret is the
// gating mechanism for the lite expansion, not user auth.
func (c *Client) GetGoldCatalog(ctx context.Context, wunestSecret string) (io.ReadCloser, *http.Response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)

	path := "/api/catalog/gold"
	if wunestSecret != "" {
		// Avoid full URL escaping for the known-safe values; the secret
		// is hex from `openssl rand -hex 32`, so no special chars.
		path += "?app=wunest&secret=" + wunestSecret
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.url(path), nil)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("do request: %w", err)
	}
	return &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}, resp, nil
}

// cancelOnClose ties a context.CancelFunc to an io.Closer so consumers don't
// need to plumb cancel manually.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	if c.cancel != nil {
		c.cancel()
	}
	return err
}
