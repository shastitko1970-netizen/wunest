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
	// Beta-gate helper: compose authRequired with RequireNestAccess. Applied
	// to endpoints that spend upstream gold or enqueue new assistant turns.
	// Read-only + CRUD-on-own-data routes stay un-gated so users can still
	// browse existing chats, rename / delete, and edit the drafts they
	// already own — the gate is strictly about "no new generation until
	// you've redeemed a code".
	betaGated := func(h http.Handler) http.Handler {
		return authRequired(auth.RequireNestAccess(h))
	}

	mux.Handle("GET /api/chats", authRequired(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/chats/search", authRequired(http.HandlerFunc(h.search)))
	mux.Handle("GET /api/chats/tags", authRequired(http.HandlerFunc(h.tagsList)))
	mux.Handle("GET /api/chats/{id}/stats", authRequired(http.HandlerFunc(h.stats)))
	// Memory / summarisation (M38.4).
	mux.Handle("GET /api/chats/{id}/summaries", authRequired(http.HandlerFunc(h.listSummaries)))
	mux.Handle("POST /api/chats/{id}/summaries", authRequired(http.HandlerFunc(h.createSummary)))
	mux.Handle("PATCH /api/chats/{id}/summaries/{sid}", authRequired(http.HandlerFunc(h.updateSummary)))
	mux.Handle("DELETE /api/chats/{id}/summaries/{sid}", authRequired(http.HandlerFunc(h.deleteSummary)))
	// Summarise runs the LLM to (re)generate the rolling auto-summary.
	// Gated behind beta access because it spends tokens (even if cheap).
	mux.Handle("POST /api/chats/{id}/summarize", betaGated(http.HandlerFunc(h.summarize)))
	mux.Handle("POST /api/chats", authRequired(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/chats/{id}", authRequired(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/chats/{id}", authRequired(http.HandlerFunc(h.rename)))
	mux.Handle("PUT /api/chats/{id}/sampler", authRequired(http.HandlerFunc(h.setSampler)))
	mux.Handle("PUT /api/chats/{id}/persona", authRequired(http.HandlerFunc(h.setPersona)))
	mux.Handle("PUT /api/chats/{id}/authors-note", authRequired(http.HandlerFunc(h.setAuthorsNote)))
	mux.Handle("PUT /api/chats/{id}/byok", authRequired(http.HandlerFunc(h.setBYOK)))
	mux.Handle("DELETE /api/chats/{id}", authRequired(http.HandlerFunc(h.delete)))
	// Generation endpoints — gated. Regenerate + sendMessage + swipe all
	// stream a new assistant turn from the upstream provider.
	mux.Handle("POST /api/chats/{id}/regenerate", betaGated(http.HandlerFunc(h.regenerate)))
	mux.Handle("GET /api/chats/{id}/messages", authRequired(http.HandlerFunc(h.listMessages)))
	mux.Handle("POST /api/chats/{id}/messages", betaGated(http.HandlerFunc(h.sendMessage)))
	mux.Handle("PATCH /api/chats/{id}/messages/{mid}", authRequired(http.HandlerFunc(h.editMessage)))
	mux.Handle("DELETE /api/chats/{id}/messages/{mid}", authRequired(http.HandlerFunc(h.deleteMessage)))
	// Swipes — alternate assistant outputs for the same turn. Creating a new
	// swipe is a generation (gated); selecting among existing swipes is not.
	mux.Handle("POST /api/chats/{id}/messages/{mid}/swipe", betaGated(http.HandlerFunc(h.swipeMessage)))
	// Continue — extends the last assistant message with more content.
	// Unlike regenerate (which discards + retries) continue appends to
	// what's already there, useful when a response was cut short.
	mux.Handle("POST /api/chats/{id}/messages/{mid}/continue", betaGated(http.HandlerFunc(h.continueMessage)))
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
	// CharacterIDs is the preferred modern field — a list of character
	// participants. Single-character chats send a 1-element array (or
	// just CharacterID for backward compat). Group chats send 2+.
	CharacterIDs []uuid.UUID     `json:"character_ids,omitempty"`
	Name         string          `json:"name,omitempty"`
	Metadata     json.RawMessage `json:"chat_metadata,omitempty"`
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

	// Reconcile old/new shape so the rest of the handler can treat
	// CharacterIDs as the single source of truth.
	characterIDs := req.CharacterIDs
	if len(characterIDs) == 0 && req.CharacterID != nil {
		characterIDs = []uuid.UUID{*req.CharacterID}
	}

	// Validate every participant belongs to this user (cheap check —
	// prevents a malicious caller from "starting a group chat" with
	// somebody else's character ID to probe existence).
	for _, cid := range characterIDs {
		if _, err := h.Characters.Get(r.Context(), user.ID, cid); err != nil {
			http.Error(w, "character not found: "+cid.String(), http.StatusBadRequest)
			return
		}
	}

	// Default the chat name from the first character if not provided.
	name := req.Name
	if name == "" && len(characterIDs) > 0 {
		if ch, err := h.Characters.Get(r.Context(), user.ID, characterIDs[0]); err == nil {
			name = ch.Name
			// Append a " + N" suffix for group chats so the list view
			// makes the participant count obvious without expanding
			// each row.
			if len(characterIDs) > 1 {
				name = fmt.Sprintf("%s + %d", ch.Name, len(characterIDs)-1)
			}
		}
	}
	if name == "" {
		name = "New chat"
	}

	// M30: Sampler state moved out of chat_metadata into per-user active
	// presets. Chats no longer carry their own sampler snapshot; every turn
	// reads the user's active sampler + sysprompt from settings at send
	// time. New chats start with an empty metadata envelope; per-chat
	// persona / byok / authors_note are still stored inline here.
	metadata := req.Metadata

	chat, err := h.Repo.CreateChat(r.Context(), CreateChatInput{
		UserID:       user.ID,
		CharacterIDs: characterIDs,
		Name:         name,
		Metadata:     metadata,
	})
	if err != nil {
		h.writeErr(w, err)
		return
	}

	// Seed the first assistant message with the character's greeting. When
	// the character has alternate_greetings we collect them all into the
	// message's swipes[] so the user can page through every greeting the
	// author wrote — matches SillyTavern's behavior.
	//
	// Macros ({{user}}, {{char}}, {{random::…}}) get expanded on each
	// greeting independently. If first_mes is blank but there ARE
	// alternate_greetings, the first alt becomes the visible greeting so
	// the UI isn't empty.
	// Seed the first assistant message with greetings.
	//
	// For SINGLE-character chats: first_mes + alternate_greetings as
	// swipes, same as ST.
	//
	// For GROUP chats: collect each character's first_mes (+ their
	// non-empty alternate_greetings[0] as a fallback for characters
	// without first_mes) into a multi-speaker swipes pool. Each swipe
	// is attributed via swipe_character_ids so MessageBubble can label
	// who said what when the user pages through.
	if len(characterIDs) > 1 {
		h.seedGroupGreetings(r.Context(), chat.ID, user, characterIDs)
	} else if len(characterIDs) == 1 {
		if ch, err := h.Characters.Get(r.Context(), user.ID, characterIDs[0]); err == nil {
			personaName := user.DisplayName()
			if h.Personas != nil {
				if p, err := h.Personas.Default(r.Context(), user.ID); err == nil && p.Name != "" {
					personaName = p.Name
				}
			}
			macroCtx := PromptInput{Character: ch, UserName: personaName}

			// Build the full greeting pool: first_mes first, then
			// alternate_greetings in declared order. Empties are dropped
			// so an empty first_mes slot doesn't become an empty swipe.
			pool := make([]string, 0, 1+len(ch.Data.AlternateGreetings))
			if g := strings.TrimSpace(ch.Data.FirstMes); g != "" {
				pool = append(pool, SubstituteMacros(ch.Data.FirstMes, macroCtx))
			}
			for _, alt := range ch.Data.AlternateGreetings {
				if strings.TrimSpace(alt) == "" {
					continue
				}
				pool = append(pool, SubstituteMacros(alt, macroCtx))
			}

			switch len(pool) {
			case 0:
				// No greetings at all — skip. UI shows the "say hi" empty state.
			case 1:
				if _, err := h.Repo.AppendMessage(r.Context(), chat.ID, RoleAssistant, pool[0], &MessageExtras{
					Model: "greeting",
				}); err != nil {
					slog.Warn("seed greeting failed", "err", err)
				}
			default:
				// Multi-greeting — store as swipes so the user can page
				// through. First greeting visible; swipe_id=0.
				if _, err := h.Repo.AppendMessageWithSwipes(
					r.Context(), chat.ID, RoleAssistant,
					pool[0], pool, 0,
					&MessageExtras{Model: "greeting"},
				); err != nil {
					slog.Warn("seed greeting with swipes failed", "err", err)
				}
			}
		}
	}

	writeJSON(w, http.StatusCreated, chat)
}

// patchRequest is the body of PATCH /api/chats/:id. Pointer fields so the
// client can send just the field it wants to change — nil = leave alone.
type patchRequest struct {
	Name *string   `json:"name,omitempty"`
	Tags *[]string `json:"tags,omitempty"`
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

// rename is the PATCH /api/chats/:id handler. Despite the name it's a
// general partial-update now — body is `patchRequest` with optional
// `name` + `tags`. Original endpoint was rename-only; kept the name
// for route consistency.
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
	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name != nil {
		if err := h.Repo.RenameChat(r.Context(), user.ID, id, *req.Name); err != nil {
			h.writeErr(w, err)
			return
		}
	}
	if req.Tags != nil {
		if err := h.Repo.SetTags(r.Context(), user.ID, id, *req.Tags); err != nil {
			h.writeErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Memory / summaries (M38.4) ──────────────────────────────────

// listSummaries returns every summary row for the chat, all roles
// interleaved (UI slots them by role).
func (h *Handler) listSummaries(w http.ResponseWriter, r *http.Request) {
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
	if _, err := h.Repo.GetChat(r.Context(), user.ID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	summaries, err := h.Repo.ListSummaries(r.Context(), id)
	if err != nil {
		slog.Error("list summaries", "err", err, "chat_id", id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": summaries})
}

// createSummary inserts a manual or pinned summary row.
// Body: { content: "...", pinned: bool }
func (h *Handler) createSummary(w http.ResponseWriter, r *http.Request) {
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
	if _, err := h.Repo.GetChat(r.Context(), user.ID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	var req struct {
		Content string `json:"content"`
		Pinned  bool   `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, "content required", http.StatusBadRequest)
		return
	}
	s, err := h.Repo.CreateManualSummary(r.Context(), id, req.Content, req.Pinned)
	if err != nil {
		slog.Error("create summary", "err", err, "chat_id", id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// updateSummary edits an existing summary's content and optionally
// promotes it to a different role (e.g. auto → manual to stop
// auto-regen from overwriting user tweaks).
// Body: { content: "...", role: "auto"|"manual"|"pinned" }
func (h *Handler) updateSummary(w http.ResponseWriter, r *http.Request) {
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
	if _, err := h.Repo.GetChat(r.Context(), user.ID, chatID); err != nil {
		h.writeErr(w, err)
		return
	}
	sid, err := uuid.Parse(r.PathValue("sid"))
	if err != nil {
		http.Error(w, "invalid summary id", http.StatusBadRequest)
		return
	}
	var req struct {
		Content string `json:"content"`
		Role    string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	// Validate role if provided.
	if req.Role != "" && req.Role != "auto" && req.Role != "manual" && req.Role != "pinned" {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}
	if err := h.Repo.UpdateSummary(r.Context(), sid, req.Content, req.Role); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteSummary removes a summary row. Ownership enforced via chat
// lookup (deleting summaries of other people's chats is impossible
// because sid is UUID-random + chat ownership is verified first).
func (h *Handler) deleteSummary(w http.ResponseWriter, r *http.Request) {
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
	if _, err := h.Repo.GetChat(r.Context(), user.ID, chatID); err != nil {
		h.writeErr(w, err)
		return
	}
	sid, err := uuid.Parse(r.PathValue("sid"))
	if err != nil {
		http.Error(w, "invalid summary id", http.StatusBadRequest)
		return
	}
	if err := h.Repo.DeleteSummary(r.Context(), sid); err != nil {
		h.writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// summarize triggers (re)generation of the rolling auto-summary for
// the chat. Calls the summariser LLM, folds in any new messages,
// upserts the summary row.
//
// Body (all optional):
//
//	model: string   — summariser model to use (default Gemini 2.5 Flash)
//
// Never touches auto-generated-previously summaries with role≠auto, so
// a user who promoted a summary to role=manual will keep their edits.
func (h *Handler) summarize(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Model string `json:"model,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Load current history (include hidden) + existing auto summary.
	history, err := h.Repo.ListMessages(r.Context(), chatID, true)
	if err != nil {
		http.Error(w, "load history: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var prevContent string
	var prevCovered int64
	if auto, _ := h.Repo.GetAutoSummary(r.Context(), chatID); auto != nil {
		prevContent = auto.Content
		if auto.CoveredThroughMessageID != nil {
			prevCovered = *auto.CoveredThroughMessageID
		}
	}
	toFold, _ := PickSummariserBounds(history, prevCovered)
	if len(toFold) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"summary":  nil,
			"message":  "nothing new to summarise",
			"folded":   0,
		})
		return
	}

	// Upstream resolution: same rules as send — BYOK if pinned, else
	// WuApi via the session's API key.
	up := h.resolveUpstream(r.Context(), user.ID, chat.Metadata, session.WuApi.APIKey)
	if up.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}

	// Speaker-name map for group-chat "assistant (Alice):" prefixing.
	speakerNames := map[string]string{}
	if chat.IsGroupChat && h.Characters != nil {
		for _, cid := range chat.CharacterIDs {
			if c, err := h.Characters.Get(r.Context(), user.ID, cid); err == nil {
				speakerNames[cid.String()] = c.Name
			}
		}
	}

	res, err := h.SummariseChat(r.Context(), SummariseInput{
		ChatID:                  chatID.String(),
		Model:                   req.Model,
		PreviousSummary:         prevContent,
		Messages:                toFold,
		PromptAPIKey:            up.APIKey,
		PromptBaseURL:           up.BaseURL,
		SpeakerNameByCharacterID: speakerNames,
	})
	if err != nil {
		slog.Warn("summarise failed", "err", err, "chat_id", chatID)
		http.Error(w, "summarise failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	saved, err := h.Repo.UpsertAutoSummary(
		r.Context(), chatID,
		res.Content, res.CoveredThroughMessageID,
		res.TokenCount, res.Model,
	)
	if err != nil {
		slog.Error("save summary", "err", err, "chat_id", chatID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": saved,
		"folded":  len(toFold),
	})
}

// stats returns aggregate metrics for one chat (message counts by
// role, total tokens in/out, swipes, first/last message time, unique
// models used). Ownership is verified by GetChat which also scopes on
// user_id — we don't leak stats of other users' chats even with a
// valid chat id guess.
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
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
	if _, err := h.Repo.GetChat(r.Context(), user.ID, id); err != nil {
		h.writeErr(w, err)
		return
	}
	stats, err := h.Repo.GetChatStats(r.Context(), id)
	if err != nil {
		slog.Error("chat stats", "err", err, "chat_id", id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// tagsList returns all distinct tags the user has ever applied to any
// of their chats. Used for autocomplete + filter dropdown.
func (h *Handler) tagsList(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	tags, err := h.Repo.DistinctTags(r.Context(), user.ID)
	if err != nil {
		slog.Error("distinct tags", "err", err, "user_id", user.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tags})
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
	// Empty content is allowed ONLY in group chats, where it means
	// "continue the scene — next speaker, no user turn". Single-char
	// chats still require content (there'd be nothing to respond to).
	if in.Content == "" && !chat.IsGroupChat {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	up := h.resolveUpstream(r.Context(), user.ID, chat.Metadata, session.WuApi.APIKey)
	if up.APIKey == "" {
		http.Error(w, "no api key available", http.StatusPreconditionFailed)
		return
	}

	userName, userDesc := h.resolvePersona(r.Context(), user.ID, chat.Metadata, session.WuApi.FirstName, session.WuApi.Username)

	// M30: Fill in sampler + sysprompt from the user's active presets
	// (global; one active per type, applies to every chat). Per-turn
	// overrides in the request body win; chat_metadata.sampler is no
	// longer consulted (old chats may still carry it in DB — ignored).
	in = h.applyActivePresets(r.Context(), user.ID, in)
	in.AuthorsNote = readAuthorsNote(chat.Metadata)
	in = h.applyUserDefaults(r.Context(), user.ID, in)

	// M32: Run user-input regex scripts BEFORE streaming so the message
	// stored in DB matches what the model will see. Typical ST scripts
	// replace spaces with unicode-invisible chars (jailbreak) or strip
	// HTML; harmless no-op when the bundle has no placement=1 scripts.
	in.Content = ApplyRegexToUserInput(in.Bundle, in.Content)

	// Resolve which character responds this turn.
	//   - Single-char chat: always chat.CharacterID (client may pass nil
	//     speaker_id, that's fine — the one participant speaks).
	//   - Group chat: require speaker_id AND it must be in character_ids,
	//     otherwise fall back to the first character as a safe default
	//     rather than 400-erroring (better DX: lead character just speaks).
	respondingCharID := chat.CharacterID
	if in.SpeakerID != nil {
		ok := false
		for _, cid := range chat.CharacterIDs {
			if cid == *in.SpeakerID {
				ok = true
				break
			}
		}
		// Legacy: chat has character_ids empty (pre-migration backfill
		// edge case) — accept speaker_id anyway if it was requested.
		if !ok && len(chat.CharacterIDs) == 0 {
			ok = true
		}
		if ok {
			respondingCharID = in.SpeakerID
		}
	} else if chat.IsGroupChat && len(chat.CharacterIDs) > 0 {
		// Group chat without speaker_id: default to first participant.
		first := chat.CharacterIDs[0]
		respondingCharID = &first
	}

	// Stream.
	others := otherParticipantIDs(chat.CharacterIDs, respondingCharID)
	h.streamChat(w, r, user.ID, chat.ID, respondingCharID, others, up, userName, userDesc, in)
}

// otherParticipantIDs returns all character IDs in the group except the
// current speaker. Empty / nil for single-character chats (len<=1) so
// prompt builder's IsGroupChat() stays false.
func otherParticipantIDs(all []uuid.UUID, speaker *uuid.UUID) []uuid.UUID {
	if len(all) <= 1 {
		return nil
	}
	out := make([]uuid.UUID, 0, len(all)-1)
	for _, id := range all {
		if speaker != nil && id == *speaker {
			continue
		}
		out = append(out, id)
	}
	return out
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

// applyActivePresets fills SendMessageInput fields from the user's currently
// active presets (M30 "variant 1": presets are global, one active per type,
// applies to every chat the user runs).
//
// Sources, lowest to highest priority:
//   1. Request body (explicit per-turn override).
//   2. Active sysprompt preset (only if sampler system prompt is unset).
//   3. Active sampler preset (fills numeric knobs + stop + reasoning flag).
//
// Each source only writes fields the higher-priority source left unset, so
// a request can tweak one slider without dropping everything else from the
// active preset. Errors loading settings are logged-and-ignored — generation
// should degrade to raw upstream defaults, not 500.
func (h *Handler) applyActivePresets(ctx context.Context, userID uuid.UUID, in SendMessageInput) SendMessageInput {
	if h.Users == nil || h.Presets == nil {
		return in
	}
	settings, err := h.Users.LoadSettings(ctx, userID)
	if err != nil || settings == nil || len(settings.DefaultPresets) == 0 {
		return in
	}

	// Sampler — numeric knobs + stop + reasoning + fallback system prompt.
	// We also decode the FULL ST bundle (prompts + prompt_order + regex)
	// so prompt assembly downstream can use it when present. One Get() call
	// serves both paths; AsSampler and AsOpenAIBundle share the JSONB.
	if id, ok := settings.DefaultPresets[string(presets.TypeSampler)]; ok && id != uuid.Nil {
		if p, err := h.Presets.Get(ctx, userID, id); err == nil && p != nil {
			s := p.AsSampler()
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
			// Full bundle — attach when the preset carries Prompt Manager
			// or regex data. Cheap if absent (empty slices).
			bundle := p.AsOpenAIBundle()
			if len(bundle.Prompts) > 0 || len(bundle.Extensions.RegexScripts) > 0 {
				in.Bundle = &bundle
				// ST often stores max_tokens under `openai_max_tokens`;
				// surface it to the sampler flow if still unset.
				if in.MaxTokens == nil && bundle.OpenAIMaxTokens != nil {
					in.MaxTokens = bundle.OpenAIMaxTokens
				}
			}
		}
	}

	// Sysprompt — wins over the sampler's own system_prompt for users who
	// keep a dedicated sysprompt preset active. The sampler-level override
	// above already ran, so only fills when still unset.
	if id, ok := settings.DefaultPresets[string(presets.TypeSysprompt)]; ok && id != uuid.Nil {
		if p, err := h.Presets.Get(ctx, userID, id); err == nil && p != nil {
			sp := p.AsSysprompt()
			if in.SystemPromptOverride == "" && sp.Content != "" {
				in.SystemPromptOverride = sp.Content
			}
		}
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

// editMessageRequest is the JSON body for PATCH /messages/:mid. Both
// fields optional; send whichever you want to change. `hidden` is the
// silent-message toggle — when true, the message still feeds into the
// prompt (so model context stays intact) but the UI greys it out.
type editMessageRequest struct {
	Content *string `json:"content,omitempty"`
	Hidden  *bool   `json:"hidden,omitempty"`
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
	if req.Content != nil {
		if err := h.Repo.EditMessageContent(r.Context(), chatID, mid, *req.Content); err != nil {
			h.writeErr(w, err)
			return
		}
	}
	if req.Hidden != nil {
		if err := h.Repo.SetMessageHidden(r.Context(), chatID, mid, *req.Hidden); err != nil {
			h.writeErr(w, err)
			return
		}
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
	// blank lines. We accept TWO flavours:
	//
	//   1. WuNest native format — `{"type":"chat_meta",...}` + `{"type":"message",...}`
	//      lines (what exportChat emits).
	//   2. SillyTavern chat exports — meta line with `user_name`/`character_name`
	//      + message lines with `name`/`is_user`/`mes`/`swipes`. No `type` field.
	//
	// We auto-detect on the first line: if it lacks `type:"chat_meta"` but
	// DOES have ST-style keys, transparently translate. This saves users
	// the trouble of converting files by hand when migrating from ST.
	lines := splitJSONL(body)
	if len(lines) == 0 {
		http.Error(w, "no JSON lines", http.StatusBadRequest)
		return
	}

	// Generic peek to detect format.
	var peek map[string]json.RawMessage
	if err := json.Unmarshal(lines[0], &peek); err != nil {
		http.Error(w, "first line is not valid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	format := detectChatFormat(peek)

	var metaLine struct {
		Type          string          `json:"type"`
		Name          string          `json:"name"`
		CharacterName string          `json:"character_name"`
		Metadata      json.RawMessage `json:"metadata"`
	}
	switch format {
	case "wunest":
		if err := json.Unmarshal(lines[0], &metaLine); err != nil || metaLine.Type != "chat_meta" {
			http.Error(w, "first line must be chat_meta", http.StatusBadRequest)
			return
		}
	case "silly-tavern":
		// ST meta: {"user_name":"...","character_name":"...","create_date":"...","chat_metadata":{...}}
		var stMeta struct {
			UserName      string          `json:"user_name"`
			CharacterName string          `json:"character_name"`
			CreateDate    string          `json:"create_date"`
			ChatMetadata  json.RawMessage `json:"chat_metadata"`
		}
		if err := json.Unmarshal(lines[0], &stMeta); err != nil {
			http.Error(w, "ST meta line invalid: "+err.Error(), http.StatusBadRequest)
			return
		}
		metaLine.CharacterName = stMeta.CharacterName
		// ST files don't include a chat name — default to "{char} chat" or date.
		if stMeta.CharacterName != "" {
			metaLine.Name = "ST: " + stMeta.CharacterName
		} else {
			metaLine.Name = "Imported ST chat"
		}
		metaLine.Metadata = stMeta.ChatMetadata
	default:
		http.Error(w,
			"unrecognised chat format — expected a WuNest export "+
				`({"type":"chat_meta",...}) or a SillyTavern export `+
				`({"user_name":"...","character_name":"..."})`,
			http.StatusBadRequest)
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
		var (
			role    Role
			content string
			swipes  json.RawMessage
			swipeID int
			extras  *MessageExtras
		)
		switch format {
		case "wunest":
			var m msgLine
			if err := json.Unmarshal(line, &m); err != nil {
				recordSkip(lineIdx, "invalid json: "+err.Error())
				continue
			}
			if m.Type != "message" {
				recordSkip(lineIdx, "unknown type "+strconv.Quote(m.Type))
				continue
			}
			role = Role(m.Role)
			content = m.Content
			swipes = m.Swipes
			swipeID = m.SwipeID
			extras = parseMessageExtras(m.Extras)

		case "silly-tavern":
			// ST message shape:
			//   {"name":"Alice","is_user":false,"is_system":false,
			//    "send_date":"...","mes":"...",
			//    "swipe_id":0,"swipes":["..."]}
			var st struct {
				Name     string          `json:"name"`
				IsUser   bool            `json:"is_user"`
				IsSystem bool            `json:"is_system"`
				Mes      string          `json:"mes"`
				SwipeID  int             `json:"swipe_id"`
				Swipes   json.RawMessage `json:"swipes"`
			}
			if err := json.Unmarshal(line, &st); err != nil {
				recordSkip(lineIdx, "invalid json: "+err.Error())
				continue
			}
			switch {
			case st.IsSystem:
				role = RoleSystem
			case st.IsUser:
				role = RoleUser
			default:
				role = RoleAssistant
			}
			content = st.Mes
			swipes = st.Swipes
			swipeID = st.SwipeID
			// Skip ST's first-message placeholder (is_user=false, often
			// empty mes for greeting that was never shown). Non-empty
			// first_mes IS the greeting — let it through.
			if content == "" && len(swipes) == 0 {
				recordSkip(lineIdx, "empty ST message with no swipes")
				continue
			}
			extras = nil
		}
		if role != RoleUser && role != RoleAssistant && role != RoleSystem {
			recordSkip(lineIdx, "invalid role "+strconv.Quote(string(role)))
			continue
		}
		appended, err := h.Repo.AppendMessage(r.Context(), chat.ID, role, content, extras)
		if err != nil {
			slog.Warn("import: append", "err", err, "line", lineIdx)
			recordSkip(lineIdx, "db insert failed")
			continue
		}
		if len(swipes) > 0 && string(swipes) != "null" && string(swipes) != "[]" {
			if err := h.Repo.RestoreSwipes(r.Context(), chat.ID, appended.ID, swipes, swipeID); err != nil {
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

// detectChatFormat sniffs the first-line object and decides whether the
// file is a WuNest export, a SillyTavern export, or something else.
//
// Rules (cheap key-presence checks, no value parsing):
//
//   - `"type":"chat_meta"` present → "wunest" (that's exactly what exportChat writes).
//   - Otherwise, if any of `user_name`/`character_name`/`chat_metadata`/`create_date`
//     is present → "silly-tavern". ST meta lines reliably ship at least
//     `character_name`; `chat_metadata` and `create_date` are near-universal too.
//   - Everything else → "unknown", surfaced as a 400 so the user gets a
//     clear "we don't recognise this file" message instead of a confusing
//     "first line must be chat_meta" when they tried to import ST.
func detectChatFormat(first map[string]json.RawMessage) string {
	if t, ok := first["type"]; ok {
		// Fast path — if the first line literally says `"type":"chat_meta"`,
		// it's our native shape regardless of what else is there.
		if string(t) == `"chat_meta"` {
			return "wunest"
		}
	}
	for _, k := range []string{"user_name", "character_name", "chat_metadata", "create_date"} {
		if _, ok := first[k]; ok {
			return "silly-tavern"
		}
	}
	return "unknown"
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
	in = h.applyActivePresets(r.Context(), user.ID, in)
	in.AuthorsNote = readAuthorsNote(chat.Metadata)
	in = h.applyUserDefaults(r.Context(), user.ID, in)

	// Swipe should re-generate as the SAME speaker who produced the
	// original message. For group chats this pulls character_id off the
	// row; single-char chats fall back to chat.CharacterID.
	respondingCharID := chat.CharacterID
	if msg.CharacterID != nil {
		respondingCharID = msg.CharacterID
	}
	others := otherParticipantIDs(chat.CharacterIDs, respondingCharID)
	h.streamChatSwipe(w, r, user.ID, chatID, respondingCharID, others, up, userName, userDesc, mid, newSwipeID, in)
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
	// prompt from the preceding user turn. Capture its character_id so a
	// group-chat regenerate re-runs as the SAME speaker (otherwise the
	// regen would silently fall back to the first participant).
	var lastCharID *uuid.UUID
	if last, err := h.Repo.LastAssistantMessage(r.Context(), chatID); err == nil {
		lastCharID = last.CharacterID
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

	// Same active-preset merge as sendMessage so a regenerate honours the
	// user's current sampler + sysprompt.
	in = h.applyActivePresets(r.Context(), user.ID, in)
	in.AuthorsNote = readAuthorsNote(chat.Metadata)
	in = h.applyUserDefaults(r.Context(), user.ID, in)

	// Prefer the original message's speaker for regen; fall back to the
	// chat's primary character (single-char chats or pre-group-migration).
	respondingCharID := chat.CharacterID
	if lastCharID != nil {
		respondingCharID = lastCharID
	} else if in.SpeakerID != nil {
		// Client explicitly switched speaker before regen (e.g. "try this
		// as a different character").
		respondingCharID = in.SpeakerID
	}
	others := otherParticipantIDs(chat.CharacterIDs, respondingCharID)
	h.streamChatRegen(w, r, user.ID, chat.ID, respondingCharID, others, up, userName, userDesc, in)
}

// search runs a full-text search across the user's chat messages.
// Returns up to 50 hits ranked by ts_rank_cd then recency.
//
// Query params:
//
//	q            — search string, required; non-empty after trim
//	character_id — optional UUID to scope search to one character's chats
//	limit        — optional 1..200, default 50
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	var charFilter *uuid.UUID
	if cs := r.URL.Query().Get("character_id"); cs != "" {
		if id, err := uuid.Parse(cs); err == nil {
			charFilter = &id
		}
	}
	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if v, err := strconv.Atoi(ls); err == nil && v > 0 {
			limit = v
		}
	}
	hits, err := h.Repo.SearchMessages(r.Context(), user.ID, q, charFilter, limit)
	if err != nil {
		slog.Error("chat search", "err", err, "user_id", user.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": hits})
}

// continueMessage extends the last assistant message with more content.
//
// Unlike regenerate (which discards + retries) continue APPENDS to the
// existing content — useful when a response was cut short by max_tokens
// or user curiosity («tell me more about that»). Works by:
//   1. Loading the target message (must be assistant + latest visible)
//   2. Building a prompt with the existing content as `assistant_prefill`
//   3. Streaming the continuation
//   4. Appending streamed tokens to the existing message's content
//
// Provider support varies: OpenAI-family allows `{role:"assistant",
// content:"..."}` as trailing prefill (they resume). Anthropic requires
// the same. Bundle's `continue_prefill` field can override prefill text.
func (h *Handler) continueMessage(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "can only continue assistant messages", http.StatusBadRequest)
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
	in = h.applyActivePresets(r.Context(), user.ID, in)
	in.AuthorsNote = readAuthorsNote(chat.Metadata)
	in = h.applyUserDefaults(r.Context(), user.ID, in)

	// Continue runs as the SAME speaker who produced the original
	// message. Matches swipe semantics — regenerating the same turn
	// shouldn't switch voice without explicit user action.
	respondingCharID := chat.CharacterID
	if msg.CharacterID != nil {
		respondingCharID = msg.CharacterID
	}
	others := otherParticipantIDs(chat.CharacterIDs, respondingCharID)
	h.streamChatContinue(w, r, user.ID, chatID, respondingCharID, others, up, userName, userDesc, msg, in)
}

// seedGroupGreetings collects a greeting from each participating
// character and persists them as attributed swipes on the chat's first
// assistant message. The visible one defaults to index 0 (first
// character's greeting); the user can swipe to read how each character
// opens the scene, then continue the chat from whichever resonates.
//
// Characters without any greeting (empty first_mes and empty
// alternate_greetings) are skipped — their slot isn't reserved.
//
// Best-effort: any failure is logged but doesn't fail chat creation.
// Returning early leaves the group chat empty (same as pre-M36 group
// behaviour) which is a valid, usable state.
func (h *Handler) seedGroupGreetings(ctx context.Context, chatID uuid.UUID, user *models.NestUser, characterIDs []uuid.UUID) {
	// Persona for {{user}} macro expansion. Same resolution as the
	// single-char path — default persona if one is set, else WuApi
	// first_name via user.DisplayName().
	personaName := user.DisplayName()
	if h.Personas != nil {
		if p, err := h.Personas.Default(ctx, user.ID); err == nil && p.Name != "" {
			personaName = p.Name
		}
	}
	userID := user.ID

	type greetingSwipe struct {
		content     string
		characterID uuid.UUID
	}
	pool := make([]greetingSwipe, 0, len(characterIDs))

	for _, cid := range characterIDs {
		ch, err := h.Characters.Get(ctx, userID, cid)
		if err != nil {
			slog.Warn("group greetings: load character", "err", err, "character_id", cid)
			continue
		}
		// Pick the best greeting: prefer first_mes, fall back to the
		// first non-empty alternate_greetings entry. Skip if neither
		// exists — the character just doesn't contribute an opener.
		macroCtx := PromptInput{Character: ch, UserName: personaName}
		greeting := strings.TrimSpace(ch.Data.FirstMes)
		if greeting == "" {
			for _, alt := range ch.Data.AlternateGreetings {
				if strings.TrimSpace(alt) != "" {
					greeting = alt
					break
				}
			}
		}
		if greeting == "" {
			continue
		}
		pool = append(pool, greetingSwipe{
			content:     SubstituteMacros(greeting, macroCtx),
			characterID: ch.ID,
		})
	}

	if len(pool) == 0 {
		// Nobody had a greeting — leave the chat empty, user sends first.
		return
	}

	// Flatten into parallel arrays for the repo call.
	texts := make([]string, len(pool))
	ids := make([]uuid.UUID, len(pool))
	for i, g := range pool {
		texts[i] = g.content
		ids[i] = g.characterID
	}

	firstID := pool[0].characterID
	if _, err := h.Repo.AppendMessageWithSwipesAttributed(
		ctx, chatID, RoleAssistant,
		texts[0], texts, ids, 0,
		&firstID,
		&MessageExtras{Model: "greeting"},
	); err != nil {
		slog.Warn("group greetings: append failed", "err", err, "chat_id", chatID)
	}
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
