// Package users maps WuApi's int64 user_id to a local nest_users UUID.
//
// A nest_users row is a minimal shadow record that links our domain tables
// (characters, chats, etc.) to a WuApi user. It is upserted on first contact.
package users

import (
	"context"
	"errors"
	"fmt"

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
func (r *Resolver) Resolve(ctx context.Context, wuapiUserID int64) (*models.NestUser, error) {
	const upsert = `
		INSERT INTO nest_users (wuapi_user_id, last_active_at)
		VALUES ($1, NOW())
		ON CONFLICT (wuapi_user_id)
		  DO UPDATE SET last_active_at = NOW()
		RETURNING id, wuapi_user_id, settings, created_at, last_active_at
	`
	var u models.NestUser
	err := r.pg.QueryRow(ctx, upsert, wuapiUserID).Scan(
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
