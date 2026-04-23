package characters

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/storage"
	"github.com/shastitko1970-netizen/wunest/internal/users"
)

// BookExtractor is implemented by whoever can persist a character's embedded
// lorebook (`data.character_book`) as a standalone Lorebook attached to the
// new character row. The server package wires this to worldinfo.Repository
// via a small adapter — we keep the interface here to avoid a cycle
// (worldinfo imports characters, not the other way around).
type BookExtractor interface {
	CreateAndAttach(
		ctx context.Context,
		userID, characterID uuid.UUID,
		name, description string,
		entries []byte, // JSON bytes of []CharacterBookEntry
	) error
}

// Handler groups all /api/characters HTTP endpoints.
//
// It depends on:
//   - Repository  — DB access for nest_characters
//   - users.Resolver — upserts nest_users rows from WuApi profiles
//   - Books (optional) — post-create extraction of embedded lorebooks
//   - Storage (optional) — MinIO client for avatar upload; nil is OK,
//     importCard falls back to not setting avatar_url
type Handler struct {
	Repo    *Repository
	Users   *users.Resolver
	Books   BookExtractor
	Storage *storage.Client
}

// maxUploadSize is the cap on a single PNG upload.
// Character cards are tiny (tens of KB typical); 16 MiB is generous.
const maxUploadSize = 16 * 1024 * 1024

// Register wires the handler onto the mux with the standard auth-required
// middleware. Callers supply the middleware so we don't depend on config here.
func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/characters", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/characters", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("POST /api/characters/import", authRequired(http.HandlerFunc(h.importCard)))
	mux.Handle("GET /api/characters/{id}", authRequired(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/characters/{id}", authRequired(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/characters/{id}", authRequired(http.HandlerFunc(h.delete)))
}

// --- endpoints ---

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	items, err := h.Repo.List(r.Context(), user.ID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	c, err := h.Repo.Get(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// createRequest is the JSON body for POST /api/characters.
type createRequest struct {
	Name      string        `json:"name"`
	Data      CharacterData `json:"data"`
	AvatarURL string        `json:"avatar_url,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
	Favorite  bool          `json:"favorite,omitempty"`
	Spec      string        `json:"spec,omitempty"`
	SourceURL string        `json:"source_url,omitempty"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := req.Name
	if name == "" {
		name = req.Data.Name
	}
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// Keep the data.name canonical if the outer name was overridden.
	req.Data.Name = name

	c, err := h.Repo.Create(r.Context(), CreateInput{
		UserID:    user.ID,
		Name:      name,
		Data:      req.Data,
		AvatarURL: req.AvatarURL,
		Tags:      normalizeTags(req.Tags, req.Data.Tags),
		Favorite:  req.Favorite,
		Spec:      req.Spec,
		SourceURL: req.SourceURL,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// importCard accepts a multipart/form-data upload with a single "file" field
// containing either a PNG character card (V2/V3 metadata in a tEXt chunk)
// or a bare JSON export. Format is sniffed from the magic bytes — the
// client doesn't have to declare it.
func (h *Handler) importCard(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "failed to parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if header.Size > maxUploadSize {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	raw, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		return
	}

	data, spec, err := ParseCard(raw)
	if err != nil {
		http.Error(w, "failed to parse card: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	tags := normalizeTags(nil, data.Tags)

	// When the upload is a PNG card, the image bytes ARE the avatar —
	// SillyTavern's convention is that the card artwork is embedded in the
	// same file as the metadata. Upload it to MinIO and set avatar_url to
	// the thumbnail. JSON imports don't carry an image, so storage stays
	// empty in that case.
	//
	// Upload failures are logged but do NOT block import; a character
	// without an avatar is still useful.
	avatarURL, avatarOriginalURL := h.maybeUploadAvatar(r.Context(), raw)

	c, err := h.Repo.Create(r.Context(), CreateInput{
		UserID:            user.ID,
		Name:              data.Name,
		Data:              *data,
		AvatarURL:         avatarURL,
		AvatarOriginalURL: avatarOriginalURL,
		Tags:              tags,
		Spec:              spec,
		SourceURL:         r.FormValue("source_url"),
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	// If the card ships an embedded character_book with entries, promote it
	// to a standalone Lorebook and attach — so it shows up in Library →
	// Worlds and activates during generation exactly like a hand-made book.
	h.extractEmbeddedBook(r.Context(), user.ID, c.ID, data)
	writeJSON(w, http.StatusCreated, c)
}

// maybeUploadAvatar ships raw PNG bytes to MinIO, producing a thumbnail
// for avatar_url and keeping the original for detail views. Returns empty
// strings on every non-success path — the caller is expected to persist
// the character with no avatar, rather than failing the import.
//
// Only PNG uploads are routed here: JSON character cards don't carry
// embedded art. We sniff the leading bytes against the PNG signature
// rather than trusting client-supplied content-types.
func (h *Handler) maybeUploadAvatar(ctx context.Context, raw []byte) (string, string) {
	if h.Storage == nil {
		return "", ""
	}
	if len(raw) < len(pngSignature) {
		return "", ""
	}
	isPNG := true
	for i, b := range pngSignature {
		if raw[i] != b {
			isPNG = false
			break
		}
	}
	if !isPNG {
		return "", ""
	}
	urls, err := h.Storage.PutAvatar(ctx, raw)
	if err != nil {
		slog.Warn("storage: avatar upload failed — character imported without avatar", "err", err)
		return "", ""
	}
	return urls.Thumbnail, urls.Original
}

// extractEmbeddedBook is a best-effort promotion of data.character_book into
// a standalone Lorebook + attachment. Any failure is logged; the character
// has already been persisted successfully and returning an error here would
// leave the user with a half-finished import.
func (h *Handler) extractEmbeddedBook(ctx context.Context, userID, characterID uuid.UUID, data *CharacterData) {
	if h.Books == nil || data == nil || data.CharacterBook == nil {
		return
	}
	entries := data.CharacterBook.Entries
	if len(entries) == 0 {
		return
	}
	name := data.CharacterBook.Name
	if name == "" {
		name = data.Name + " — Lorebook"
	}
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		slog.Warn("marshal embedded character_book", "err", err, "character_id", characterID)
		return
	}
	if err := h.Books.CreateAndAttach(ctx, userID, characterID, name, data.CharacterBook.Description, entriesJSON); err != nil {
		slog.Warn("extract embedded character_book", "err", err, "character_id", characterID)
	}
}

// updateRequest uses JSON nullable fields so the caller can distinguish
// "unset" (leave as-is) from "set to empty" (clear the field).
//
// Pointer types + omitempty give us: field present → patch; field absent →
// unchanged.
type updateRequest struct {
	Name      *string        `json:"name,omitempty"`
	Data      *CharacterData `json:"data,omitempty"`
	AvatarURL *string        `json:"avatar_url,omitempty"`
	Tags      *[]string      `json:"tags,omitempty"`
	Favorite  *bool          `json:"favorite,omitempty"`
	SourceURL *string        `json:"source_url,omitempty"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	c, err := h.Repo.Update(r.Context(), user.ID, id, UpdatePatch{
		Name:      req.Name,
		Data:      req.Data,
		AvatarURL: req.AvatarURL,
		Tags:      req.Tags,
		Favorite:  req.Favorite,
		SourceURL: req.SourceURL,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.Repo.Delete(r.Context(), user.ID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

// currentUser resolves the WuApi profile attached by auth middleware into
// a local nest_users row, upserting on first login.
func (h *Handler) currentUser(ctx context.Context, r *http.Request) (*models.NestUser, error) {
	session := auth.FromContext(ctx)
	if session == nil {
		return nil, errUnauthorized
	}
	return h.Users.Resolve(ctx, session.WuApi.ID)
}

var errUnauthorized = errors.New("unauthorized")

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, errUnauthorized):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	default:
		slog.Error("characters handler", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// normalizeTags merges user-provided tags with tags from the card data,
// de-duplicates (case-sensitive), and drops empties.
func normalizeTags(explicit, fromCard []string) []string {
	seen := make(map[string]struct{}, len(explicit)+len(fromCard))
	out := make([]string, 0, len(explicit)+len(fromCard))
	for _, t := range append(append([]string{}, explicit...), fromCard...) {
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
