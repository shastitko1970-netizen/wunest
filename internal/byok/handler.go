package byok

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/users"
)

// Handler wires /api/byok endpoints onto an http.ServeMux.
//
// Redis is optional — model-list caching degrades gracefully to a pass-through
// when it's nil (e.g. dev laptops without Redis running).
type Handler struct {
	Repo  *Repository
	Users *users.Resolver
	Redis *redis.Client
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/byok", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/byok", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("DELETE /api/byok/{id}", authRequired(http.HandlerFunc(h.delete)))
	// Exposed for the SPA's provider picker so the form dropdown doesn't
	// drift from the server-side allow-list.
	mux.Handle("GET /api/byok/providers", authRequired(http.HandlerFunc(h.providers)))
	// Live-fetch the provider's model catalogue. Redis-cached for 10 min so
	// picker open latency stays in single-digit ms after a warm-up call.
	// `?refresh=1` bypasses the cache on demand.
	mux.Handle("GET /api/byok/{id}/models", authRequired(http.HandlerFunc(h.models)))
	// Explicit key-check: pings the provider and returns {ok, model_count,
	// sample[]} or {ok:false, error}. Used by the "Test" button in the
	// BYOK settings panel.
	mux.Handle("POST /api/byok/{id}/test", authRequired(http.HandlerFunc(h.test)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	keys, err := h.Repo.List(r.Context(), user.ID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": keys})
}

type createReq struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Key      string `json:"key"`
	BaseURL  string `json:"base_url"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Label = strings.TrimSpace(req.Label)
	req.Key = strings.TrimSpace(req.Key)
	req.BaseURL = strings.TrimSpace(req.BaseURL)

	if !IsSupportedProvider(req.Provider) {
		http.Error(w, "unsupported provider", http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	// Soft upper bound — real provider keys are ≤200 chars. Anything bigger
	// is either pasted junk or someone probing the endpoint.
	if len(req.Key) > 2048 {
		http.Error(w, "key too long", http.StatusBadRequest)
		return
	}
	// Base URL validation: must start with http(s):// or be empty (we'll
	// fill the default per provider). "custom" REQUIRES an explicit URL.
	if req.BaseURL != "" {
		if !strings.HasPrefix(req.BaseURL, "http://") && !strings.HasPrefix(req.BaseURL, "https://") {
			http.Error(w, "base_url must start with http:// or https://", http.StatusBadRequest)
			return
		}
		// Strip trailing slash so we can concatenate `/chat/completions` later
		// without producing double slashes.
		req.BaseURL = strings.TrimRight(req.BaseURL, "/")
	}
	if req.Provider == "custom" && req.BaseURL == "" {
		http.Error(w, "base_url required for custom provider", http.StatusBadRequest)
		return
	}
	created, err := h.Repo.Create(r.Context(), CreateInput{
		UserID:   user.ID,
		Provider: req.Provider,
		Label:    req.Label,
		Key:      req.Key,
		BaseURL:  req.BaseURL,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	// Clear plaintext from our local buffer before responding (best-effort
	// — Go strings are immutable, so we can only drop the reference; the
	// request body buffer is GC'd by net/http after the handler returns).
	req.Key = ""
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.Repo.Delete(r.Context(), user.ID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	// Drop any cached model list for the deleted key; stops a future key with
	// the same UUID (vanishingly unlikely) from seeing stale data.
	InvalidateModelsCache(r.Context(), h.Redis, id)
	w.WriteHeader(http.StatusNoContent)
}

// models live-fetches the catalogue of models offered by the provider behind
// the given BYOK key. Cached in Redis (10 min TTL); pass `?refresh=1` to skip
// the cache.
//
// Flow:
//  1. Cache hit → return immediately.
//  2. Reveal the key (decrypted only here, never sent back to the SPA).
//  3. Call `{baseURL}/models` with provider-appropriate auth.
//  4. Cache the result & return.
//
// Upstream failures surface as 502 with the provider's own error message
// (truncated) so the user can tell whether it's a bad key vs. a dead URL.
func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	refresh := r.URL.Query().Get("refresh") == "1"
	if !refresh {
		if cached, ok := GetCachedModels(r.Context(), h.Redis, id); ok {
			writeJSON(w, http.StatusOK, cached)
			return
		}
	}

	// Need provider to know which auth header to set. One cheap lookup — this
	// path isn't in the chat hot loop.
	provider, err := h.Repo.GetProvider(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	revealed, err := h.Repo.Reveal(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	list, err := FetchModels(r.Context(), provider, revealed)
	if err != nil {
		// Log the full error server-side (with base URL, provider, message)
		// so we can diagnose when a user reports "list is empty". User-
		// visible body stays the truncated provider message.
		slog.Error("byok fetch models",
			"err", err,
			"provider", provider,
			"base_url", revealed.BaseURL,
			"byok_id", id,
		)
		if errors.Is(err, ErrUpstream) {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	SetCachedModels(r.Context(), h.Redis, id, list)
	writeJSON(w, http.StatusOK, list)
}

// test pings the provider with a cheap request (model list fetch) to verify
// the stored key actually works. Surfaces the provider's own error message
// so the user can see whether it's a bad key, wrong base URL, rate-limited,
// etc. Returns 200 on success with `{ok:true, model_count:N}`; 4xx/5xx with
// `{ok:false, error: "..."}` on any failure.
//
// Bypasses the Redis cache so it's a real live ping. Does NOT write to the
// cache — a `Test` click that happens to succeed shouldn't silently prime
// the cache with a stale list seconds before it expires naturally.
func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	provider, err := h.Repo.GetProvider(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	revealed, err := h.Repo.Reveal(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	list, err := FetchModels(r.Context(), provider, revealed)
	if err != nil {
		slog.Warn("byok test failed",
			"err", err,
			"provider", provider,
			"base_url", revealed.BaseURL,
		)
		// 200 with ok:false so the frontend doesn't have to parse 4xx body
		// shapes that differ by provider. The SPA renders `error` inline
		// under the key card.
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"model_count": len(list.Data),
		"sample":      sampleModelIDs(list, 3),
	})
}

// sampleModelIDs picks up to n model ids from the catalogue — useful in the
// Test response so the user sees "yes it really returned {gpt-4o, gpt-4o-mini,
// o1}", confirming both the auth AND the base URL are right.
func sampleModelIDs(list *ModelList, n int) []string {
	if list == nil {
		return nil
	}
	if n > len(list.Data) {
		n = len(list.Data)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, list.Data[i].ID)
	}
	return out
}

// ProviderInfo is a single entry in the providers allow-list returned to
// the SPA. The UI uses DefaultURL to pre-fill the base-URL field when
// the user picks a provider.
type ProviderInfo struct {
	ID         string `json:"id"`
	DefaultURL string `json:"default_url,omitempty"`
}

func (h *Handler) providers(w http.ResponseWriter, _ *http.Request) {
	items := make([]ProviderInfo, 0, len(SupportedProviders))
	for _, id := range SupportedProviders {
		items = append(items, ProviderInfo{
			ID:         id,
			DefaultURL: DefaultBaseURL(id),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ─── helpers ───────────────────────────────────────────────────────

func (h *Handler) currentUser(ctx context.Context, r *http.Request) (*models.NestUser, error) {
	session := auth.FromContext(ctx)
	if session == nil {
		return nil, errUnauthorized
	}
	return h.Users.Resolve(ctx, session.WuApi.ID)
}

var errUnauthorized = errors.New("unauthorized")

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, errUnauthorized):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	default:
		slog.Error("byok handler", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
