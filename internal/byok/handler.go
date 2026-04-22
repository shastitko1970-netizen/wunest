package byok

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/users"
)

// Handler wires /api/byok endpoints onto an http.ServeMux.
type Handler struct {
	Repo  *Repository
	Users *users.Resolver
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/byok", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/byok", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("DELETE /api/byok/{id}", authRequired(http.HandlerFunc(h.delete)))
	// Exposed for the SPA's provider picker so the form dropdown doesn't
	// drift from the server-side allow-list.
	mux.Handle("GET /api/byok/providers", authRequired(http.HandlerFunc(h.providers)))
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
	created, err := h.Repo.Create(r.Context(), CreateInput{
		UserID:   user.ID,
		Provider: req.Provider,
		Label:    req.Label,
		Key:      req.Key,
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
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) providers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": SupportedProviders})
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
