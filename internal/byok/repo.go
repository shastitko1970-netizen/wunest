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
//
// Rows created before M24 have an empty `base_url`; we fill in the
// canonical URL for known providers at read time so the UI can display
// it without asking the user to migrate their data.
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]Key, error) {
	const q = `
		SELECT id, provider, COALESCE(label, ''), masked, COALESCE(base_url, ''), created_at
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
		if err := rows.Scan(&k.ID, &k.Provider, &k.Label, &k.Masked, &k.BaseURL, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan byok: %w", err)
		}
		if k.BaseURL == "" {
			k.BaseURL = DefaultBaseURL(k.Provider)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Create encrypts plaintext and persists the row. Returns the Key with
// masked preview but never plaintext — caller should forget in.Key after
// this returns.
//
// If BaseURL is empty the repo fills in the provider's default; "custom"
// with no BaseURL is rejected so we never persist a routable-nowhere key.
func (r *Repository) Create(ctx context.Context, in CreateInput) (*Key, error) {
	if in.Key == "" {
		return nil, fmt.Errorf("byok: empty key")
	}
	baseURL := in.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL(in.Provider)
	}
	if baseURL == "" {
		return nil, fmt.Errorf("byok: base URL required for provider %q", in.Provider)
	}

	ct, nonce, err := Encrypt(r.secretKey, in.Key)
	if err != nil {
		return nil, err
	}
	masked := Mask(in.Key)
	id := uuid.New()

	const q = `
		INSERT INTO nest_byok (id, user_id, provider, key_encrypted, key_nonce, label, masked, base_url)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8)
		RETURNING created_at
	`
	var createdAt time.Time
	if err := r.pg.QueryRow(ctx, q, id, in.UserID, in.Provider, ct, nonce, in.Label, masked, baseURL).
		Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("insert byok: %w", err)
	}
	return &Key{
		ID:        id,
		Provider:  in.Provider,
		Label:     in.Label,
		Masked:    masked,
		BaseURL:   baseURL,
		CreatedAt: createdAt,
	}, nil
}

// GetProvider returns just the provider string for an owned key id. Cheap
// scan used by the model-catalogue handler to pick the right auth scheme
// without paying the AES decrypt cost.
func (r *Repository) GetProvider(ctx context.Context, userID, id uuid.UUID) (string, error) {
	var provider string
	err := r.pg.QueryRow(ctx,
		`SELECT provider FROM nest_byok WHERE user_id = $1 AND id = $2`,
		userID, id).Scan(&provider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get provider: %w", err)
	}
	return provider, nil
}

// Reveal decrypts and returns the plaintext key plus the base URL to
// route to. Scoped by user_id so a leaked key id can't pull another
// user's secret. Called from the chat stream when a chat is pinned to a
// BYOK key instead of the WuApi pool.
func (r *Repository) Reveal(ctx context.Context, userID, id uuid.UUID) (Revealed, error) {
	const q = `
		SELECT key_encrypted, key_nonce, provider, COALESCE(base_url, '')
		  FROM nest_byok
		 WHERE user_id = $1 AND id = $2
	`
	var ct, nonce []byte
	var provider, baseURL string
	if err := r.pg.QueryRow(ctx, q, userID, id).Scan(&ct, &nonce, &provider, &baseURL); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Revealed{}, ErrNotFound
		}
		return Revealed{}, fmt.Errorf("select byok: %w", err)
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}
	plaintext, err := Decrypt(r.secretKey, ct, nonce)
	if err != nil {
		return Revealed{}, err
	}
	return Revealed{Key: plaintext, BaseURL: baseURL}, nil
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
