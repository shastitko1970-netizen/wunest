// Package users maps WuApi's int64 user_id to a local nest_users UUID.
//
// A nest_users row is a minimal shadow record that links our domain tables
// (characters, chats, etc.) to a WuApi user. It is upserted on first contact.
package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/models"
)

type Resolver struct {
	pg *db.Postgres
}

func NewResolver(pg *db.Postgres) *Resolver {
	return &Resolver{pg: pg}
}

// Resolve returns the local NestUser for a given WuApi user id.
// Creates the row on first call.
//
// Also bumps last_active_at on each call (cheap, within the same UPDATE).
//
// UUID is generated app-side via uuid.New() so the schema doesn't need the
// pgcrypto extension or Postgres 13+ `gen_random_uuid()`. Existing rows
// with their server-generated UUIDs continue to work — the ON CONFLICT
// branch returns the existing id, not our candidate.
func (r *Resolver) Resolve(ctx context.Context, wuapiUserID int64) (*models.NestUser, error) {
	const upsert = `
		INSERT INTO nest_users (id, wuapi_user_id, last_active_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (wuapi_user_id)
		  DO UPDATE SET last_active_at = NOW()
		RETURNING id, wuapi_user_id, settings, created_at, last_active_at
	`
	var u models.NestUser
	err := r.pg.QueryRow(ctx, upsert, uuid.New(), wuapiUserID).Scan(
		&u.ID, &u.WuApiUserID, &u.Settings, &u.CreatedAt, &u.LastActiveAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Shouldn't happen with INSERT ... RETURNING, but be explicit.
		return nil, fmt.Errorf("unexpected no-rows from upsert")
	}
	if err != nil {
		return nil, fmt.Errorf("upsert nest_users: %w", err)
	}
	return &u, nil
}

// Settings is the typed view of nest_users.settings JSONB.
//
// Grows ad-hoc — every feature that needs a per-user preference adds its
// own named sub-object so reads are self-documenting.
type Settings struct {
	// DefaultPresets maps preset-type → preset-id. Nil / missing values
	// mean "no default for this type; chats fall back to hard-coded
	// defaults". Keys follow presets.PresetType ("sampler", "instruct", …).
	DefaultPresets map[string]uuid.UUID `json:"default_presets,omitempty"`

	// Appearance is the raw user-authored theme blob: font scale, chat width,
	// accent color, custom CSS, etc. We store it as a raw JSON object so the
	// client can evolve the schema without a server migration every time.
	// Default values live on the client; a missing Appearance means "use
	// per-theme defaults" (not a blank page).
	Appearance json.RawMessage `json:"appearance,omitempty"`

	// DefaultModel is the preferred model id used for generation when a
	// request doesn't override it and no other source (chat-level, e.g.)
	// provides one. Empty = fall back to the server-side constant. Stored
	// as a string so we don't pin ourselves to WuApi's evolving catalogue.
	DefaultModel string `json:"default_model,omitempty"`
}

// LoadSettings reads nest_users.settings and returns a typed view.
// Missing / malformed settings return a zero-value Settings (no error).
func (r *Resolver) LoadSettings(ctx context.Context, userID uuid.UUID) (*Settings, error) {
	const q = `SELECT settings FROM nest_users WHERE id = $1`
	var raw []byte
	if err := r.pg.QueryRow(ctx, q, userID).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("load settings: %w", err)
	}
	var s Settings
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	return &s, nil
}

// SaveSettings writes the typed settings back into nest_users.settings.
// Full replacement — callers load → mutate → save.
func (r *Resolver) SaveSettings(ctx context.Context, userID uuid.UUID, s *Settings) error {
	payload, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	const q = `UPDATE nest_users SET settings = $2 WHERE id = $1`
	tag, err := r.pg.Exec(ctx, q, userID, payload)
	if err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// SetDefaultPreset records `preset_id` as the default for `presetType`.
// Passing uuid.Nil clears the default.
func (r *Resolver) SetDefaultPreset(ctx context.Context, userID uuid.UUID, presetType string, presetID uuid.UUID) error {
	s, err := r.LoadSettings(ctx, userID)
	if err != nil {
		return err
	}
	if s.DefaultPresets == nil {
		s.DefaultPresets = make(map[string]uuid.UUID)
	}
	if presetID == uuid.Nil {
		delete(s.DefaultPresets, presetType)
	} else {
		s.DefaultPresets[presetType] = presetID
	}
	return r.SaveSettings(ctx, userID, s)
}

// GetDefaultPreset returns the user's default preset id for the given
// type, or uuid.Nil if none is configured.
func (r *Resolver) GetDefaultPreset(ctx context.Context, userID uuid.UUID, presetType string) (uuid.UUID, error) {
	s, err := r.LoadSettings(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if s.DefaultPresets == nil {
		return uuid.Nil, nil
	}
	return s.DefaultPresets[presetType], nil
}

// SetAppearance replaces settings.appearance with the given raw JSON blob.
// Nil / empty blob clears the field (client reverts to default theme).
func (r *Resolver) SetAppearance(ctx context.Context, userID uuid.UUID, blob json.RawMessage) error {
	s, err := r.LoadSettings(ctx, userID)
	if err != nil {
		return err
	}
	s.Appearance = blob
	return r.SaveSettings(ctx, userID, s)
}

// SetDefaultModel stores the user's preferred model id. Empty string clears
// the preference so generation falls back to the server constant.
func (r *Resolver) SetDefaultModel(ctx context.Context, userID uuid.UUID, modelID string) error {
	s, err := r.LoadSettings(ctx, userID)
	if err != nil {
		return err
	}
	s.DefaultModel = modelID
	return r.SaveSettings(ctx, userID, s)
}
