package personas

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shastitko1970-netizen/wunest/internal/db"
)

var ErrNotFound = errors.New("persona not found")

type Repository struct {
	pg *db.Postgres
}

func NewRepository(pg *db.Postgres) *Repository {
	return &Repository{pg: pg}
}

// List returns all personas owned by a user, default first, then alpha.
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]Persona, error) {
	const q = `
		SELECT id, name, description, avatar_url, is_default, created_at
		  FROM nest_personas
		 WHERE user_id = $1
		 ORDER BY is_default DESC, name ASC
	`
	rows, err := r.pg.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list personas: %w", err)
	}
	defer rows.Close()

	out := make([]Persona, 0)
	for rows.Next() {
		var p Persona
		var avatar *string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &avatar, &p.IsDefault, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan persona: %w", err)
		}
		if avatar != nil {
			p.AvatarURL = *avatar
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountByUserID returns the number of personas owned by the user. Used
// by the limits package (M54) to gate creation under Free/Plus tiers.
func (r *Repository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.pg.QueryRow(ctx,
		`SELECT COUNT(*) FROM nest_personas WHERE user_id = $1`,
		userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count personas: %w", err)
	}
	return n, nil
}

// Get fetches one persona, ownership-scoped.
func (r *Repository) Get(ctx context.Context, userID, id uuid.UUID) (*Persona, error) {
	const q = `
		SELECT id, name, description, avatar_url, is_default, created_at
		  FROM nest_personas
		 WHERE user_id = $1 AND id = $2
	`
	var p Persona
	var avatar *string
	err := r.pg.QueryRow(ctx, q, userID, id).Scan(
		&p.ID, &p.Name, &p.Description, &avatar, &p.IsDefault, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get persona: %w", err)
	}
	if avatar != nil {
		p.AvatarURL = *avatar
	}
	return &p, nil
}

// Default returns the user's default persona (is_default=true), if any.
func (r *Repository) Default(ctx context.Context, userID uuid.UUID) (*Persona, error) {
	const q = `
		SELECT id, name, description, avatar_url, is_default, created_at
		  FROM nest_personas
		 WHERE user_id = $1 AND is_default = TRUE
		 LIMIT 1
	`
	var p Persona
	var avatar *string
	err := r.pg.QueryRow(ctx, q, userID).Scan(
		&p.ID, &p.Name, &p.Description, &avatar, &p.IsDefault, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("default persona: %w", err)
	}
	if avatar != nil {
		p.AvatarURL = *avatar
	}
	return &p, nil
}

// Create inserts a new persona. If in.IsDefault is true, any previous default
// is demoted in the same transaction so only one is_default=true per user.
func (r *Repository) Create(ctx context.Context, in CreateInput) (*Persona, error) {
	tx, err := r.pg.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if in.IsDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE nest_personas SET is_default = FALSE WHERE user_id = $1 AND is_default = TRUE`,
			in.UserID,
		); err != nil {
			return nil, fmt.Errorf("clear prior default: %w", err)
		}
	}

	id := uuid.New() // app-side UUID
	const q = `
		INSERT INTO nest_personas (id, user_id, name, description, avatar_url, is_default)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		RETURNING created_at
	`
	var createdAt time.Time
	if err := tx.QueryRow(ctx, q, id, in.UserID, in.Name, in.Description, in.AvatarURL, in.IsDefault).
		Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("insert persona: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Persona{
		ID:          id,
		Name:        in.Name,
		Description: in.Description,
		AvatarURL:   in.AvatarURL,
		IsDefault:   in.IsDefault,
		CreatedAt:   createdAt,
	}, nil
}

// Update patches the mutable fields. Does NOT touch is_default; use SetDefault.
func (r *Repository) Update(ctx context.Context, userID, id uuid.UUID, patch UpdatePatch) (*Persona, error) {
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
	if patch.AvatarURL != nil {
		cur.AvatarURL = *patch.AvatarURL
	}
	const q = `
		UPDATE nest_personas
		   SET name = $3, description = $4, avatar_url = NULLIF($5, '')
		 WHERE user_id = $1 AND id = $2
	`
	tag, err := r.pg.Exec(ctx, q, userID, id, cur.Name, cur.Description, cur.AvatarURL)
	if err != nil {
		return nil, fmt.Errorf("update persona: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return cur, nil
}

// SetDefault flips is_default on one persona and demotes any prior default,
// atomically. Passing id = uuid.Nil clears the default entirely.
func (r *Repository) SetDefault(ctx context.Context, userID, id uuid.UUID) error {
	tx, err := r.pg.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`UPDATE nest_personas SET is_default = FALSE WHERE user_id = $1 AND is_default = TRUE`,
		userID,
	); err != nil {
		return fmt.Errorf("clear prior default: %w", err)
	}

	if id != uuid.Nil {
		tag, err := tx.Exec(ctx,
			`UPDATE nest_personas SET is_default = TRUE WHERE user_id = $1 AND id = $2`,
			userID, id,
		)
		if err != nil {
			return fmt.Errorf("set default: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM nest_personas WHERE user_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, userID, id)
	if err != nil {
		return fmt.Errorf("delete persona: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
