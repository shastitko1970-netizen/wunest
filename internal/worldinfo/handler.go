package worldinfo

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/limits"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/users"
)

// Handler wires /api/worlds and character-attachment routes onto http.ServeMux.
type Handler struct {
	Repo       *Repository
	Users      *users.Resolver
	Characters *characters.Repository // for ownership check on attachment
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/worlds", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/worlds", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("POST /api/worlds/import", authRequired(http.HandlerFunc(h.importST)))
	mux.Handle("GET /api/worlds/{id}", authRequired(http.HandlerFunc(h.get)))
	mux.Handle("PUT /api/worlds/{id}", authRequired(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/worlds/{id}", authRequired(http.HandlerFunc(h.delete)))

	// Per-character attachment.
	mux.Handle("GET /api/characters/{id}/worlds", authRequired(http.HandlerFunc(h.listForChar)))
	mux.Handle("PUT /api/characters/{id}/worlds/{wid}", authRequired(http.HandlerFunc(h.attach)))
	mux.Handle("DELETE /api/characters/{id}/worlds/{wid}", authRequired(http.HandlerFunc(h.detach)))
}

// ─── CRUD ───────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	items, err := h.Repo.List(r.Context(), user.ID)
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
	wl, err := h.Repo.Get(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wl)
}

type createReq struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Entries     []Entry `json:"entries"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	// M54.2 — slot-cap enforcement (Free=3 / Plus=10 / Pro=∞).
	if err := h.enforceCreateLimit(r, user.ID); err != nil {
		if le, ok := limits.IsLimitReached(err); ok {
			limits.WriteError(w, le)
			return
		}
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
	wl, err := h.Repo.Create(r.Context(), user.ID, strings.TrimSpace(req.Name), req.Description, req.Entries)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, wl)
}

type updateReq struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Entries     *[]Entry `json:"entries,omitempty"`
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
	wl, err := h.Repo.Update(r.Context(), user.ID, id, UpdatePatch{
		Name:        req.Name,
		Description: req.Description,
		Entries:     req.Entries,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wl)
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

// ─── ST .json import ────────────────────────────────────────────────

// stImportReq accepts either ST's newer shape ({"name","entries":[...]}) or
// the classic one ({"name","entries":{"0":{...}}}). The handler normalises to
// []Entry on the way in.
type stImportReq struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Entries     json.RawMessage `json:"entries"`
}

// stEntry is the ST wire shape for a lorebook entry — field names differ
// slightly from our canonical Entry so we map them explicitly.
type stEntry struct {
	UID              int      `json:"uid"`
	ID               int      `json:"id"`
	Key              []string `json:"key"`
	Keys             []string `json:"keys"`
	KeySecondary     []string `json:"keysecondary"`
	SecondaryKeys    []string `json:"secondary_keys"`
	Comment          string   `json:"comment"`
	Name             string   `json:"name"`
	Content          string   `json:"content"`
	Constant         bool     `json:"constant"`
	Selective        bool     `json:"selective"`
	Disable          bool     `json:"disable"`
	Enabled          *bool    `json:"enabled"`
	Order            int      `json:"order"`
	InsertionOrder   int      `json:"insertion_order"`
	Position         any      `json:"position"` // ST sometimes uses int (0..4), sometimes string
	Depth            int      `json:"depth"`
	ScanDepth        int      `json:"scan_depth"`
	CaseSensitive    *bool    `json:"case_sensitive"`
	Probability      int      `json:"probability"`
	UseProbability   *bool    `json:"useProbability"`
	ExtraDontActivate bool    `json:"extra_dont_activate"`
}

func (h *Handler) importST(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	// M54.2 — imports count the same as creates against the slot cap.
	if err := h.enforceCreateLimit(r, user.ID); err != nil {
		if le, ok := limits.IsLimitReached(err); ok {
			limits.WriteError(w, le)
			return
		}
		h.writeErr(w, err)
		return
	}

	// Accept up to 8 MiB of lorebook JSON (many community books are large).
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024*1024)

	var req stImportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	entries, err := parseSTEntries(req.Entries)
	if err != nil {
		http.Error(w, "invalid entries: "+err.Error(), http.StatusBadRequest)
		return
	}

	wl, err := h.Repo.Create(r.Context(), user.ID, strings.TrimSpace(req.Name), req.Description, entries)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, wl)
}

// parseSTEntries handles both ST shapes: array or object-keyed map.
func parseSTEntries(raw json.RawMessage) ([]Entry, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []Entry{}, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var arr []stEntry
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, err
		}
		return convertSTEntries(arr), nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var asMap map[string]stEntry
		if err := json.Unmarshal(raw, &asMap); err != nil {
			return nil, err
		}
		arr := make([]stEntry, 0, len(asMap))
		for _, v := range asMap {
			arr = append(arr, v)
		}
		return convertSTEntries(arr), nil
	}
	return nil, errors.New("entries must be an array or object")
}

func convertSTEntries(in []stEntry) []Entry {
	out := make([]Entry, 0, len(in))
	for _, s := range in {
		// Enabled is derived: ST uses `disable:true` to mean off; `enabled` is
		// newer. If neither is set, treat as enabled.
		enabled := true
		if s.Enabled != nil {
			enabled = *s.Enabled
		} else if s.Disable {
			enabled = false
		}

		keys := s.Keys
		if len(keys) == 0 {
			keys = s.Key
		}
		sec := s.SecondaryKeys
		if len(sec) == 0 {
			sec = s.KeySecondary
		}

		id := s.ID
		if id == 0 && s.UID != 0 {
			id = s.UID
		}

		insOrder := s.InsertionOrder
		if insOrder == 0 && s.Order != 0 {
			insOrder = s.Order
		}

		depth := s.Depth
		if depth == 0 && s.ScanDepth != 0 {
			depth = s.ScanDepth
		}

		name := s.Name
		if name == "" {
			name = s.Comment
		}

		pos := stringifyPosition(s.Position)

		out = append(out, Entry{
			ID:             id,
			Name:           name,
			Comment:        s.Comment,
			Keys:           keys,
			SecondaryKeys:  sec,
			Content:        s.Content,
			Enabled:        enabled && !s.ExtraDontActivate,
			Selective:      s.Selective,
			Constant:       s.Constant,
			InsertionOrder: insOrder,
			Position:       pos,
			Depth:          depth,
			CaseSensitive:  s.CaseSensitive,
		})
	}
	return out
}

// stringifyPosition maps ST's int positions into our string scheme:
//   0,1 → before_char
//   2,3 → after_char
//   4   → after_char (at depth; depth not honoured yet in v1)
// A string value is forwarded as-is if it matches a supported constant.
func stringifyPosition(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		if x == PositionBeforeChar || x == PositionAfterChar {
			return x
		}
		return ""
	case float64:
		if int(x) >= 2 {
			return PositionAfterChar
		}
		return PositionBeforeChar
	}
	return ""
}

// ─── Character attachment ──────────────────────────────────────────

func (h *Handler) listForChar(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	cid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid character id", http.StatusBadRequest)
		return
	}
	// Ownership check: caller must own the character. (This also serves as
	// 404 for a character id that doesn't exist at all.)
	if _, err := h.Characters.Get(r.Context(), user.ID, cid); err != nil {
		h.writeErr(w, err)
		return
	}
	ids, err := h.Repo.AttachedIDs(r.Context(), user.ID, cid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"world_ids": ids})
}

func (h *Handler) attach(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	cid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid character id", http.StatusBadRequest)
		return
	}
	wid, err := uuid.Parse(r.PathValue("wid"))
	if err != nil {
		http.Error(w, "invalid world id", http.StatusBadRequest)
		return
	}
	// Verify both belong to this user before creating the link.
	if _, err := h.Characters.Get(r.Context(), user.ID, cid); err != nil {
		h.writeErr(w, err)
		return
	}
	if _, err := h.Repo.Get(r.Context(), user.ID, wid); err != nil {
		h.writeErr(w, err)
		return
	}
	if err := h.Repo.Attach(r.Context(), cid, wid); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) detach(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	cid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid character id", http.StatusBadRequest)
		return
	}
	wid, err := uuid.Parse(r.PathValue("wid"))
	if err != nil {
		http.Error(w, "invalid world id", http.StatusBadRequest)
		return
	}
	// Ownership check on the character is enough — if the link row exists
	// but the character isn't ours, we wouldn't have the cid anyway.
	if _, err := h.Characters.Get(r.Context(), user.ID, cid); err != nil {
		h.writeErr(w, err)
		return
	}
	if err := h.Repo.Detach(r.Context(), cid, wid); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── helpers ───────────────────────────────────────────────────────

func (h *Handler) currentUser(ctx context.Context, r *http.Request) (*models.NestUser, error) {
	session := auth.FromContext(ctx)
	if session == nil {
		return nil, errUnauthorized
	}
	return h.Users.Resolve(ctx, session.WuApi.ID)
}

// enforceCreateLimit returns nil if the user can create another lorebook,
// *limits.ErrLimitReached when they've hit their slot cap. Reused by
// both `create` and `importST` so PNG-extracted books, ST imports and
// fresh creations all share the same gate.
func (h *Handler) enforceCreateLimit(r *http.Request, userID uuid.UUID) error {
	session := auth.FromContext(r.Context())
	if session == nil {
		return errUnauthorized
	}
	level := session.WuApi.CurrentNestLevel()
	count, err := h.Repo.CountByUserID(r.Context(), userID)
	if err != nil {
		return err
	}
	return limits.Check(level, limits.ResourceLorebook, count)
}

var errUnauthorized = errors.New("unauthorized")

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, characters.ErrNotFound):
		http.Error(w, "character not found", http.StatusNotFound)
	case errors.Is(err, errUnauthorized):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	default:
		slog.Error("worldinfo handler", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
