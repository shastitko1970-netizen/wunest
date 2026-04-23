// Package uploads exposes POST endpoints that accept a multipart/form-data
// file upload and return a public URL from MinIO. Upload is decoupled from
// entity writes: the browser uploads first (getting a URL back), then sends
// the URL as part of the entity's normal PATCH/POST payload. This keeps the
// storage layer orthogonal to the domain packages (characters/chats/users)
// and means a failed upload never leaves a half-saved entity.
//
// Three endpoints:
//
//	POST /api/uploads/avatar      — character/user avatar → nest-avatars
//	POST /api/uploads/attachment  — message attachment    → nest-attachments
//	POST /api/uploads/background  — user chat background  → nest-backgrounds
//
// Each accepts a single `file` form field. Auth is required (same as the
// rest of /api/*); users can only upload under their own session.
package uploads

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/storage"
)

// Handler groups the upload endpoints.
//
// Storage is required — when nil, every endpoint returns 503 with a
// machine-readable error so clients can distinguish "server misconfigured"
// from "user upload was rejected".
type Handler struct {
	Storage *storage.Client
}

// Register wires the endpoints onto the mux with the given auth middleware.
func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("POST /api/uploads/avatar", authRequired(http.HandlerFunc(h.uploadAvatar)))
	mux.Handle("POST /api/uploads/attachment", authRequired(http.HandlerFunc(h.uploadAttachment)))
	mux.Handle("POST /api/uploads/background", authRequired(http.HandlerFunc(h.uploadBackground)))
}

// avatarResponse is what the client gets back on a successful avatar upload.
type avatarResponse struct {
	AvatarURL         string `json:"avatar_url"`          // thumbnail, 400px max
	AvatarOriginalURL string `json:"avatar_original_url"` // full-size original
}

func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(w) {
		return
	}
	if !h.requireAuth(w, r) {
		return
	}
	raw, contentType, ok := h.readFile(w, r, storage.MaxAvatarSize)
	if !ok {
		return
	}
	_ = contentType // PutAvatar re-sniffs; we only needed it for the read path

	urls, err := h.Storage.PutAvatar(r.Context(), raw)
	if err != nil {
		slog.Warn("uploads: avatar failed", "err", err, "size", len(raw))
		writeErr(w, http.StatusUnprocessableEntity, "upload_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, avatarResponse{
		AvatarURL:         urls.Thumbnail,
		AvatarOriginalURL: urls.Original,
	})
}

// attachmentResponse carries the attachment URL back to the client.
// Size + content_type let the client render a rich preview without a second
// round-trip.
type attachmentResponse struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

func (h *Handler) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(w) {
		return
	}
	if !h.requireAuth(w, r) {
		return
	}
	raw, contentType, ok := h.readFile(w, r, storage.MaxAttachmentSize)
	if !ok {
		return
	}
	url, err := h.Storage.PutAttachment(r.Context(), raw, contentType)
	if err != nil {
		slog.Warn("uploads: attachment failed", "err", err, "size", len(raw))
		writeErr(w, http.StatusUnprocessableEntity, "upload_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, attachmentResponse{
		URL:         url,
		ContentType: contentType,
		Size:        len(raw),
	})
}

// backgroundResponse is identical in shape to the attachment response but
// separate for API clarity (clients may handle them differently).
type backgroundResponse struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

func (h *Handler) uploadBackground(w http.ResponseWriter, r *http.Request) {
	if !h.requireEnabled(w) {
		return
	}
	if !h.requireAuth(w, r) {
		return
	}
	raw, contentType, ok := h.readFile(w, r, storage.MaxBackgroundSize)
	if !ok {
		return
	}
	url, err := h.Storage.PutBackground(r.Context(), raw, contentType)
	if err != nil {
		slog.Warn("uploads: background failed", "err", err, "size", len(raw))
		writeErr(w, http.StatusUnprocessableEntity, "upload_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, backgroundResponse{
		URL:         url,
		ContentType: contentType,
		Size:        len(raw),
	})
}

// --- helpers ---

// requireEnabled reports 503 when object storage is not configured.
func (h *Handler) requireEnabled(w http.ResponseWriter) bool {
	if h.Storage == nil {
		writeErr(w, http.StatusServiceUnavailable, "storage_disabled",
			"object storage is not configured on this instance")
		return false
	}
	return true
}

// requireAuth is a belt-and-suspenders check — the route middleware already
// guards, but returning a structured error is nicer than a redirect here.
func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if auth.FromContext(r.Context()) == nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "sign in required")
		return false
	}
	return true
}

// readFile pulls the `file` form field off a multipart request and returns
// its bytes + detected content-type. max caps the accepted size BEFORE
// buffering — a well-behaved client respects it; a misbehaving one gets a
// 413 without us spending memory.
func (h *Handler) readFile(w http.ResponseWriter, r *http.Request, max int) ([]byte, string, bool) {
	if err := r.ParseMultipartForm(int64(max)); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_upload", err.Error())
		return nil, "", false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file_missing", "missing `file` field")
		return nil, "", false
	}
	defer file.Close()

	if header.Size > int64(max) {
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "file exceeds size limit")
		return nil, "", false
	}

	raw, err := io.ReadAll(io.LimitReader(file, int64(max)+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read_failed", err.Error())
		return nil, "", false
	}
	if len(raw) > max {
		writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "file exceeds size limit")
		return nil, "", false
	}

	// Prefer the client-declared content-type if it's plausible; otherwise
	// trust the sniffer. Clients sometimes lie ("application/octet-stream"
	// for everything), so the sniffer is a useful sanity check.
	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(raw)
	}
	return raw, contentType, true
}

// writeJSON serialises `v` as JSON with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr is the structured-error shape the SPA expects:
//
//	{ "error": { "type": "too_large", "message": "..." } }
//
// Matches the convention in other packages (chats/presets/byok) so client
// error handling stays uniform.
func writeErr(w http.ResponseWriter, code int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"type":    errType,
			"message": msg,
		},
	})
}

// ErrNoStorage is returned from direct helpers when the handler is
// constructed without a storage client. Kept exported for tests.
var ErrNoStorage = errors.New("uploads: storage not configured")
