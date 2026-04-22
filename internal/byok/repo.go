package byok

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shastitko1970-netizen/wunest/internal/db"
)

var ErrNotFound = errors.New("byok key not found")

// Repository is the storage layer for encrypted BYOK keys. Takes a
// 32-byte master key from config at construction; refuses if the key
// is the wrong size so a mis-configured server dies at startup rather
// than silently writing unreadable rows.
type Repository struct {
	pg        *db.Postgres
	secretKey []byte
}

func NewRepository(pg *db.Postgres, secretKey []byte) (*Repository, error) {
	if len(secretKey) != 32 {
		return nil, fmt.Errorf("byok repo: secret key must be 32 bytes, got %d", len(secretKey))
	}
	return &Repository{pg: pg, secretKey: secretKey}, nil
}

// List returns the user's stored keys without decrypting. We carry a
// pre-computed `masked` preview stored alongside the ciphertext so the
// list view never has to touch AES — only the chat stream path does.
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]Key, error) {
	const q = `
		SELECT id, provider, COALESCE(label, ''), masked, created_at
		  FROM nest_byok
		 WHERE user_id = $1
		 ORDER BY provider ASC, created_at DESC
	`
	rows, err := r.pg.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list byok: %w", err)
	}
	defer rows.Close()

	out := make([]Key, 0)
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Provider, &k.Label, &k.Masked, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan byok: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Create encrypts plaintext and persists the row. Returns the Key with
// masked preview but never plaintext — caller should forget in.Key after
// this returns.
func (r *Repository) Create(ctx context.Context, in CreateInput) (*Key, error) {
	if in.Key == "" {
		return nil, fmt.Errorf("byok: empty key")
	}
	ct, nonce, err := Encrypt(r.secretKey, in.Key)
	if err != nil {
		return nil, err
	}
	masked := Mask(in.Key)
	id := uuid.New()

	const q = `
		INSERT INTO nest_byok (id, user_id, provider, key_encrypted, key_nonce, label, masked)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)
		RETURNING created_at
	`
	var createdAt time.Time
	if err := r.pg.QueryRow(ctx, q, id, in.UserID, in.Provider, ct, nonce, in.Label, masked).
		Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("insert byok: %w", err)
	}
	return &Key{
		ID:        id,
		Provider:  in.Provider,
		Label:     in.Label,
		Masked:    masked,
		CreatedAt: createdAt,
	}, nil
}

// Reveal decrypts and returns the plaintext key. Scoped by user_id so a
// leaked key id can't pull another user's secret. Called from the chat
// stream when a chat is pinned to a BYOK key instead of the WuApi pool.
func (r *Repository) Reveal(ctx context.Context, userID, id uuid.UUID) (string, error) {
	const q = `
		SELECT key_encrypted, key_nonce
		  FROM nest_byok
		 WHERE user_id = $1 AND id = $2
	`
	var ct, nonce []byte
	if err := r.pg.QueryRow(ctx, q, userID, id).Scan(&ct, &nonce); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("select byok: %w", err)
	}
	return Decrypt(r.secretKey, ct, nonce)
}

// Delete removes one key row (ownership-checked).
func (r *Repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pg.Exec(ctx,
		`DELETE FROM nest_byok WHERE user_id = $1 AND id = $2`,
		userID, id)
	if err != nil {
		return fmt.Errorf("delete byok: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
