package characters

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

// ErrNotFound is returned by the repository when a character row is absent.
var ErrNotFound = errors.New("character not found")

type Repository struct {
	pg *db.Postgres
}

func NewRepository(pg *db.Postgres) *Repository {
	return &Repository{pg: pg}
}

// CreateInput captures the subset of fields a caller can supply when
// creating a character. Server-side fields (id, timestamps) are not part of
// the input.
type CreateInput struct {
	UserID            uuid.UUID
	Name              string
	Data              CharacterData
	AvatarURL         string
	AvatarOriginalURL string
	Tags              []string
	Favorite          bool
	Spec              string
	SourceURL         string
}

// UpdatePatch is the sparse subset of fields that can be changed on an
// existing character. nil = leave as-is.
type UpdatePatch struct {
	Name              *string
	Data              *CharacterData
	AvatarURL         *string
	AvatarOriginalURL *string
	Tags              *[]string
	Favorite          *bool
	SourceURL         *string
}

// List returns all characters owned by the given user, newest-first.
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]Character, error) {
	const q = `
		SELECT id, name, data, COALESCE(avatar_url, ''), COALESCE(avatar_original_url, ''),
		       tags, favorite, spec, COALESCE(source_url, ''),
		       created_at, updated_at
		  FROM nest_characters
		 WHERE user_id = $1
		 ORDER BY favorite DESC, updated_at DESC
	`
	rows, err := r.pg.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list characters: %w", err)
	}
	defer rows.Close()

	out := make([]Character, 0)
	for rows.Next() {
		var c Character
		var dataBytes []byte
		if err := rows.Scan(
			&c.ID, &c.Name, &dataBytes, &c.AvatarURL, &c.AvatarOriginalURL,
			&c.Tags, &c.Favorite, &c.Spec, &c.SourceURL, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan character: %w", err)
		}
		if err := json.Unmarshal(dataBytes, &c.Data); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountByUserID returns the number of characters owned by the user.
// Used by the limits package (M54) to gate the create endpoint when the
// caller is on a Free or Plus tier. Lightweight COUNT(*) — much cheaper
// than List+len in scenarios where we don't need the rows themselves.
func (r *Repository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.pg.QueryRow(ctx,
		`SELECT COUNT(*) FROM nest_characters WHERE user_id = $1`,
		userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count characters: %w", err)
	}
	return n, nil
}

// Get returns a single character by id, scoped to the given user.
func (r *Repository) Get(ctx context.Context, userID, id uuid.UUID) (*Character, error) {
	const q = `
		SELECT id, name, data, COALESCE(avatar_url, ''), COALESCE(avatar_original_url, ''),
		       tags, favorite, spec, COALESCE(source_url, ''),
		       created_at, updated_at
		  FROM nest_characters
		 WHERE user_id = $1 AND id = $2
	`
	var c Character
	var dataBytes []byte
	err := r.pg.QueryRow(ctx, q, userID, id).Scan(
		&c.ID, &c.Name, &dataBytes, &c.AvatarURL, &c.AvatarOriginalURL,
		&c.Tags, &c.Favorite, &c.Spec, &c.SourceURL, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get character: %w", err)
	}
	if err := json.Unmarshal(dataBytes, &c.Data); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	return &c, nil
}

// FindByName does a case-insensitive name lookup scoped to a user, returning
// the first match. Used by chat import to re-associate a restored chat with
// its character when the export carried a `character_name`. Returns
// ErrNotFound when nothing matches.
func (r *Repository) FindByName(ctx context.Context, userID uuid.UUID, name string) (*Character, error) {
	const q = `
		SELECT id, name, data, COALESCE(avatar_url, ''), COALESCE(avatar_original_url, ''),
		       tags, favorite, spec, COALESCE(source_url, ''),
		       created_at, updated_at
		  FROM nest_characters
		 WHERE user_id = $1 AND lower(name) = lower($2)
		 ORDER BY created_at ASC
		 LIMIT 1
	`
	var c Character
	var dataBytes []byte
	err := r.pg.QueryRow(ctx, q, userID, name).Scan(
		&c.ID, &c.Name, &dataBytes, &c.AvatarURL, &c.AvatarOriginalURL,
		&c.Tags, &c.Favorite, &c.Spec, &c.SourceURL, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find by name: %w", err)
	}
	if err := json.Unmarshal(dataBytes, &c.Data); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	return &c, nil
}

// Create inserts a new character row and returns the hydrated model.
func (r *Repository) Create(ctx context.Context, in CreateInput) (*Character, error) {
	dataBytes, err := json.Marshal(in.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	if in.Spec == "" {
		in.Spec = "chara_card_v3"
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}

	// UUID is minted app-side so the DB default (gen_random_uuid) is only a
	// safety net. Keeps us independent of the pgcrypto extension on
	// restricted hosts (Supabase free tier, etc.).
	id := uuid.New()
	const q = `
		INSERT INTO nest_characters (
		    id, user_id, name, data, avatar_url, avatar_original_url,
		    tags, favorite, spec, source_url
		)
		VALUES (
		    $1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''),
		    $7, $8, $9, NULLIF($10, '')
		)
		RETURNING created_at, updated_at
	`
	var (
		createdAt time.Time
		updatedAt time.Time
	)
	err = r.pg.QueryRow(ctx, q,
		id, in.UserID, in.Name, dataBytes, in.AvatarURL, in.AvatarOriginalURL,
		in.Tags, in.Favorite, in.Spec, in.SourceURL,
	).Scan(&createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert character: %w", err)
	}

	return &Character{
		ID:                id,
		Name:              in.Name,
		Data:              in.Data,
		AvatarURL:         in.AvatarURL,
		AvatarOriginalURL: in.AvatarOriginalURL,
		Tags:              in.Tags,
		Favorite:          in.Favorite,
		Spec:              in.Spec,
		SourceURL:         in.SourceURL,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}, nil
}

// Update applies a sparse patch to an existing character row.
func (r *Repository) Update(ctx context.Context, userID, id uuid.UUID, patch UpdatePatch) (*Character, error) {
	// Load current row so we can merge and re-save.
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
	if patch.AvatarURL != nil {
		cur.AvatarURL = *patch.AvatarURL
	}
	if patch.AvatarOriginalURL != nil {
		cur.AvatarOriginalURL = *patch.AvatarOriginalURL
	}
	if patch.Tags != nil {
		cur.Tags = *patch.Tags
	}
	if patch.Favorite != nil {
		cur.Favorite = *patch.Favorite
	}
	if patch.SourceURL != nil {
		cur.SourceURL = *patch.SourceURL
	}

	dataBytes, err := json.Marshal(cur.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	const q = `
		UPDATE nest_characters
		   SET name = $3, data = $4,
		       avatar_url = NULLIF($5, ''),
		       avatar_original_url = NULLIF($6, ''),
		       tags = $7, favorite = $8,
		       source_url = NULLIF($9, ''), updated_at = NOW()
		 WHERE user_id = $1 AND id = $2
		 RETURNING updated_at
	`
	err = r.pg.QueryRow(ctx, q, userID, id,
		cur.Name, dataBytes, cur.AvatarURL, cur.AvatarOriginalURL,
		cur.Tags, cur.Favorite, cur.SourceURL,
	).Scan(&cur.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update character: %w", err)
	}
	return cur, nil
}

// Delete removes a character row.
func (r *Repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM nest_characters WHERE user_id = $1 AND id = $2`
	tag, err := r.pg.Exec(ctx, q, userID, id)
	if err != nil {
		return fmt.Errorf("delete character: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

