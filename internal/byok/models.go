package byok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shastitko1970-netizen/wunest/internal/outboundproxy"
)

// ModelList mirrors OpenAI's GET /v1/models shape. Any provider worth
// supporting returns compatible JSON here; the handful that don't (Anthropic's
// native endpoint) we normalise into this shape before returning.
type ModelList struct {
	Object string         `json:"object"`
	Data   []ModelCatalog `json:"data"`
}

// ModelCatalog — we use this name instead of `Model` so it doesn't collide
// with anything else a caller might import from this package.
type ModelCatalog struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
	Created int64  `json:"created,omitempty"`
}

// ErrUpstream is returned when the provider rejects the request (any non-2xx).
// Caller should surface the inner message to the user (it's typically a
// human-readable "invalid API key" or "no access to this model family").
var ErrUpstream = errors.New("provider rejected the request")

// ErrGeoBlocked is returned when the provider's 403 is specifically a
// geographic restriction (Cloudflare-style "your country/region is not
// supported"). This fires for OpenAI/Anthropic calls made from jurisdictions
// those providers block — Russia, China, Iran, a handful of others.
//
// The SPA treats this differently from ErrUpstream: it surfaces an "use
// OpenRouter instead" hint rather than "check your API key", since no key
// will ever work from a blocked IP.
var ErrGeoBlocked = errors.New("provider blocks requests from this server's region")

// FetchModels calls `{baseURL}/models` with the user's key and returns the
// decoded list. Per-provider quirks:
//
//   - anthropic: uses x-api-key header + anthropic-version, not Bearer.
//     The canonical base URL already ends at /v1 so /models lands on the
//     right native endpoint.
//   - google: OAI-compat layer returns ids like "models/gemini-2.5-flash";
//     we strip the "models/" prefix so chat completions pick them up cleanly.
//   - everyone else (openai/openrouter/deepseek/mistral/custom): standard
//     Bearer + /models.
//
// If `pool` is non-nil the request routes through one of the pool's HTTP
// proxies (needed for OpenAI/Anthropic from geo-blocked server IPs). A nil
// pool goes direct — fine for OpenRouter/DeepSeek/Mistral/Google.
//
// Timeout is 20s — OpenRouter's list is ~400 rows and has been known to take
// 10s+ on cold caches.
func FetchModels(ctx context.Context, provider string, revealed Revealed, pool *outboundproxy.Pool) (*ModelList, error) {
	base := strings.TrimRight(revealed.BaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("byok models: empty base URL")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	switch provider {
	case "anthropic":
		req.Header.Set("x-api-key", revealed.Key)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+revealed.Key)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Transport: pool.Transport()}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		// Geo-block detection: Cloudflare-style 403 with country-blocked
		// markers in the body. Seen from Russia, Iran, China IPs against
		// OpenAI ("unsupported_country_region_territory") and Anthropic
		// ("Request not allowed"). This is never fixable by the user's
		// key — the SPA should surface "use OpenRouter" instead of "check
		// your key" when we see this.
		if resp.StatusCode == http.StatusForbidden && isGeoBlockedMessage(msg) {
			return nil, fmt.Errorf("%w: %s", ErrGeoBlocked, msg)
		}
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrUpstream, resp.StatusCode, msg)
	}

	var list ModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("decode: %w (body: %s)", err, truncate(string(body), 200))
	}

	// Per-provider post-processing.
	switch provider {
	case "google":
		for i := range list.Data {
			list.Data[i].ID = strings.TrimPrefix(list.Data[i].ID, "models/")
		}
	}

	// Sort by id ascending — OpenRouter's list comes back unsorted which
	// makes the picker jarring to read.
	sort.Slice(list.Data, func(i, j int) bool {
		return list.Data[i].ID < list.Data[j].ID
	})

	if list.Object == "" {
		list.Object = "list"
	}
	return &list, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// isGeoBlockedMessage looks for the well-known phrases providers return in
// their 403 bodies when the request IP is in a blocked jurisdiction. We
// check the raw body (case-insensitive) rather than trying to parse per-
// provider JSON — the markers are provider-stable and any regex would just
// re-encode the same substring checks.
func isGeoBlockedMessage(body string) bool {
	low := strings.ToLower(body)
	markers := []string{
		"unsupported_country_region_territory", // OpenAI
		"country, region, or territory not supported",
		"request not allowed", // Anthropic
		"access denied from your region",
		"your region is not supported",
		"geo restricted",
		"geo-restricted",
	}
	for _, m := range markers {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

// ─── Redis cache ─────────────────────────────────────────────────────
//
// Model lists are small (few KB) and rarely change, so we cache them per
// byok_id for 10 minutes. Key is invalidated when the user deletes the key
// (so stale state doesn't linger for other keys in the same provider).

const modelsCacheTTL = 10 * time.Minute

func modelsCacheKey(byokID uuid.UUID) string {
	return "nest:byok-models:" + byokID.String()
}

// GetCachedModels returns a previously-cached list if present. `ok=false`
// covers both "no cache client" and "cache miss".
func GetCachedModels(ctx context.Context, rdb *redis.Client, byokID uuid.UUID) (*ModelList, bool) {
	if rdb == nil {
		return nil, false
	}
	v, err := rdb.Get(ctx, modelsCacheKey(byokID)).Result()
	if err != nil {
		return nil, false
	}
	var list ModelList
	if err := json.Unmarshal([]byte(v), &list); err != nil {
		return nil, false
	}
	return &list, true
}

// SetCachedModels stores the list with the TTL. Errors are swallowed — a
// cache-write failure should never break a user-visible request.
func SetCachedModels(ctx context.Context, rdb *redis.Client, byokID uuid.UUID, list *ModelList) {
	if rdb == nil {
		return
	}
	b, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = rdb.Set(ctx, modelsCacheKey(byokID), b, modelsCacheTTL).Err()
}

// InvalidateModelsCache drops the cached list for a key. Called on delete and
// whenever the user explicitly refreshes.
func InvalidateModelsCache(ctx context.Context, rdb *redis.Client, byokID uuid.UUID) {
	if rdb == nil {
		return
	}
	_ = rdb.Del(ctx, modelsCacheKey(byokID)).Err()
}
