package presets

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/users"
)

// Handler wires /api/presets onto an http.ServeMux.
type Handler struct {
	Repo  *Repository
	Users *users.Resolver
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/presets", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/presets", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/presets/{id}", authRequired(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/presets/{id}", authRequired(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/presets/{id}", authRequired(http.HandlerFunc(h.delete)))
}

// --- endpoints ---

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	typ := PresetType(r.URL.Query().Get("type")) // optional filter
	items, err := h.Repo.List(r.Context(), user.ID, typ)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
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
	p, err := h.Repo.Get(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type createReq struct {
	Type string      `json:"type"`
	Name string      `json:"name"`
	Data SamplerData `json:"data"`
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
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = string(TypeSampler)
	}
	if !validType(req.Type) {
		http.Error(w, "invalid type", http.StatusBadRequest)
		return
	}
	p, err := h.Repo.Create(r.Context(), CreateInput{
		UserID: user.ID,
		Type:   PresetType(req.Type),
		Name:   strings.TrimSpace(req.Name),
		Data:   req.Data,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "preset with that name already exists", http.StatusConflict)
			return
		}
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

type updateReq struct {
	Name *string      `json:"name,omitempty"`
	Data *SamplerData `json:"data,omitempty"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
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
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	p, err := h.Repo.Update(r.Context(), user.ID, id, UpdatePatch{
		Name: req.Name,
		Data: req.Data,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "preset with that name already exists", http.StatusConflict)
			return
		}
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
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

// --- helpers ---

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
		slog.Error("presets handler", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func validType(s string) bool {
	switch PresetType(s) {
	case TypeSampler, TypeOpenAI, TypeInstruct, TypeContext, TypeSysprompt, TypeReasoning:
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
