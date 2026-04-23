package chats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/byok"
	"github.com/shastitko1970-netizen/wunest/internal/personas"
	"github.com/shastitko1970-netizen/wunest/internal/presets"
	"github.com/shastitko1970-netizen/wunest/internal/users"
	"github.com/shastitko1970-netizen/wunest/internal/worldinfo"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// Handler wires the chat-related HTTP routes onto a mux. Kept as a struct
// (not interface) because we don't need test doubles at this depth yet.
type Handler struct {
	Repo       *Repository
	Users      *users.Resolver
	Characters *characters.Repository
	Presets    *presets.Repository
	Worlds     *worldinfo.Repository
	Personas   *personas.Repository
	BYOK       *byok.Repository
	WuApi      *wuapi.Client
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/chats", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/chats", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/chats/{id}", authRequired(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/chats/{id}", authRequired(http.HandlerFunc(h.rename)))
	mux.Handle("PUT /api/chats/{id}/sampler", authRequired(http.HandlerFunc(h.setSampler)))
	mux.Handle("PUT /api/chats/{id}/persona", authRequired(http.HandlerFunc(h.setPersona)))
	mux.Handle("PUT /api/chats/{id}/authors-note", authRequired(http.HandlerFunc(h.setAuthorsNote)))
	mux.Handle("PUT /api/chats/{id}/byok", authRequired(http.HandlerFunc(h.setBYOK)))
	mux.Handle("DELETE /api/chats/{id}", authRequired(http.HandlerFunc(h.delete)))
	mux.Handle("POST /api/chats/{id}/regenerate", authRequired(http.HandlerFunc(h.regenerate)))
	mux.Handle("GET /api/chats/{id}/messages", authRequired(http.HandlerFunc(h.listMessages)))
	mux.Handle("POST /api/chats/{id}/messages", authRequired(http.HandlerFunc(h.sendMessage)))
	mux.Handle("PATCH /api/chats/{id}/messages/{mid}", authRequired(http.HandlerFunc(h.editMessage)))
	mux.Handle("DELETE /api/chats/{id}/messages/{mid}", authRequired(http.HandlerFunc(h.deleteMessage)))
	// Swipes — alternate assistant outputs for the same turn.
	mux.Handle("POST /api/chats/{id}/messages/{mid}/swipe", authRequired(http.HandlerFunc(h.swipeMessage)))
	mux.Handle("PATCH /api/chats/{id}/messages/{mid}/swipe", authRequired(http.HandlerFunc(h.selectSwipe)))
	// Portability: export current chat as JSONL, import a .jsonl into a new chat.
	mux.Handle("GET /api/chats/{id}/export", authRequired(http.HandlerFunc(h.exportChat)))
	mux.Handle("POST /api/chats/import", authRequired(http.HandlerFunc(h.importChat)))
}

// --- chat CRUD ---

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	items, err := h.Repo.ListChats(r.Context(), user.ID)
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
	chat, err := h.Repo.GetChat(r.Context(), user.ID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chat)
}

type createChatRequest struct {
	CharacterID *uuid.UUID      `json:"character_id,omitempty"`
	Name        string          `json:"name,omitempty"`
	Metadata    json.RawMessage `json:"chat_metadata,omitempty"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	var req createChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Default the chat name from the character if not provided.
	name := req.Name
	if name == "" && req.CharacterID != nil {
		if ch, err := h.Characters.Get(r.Context(), user.ID, *req.CharacterID); err == nil {
			name = ch.Name
		}
	}
	if name == "" {
		name = "New chat"
	}

	// Seed chat_metadata.sampler from the user's default sampler preset if
	// one is set. The metadata payload wins if the caller explicitly sent
	// one — default is applied only to a bare Metadata.
	metadata := req.Metadata
	if len(metadata) == 0 {
		if seeded, err := h.seedDefaultSampler(r.Context(), user.ID); err == nil && seeded != nil {
			metadata = seeded
		}
	}

	chat, err := h.Repo.CreateChat(r.Context(), CreateChatInput{
		UserID:      user.ID,
		CharacterID: req.CharacterID,
		Name:        name,
		Metadata:    metadata,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}

	// Seed with the character's first message (greeting) as a hidden assistant
	// turn so the UI has something to render on open. We insert it visibly;
	// users can delete if unwanted.
	if req.CharacterID != nil {
		if ch, err := h.Characters.Get(r.Context(), user.ID, *req.CharacterID); err == nil {
			greeting := ch.Data.FirstMes
			if greeting != "" {
				// Light macro pass so {{user}}/{{char}} render. Use the user's
				// default persona name if one is set; session name otherwise.
				personaName := user.DisplayName()
				if h.Personas != nil {
					if p, err := h.Personas.Default(r.Context(), user.ID); err == nil && p.Name != "" {
						personaName = p.Name
					}
				}
				greeting = SubstituteMacros(greeting, PromptInput{
					Character: ch,
					UserName:  personaName,
				})
				if _, err := h.Repo.AppendMessage(r.Context(), chat.ID, RoleAssistant, greeting, &MessageExtras{
					Model: "greeting",
				}); err != nil {
					slog.Warn("seed greeting failed", "err", err)
				}
			}
		}
	}

	writeJSON(w, http.StatusCreated, chat)
}

type renameRequest struct {
	Name string `json:"name"`
}

// setSampler overwrites chat_metadata.sampler. Body is ChatSamplerMetadata
// JSON; nil fields mean "unset, fall back to server default at generation
// time". Response is 204 No Content on success.
func (h *Handler) setSampler(w http.ResponseWriter, r *http.Request) {
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
	var req ChatSamplerMetadata
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.Repo.SetSampler(r.Context(), user.ID, id, req); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setPersona writes (or clears) chat_metadata.persona_id.
// Body: {"persona_id": "uuid" | null}. Resolution order when streaming is
// per-chat override → user's default → WuApi first_name.
func (h *Handler) setPersona(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		PersonaID *uuid.UUID `json:"persona_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	target := uuid.Nil
	if req.PersonaID != nil {
		target = *req.PersonaID
		// Ownership check: only the user's own personas are acceptable.
		if h.Personas != nil {
			if _, err := h.Personas.Get(r.Context(), user.ID, target); err != nil {
				h.writeErr(w, err)
				return
			}
		}
	}
	if err := h.Repo.SetPersona(r.Context(), user.ID, id, target); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setAuthorsNote writes (or clears) chat_metadata.authors_note. Body is the
// AuthorsNote JSON; an explicit null clears the key.
func (h *Handler) setAuthorsNote(w http.ResponseWriter, r *http.Request) {
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
	// Accept either a plain object (set) or `null` (clear). We test the raw
	// body because json.Decoder's defaults would turn null into an empty
	// struct, which is indistinguishable from a legitimate empty note.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var note *AuthorsNote
	if trimmed := strings.TrimSpace(string(body)); trimmed != "" && trimmed != "null" {
		var n AuthorsNote
		if err := json.Unmarshal(body, &n); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		// Empty content also clears — nothing to inject.
		if strings.TrimSpace(n.Content) == "" {
			note = nil
		} else {
			note = &n
		}
	}
	if err := h.Repo.SetAuthorsNote(r.Context(), user.ID, id, note); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setBYOK writes or clears chat_metadata.byok_id. Body: {"byok_id": "uuid" | null}.
// When set, the chat's stream path uses the referenced BYOK key for upstream
// calls instead of the user's WuApi key. Ownership on the BYOK key is
// validated via Repo.Reveal at send time — this endpoint only checks chat
// ownership to keep the write cheap.
func (h *Handler) setBYOK(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		BYOKID *uuid.UUID `json:"byok_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	target := uuid.Nil
	if req.BYOKID != nil {
		target = *req.BYOKID
	}
	if err := h.Repo.SetBYOK(r.Context(), user.ID, id, target); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
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
	var req renameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.Repo.RenameChat(r.Context(), user.ID, id, req.Name); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if err := h.Repo.DeleteChat(r.Context(), user.ID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- messages ---

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
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
	// Ownership check: will 404 if chat isn't ours.
	if _, err := h.Repo.GetChat(r.Context(), user.ID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	msgs, err := h.Repo.ListMessages(r.Context(), id, false)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": msgs})
}

// sendMessage is the streaming endpoint. Body is application/json (not form).
// The response is always SSE — even for error cases (client parses `event: error`).
func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	// Auth the caller and load chat ownership BEFORE switching to SSE mode —
	// a failed auth should still be a normal JSON 401.
	session := auth.FromContext(r.Context())
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.Users.Resolve(r.Context(), session.WuApi.ID)
	if err != nil {
		slog.Error("resolve user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chatID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	chat, err := h.Repo.GetChat(r.Context(), user.ID, chatID)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	var in SendMessageInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if in.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	up := h.resolveUpstream(r.Context(), user.ID, chat.Metadata, session.WuApi.APIKey)
	if up.APIKey == "" {
		http.Error(w, "no api key available", http.StatusPreconditionFailed)
		return
	}

	userName, userDesc := h.resolvePersona(r.Context(), user.ID, chat.Metadata, session.WuApi.FirstName, session.WuApi.Username)

	// Apply chat_metadata.sampler as the baseline; explicit fields in the
	// request body override (common pattern: drawer sliders set chat
	// defaults once, per-turn overrides are rare).
	in = applyChatSampler(in, readSampler(chat.Metadata))
	in.AuthorsNote = readAuthorsNote(chat.Metadata)
	in = h.applyUserDefaults(r.Context(), user.ID, in)

	// Stream.
	h.streamChat(w, r, user.ID, chat.ID, chat.CharacterID, up, userName, userDesc, in)
}

// upstream describes where a chat turn should send its chat-completions
// request. WuApi is the default (empty BaseURL → our own /v1 proxy);
// when BYOK is pinned the Key + BaseURL route directly to the user's
// provider, bypassing WuApi entirely so a raw `sk-proj-*` key works.
type upstream struct {
	APIKey  string
	BaseURL string // empty → use the WuApi client
}

// resolveUpstream picks the upstream auth key AND URL for a chat turn.
// Order:
//
//  1. chat_metadata.byok_id present AND repo can decrypt it → BYOK key
//     at its stored BaseURL (direct provider call, skip WuApi).
//  2. Otherwise → the user's session WuApi key, WuApi as upstream.
//
// On BYOK reveal failure (deleted row, bad nonce, rotated SECRETS_KEY)
// we log and silently fall back to WuApi — better than surfacing an
// opaque auth error to the user mid-generation.
func (h *Handler) resolveUpstream(ctx context.Context, userID uuid.UUID, metadata []byte, wuapiKey string) upstream {
	if h.BYOK == nil {
		return upstream{APIKey: wuapiKey}
	}
	byokID := readBYOKID(metadata)
	if byokID == uuid.Nil {
		return upstream{APIKey: wuapiKey}
	}
	rev, err := h.BYOK.Reveal(ctx, userID, byokID)
	if err != nil {
		slog.Warn("byok: reveal failed, falling back to wuapi key", "err", err, "byok_id", byokID)
		return upstream{APIKey: wuapiKey}
	}
	return upstream{APIKey: rev.Key, BaseURL: rev.BaseURL}
}

// applyUserDefaults populates SendMessageInput fields from per-user settings
// when the request didn't override them. Right now this covers DefaultModel;
// future additions (e.g. token-budget ceilings) go here so call sites don't
// sprout parallel settings lookups.
func (h *Handler) applyUserDefaults(ctx context.Context, userID uuid.UUID, in SendMessageInput) SendMessageInput {
	if in.Model != "" {
		return in
	}
	s, err := h.Users.LoadSettings(ctx, userID)
	if err != nil || s == nil {
		return in
	}
	if s.DefaultModel != "" {
		in.Model = s.DefaultModel
	}
	return in
}

// resolvePersona picks the {{user}} name and "About the user" description
// for a turn. Order: chat_metadata.persona_id → user's default persona →
// session fallback (first_name then username). Missing personas are tolerated.
func (h *Handler) resolvePersona(ctx context.Context, userID uuid.UUID, metadata []byte, firstName, username string) (string, string) {
	fallback := firstName
	if fallback == "" {
		fallback = username
	}
	if h.Personas == nil {
		return fallback, ""
	}
	// 1. Per-chat override.
	if pid := readPersonaID(metadata); pid != uuid.Nil {
		if p, err := h.Personas.Get(ctx, userID, pid); err == nil {
			return p.Name, p.Description
		}
		// Fall through on error — persona was deleted etc.
	}
	// 2. Account-wide default.
	if p, err := h.Personas.Default(ctx, userID); err == nil {
		return p.Name, p.Description
	}
	// 3. Session name.
	return fallback, ""
}

// applyChatSampler merges ChatSamplerMetadata defaults into SendMessageInput
// fields that were left unset by the caller. Explicit in.X wins if set.
func applyChatSampler(in SendMessageInput, s ChatSamplerMetadata) SendMessageInput {
	if in.Temperature == nil && s.Temperature != nil {
		in.Temperature = s.Temperature
	}
	if in.TopP == nil && s.TopP != nil {
		in.TopP = s.TopP
	}
	if in.TopK == nil && s.TopK != nil {
		in.TopK = s.TopK
	}
	if in.MinP == nil && s.MinP != nil {
		in.MinP = s.MinP
	}
	if in.MaxTokens == nil && s.MaxTokens != nil {
		in.MaxTokens = s.MaxTokens
	}
	if in.FrequencyPenalty == nil && s.FrequencyPenalty != nil {
		in.FrequencyPenalty = s.FrequencyPenalty
	}
	if in.PresencePenalty == nil && s.PresencePenalty != nil {
		in.PresencePenalty = s.PresencePenalty
	}
	if in.RepetitionPenalty == nil && s.RepetitionPenalty != nil {
		in.RepetitionPenalty = s.RepetitionPenalty
	}
	if in.Seed == nil && s.Seed != nil {
		in.Seed = s.Seed
	}
	if len(in.Stop) == 0 && len(s.Stop) > 0 {
		in.Stop = s.Stop
	}
	if in.ReasoningEnabled == nil && s.ReasoningEnabled != nil {
		in.ReasoningEnabled = s.ReasoningEnabled
	}
	if in.SystemPromptOverride == "" && s.SystemPromptOverride != "" {
		in.SystemPromptOverride = s.SystemPromptOverride
	}
	return in
}

// readSamplerFromChat surfaces the chat's stored sampler metadata so
// streamChat/regen can consult it for the system-prompt override. Named
// distinctly from readSampler (the repo helper) just to avoid a cycle.
func readSamplerFromChat(c *Chat) ChatSamplerMetadata {
	if c == nil {
		return ChatSamplerMetadata{}
	}
	return readSampler(c.Metadata)
}

// seedDefaultSampler returns a chat_metadata bytes payload with the user's
// default sampler preset pre-applied, or (nil, nil) if no default is set.
// Errors are non-fatal for chat creation — we just log and return nil so
// the new chat ships without sampler metadata.
func (h *Handler) seedDefaultSampler(ctx context.Context, userID uuid.UUID) (json.RawMessage, error) {
	if h.Presets == nil {
		return nil, nil
	}
	defaultID, err := h.Users.GetDefaultPreset(ctx, userID, string(presets.TypeSampler))
	if err != nil {
		slog.Warn("load default sampler: settings read failed", "err", err)
		return nil, nil
	}
	if defaultID == uuid.Nil {
		return nil, nil
	}
	preset, err := h.Presets.Get(ctx, userID, defaultID)
	if err != nil {
		// Preset may have been deleted — fall through silently.
		return nil, nil
	}
	sampler := preset.AsSampler()
	sm := ChatSamplerMetadata{
		Temperature:          sampler.Temperature,
		TopP:                 sampler.TopP,
		MaxTokens:            sampler.MaxTokens,
		FrequencyPenalty:     sampler.FrequencyPenalty,
		PresencePenalty:      sampler.PresencePenalty,
		SystemPromptOverride: sampler.SystemPromptOverride,
		PresetID:             &defaultID,
	}
	envelope := map[string]any{"sampler": sm}
	b, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func (h *Handler) deleteMessage(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	chatID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid chat id", http.StatusBadRequest)
		return
	}
	var mid int64
	if _, err := fmt.Sscan(r.PathValue("mid"), &mid); err != nil || mid <= 0 {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}
	if _, err := h.Repo.GetChat(r.Context(), user.ID, chatID); err != nil {
		h.writeErr(w, err)
		return
	}
	if err := h.Repo.DeleteMessage(r.Context(), chatID, mid); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// editMessageRequest is the JSON body for PATCH /messages/:mid.
type editMessageRequest struct {
	Content string `json:"content"`
}

// editMessage patches the content of a user OR assistant message in place.
// Does NOT touch swipes or extras; use for typo corrections / manual rewrites.
// For re-generating an assistant turn use POST /regenerate.
func (h *Handler) editMessage(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	chatID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid chat id", http.StatusBadRequest)
		return
	}
	var mid int64
	if _, err := fmt.Sscan(r.PathValue("mid"), &mid); err != nil || mid <= 0 {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}
	if _, err := h.Repo.GetChat(r.Context(), user.ID, chatID); err != nil {
		h.writeErr(w, err)
		return
	}

	var req editMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.Repo.EditMessageContent(r.Context(), chatID, mid, req.Content); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exportChat streams the full chat (metadata + all messages) as JSONL.
// First line is a `{"type":"chat_meta",...}` envelope; subsequent lines are
// one message each (`{"type":"message","role":...,"content":...}`).
// Designed to round-trip with importChat, and to be readable-ish by SillyTavern
// users who'd manually massage the file.
func (h *Handler) exportChat(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	chatID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	chat, err := h.Repo.GetChat(r.Context(), user.ID, chatID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	msgs, err := h.Repo.ListMessages(r.Context(), chatID, true) // include hidden for fidelity
	if err != nil {
		h.writeErr(w, err)
		return
	}

	// Optional character name for the meta line. Silent fallback if missing.
	var charName string
	if chat.CharacterID != nil && h.Characters != nil {
		if c, err := h.Characters.Get(r.Context(), user.ID, *chat.CharacterID); err == nil {
			charName = c.Name
		}
	}

	filename := sanitiseFilename(chat.Name) + ".jsonl"
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	enc := json.NewEncoder(w)

	// Meta line. chat_metadata is the raw JSONB, so the export carries
	// whatever knobs (sampler, persona_id, authors_note) the chat had.
	meta := map[string]any{
		"type":           "chat_meta",
		"schema":         "wunest-chat-v1",
		"name":           chat.Name,
		"character_name": charName,
		"created_at":     chat.CreatedAt,
		"metadata":       json.RawMessage(chat.Metadata),
	}
	if err := enc.Encode(meta); err != nil {
		slog.Error("export meta", "err", err)
		return
	}
	// Message lines. Preserves swipes + extras verbatim.
	for _, m := range msgs {
		line := map[string]any{
			"type":       "message",
			"role":       m.Role,
			"content":    m.Content,
			"swipes":     json.RawMessage(m.Swipes),
			"swipe_id":   m.SwipeID,
			"extras":     json.RawMessage(m.Extras),
			"hidden":     m.Hidden,
			"created_at": m.CreatedAt,
		}
		if err := enc.Encode(line); err != nil {
			slog.Error("export message", "err", err, "id", m.ID)
			return
		}
	}
}

// importChat consumes a JSONL file upload (same shape as exportChat's output)
// and creates a fresh chat + messages owned by the caller. Character is
// re-resolved by name if one of the user's characters matches; otherwise
// the chat comes in character-less.
func (h *Handler) importChat(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	// Accept either multipart (file upload) or direct application/x-ndjson.
	// 16 MiB cap — generous for even long chats.
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024*1024)

	var body []byte
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(16 * 1024 * 1024); err != nil {
			http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer f.Close()
		body, err = io.ReadAll(f)
		if err != nil {
			http.Error(w, "read file", http.StatusBadRequest)
			return
		}
	} else {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
	}
	if len(body) == 0 {
		http.Error(w, "empty payload", http.StatusBadRequest)
		return
	}

	// Line-by-line decode. First meta, rest messages. Tolerant of leading
	// blank lines. Strict on unknown `type` values — fail fast beats silent
	// partial imports.
	lines := splitJSONL(body)
	if len(lines) == 0 {
		http.Error(w, "no JSON lines", http.StatusBadRequest)
		return
	}

	var metaLine struct {
		Type          string          `json:"type"`
		Name          string          `json:"name"`
		CharacterName string          `json:"character_name"`
		Metadata      json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(lines[0], &metaLine); err != nil || metaLine.Type != "chat_meta" {
		http.Error(w, "first line must be chat_meta", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(metaLine.Name)
	if name == "" {
		name = "Imported chat"
	}

	// Resolve character by name (best-effort). Not fatal if missing — the
	// chat still gets created, just character-less.
	var charID *uuid.UUID
	if metaLine.CharacterName != "" && h.Characters != nil {
		if found, err := h.Characters.FindByName(r.Context(), user.ID, metaLine.CharacterName); err == nil && found != nil {
			id := found.ID
			charID = &id
		}
	}

	chat, err := h.Repo.CreateChat(r.Context(), CreateChatInput{
		UserID:      user.ID,
		CharacterID: charID,
		Name:        name,
		Metadata:    metaLine.Metadata,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}

	// Messages — insert in order, preserving role/content/swipes/extras.
	// Collect skipped-line reports so the client can tell the user which
	// rows didn't land, rather than silently ingesting a partial chat.
	type msgLine struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content string          `json:"content"`
		Swipes  json.RawMessage `json:"swipes"`
		SwipeID int             `json:"swipe_id"`
		Extras  json.RawMessage `json:"extras"`
		Hidden  bool            `json:"hidden"`
	}
	type skippedLine struct {
		Line   int    `json:"line"`   // 1-based, counting the meta row as line 1
		Reason string `json:"reason"`
	}
	imported := 0
	skipped := make([]skippedLine, 0)
	// Cap the skipped-list length so a truly malformed file doesn't inflate
	// the response to multi-MB of error records — show the first N, count
	// the rest as "… and M more".
	const maxSkippedReported = 50
	overflow := 0
	recordSkip := func(lineIdx int, reason string) {
		if len(skipped) < maxSkippedReported {
			skipped = append(skipped, skippedLine{Line: lineIdx, Reason: reason})
		} else {
			overflow++
		}
	}
	for i, line := range lines[1:] {
		lineIdx := i + 2 // +1 for meta, +1 for 1-based numbering
		var m msgLine
		if err := json.Unmarshal(line, &m); err != nil {
			recordSkip(lineIdx, "invalid json: "+err.Error())
			continue
		}
		if m.Type != "message" {
			recordSkip(lineIdx, "unknown type "+strconv.Quote(m.Type))
			continue
		}
		role := Role(m.Role)
		if role != RoleUser && role != RoleAssistant && role != RoleSystem {
			recordSkip(lineIdx, "invalid role "+strconv.Quote(m.Role))
			continue
		}
		extras := parseMessageExtras(m.Extras)
		appended, err := h.Repo.AppendMessage(r.Context(), chat.ID, role, m.Content, extras)
		if err != nil {
			slog.Warn("import: append", "err", err, "line", lineIdx)
			recordSkip(lineIdx, "db insert failed")
			continue
		}
		if len(m.Swipes) > 0 && string(m.Swipes) != "null" && string(m.Swipes) != "[]" {
			if err := h.Repo.RestoreSwipes(r.Context(), chat.ID, appended.ID, m.Swipes, m.SwipeID); err != nil {
				// Not fatal — content is in; just note the swipes slot didn't.
				slog.Warn("import: restore swipes", "err", err, "line", lineIdx)
			}
		}
		imported++
	}

	resp := map[string]any{
		"chat":              chat,
		"imported":          imported,
		"skipped":           len(skipped) + overflow,
		"skipped_details":   skipped,
		"skipped_overflow":  overflow,
		"total_data_lines":  len(lines) - 1,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// splitJSONL trims each line of its surrounding whitespace and skips empties.
// Accepts LF and CRLF. Not a general JSON-streaming decoder — we trust the
// export shape (one object per line, no line-break inside objects).
func splitJSONL(body []byte) [][]byte {
	lines := strings.Split(string(body), "\n")
	out := make([][]byte, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimRight(l, "\r\t ")
		trimmed = strings.TrimLeft(trimmed, "\t ")
		if trimmed == "" {
			continue
		}
		out = append(out, []byte(trimmed))
	}
	return out
}

// parseMessageExtras turns a JSON blob back into *MessageExtras for insert.
// Nil / empty / parse-failure → nil, which AppendMessage treats as "no extras".
func parseMessageExtras(raw json.RawMessage) *MessageExtras {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var e MessageExtras
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil
	}
	return &e
}

// sanitiseFilename strips characters that are unsafe in most filesystems
// plus trims runs of separators. Output is 1..80 chars, ASCII-safe enough
// for Content-Disposition. Cyrillic + punctuation are preserved because
// modern browsers handle UTF-8 filenames in headers.
func sanitiseFilename(s string) string {
	if s == "" {
		return "chat"
	}
	replacer := strings.NewReplacer(`"`, `'`, "/", "-", "\\", "-", "\n", " ", "\r", " ")
	out := replacer.Replace(s)
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

// swipeMessage generates a new variant for an existing assistant message
// without destroying the previous one. The flow:
//  1. Repo.BeginSwipe captures the current content into swipes[] (if not
//     already there), appends an empty slot, clears `content`, and bumps
//     `swipe_id` to the new index.
//  2. We build the prompt from history UP TO but NOT INCLUDING the target
//     message — we're regenerating its content fresh.
//  3. streamChatSwipe reuses the existing message as the placeholder and
//     runs pipeStream; on done, Repo.FinalizeSwipe mirrors the final
//     content into swipes[swipe_id].
//
// Response shape matches POST /messages / regenerate (SSE), with an extra
// `swipe_start` event up front carrying the new swipe_id and swipes length
// so the client can update its local pagination state immediately.
func (h *Handler) swipeMessage(w http.ResponseWriter, r *http.Request) {
	session := auth.FromContext(r.Context())
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.Users.Resolve(r.Context(), session.WuApi.ID)
	if err != nil {
		slog.Error("resolve user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chatID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid chat id", http.StatusBadRequest)
		return
	}
	mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
	if err != nil {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}
	chat, err := h.Repo.GetChat(r.Context(), user.ID, chatID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	msg, err := h.Repo.GetMessage(r.Context(), chatID, mid)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if msg.Role != RoleAssistant {
		http.Error(w, "can only swipe assistant messages", http.StatusBadRequest)
		return
	}
	newSwipeID, err := h.Repo.BeginSwipe(r.Context(), chatID, mid)
	if err != nil {
		slog.Error("begin swipe", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	up := h.resolveUpstream(r.Context(), user.ID, chat.Metadata, session.WuApi.APIKey)
	if up.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}

	var in SendMessageInput
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	userName, userDesc := h.resolvePersona(r.Context(), user.ID, chat.Metadata, session.WuApi.FirstName, session.WuApi.Username)
	in = applyChatSampler(in, readSamplerFromChat(chat))
	in.AuthorsNote = readAuthorsNote(chat.Metadata)
	in = h.applyUserDefaults(r.Context(), user.ID, in)

	h.streamChatSwipe(w, r, user.ID, chatID, chat.CharacterID, up, userName, userDesc, mid, newSwipeID, in)
}

// selectSwipe picks an existing variant for the message. Body: {swipe_id}.
// Returns the updated message JSON (not a stream — no LLM call).
func (h *Handler) selectSwipe(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	chatID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid chat id", http.StatusBadRequest)
		return
	}
	mid, err := strconv.ParseInt(r.PathValue("mid"), 10, 64)
	if err != nil {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}
	// Ownership check via chat.
	if _, err := h.Repo.GetChat(r.Context(), user.ID, chatID); err != nil {
		h.writeErr(w, err)
		return
	}
	var req struct {
		SwipeID int `json:"swipe_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	msg, err := h.Repo.SelectSwipe(r.Context(), chatID, mid, req.SwipeID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

// regenerate drops the most recent assistant message and streams a fresh
// completion for the same conversation state. Body accepts an optional
// `model` / `temperature` / `max_tokens` override. Response is SSE in the
// same shape as POST /messages.
//
// V1 semantics: one assistant reply is visible at a time. When swipes
// land (future), this will append a new swipe to an existing placeholder
// instead of deleting it.
func (h *Handler) regenerate(w http.ResponseWriter, r *http.Request) {
	session := auth.FromContext(r.Context())
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.Users.Resolve(r.Context(), session.WuApi.ID)
	if err != nil {
		slog.Error("resolve user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chatID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	chat, err := h.Repo.GetChat(r.Context(), user.ID, chatID)
	if err != nil {
		h.writeErr(w, err)
		return
	}

	var in SendMessageInput
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}

	// Drop the latest assistant message so streamChatRegen builds the
	// prompt from the preceding user turn.
	if last, err := h.Repo.LastAssistantMessage(r.Context(), chatID); err == nil {
		if err := h.Repo.DeleteMessage(r.Context(), chatID, last.ID); err != nil {
			slog.Warn("regenerate: delete last assistant failed", "err", err, "id", last.ID)
		}
	}

	up := h.resolveUpstream(r.Context(), user.ID, chat.Metadata, session.WuApi.APIKey)
	if up.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}
	userName, userDesc := h.resolvePersona(r.Context(), user.ID, chat.Metadata, session.WuApi.FirstName, session.WuApi.Username)

	// Same sampler merge as sendMessage so a regenerate honours the same
	// drawer-saved defaults.
	in = applyChatSampler(in, readSamplerFromChat(chat))
	in.AuthorsNote = readAuthorsNote(chat.Metadata)
	in = h.applyUserDefaults(r.Context(), user.ID, in)

	h.streamChatRegen(w, r, user.ID, chat.ID, chat.CharacterID, up, userName, userDesc, in)
}

// --- helpers ---

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
		slog.Error("chats handler", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
