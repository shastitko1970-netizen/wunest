package library

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/users"
)

// Handler hosts /api/library endpoints.
type Handler struct {
	Client          *Client
	Users           *users.Resolver
	CharactersRepo  *characters.Repository
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/library/chub/search", authRequired(http.HandlerFunc(h.chubSearch)))
	mux.Handle("POST /api/library/chub/import", authRequired(http.HandlerFunc(h.chubImport)))
}

// ─── Search ─────────────────────────────────────────────────────────

func (h *Handler) chubSearch(w http.ResponseWriter, r *http.Request) {
	if _, err := h.currentUser(r.Context(), r); err != nil {
		h.writeErr(w, err)
		return
	}

	q := r.URL.Query()
	opts := SearchOptions{
		Query:       q.Get("q"),
		Sort:        q.Get("sort"),
		IncludeNSFW: q.Get("nsfw") == "true",
	}
	if p, err := strconv.Atoi(q.Get("page")); err == nil {
		opts.Page = p
	}
	if p, err := strconv.Atoi(q.Get("per_page")); err == nil {
		opts.PerPage = p
	}
	if t := q.Get("tags"); t != "" {
		opts.IncludeTags = splitAndTrim(t)
	}
	if t := q.Get("exclude_tags"); t != "" {
		opts.ExcludeTags = splitAndTrim(t)
	}

	results, total, err := h.Client.SearchChub(r.Context(), opts)
	if err != nil {
		slog.Error("chub search", "err", err, "query", opts.Query)
		http.Error(w, "upstream search failed", http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": results,
		"count": total,
	})
}

// ─── Import ─────────────────────────────────────────────────────────

type chubImportReq struct {
	FullPath string `json:"full_path"`
}

func (h *Handler) chubImport(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	var req chubImportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	fullPath := strings.TrimSpace(req.FullPath)
	if fullPath == "" || strings.Count(fullPath, "/") != 1 {
		http.Error(w, "full_path must be <creator>/<slug>", http.StatusBadRequest)
		return
	}

	card, err := h.Client.ImportChub(r.Context(), fullPath)
	if err != nil {
		slog.Error("chub import", "err", err, "full_path", fullPath)
		http.Error(w, "chub import failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	created, err := h.CharactersRepo.Create(r.Context(), characters.CreateInput{
		UserID:    user.ID,
		Name:      card.Name,
		Data:      card.Data,
		AvatarURL: card.AvatarURL,
		Tags:      card.Tags,
		Spec:      card.Spec,
		SourceURL: card.SourceURL,
	})
	if err != nil {
		slog.Error("persist chub import", "err", err)
		http.Error(w, "failed to save character", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// ─── helpers ────────────────────────────────────────────────────────

func (h *Handler) currentUser(ctx context.Context, r *http.Request) (*models.NestUser, error) {
	session := auth.FromContext(ctx)
	if session == nil {
		return nil, errUnauthorized
	}
	return h.Users.Resolve(ctx, session.WuApi.ID)
}

var errUnauthorized = errors.New("unauthorized")

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, errUnauthorized) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	slog.Error("library handler", "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func splitAndTrim(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
