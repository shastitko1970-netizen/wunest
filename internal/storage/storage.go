// Package storage is the object-storage layer for user-generated media.
//
// Backed by MinIO (S3-compatible), fronted by nginx at `/images/*`. Three
// public buckets:
//
//	nest-avatars      — 400×400 thumbnails + originals of character/user avatars
//	nest-attachments  — images attached to chat messages
//	nest-backgrounds  — user chat backgrounds
//
// All buckets are anonymous-readable (public GET); write access requires the
// access+secret keys configured via MINIO_* env vars.
//
// Filenames are content-hashed (SHA-256, first 12 bytes → 24 hex chars) and
// served with `Cache-Control: public, max-age=31536000, immutable`. Re-uploads
// of identical content are idempotent.
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	_ "image/gif"  // register gif decoder
	_ "image/jpeg" // register jpeg decoder
	_ "image/png"  // register png decoder

	"github.com/disintegration/imaging"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	_ "golang.org/x/image/bmp"  // register bmp decoder
	_ "golang.org/x/image/tiff" // register tiff decoder
	_ "golang.org/x/image/webp" // register webp decoder — modern browsers serve WebP
)

// Bucket names — must match the `mc mb` bootstrap and the nginx /images/*
// location blocks on the server.
const (
	BucketAvatars     = "nest-avatars"
	BucketAttachments = "nest-attachments"
	BucketBackgrounds = "nest-backgrounds"
)

// AvatarThumbSize is the max dimension (in pixels) of the generated avatar
// thumbnail. 400 matches a 2× hi-DPI 200-px card preview.
const AvatarThumbSize = 400

// Size caps per upload type. Requests above the cap are refused so a
// single upload can't OOM the Go process during decode.
const (
	MaxAvatarSize     = 10 * 1024 * 1024  // 10 MiB
	MaxAttachmentSize = 25 * 1024 * 1024  // 25 MiB
	MaxBackgroundSize = 10 * 1024 * 1024  // 10 MiB
)

// Config is what Load() in internal/config produces from MINIO_* env vars.
type Config struct {
	// Endpoint is the MinIO host:port reachable from the Go process (NOT
	// the public URL). Typically "127.0.0.1:9000" in prod, "minio:9000" in
	// compose.
	Endpoint string

	AccessKey string
	SecretKey string

	// UseSSL only toggles TLS between Go and MinIO. Our production setup
	// runs both on loopback so this is false in prod.
	UseSSL bool

	// PublicBaseURL is the origin where the `/images/*` proxy is served
	// (e.g. "https://nest.wusphere.ru"). Must NOT have a trailing slash.
	PublicBaseURL string
}

// Enabled reports whether the config has the minimum fields to operate.
func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.AccessKey != "" && c.SecretKey != "" && c.PublicBaseURL != ""
}

// Client is the high-level wrapper around minio-go that produces public
// URLs scoped to the three WuNest buckets.
//
// A nil *Client is a valid "disabled" value: every Put* method returns
// ErrDisabled so callers don't crash on a dev laptop with no MinIO running.
type Client struct {
	mc  *minio.Client
	cfg Config
}

// ErrDisabled is returned from Put* when the client is not configured.
// Callers should treat it as "don't upload" rather than a hard failure.
var ErrDisabled = errors.New("storage: object storage not configured")

// New constructs a Client. If the config is not fully populated, returns
// (nil, nil) — the nil Client is safe to call and returns ErrDisabled.
func New(cfg Config) (*Client, error) {
	if !cfg.Enabled() {
		return nil, nil
	}
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: minio.New: %w", err)
	}
	return &Client{mc: mc, cfg: cfg}, nil
}

// AvatarURLs is the result of a successful PutAvatar.
type AvatarURLs struct {
	// Thumbnail is the small preview (≤400 px on the long side, PNG).
	// This is what avatar_url on the entity row should be set to.
	Thumbnail string `json:"thumbnail"`

	// Original is the uploaded file as-is. Shown in detail views when the
	// user wants the full asset.
	Original string `json:"original"`
}

// PutAvatar uploads `raw` to nest-avatars (original) and a generated
// thumbnail. Returns public URLs.
//
// `raw` may be PNG, JPEG, GIF, or any format registered in image/.
// Unsupported formats return an error before any upload is attempted.
//
// On partial failure (original uploaded, thumb failed) the original remains
// in the bucket — caller receives the error and should not persist the
// Original URL.
func (c *Client) PutAvatar(ctx context.Context, raw []byte) (AvatarURLs, error) {
	if c == nil {
		return AvatarURLs{}, ErrDisabled
	}
	if err := capSize(raw, MaxAvatarSize, "avatar"); err != nil {
		return AvatarURLs{}, err
	}

	contentType := http.DetectContentType(raw)
	if !strings.HasPrefix(contentType, "image/") {
		return AvatarURLs{}, fmt.Errorf("storage: avatar is not an image (sniffed %q)", contentType)
	}

	// Decode the original for thumbnailing.
	img, err := imaging.Decode(bytes.NewReader(raw))
	if err != nil {
		return AvatarURLs{}, fmt.Errorf("storage: decode avatar: %w", err)
	}

	// Upload original.
	origKey := contentHashKey(raw) + extForContentType(contentType)
	if err := c.putObject(ctx, BucketAvatars, origKey, raw, contentType); err != nil {
		return AvatarURLs{}, fmt.Errorf("storage: put original avatar: %w", err)
	}

	// Thumbnail: fit inside a 400×400 box preserving aspect ratio.
	thumbImg := imaging.Fit(img, AvatarThumbSize, AvatarThumbSize, imaging.Lanczos)

	// Encode as PNG — character cards commonly have transparency and the
	// size difference at 400 px is small enough that we don't care.
	var thumbBuf bytes.Buffer
	if err := imaging.Encode(&thumbBuf, thumbImg, imaging.PNG); err != nil {
		return AvatarURLs{}, fmt.Errorf("storage: encode avatar thumb: %w", err)
	}
	thumbBytes := thumbBuf.Bytes()
	thumbKey := contentHashKey(thumbBytes) + ".png"
	if err := c.putObject(ctx, BucketAvatars, thumbKey, thumbBytes, "image/png"); err != nil {
		return AvatarURLs{}, fmt.Errorf("storage: put avatar thumb: %w", err)
	}

	return AvatarURLs{
		Thumbnail: c.urlFor(BucketAvatars, thumbKey),
		Original:  c.urlFor(BucketAvatars, origKey),
	}, nil
}

// PutAttachment uploads `raw` to nest-attachments as-is. Returns a public
// URL. `contentType` is optional — when empty, we sniff from the bytes.
func (c *Client) PutAttachment(ctx context.Context, raw []byte, contentType string) (string, error) {
	if c == nil {
		return "", ErrDisabled
	}
	if err := capSize(raw, MaxAttachmentSize, "attachment"); err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = http.DetectContentType(raw)
	}
	key := contentHashKey(raw) + extForContentType(contentType)
	if err := c.putObject(ctx, BucketAttachments, key, raw, contentType); err != nil {
		return "", fmt.Errorf("storage: put attachment: %w", err)
	}
	return c.urlFor(BucketAttachments, key), nil
}

// PutBackground uploads `raw` to nest-backgrounds as-is. Returns a public
// URL. Used for user-chosen chat backgrounds.
func (c *Client) PutBackground(ctx context.Context, raw []byte, contentType string) (string, error) {
	if c == nil {
		return "", ErrDisabled
	}
	if err := capSize(raw, MaxBackgroundSize, "background"); err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = http.DetectContentType(raw)
	}
	key := contentHashKey(raw) + extForContentType(contentType)
	if err := c.putObject(ctx, BucketBackgrounds, key, raw, contentType); err != nil {
		return "", fmt.Errorf("storage: put background: %w", err)
	}
	return c.urlFor(BucketBackgrounds, key), nil
}

// putObject is the low-level PutObject wrapper with a per-call timeout so
// a wedged MinIO can't hang a request path indefinitely.
func (c *Client) putObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := c.mc.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=31536000, immutable",
	})
	if err != nil {
		slog.Error("storage.putObject", "bucket", bucket, "key", key, "size", len(data), "err", err)
		return err
	}
	slog.Debug("storage.putObject", "bucket", bucket, "key", key, "size", len(data), "content_type", contentType)
	return nil
}

// urlFor maps (bucket, key) → public URL.
//
// The nginx /images/* proxy strips /images/<subpath>/ and rewrites to
// /<bucket>/ on MinIO — so:
//
//	bucket nest-avatars      → /images/avatars/<key>
//	bucket nest-attachments  → /images/attachments/<key>
//	bucket nest-backgrounds  → /images/backgrounds/<key>
func (c *Client) urlFor(bucket, key string) string {
	base := strings.TrimRight(c.cfg.PublicBaseURL, "/")
	var seg string
	switch bucket {
	case BucketAvatars:
		seg = "/images/avatars/"
	case BucketAttachments:
		seg = "/images/attachments/"
	case BucketBackgrounds:
		seg = "/images/backgrounds/"
	default:
		seg = "/images/" + bucket + "/"
	}
	return base + seg + key
}

// contentHashKey returns the first 24 hex chars of SHA-256(data).
// ~96 bits of collision resistance — more than enough for a single user's
// upload set. Content-addressing gives us cheap de-dup + perfect cache keys.
func contentHashKey(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12])
}

// extForContentType picks a file extension for a detected content-type.
// The extension is informational (MinIO serves by Content-Type), but makes
// logs + object-browser listings readable.
func extForContentType(ct string) string {
	// Strip charset/parameters: "image/png; charset=utf-8" → "image/png".
	if i := strings.IndexByte(ct, ';'); i > 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	default:
		return ""
	}
}

// capSize returns an error if `raw` exceeds `max`. Done before decode so
// a multi-hundred-MB upload can't buffer into a decoder.
func capSize(raw []byte, max int, kind string) error {
	if len(raw) == 0 {
		return fmt.Errorf("storage: empty %s", kind)
	}
	if len(raw) > max {
		return fmt.Errorf("storage: %s too large (%d bytes > %d)", kind, len(raw), max)
	}
	return nil
}

// DrainAndReject is a helper for HTTP handlers: drain a request body up to
// `max+1` bytes and return the contents + an error if the cap was exceeded.
// Use when the content is not already multipart-parsed with a size limit.
func DrainAndReject(r io.Reader, max int64) ([]byte, error) {
	lr := io.LimitReader(r, max+1)
	buf, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("storage: body exceeds %d bytes", max)
	}
	return buf, nil
}
