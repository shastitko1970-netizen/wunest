// Package quickreplies manages the per-user list of quick-reply
// templates — short text snippets the UI renders as chips above the
// composer.
//
// Click behaviour:
//   - SendNow=false (default): inserts text into the draft, user
//     can edit before hitting Send.
//   - SendNow=true: inserts + auto-sends. Useful for idempotent
//     prompts like "/continue" or "What happens next?".
//
// Macros inside the text (e.g. {{user}}, {{roll::d20}}) expand at
// generation time — quick replies are just a text-delivery
// mechanism, not a separate macro scope.
package quickreplies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/users"
)

var ErrNotFound = errors.New("quick reply not found")

// Reply is one row from nest_quick_replies.
type Reply struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Label     string    `json:"label"`
	Text      string    `json:"text"`
	Position  int       `json:"position"`
	SendNow   bool      `json:"send_now"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Repository struct {
	pg *db.Postgres
}

func NewRepository(pg *db.Postgres) *Repository {
	return &Repository{pg: pg}
}

// List returns the user's quick-replies ordered by position asc
// (lowest = top-left chip).
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]Reply, error) {
	const q = `
		SELECT id, user_id, label, text, position, send_now, created_at, updated_at
		  FROM nest_quick_replies
		 WHERE user_id = $1
		 ORDER BY position ASC, created_at ASC
	`
	rows, err := r.pg.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list quick replies: %w", err)
	}
	defer rows.Close()
	out := make([]Reply, 0)
	for rows.Next() {
		var rep Reply
		if err := rows.Scan(
			&rep.ID, &rep.UserID, &rep.Label, &rep.Text,
			&rep.Position, &rep.SendNow, &rep.CreatedAt, &rep.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan reply: %w", err)
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

type CreateInput struct {
	Label   string
	Text    string
	SendNow bool
}

// Create inserts at position = MAX+1 so new replies go to the end of
// the strip. Users reorder via Update (or a dedicated reorder endpoint
// later if we add drag-drop UI).
func (r *Repository) Create(ctx context.Context, userID uuid.UUID, in CreateInput) (*Reply, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, errors.New("text required")
	}
	if strings.TrimSpace(in.Label) == "" {
		// Fallback to a shortened text snippet for the chip label so
		// the user doesn't have to think twice.
		in.Label = in.Text
		if len(in.Label) > 24 {
			in.Label = in.Label[:24] + "…"
		}
	}
	id := uuid.New()
	const q = `
		INSERT INTO nest_quick_replies (id, user_id, label, text, position, send_now)
		VALUES ($1, $2, $3, $4,
		    COALESCE((SELECT MAX(position)+1 FROM nest_quick_replies WHERE user_id = $2), 0),
		    $5)
		RETURNING position, created_at, updated_at
	`
	rep := &Reply{
		ID:      id,
		UserID:  userID,
		Label:   in.Label,
		Text:    in.Text,
		SendNow: in.SendNow,
	}
	if err := r.pg.QueryRow(ctx, q, id, userID, in.Label, in.Text, in.SendNow).
		Scan(&rep.Position, &rep.CreatedAt, &rep.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert quick reply: %w", err)
	}
	return rep, nil
}

type UpdatePatch struct {
	Label    *string
	Text     *string
	Position *int
	SendNow  *bool
}

// Update applies a sparse patch. Nil fields are left alone.
func (r *Repository) Update(ctx context.Context, userID, id uuid.UUID, patch UpdatePatch) (*Reply, error) {
	cur, err := r.get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if patch.Label != nil {
		cur.Label = *patch.Label
	}
	if patch.Text != nil {
		cur.Text = *patch.Text
	}
	if patch.Position != nil {
		cur.Position = *patch.Position
	}
	if patch.SendNow != nil {
		cur.SendNow = *patch.SendNow
	}
	const q = `
		UPDATE nest_quick_replies
		   SET label = $3, text = $4, position = $5, send_now = $6, updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
		 RETURNING updated_at
	`
	if err := r.pg.QueryRow(ctx, q,
		userID, id, cur.Label, cur.Text, cur.Position, cur.SendNow,
	).Scan(&cur.UpdatedAt); err != nil {
		return nil, fmt.Errorf("update quick reply: %w", err)
	}
	return cur, nil
}

func (r *Repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pg.Exec(ctx,
		`DELETE FROM nest_quick_replies WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	if err != nil {
		return fmt.Errorf("delete quick reply: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) get(ctx context.Context, userID, id uuid.UUID) (*Reply, error) {
	const q = `
		SELECT id, user_id, label, text, position, send_now, created_at, updated_at
		  FROM nest_quick_replies
		 WHERE user_id = $1 AND id = $2
	`
	var rep Reply
	err := r.pg.QueryRow(ctx, q, userID, id).Scan(
		&rep.ID, &rep.UserID, &rep.Label, &rep.Text,
		&rep.Position, &rep.SendNow, &rep.CreatedAt, &rep.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get quick reply: %w", err)
	}
	return &rep, nil
}

// ── HTTP handler ──────────────────────────────────────────────────

type Handler struct {
	Repo  *Repository
	Users *users.Resolver
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/quick-replies", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/quick-replies", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /api/quick-replies/{id}", authRequired(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/quick-replies/{id}", authRequired(http.HandlerFunc(h.delete)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	items, err := h.Repo.List(r.Context(), user.ID)
	if err != nil {
		slog.Error("list quick replies", "err", err, "user_id", user.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		Label   string `json:"label"`
		Text    string `json:"text"`
		SendNow bool   `json:"send_now"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	rep, err := h.Repo.Create(r.Context(), user.ID, CreateInput{
		Label: req.Label, Text: req.Text, SendNow: req.SendNow,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, rep)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Label    *string `json:"label,omitempty"`
		Text     *string `json:"text,omitempty"`
		Position *int    `json:"position,omitempty"`
		SendNow  *bool   `json:"send_now,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	rep, err := h.Repo.Update(r.Context(), user.ID, id, UpdatePatch{
		Label: req.Label, Text: req.Text, Position: req.Position, SendNow: req.SendNow,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.Repo.Delete(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func (h *Handler) currentUser(ctx context.Context, r *http.Request) (*models.NestUser, error) {
	session := auth.FromContext(ctx)
	if session == nil {
		return nil, errors.New("unauthorized")
	}
	return h.Users.Resolve(ctx, session.WuApi.ID)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
