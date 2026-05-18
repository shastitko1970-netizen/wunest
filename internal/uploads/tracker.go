package uploads

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/storage"
)

// Kind identifies a staged-upload stream (one active draft per user per kind).
type Kind string

const (
	KindAvatar     Kind = "avatar"
	KindBackground Kind = "background"
)

// Tracker records upload→save lifecycle so the storage reaper can keep the
// latest unsaved draft while reaping superseded uploads.
type Tracker struct {
	pg *db.Postgres
}

func NewTracker(pg *db.Postgres) *Tracker {
	if pg == nil {
		return nil
	}
	return &Tracker{pg: pg}
}

// StageActive supersedes the user's previous active draft for kind and stores
// object keys for the new upload. Errors are non-fatal for callers.
func (t *Tracker) StageActive(ctx context.Context, userID uuid.UUID, kind Kind, publicURLs ...string) error {
	if t == nil || t.pg == nil {
		return nil
	}
	keys := storage.ObjectKeysFromPublicURLs(publicURLs...)
	if len(keys) == 0 {
		return nil
	}

	tx, err := t.pg.Begin(ctx)
	if err != nil {
		return fmt.Errorf("staged upload begin: %w", err)
	}
	defer tx.Rollback(ctx)

	const supersede = `
		UPDATE nest_staged_uploads
		   SET is_active = FALSE, superseded_at = NOW()
		 WHERE user_id = $1 AND kind = $2
		   AND is_active AND claimed_at IS NULL AND superseded_at IS NULL
	`
	if _, err := tx.Exec(ctx, supersede, userID, string(kind)); err != nil {
		return fmt.Errorf("staged upload supersede: %w", err)
	}

	const insert = `
		INSERT INTO nest_staged_uploads (user_id, kind, object_keys)
		VALUES ($1, $2, $3)
	`
	if _, err := tx.Exec(ctx, insert, userID, string(kind), keys); err != nil {
		return fmt.Errorf("staged upload insert: %w", err)
	}
	return tx.Commit(ctx)
}

// ClaimByURLs marks active staged rows claimed when any URL matches stored keys.
func (t *Tracker) ClaimByURLs(ctx context.Context, userID uuid.UUID, kind Kind, publicURLs ...string) error {
	if t == nil || t.pg == nil {
		return nil
	}
	keys := storage.ObjectKeysFromPublicURLs(publicURLs...)
	if len(keys) == 0 {
		return nil
	}

	const q = `
		UPDATE nest_staged_uploads
		   SET claimed_at = NOW(), is_active = FALSE
		 WHERE user_id = $1 AND kind = $2
		   AND is_active AND claimed_at IS NULL AND superseded_at IS NULL
		   AND object_keys && $3::text[]
	`
	_, err := t.pg.Exec(ctx, q, userID, string(kind), keys)
	if err != nil {
		return fmt.Errorf("staged upload claim: %w", err)
	}
	return nil
}
