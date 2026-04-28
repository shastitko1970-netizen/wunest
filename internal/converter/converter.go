// Package converter implements the `/api/convert/theme` endpoint.
//
// Intent: a user uploads a SillyTavern theme `.json` (or raw `.css` file
// wrapped in the ST envelope), picks an LLM provider/model, and we ask
// the model to rewrite the theme so it feels native in WuNest — ST
// selectors swapped for `.nest-*` anchors, broken nested containers
// dropped, WuNest-specific tokens preferred, and `customCssScope` set
// to `chat` by default.
//
// Each conversion creates a row in `nest_converter_jobs` with a 24h
// expiry. Post-expiry a reaper goroutine removes the row + any cached
// output so users can share the resulting link for a day without
// accumulating permanent state in our DB.
//
// The LLM call uses the same BYOK-or-WuApi upstream the chat stream
// uses: if the caller pinned a BYOK key we bill that, otherwise the
// WuApi pool. UI copy is explicit — the user pays their own tokens.
package converter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shastitko1970-netizen/wunest/internal/db"
)

// Status values for Job.Status. Kept as a typed enum for call-site
// clarity; stored as plain TEXT in Postgres (see migration 012).
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusError   = "error"
)

// Job is one row from nest_converter_jobs.
type Job struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Status        string     `json:"status"`
	Model         string     `json:"model"`
	BYOKID        *uuid.UUID `json:"byok_id,omitempty"`
	InputSHA256   string     `json:"input_sha256"`
	InputSize     int        `json:"input_size"`
	OutputJSON    []byte     `json:"-"` // served separately via /api/convert/{id}
	ErrorMessage  string     `json:"error_message,omitempty"`
	TokensIn      int        `json:"tokens_in"`
	TokensOut     int        `json:"tokens_out"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// ErrNotFound — used by handlers to map to 404 / 410 (expired).
var ErrNotFound = errors.New("converter job not found")

// ErrRateLimited — >=3 jobs in the last hour for this user.
var ErrRateLimited = errors.New("rate limited: 3 conversions per hour max")

// Repository — thin Postgres access layer.
type Repository struct {
	pg *db.Postgres
}

func NewRepository(pg *db.Postgres) *Repository {
	return &Repository{pg: pg}
}

// CreateInput captures the pre-LLM state of a conversion request.
type CreateInput struct {
	UserID      uuid.UUID
	Model       string
	BYOKID      *uuid.UUID
	InputSHA256 string
	InputSize   int
	// M51 Sprint 2 wave 2 — raw input bytes persisted for retry. Sent
	// verbatim by retryHandler when user picks a different model. Set
	// to nil only by legacy code-paths or pre-migration rows; new
	// records always include it.
	InputData []byte
}

// Create inserts a pending row. The LLM call follows and the handler
// mutates the row to running → done/error via UpdateStatus / Finish.
func (r *Repository) Create(ctx context.Context, in CreateInput) (*Job, error) {
	const q = `
		INSERT INTO nest_converter_jobs (user_id, status, model, byok_id, input_sha256, input_size, input_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, status, model, byok_id, input_sha256, input_size,
		          tokens_in, tokens_out, created_at, expires_at, finished_at
	`
	row := r.pg.QueryRow(ctx, q, in.UserID, StatusPending, in.Model, in.BYOKID, in.InputSHA256, in.InputSize, in.InputData)
	var j Job
	if err := row.Scan(
		&j.ID, &j.UserID, &j.Status, &j.Model, &j.BYOKID,
		&j.InputSHA256, &j.InputSize, &j.TokensIn, &j.TokensOut,
		&j.CreatedAt, &j.ExpiresAt, &j.FinishedAt,
	); err != nil {
		return nil, fmt.Errorf("insert converter job: %w", err)
	}
	return &j, nil
}

// GetWithInput fetches a job AND its raw input bytes. Used by the
// retry endpoint exclusively — regular Get / ListForUser skip the
// column to keep payload sizes small for the common case.
//
// Returns ErrNotFound when the row is missing OR expired (matches Get).
// `input` is nil for legacy rows created before migration 013; callers
// should treat that as "retry not supported for this row" and surface
// a friendly 410 to the user.
func (r *Repository) GetWithInput(ctx context.Context, id uuid.UUID) (*Job, []byte, error) {
	const q = `
		SELECT id, user_id, status, model, byok_id, input_sha256, input_size,
		       output_json, error_message, tokens_in, tokens_out,
		       created_at, expires_at, finished_at, input_data
		  FROM nest_converter_jobs
		 WHERE id = $1
		   AND expires_at > NOW()
	`
	row := r.pg.QueryRow(ctx, q, id)
	var j Job
	var input []byte
	if err := row.Scan(
		&j.ID, &j.UserID, &j.Status, &j.Model, &j.BYOKID,
		&j.InputSHA256, &j.InputSize, &j.OutputJSON, &j.ErrorMessage,
		&j.TokensIn, &j.TokensOut, &j.CreatedAt, &j.ExpiresAt, &j.FinishedAt,
		&input,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("get converter job with input: %w", err)
	}
	return &j, input, nil
}

// MarkRunning flips status → running. Separate call so the handler can
// show "started LLM call" before waiting on it.
func (r *Repository) MarkRunning(ctx context.Context, id uuid.UUID) error {
	_, err := r.pg.Exec(ctx, `UPDATE nest_converter_jobs SET status = $1 WHERE id = $2`, StatusRunning, id)
	if err != nil {
		return fmt.Errorf("mark running: %w", err)
	}
	return nil
}

// Finish terminal — either success with output JSON or failure with error.
type FinishInput struct {
	Status       string // StatusDone or StatusError
	OutputJSON   []byte // nil on error
	ErrorMessage string // empty on success
	TokensIn     int
	TokensOut    int
}

// Finish writes the terminal state. Sets finished_at = NOW() so the UI
// can show real LLM latency in the job list.
func (r *Repository) Finish(ctx context.Context, id uuid.UUID, fin FinishInput) error {
	const q = `
		UPDATE nest_converter_jobs
		   SET status = $1,
		       output_json = $2,
		       error_message = $3,
		       tokens_in = $4,
		       tokens_out = $5,
		       finished_at = NOW()
		 WHERE id = $6
	`
	_, err := r.pg.Exec(ctx, q, fin.Status, fin.OutputJSON, fin.ErrorMessage, fin.TokensIn, fin.TokensOut, id)
	if err != nil {
		return fmt.Errorf("finish converter job: %w", err)
	}
	return nil
}

// Get returns a job — by id only (no user filter at repo level). Handler
// decides whether to gate by user (owner-only) or allow shareable reads.
// Returns ErrNotFound when missing OR expired (expiry treated as
// "gone" even if the row still exists in the sub-second window before
// the reaper runs).
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*Job, error) {
	const q = `
		SELECT id, user_id, status, model, byok_id, input_sha256, input_size,
		       output_json, error_message, tokens_in, tokens_out,
		       created_at, expires_at, finished_at
		  FROM nest_converter_jobs
		 WHERE id = $1
		   AND expires_at > NOW()
	`
	row := r.pg.QueryRow(ctx, q, id)
	var j Job
	if err := row.Scan(
		&j.ID, &j.UserID, &j.Status, &j.Model, &j.BYOKID,
		&j.InputSHA256, &j.InputSize, &j.OutputJSON, &j.ErrorMessage,
		&j.TokensIn, &j.TokensOut, &j.CreatedAt, &j.ExpiresAt, &j.FinishedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get converter job: %w", err)
	}
	return &j, nil
}

// ListForUser — recent jobs (last 24h) for the UI "your conversions"
// strip. Ordered newest first. Output JSON is NOT returned here to keep
// the list endpoint small; caller fetches full payload via Get.
func (r *Repository) ListForUser(ctx context.Context, userID uuid.UUID, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 20
	}
	const q = `
		SELECT id, user_id, status, model, byok_id, input_sha256, input_size,
		       error_message, tokens_in, tokens_out,
		       created_at, expires_at, finished_at
		  FROM nest_converter_jobs
		 WHERE user_id = $1
		   AND expires_at > NOW()
		 ORDER BY created_at DESC
		 LIMIT $2
	`
	rows, err := r.pg.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list converter jobs: %w", err)
	}
	defer rows.Close()
	out := make([]Job, 0)
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.UserID, &j.Status, &j.Model, &j.BYOKID,
			&j.InputSHA256, &j.InputSize, &j.ErrorMessage,
			&j.TokensIn, &j.TokensOut, &j.CreatedAt, &j.ExpiresAt, &j.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan converter job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// RecentCount — number of jobs in the last hour for this user. Used
// for rate-limit enforcement (3/hour cap). Counts ALL terminal states
// because an errored call still consumed the user's tokens and an
// attacker could otherwise spam faulty inputs for free.
func (r *Repository) RecentCount(ctx context.Context, userID uuid.UUID, window time.Duration) (int, error) {
	const q = `
		SELECT COUNT(*)
		  FROM nest_converter_jobs
		 WHERE user_id = $1
		   AND created_at > $2
	`
	// Pass the cutoff time instead of an interval string — avoids any
	// pgx interval-encoding quirks across driver versions.
	cutoff := time.Now().Add(-window)
	row := r.pg.QueryRow(ctx, q, userID, cutoff)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("recent count: %w", err)
	}
	return n, nil
}

// DeleteExpired — called by the reaper loop. Returns rows deleted so we
// can log a running total.
func (r *Repository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.pg.Exec(ctx, `DELETE FROM nest_converter_jobs WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired: %w", err)
	}
	return tag.RowsAffected(), nil
}
