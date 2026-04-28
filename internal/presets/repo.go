package presets

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

var ErrNotFound = errors.New("preset not found")

type Repository struct {
	pg *db.Postgres
}

func NewRepository(pg *db.Postgres) *Repository {
	return &Repository{pg: pg}
}

// List returns presets owned by a user, optionally filtered by type.
// Data is kept as RawMessage through the DB round-trip — no typed decode
// unless a caller explicitly wants it.
func (r *Repository) List(ctx context.Context, userID uuid.UUID, typ PresetType) ([]Preset, error) {
	q := `
		SELECT id, type, name, data, created_at, updated_at
		  FROM nest_presets
		 WHERE user_id = $1
	`
	args := []any{userID}
	if typ != "" {
		q += ` AND type = $2`
		args = append(args, string(typ))
	}
	q += ` ORDER BY type, name ASC`

	rows, err := r.pg.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	defer rows.Close()

	out := make([]Preset, 0)
	for rows.Next() {
		var p Preset
		var typeStr string
		var data []byte
		if err := rows.Scan(&p.ID, &typeStr, &p.Name, &data, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}
		p.Type = PresetType(typeStr)
		// Copy so subsequent iterations don't clobber the shared buffer.
		p.Data = append(json.RawMessage(nil), data...)
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountByUserID returns the total number of presets owned by the user
// across all types (sampler/instruct/context/sysprompt/reasoning). The
// limit is per-user-total, not per-type — matches the user-facing
// "presets" slot count in the pricing UI.
func (r *Repository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.pg.QueryRow(ctx,
		`SELECT COUNT(*) FROM nest_presets WHERE user_id = $1`,
		userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count presets: %w", err)
	}
	return n, nil
}

func (r *Repository) Get(ctx context.Context, userID, id uuid.UUID) (*Preset, error) {
	const q = `
		SELECT id, type, name, data, created_at, updated_at
		  FROM nest_presets
		 WHERE user_id = $1 AND id = $2
	`
	var p Preset
	var typeStr string
	var data []byte
	err := r.pg.QueryRow(ctx, q, userID, id).Scan(
		&p.ID, &typeStr, &p.Name, &data, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get preset: %w", err)
	}
	p.Type = PresetType(typeStr)
	p.Data = append(json.RawMessage(nil), data...)
	return &p, nil
}

func (r *Repository) Create(ctx context.Context, in CreateInput) (*Preset, error) {
	// Data can be any JSON — we just need it to be valid JSON for JSONB.
	// Default to an empty object if the caller left it nil.
	data := in.Data
	if len(data) == 0 {
		data = json.RawMessage("{}")
	}
	id := uuid.New() // app-side UUID
	const q = `
		INSERT INTO nest_presets (id, user_id, type, name, data)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at
	`
	var createdAt, updatedAt time.Time
	if err := r.pg.QueryRow(ctx, q, id, in.UserID, string(in.Type), in.Name, []byte(data)).
		Scan(&createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("insert preset: %w", err)
	}
	return &Preset{
		ID:        id,
		Type:      in.Type,
		Name:      in.Name,
		Data:      data,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func (r *Repository) Update(ctx context.Context, userID, id uuid.UUID, patch UpdatePatch) (*Preset, error) {
	cur, err := r.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		cur.Name = *patch.Name
	}
	if patch.Data != nil {
		cur.Data = *patch.Data
	}
	const q = `
		UPDATE nest_presets
		   SET name = $3, data = $4, updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
		 RETURNING updated_at
	`
	if err := r.pg.QueryRow(ctx, q, userID, id, cur.Name, []byte(cur.Data)).Scan(&cur.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update preset: %w", err)
	}
	return cur, nil
}

func (r *Repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM nest_presets WHERE user_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, userID, id)
	if err != nil {
		return fmt.Errorf("delete preset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
