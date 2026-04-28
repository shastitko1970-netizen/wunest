package worldinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shastitko1970-netizen/wunest/internal/db"
)

var ErrNotFound = errors.New("world not found")

type Repository struct {
	pg *db.Postgres
}

func NewRepository(pg *db.Postgres) *Repository {
	return &Repository{pg: pg}
}

// List returns light summaries of a user's books (no entries payload).
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]Summary, error) {
	const q = `
		SELECT id, name, description, COALESCE(jsonb_array_length(entries), 0), updated_at
		  FROM nest_worlds
		 WHERE user_id = $1
		 ORDER BY updated_at DESC
	`
	rows, err := r.pg.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list worlds: %w", err)
	}
	defer rows.Close()

	out := make([]Summary, 0)
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.EntryCount, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan world summary: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CountByUserID returns the number of lorebooks owned by the user. Used
// by the limits package (M54) to gate creation under Free/Plus tiers.
func (r *Repository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.pg.QueryRow(ctx,
		`SELECT COUNT(*) FROM nest_worlds WHERE user_id = $1`,
		userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count worlds: %w", err)
	}
	return n, nil
}

// Get fetches a full world with entries (ownership-checked).
func (r *Repository) Get(ctx context.Context, userID, id uuid.UUID) (*World, error) {
	const q = `
		SELECT id, name, description, entries, created_at, updated_at
		  FROM nest_worlds
		 WHERE user_id = $1 AND id = $2
	`
	var w World
	var entriesRaw []byte
	err := r.pg.QueryRow(ctx, q, userID, id).Scan(
		&w.ID, &w.Name, &w.Description, &entriesRaw, &w.CreatedAt, &w.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get world: %w", err)
	}
	if err := decodeEntries(entriesRaw, &w.Entries); err != nil {
		return nil, err
	}
	return &w, nil
}

// Create inserts a new world. If entries is nil, defaults to an empty array.
func (r *Repository) Create(ctx context.Context, userID uuid.UUID, name, description string, entries []Entry) (*World, error) {
	if entries == nil {
		entries = []Entry{}
	}
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal entries: %w", err)
	}
	id := uuid.New() // app-side UUID
	const q = `
		INSERT INTO nest_worlds (id, user_id, name, description, entries)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at
	`
	var createdAt, updatedAt time.Time
	if err := r.pg.QueryRow(ctx, q, id, userID, name, description, entriesJSON).
		Scan(&createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("insert world: %w", err)
	}
	return &World{
		ID:          id,
		Name:        name,
		Description: description,
		Entries:     entries,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

// UpdatePatch bundles the fields a PUT can change. A nil pointer means leave alone.
type UpdatePatch struct {
	Name        *string
	Description *string
	Entries     *[]Entry
}

func (r *Repository) Update(ctx context.Context, userID, id uuid.UUID, patch UpdatePatch) (*World, error) {
	cur, err := r.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		cur.Name = *patch.Name
	}
	if patch.Description != nil {
		cur.Description = *patch.Description
	}
	if patch.Entries != nil {
		cur.Entries = *patch.Entries
	}
	entriesJSON, err := json.Marshal(cur.Entries)
	if err != nil {
		return nil, fmt.Errorf("marshal entries: %w", err)
	}
	const q = `
		UPDATE nest_worlds
		   SET name = $3, description = $4, entries = $5, updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
		 RETURNING updated_at
	`
	if err := r.pg.QueryRow(ctx, q, userID, id, cur.Name, cur.Description, entriesJSON).
		Scan(&cur.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update world: %w", err)
	}
	return cur, nil
}

func (r *Repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM nest_worlds WHERE user_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, userID, id)
	if err != nil {
		return fmt.Errorf("delete world: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Attachment to characters ──────────────────────────────────────

// Attach links a world to a character. Both are owned by the same user;
// the caller is responsible for verifying that.
func (r *Repository) Attach(ctx context.Context, characterID, worldID uuid.UUID) error {
	const q = `
		INSERT INTO nest_character_worlds (character_id, world_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := r.pg.Exec(ctx, q, characterID, worldID)
	if err != nil {
		return fmt.Errorf("attach world: %w", err)
	}
	return nil
}

func (r *Repository) Detach(ctx context.Context, characterID, worldID uuid.UUID) error {
	const q = `DELETE FROM nest_character_worlds WHERE character_id = $1 AND world_id = $2`
	_, err := r.pg.Exec(ctx, q, characterID, worldID)
	if err != nil {
		return fmt.Errorf("detach world: %w", err)
	}
	return nil
}

// ListForCharacter returns every world attached to a character. Ownership is
// enforced via the user_id on nest_worlds so a leaked character id can't pull
// another user's books.
func (r *Repository) ListForCharacter(ctx context.Context, userID, characterID uuid.UUID) ([]World, error) {
	const q = `
		SELECT w.id, w.name, w.description, w.entries, w.created_at, w.updated_at
		  FROM nest_worlds w
		  JOIN nest_character_worlds cw ON cw.world_id = w.id
		 WHERE cw.character_id = $1 AND w.user_id = $2
		 ORDER BY w.name ASC
	`
	rows, err := r.pg.Query(ctx, q, characterID, userID)
	if err != nil {
		return nil, fmt.Errorf("list worlds for character: %w", err)
	}
	defer rows.Close()

	out := make([]World, 0)
	for rows.Next() {
		var w World
		var entriesRaw []byte
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &entriesRaw, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan world: %w", err)
		}
		if err := decodeEntries(entriesRaw, &w.Entries); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// AttachedIDs returns the set of world IDs attached to a character. Used by
// the UI to render the current selection without refetching full books.
func (r *Repository) AttachedIDs(ctx context.Context, userID, characterID uuid.UUID) ([]uuid.UUID, error) {
	const q = `
		SELECT w.id
		  FROM nest_worlds w
		  JOIN nest_character_worlds cw ON cw.world_id = w.id
		 WHERE cw.character_id = $1 AND w.user_id = $2
	`
	rows, err := r.pg.Query(ctx, q, characterID, userID)
	if err != nil {
		return nil, fmt.Errorf("attached ids: %w", err)
	}
	defer rows.Close()

	out := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// decodeEntries tolerates both array and object-keyed ST shapes, producing a
// deterministic []Entry. Migration 002 normalises storage to array, but old
// imports that snuck in as objects still round-trip cleanly.
func decodeEntries(raw []byte, dest *[]Entry) error {
	if len(raw) == 0 || string(raw) == "null" {
		*dest = []Entry{}
		return nil
	}
	// Try array first (our canonical shape).
	if err := json.Unmarshal(raw, dest); err == nil {
		if *dest == nil {
			*dest = []Entry{}
		}
		return nil
	}
	// Fall back to ST's object-keyed shape: { "0": {...}, "1": {...} }.
	var asObj map[string]Entry
	if err := json.Unmarshal(raw, &asObj); err != nil {
		return fmt.Errorf("decode entries: %w", err)
	}
	// Preserve insertion order by sorting on numeric key if possible.
	out := make([]Entry, 0, len(asObj))
	for _, v := range asObj {
		out = append(out, v)
	}
	*dest = out
	return nil
}
