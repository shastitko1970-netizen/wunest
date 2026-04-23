// nest-storage-reaper — scan MinIO buckets and delete orphan objects.
//
// An "orphan" is an object in one of our three buckets that:
//
//  1. Is older than --min-age (default 72h) — recently-uploaded objects
//     might not yet be referenced from DB because the character/message
//     save happens after the upload returns.
//  2. Is NOT referenced by any row in the WuNest database.
//
// References tracked per bucket:
//
//	nest-avatars      — nest_characters.avatar_url / avatar_original_url
//	nest-attachments  — URL substrings inside nest_messages.content + swipes
//	nest-backgrounds  — nest_users.settings.appearance.background_url
//
// Intended to run as a daily systemd timer:
//
//	systemctl enable --now nest-storage-reaper.timer
//
// Exit codes:
//
//	0  — scan completed (may have deleted 0 or more objects)
//	1  — config/DB/MinIO error (nothing deleted on this run)
//
// Env vars come from /opt/wunest/.env via the systemd EnvironmentFile
// directive — same file WuNest itself reads, so credentials stay in one
// place.
//
// Flags:
//
//	--dry-run       log what would be deleted but do not actually delete
//	--min-age=72h   minimum object age before it's eligible for reap
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/storage"
)

var (
	flagDryRun = flag.Bool("dry-run", false, "Log but do not delete")
	flagMinAge = flag.Duration("min-age", 72*time.Hour, "Min age of an object before eligible for reap")
)

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	if cfg.MinIOEndpoint == "" || cfg.MinIOAccessKey == "" {
		slog.Error("reaper: MinIO not configured (MINIO_ENDPOINT empty); nothing to do")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Ctrl-C / SIGTERM mid-run stops cleanly — useful if a systemd stop
	// lands during a long object walk.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		slog.Warn("reaper: signal received; stopping")
		cancel()
	}()

	pg, err := db.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("reaper: postgres connect failed", "err", err)
		os.Exit(1)
	}
	defer pg.Close()

	mc, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		slog.Error("reaper: minio init failed", "err", err)
		os.Exit(1)
	}

	run := runner{
		ctx:      ctx,
		pg:       pg,
		mc:       mc,
		dryRun:   *flagDryRun,
		minAge:   *flagMinAge,
		baseURL:  strings.TrimRight(cfg.MinIOPublicBaseURL, "/"),
		totalReap: 0,
		totalKeep: 0,
		totalSkip: 0,
	}

	slog.Info("reaper: starting",
		"dry_run", run.dryRun,
		"min_age", run.minAge.String(),
		"public_base", run.baseURL,
	)

	// Per-bucket: gather referenced keys from DB, walk MinIO, delete.
	for _, bucket := range []string{
		storage.BucketAvatars,
		storage.BucketAttachments,
		storage.BucketBackgrounds,
	} {
		if err := run.reap(bucket); err != nil {
			slog.Error("reaper: bucket failed", "bucket", bucket, "err", err)
			// Don't exit — try the other buckets anyway, partial success
			// is better than all-or-nothing.
		}
	}

	slog.Info("reaper: done",
		"reaped", run.totalReap,
		"kept", run.totalKeep,
		"skipped_new", run.totalSkip,
		"dry_run", run.dryRun,
	)
}

type runner struct {
	ctx     context.Context
	pg      *db.Postgres
	mc      *minio.Client
	dryRun  bool
	minAge  time.Duration
	baseURL string

	totalReap int
	totalKeep int
	totalSkip int
}

// reap scans one bucket: collect referenced keys → walk objects → delete orphans.
func (r *runner) reap(bucket string) error {
	refs, err := r.collectRefs(bucket)
	if err != nil {
		return fmt.Errorf("collect refs: %w", err)
	}

	cutoff := time.Now().Add(-r.minAge)
	slog.Info("reaper: bucket start",
		"bucket", bucket,
		"referenced_keys", len(refs),
		"cutoff", cutoff.Format(time.RFC3339),
	)

	var reaped, kept, skipped int
	for obj := range r.mc.ListObjects(r.ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			slog.Error("reaper: list error", "bucket", bucket, "err", obj.Err)
			continue
		}
		if obj.LastModified.After(cutoff) {
			skipped++
			continue
		}
		if refs[obj.Key] {
			kept++
			continue
		}

		// Orphan.
		if r.dryRun {
			slog.Info("reaper: would delete", "bucket", bucket, "key", obj.Key, "size", obj.Size, "age", time.Since(obj.LastModified).Round(time.Hour).String())
		} else {
			if err := r.mc.RemoveObject(r.ctx, bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
				slog.Error("reaper: remove failed", "bucket", bucket, "key", obj.Key, "err", err)
				continue
			}
			slog.Info("reaper: deleted", "bucket", bucket, "key", obj.Key, "size", obj.Size)
		}
		reaped++
	}

	slog.Info("reaper: bucket done",
		"bucket", bucket,
		"reaped", reaped,
		"kept", kept,
		"skipped_new", skipped,
	)
	r.totalReap += reaped
	r.totalKeep += kept
	r.totalSkip += skipped
	return nil
}

// collectRefs returns the set of MinIO object keys that are still
// referenced somewhere in Postgres for the given bucket. Keys are the
// filename part only (no bucket prefix, no URL scheme) — matches the
// `obj.Key` returned by minio.Client.ListObjects.
func (r *runner) collectRefs(bucket string) (map[string]bool, error) {
	refs := map[string]bool{}

	switch bucket {
	case storage.BucketAvatars:
		if err := r.scanAvatars(refs); err != nil {
			return nil, err
		}
	case storage.BucketAttachments:
		if err := r.scanAttachments(refs); err != nil {
			return nil, err
		}
	case storage.BucketBackgrounds:
		if err := r.scanBackgrounds(refs); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

// scanAvatars pulls both avatar_url and avatar_original_url columns from
// nest_characters. Extracts the object-key suffix and adds it to the ref set.
func (r *runner) scanAvatars(refs map[string]bool) error {
	const q = `
		SELECT COALESCE(avatar_url, ''), COALESCE(avatar_original_url, '')
		  FROM nest_characters
	`
	rows, err := r.pg.Query(r.ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var thumb, orig string
		if err := rows.Scan(&thumb, &orig); err != nil {
			return err
		}
		r.addRef(refs, thumb, "/images/avatars/")
		r.addRef(refs, orig, "/images/avatars/")
	}
	return rows.Err()
}

// attachmentURLRegex matches any URL that points at our attachments path.
// Deliberately loose on the host so relative URLs (emitted by a
// misconfigured client) still count as references — prevents accidental
// deletion of in-use assets just because PUBLIC_BASE_URL changed.
var attachmentURLRegex = regexp.MustCompile(`/images/attachments/[a-f0-9]{24}\.[a-z]{2,5}`)

// scanAttachments walks every message row for URLs in content + swipes.
// A message can carry attachments in either, so we union both.
//
// Swipes is a JSONB array of strings — pgx returns it as raw bytes;
// cheapest parse is just `regexp.FindAllString` on the raw JSON text,
// which works because the regex pattern cannot match JSON escape
// sequences (no quotes, no backslashes, no braces).
func (r *runner) scanAttachments(refs map[string]bool) error {
	const q = `SELECT content, swipes FROM nest_messages`
	rows, err := r.pg.Query(r.ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var content string
		var swipes []byte
		if err := rows.Scan(&content, &swipes); err != nil {
			return err
		}
		for _, match := range attachmentURLRegex.FindAllString(content, -1) {
			key := strings.TrimPrefix(match, "/images/attachments/")
			refs[key] = true
		}
		for _, match := range attachmentURLRegex.FindAllString(string(swipes), -1) {
			key := strings.TrimPrefix(match, "/images/attachments/")
			refs[key] = true
		}
	}
	return rows.Err()
}

// scanBackgrounds walks nest_users.settings and extracts any
// `appearance.background_url` values. Settings is a JSONB blob with
// open schema, so we probe with `->>` and tolerate missing keys.
func (r *runner) scanBackgrounds(refs map[string]bool) error {
	const q = `
		SELECT settings
		  FROM nest_users
		 WHERE settings->'appearance'->>'background_url' IS NOT NULL
		    OR settings->'appearance'->>'background' IS NOT NULL
	`
	rows, err := r.pg.Query(r.ctx, q)
	if err != nil {
		// A restrictive host might reject the JSONB operator query; fall
		// back to full-table scan so the reaper still works on a smaller
		// footprint.
		if !errors.Is(err, pgx.ErrNoRows) {
			return r.scanBackgroundsFallback(refs)
		}
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		r.extractBackgroundURLs(refs, raw)
	}
	return rows.Err()
}

func (r *runner) scanBackgroundsFallback(refs map[string]bool) error {
	rows, err := r.pg.Query(r.ctx, `SELECT settings FROM nest_users`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		r.extractBackgroundURLs(refs, raw)
	}
	return rows.Err()
}

// extractBackgroundURLs parses a settings JSON blob and harvests any
// string-typed `background_url` / `background` under `appearance`.
// Tolerant: missing keys are skipped silently.
func (r *runner) extractBackgroundURLs(refs map[string]bool, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var settings struct {
		Appearance map[string]any `json:"appearance"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return
	}
	for _, key := range []string{"background_url", "background"} {
		if v, ok := settings.Appearance[key].(string); ok {
			r.addRef(refs, v, "/images/backgrounds/")
		}
	}
}

// addRef takes a full URL (or path) and, if it contains the expected
// bucket-path prefix, records the remaining segment as a referenced key.
func (r *runner) addRef(refs map[string]bool, url, prefix string) {
	if url == "" {
		return
	}
	i := strings.Index(url, prefix)
	if i < 0 {
		return
	}
	key := url[i+len(prefix):]
	// Strip query string / fragment if any.
	if q := strings.IndexAny(key, "?#"); q >= 0 {
		key = key[:q]
	}
	if key != "" {
		refs[key] = true
	}
}
