package chats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/users"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// Handler wires the chat-related HTTP routes onto a mux. Kept as a struct
// (not interface) because we don't need test doubles at this depth yet.
type Handler struct {
	Repo       *Repository
	Users      *users.Resolver
	Characters *characters.Repository
	WuApi      *wuapi.Client
}

func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("GET /api/chats", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/chats", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/chats/{id}", authRequired(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/chats/{id}", authRequired(http.HandlerFunc(h.rename)))
	mux.Handle("PUT /api/chats/{id}/sampler", authRequired(http.HandlerFunc(h.setSampler)))
	mux.Handle("DELETE /api/chats/{id}", authRequired(http.HandlerFunc(h.delete)))
	mux.Handle("POST /api/chats/{id}/regenerate", authRequired(http.HandlerFunc(h.regenerate)))
	mux.Handle("GET /api/chats/{id}/messages", authRequired(http.HandlerFunc(h.listMessages)))
	mux.Handle("POST /api/chats/{id}/messages", authRequired(http.HandlerFunc(h.sendMessage)))
	mux.Handle("PATCH /api/chats/{id}/messages/{mid}", authRequired(http.HandlerFunc(h.editMessage)))
	mux.Handle("DELETE /api/chats/{id}/messages/{mid}", authRequired(http.HandlerFunc(h.deleteMessage)))
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

	chat, err := h.Repo.CreateChat(r.Context(), CreateChatInput{
		UserID:      user.ID,
		CharacterID: req.CharacterID,
		Name:        name,
		Metadata:    req.Metadata,
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
				// Light macro pass so {{user}}/{{char}} render.
				greeting = SubstituteMacros(greeting, PromptInput{
					Character: ch,
					UserName:  user.DisplayName(),
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

	apiKey := session.WuApi.APIKey
	if apiKey == "" {
		http.Error(w, "no api key available", http.StatusPreconditionFailed)
		return
	}

	userName := session.WuApi.FirstName
	if userName == "" {
		userName = session.WuApi.Username
	}

	// Apply chat_metadata.sampler as the baseline; explicit fields in the
	// request body override (common pattern: drawer sliders set chat
	// defaults once, per-turn overrides are rare).
	in = applyChatSampler(in, readSampler(chat.Metadata))

	// Stream.
	h.streamChat(w, r, user.ID, chat.ID, chat.CharacterID, apiKey, userName, "", in)
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
	if in.MaxTokens == nil && s.MaxTokens != nil {
		in.MaxTokens = s.MaxTokens
	}
	if in.FrequencyPenalty == nil && s.FrequencyPenalty != nil {
		in.FrequencyPenalty = s.FrequencyPenalty
	}
	if in.PresencePenalty == nil && s.PresencePenalty != nil {
		in.PresencePenalty = s.PresencePenalty
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

	apiKey := session.WuApi.APIKey
	if apiKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}
	userName := session.WuApi.FirstName
	if userName == "" {
		userName = session.WuApi.Username
	}

	// Same sampler merge as sendMessage so a regenerate honours the same
	// drawer-saved defaults.
	in = applyChatSampler(in, readSamplerFromChat(chat))

	h.streamChatRegen(w, r, user.ID, chat.ID, chat.CharacterID, apiKey, userName, "", in)
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
