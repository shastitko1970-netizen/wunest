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

// List returns all presets owned by a user, optionally filtered by type.
// Sorted alphabetically by name so the UI picker is predictable.
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
	q += ` ORDER BY name ASC`

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
		if err := json.Unmarshal(data, &p.Data); err != nil {
			return nil, fmt.Errorf("unmarshal preset data: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns one preset by id, scoped to the user.
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
	if err := json.Unmarshal(data, &p.Data); err != nil {
		return nil, fmt.Errorf("unmarshal preset data: %w", err)
	}
	return &p, nil
}

// Create inserts a new preset row. Conflicts on (user_id, type, name)
// return a pgx unique-violation error — the handler translates that into
// a 409 so callers can retry with a different name.
func (r *Repository) Create(ctx context.Context, in CreateInput) (*Preset, error) {
	data, err := json.Marshal(in.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	const q = `
		INSERT INTO nest_presets (user_id, type, name, data)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	var (
		id                       uuid.UUID
		createdAt, updatedAt     time.Time
	)
	if err := r.pg.QueryRow(ctx, q, in.UserID, string(in.Type), in.Name, data).
		Scan(&id, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("insert preset: %w", err)
	}
	return &Preset{
		ID:        id,
		Type:      in.Type,
		Name:      in.Name,
		Data:      in.Data,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// Update applies a sparse patch.
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
	data, err := json.Marshal(cur.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	const q = `
		UPDATE nest_presets
		   SET name = $3, data = $4, updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
		 RETURNING updated_at
	`
	if err := r.pg.QueryRow(ctx, q, userID, id, cur.Name, data).Scan(&cur.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update preset: %w", err)
	}
	return cur, nil
}

// Delete removes a preset row.
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
