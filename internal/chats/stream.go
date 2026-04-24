package chats

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/worldinfo"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// streamChatRegen re-streams an assistant turn without saving a new user
// message. Used by POST /chats/:id/regenerate after the previous assistant
// message has already been deleted.
//
// Shares all stages of streamChat except the "persist user message" step.
func (h *Handler) streamChatRegen(
	w http.ResponseWriter,
	r *http.Request,
	userID uuid.UUID,
	chatID uuid.UUID,
	charID *uuid.UUID,
	otherCharIDs []uuid.UUID,
	ups upstream,
	userName string,
	personaDesc string,
	in SendMessageInput,
) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	// Load speaker character (optional).
	var ch *characters.Character
	if charID != nil {
		if c, err := h.Characters.Get(ctx, userID, *charID); err == nil {
			ch = c
		}
	}

	// Load other participants for group-chat manifest + history prefix.
	others := h.loadOtherCharacters(ctx, userID, otherCharIDs)

	// Load history (already without the deleted assistant message).
	// Include hidden messages in the prompt — M38 silent-message
	// semantics: "hidden from UI, still in model context". Actual
	// deletion uses DeleteMessage, not hidden=true.
	history, err := h.Repo.ListMessages(ctx, chatID, true)
	if err == nil {
		// M38.4 memory — drop messages already covered by the rolling
		// auto-summary so the model doesn't see them twice. Summaries
		// are loaded just below and injected via PromptInput.Summaries.
		if auto, _ := h.Repo.GetAutoSummary(ctx, chatID); auto != nil {
			history = FilterHistoryForPrompt(history, auto)
		}
	}
	if err != nil {
		writeSSEError(w, flusher, "load_history", err)
		return
	}

	// Load attached lorebooks for this character, if any. Best-effort: a DB
	// hiccup here should not kill the regen — we just skip WI for this turn.
	worlds := loadAttachedWorlds(ctx, h, userID, charID)

	model := in.Model
	if model == "" {
		model = defaultModel
	}

	// Inline the pipeline: prompt → placeholder → upstream → pipe.
	// Load summaries (auto + manual + pinned) for memory injection.
	// Best-effort: on DB error we skip memory rather than fail the
	// generation — model falls back to raw history only.
	summaries, _ := h.Repo.ListSummaries(ctx, chatID)

	promptMsgs := Build(PromptInput{
		Character:            ch,
		OtherCharacters:      others,
		History:              history,
		UserName:             userName,
		UserDesc:             personaDesc,
		SystemPromptOverride: in.SystemPromptOverride,
		Worlds:               worlds,
		AuthorsNote:          in.AuthorsNote,
		Summaries:            summaries,
		Bundle:               in.Bundle,
	})
	up := make([]map[string]any, 0, len(promptMsgs))
	for _, m := range promptMsgs {
		up = append(up, map[string]any{"role": m.Role, "content": m.Content})
	}
	// M37: transform user messages with attachment URLs into multi-part
	// content when the model supports vision. No-op for text-only models
	// (URLs stay as-is in content string). Inlining is gated by preset's
	// image_inlining flag (defaults ON for vision models).
	up = ApplyMultimodal(up, DecideMultimodal(model, in.Bundle))

	placeholder, err := h.Repo.AppendMessageForCharacter(ctx, chatID, RoleAssistant, "", charID, &MessageExtras{Model: model})
	if err != nil {
		writeSSEError(w, flusher, "save_placeholder", err)
		return
	}
	writeSSE(w, flusher, "assistant_start", map[string]any{"id": placeholder.ID, "model": model, "character_id": charID})

	h.pipeStream(w, flusher, ctx, placeholder, model, ups, in, up)
}

// pipeStream runs the upstream call + SSE pass-through + persistence.
// Extracted so streamChat and streamChatRegen share the same hot loop.
//
// `ups` determines where the request actually goes: empty BaseURL →
// WuApi proxy (the default), non-empty BaseURL → direct BYOK call to
// the user's provider.
func (h *Handler) pipeStream(
	w http.ResponseWriter,
	flusher http.Flusher,
	ctx context.Context,
	placeholder *Message,
	model string,
	ups upstream,
	in SendMessageInput,
	up []map[string]any,
) {
	started := time.Now()

	req := wuapi.ChatCompletionRequest{
		Model:             model,
		Messages:          up,
		Temperature:       in.Temperature,
		TopP:              in.TopP,
		TopK:              in.TopK,
		MinP:              in.MinP,
		MaxTokens:         in.MaxTokens,
		FrequencyPenalty:  in.FrequencyPenalty,
		PresencePenalty:   in.PresencePenalty,
		RepetitionPenalty: in.RepetitionPenalty,
		Seed:              in.Seed,
		Stop:              in.Stop,
		Extra:             mergeReasoning(in.Overrides, in.ReasoningEnabled),
	}

	body, resp, err := h.openChatStream(ctx, ups, req)
	if err != nil {
		finalizeError(ctx, h, placeholder.ID, model, err)
		writeSSEError(w, flusher, "upstream_connect", err)
		return
	}
	defer body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(body, 4096))
		upErr := fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		finalizeError(ctx, h, placeholder.ID, model, upErr)
		writeSSEError(w, flusher, "upstream_status", upErr)
		return
	}

	var (
		accumulator          strings.Builder
		finishReason         string
		tokensIn, tokensOut  int
	)
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "[DONE]" {
			break
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			writeSSERaw(w, flusher, "raw", data)
			continue
		}
		for _, ch := range event.Choices {
			if ch.Delta.Content != "" {
				accumulator.WriteString(ch.Delta.Content)
				writeSSE(w, flusher, "token", map[string]any{"content": ch.Delta.Content})
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
		if event.Usage.PromptTokens > 0 {
			tokensIn = event.Usage.PromptTokens
		}
		if event.Usage.CompletionTokens > 0 {
			tokensOut = event.Usage.CompletionTokens
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("chats stream scanner", "err", err)
	}

	final := accumulator.String()
	cleanContent, reasoning := ExtractThinking(final)
	// M32: Run assistant-output regex scripts on the final content before
	// persisting. Typical use: strip HTML that jailbreak prompts request,
	// remove the unicode-invisible chars used to bypass filters, etc.
	// No-op when the bundle has no placement=2 scripts (including when
	// there's no bundle at all).
	cleanContent = ApplyRegexToAIOutput(in.Bundle, cleanContent)
	extras := &MessageExtras{
		Model:        model,
		Reasoning:    reasoning,
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
		LatencyMs:    int(time.Since(started).Milliseconds()),
		FinishReason: finishReason,
	}
	if err := h.Repo.UpdateMessageContent(ctx, placeholder.ID, cleanContent, extras); err != nil {
		slog.Error("update placeholder", "err", err, "id", placeholder.ID)
	}
	writeSSE(w, flusher, "done", map[string]any{
		"id":            placeholder.ID,
		"content":       cleanContent,
		"reasoning":     reasoning,
		"tokens_in":     tokensIn,
		"tokens_out":    tokensOut,
		"latency_ms":    extras.LatencyMs,
		"finish_reason": finishReason,
	})
}

// streamChat runs the full send-and-stream cycle for one user turn:
//
//  1. Save the user message.
//  2. Insert an empty placeholder assistant row (so the UI has an id).
//  3. Build the prompt from character + history.
//  4. Call WuApi with stream=true.
//  5. Pipe SSE events to the client; concurrently parse delta.content to
//     accumulate the assistant text.
//  6. On close, patch the placeholder with the final content and extras.
//
// The handler sends a final `event: done` with the saved message id so the
// client can replace its optimistic row with the persisted one.
func (h *Handler) streamChat(
	w http.ResponseWriter,
	r *http.Request,
	userID uuid.UUID,
	chatID uuid.UUID,
	charID *uuid.UUID,
	otherCharIDs []uuid.UUID,
	ups upstream,
	userName string,
	personaDesc string,
	in SendMessageInput,
) {
	ctx := r.Context()

	// SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx hint

	flusher, _ := w.(http.Flusher)

	// 1. Persist user message — unless the content is empty, in which
	// case we're in group-chat "continue" mode (advance the scene
	// without a user turn). The handler has already rejected empty
	// content for single-char chats, so reaching this branch implies
	// a group chat asking for the next speaker's line.
	if in.Content != "" {
		userMsg, err := h.Repo.AppendMessage(ctx, chatID, RoleUser, in.Content, &MessageExtras{
			Model: in.Model,
		})
		if err != nil {
			writeSSEError(w, flusher, "save_user_message", err)
			return
		}
		writeSSE(w, flusher, "user_message", userMsg)
	}

	// 2. Load speaker character (if any).
	var ch *characters.Character
	if charID != nil {
		c, err := h.Characters.Get(ctx, userID, *charID)
		// If the character was deleted after the chat was created, fall
		// through silently with no character — the system prompt becomes empty.
		if err == nil {
			ch = c
		}
	}

	// 2b. Load other participants for group-chat manifest + history prefix.
	others := h.loadOtherCharacters(ctx, userID, otherCharIDs)

	// 3. Load history (visible only).
	// Include hidden messages in the prompt — M38 silent-message
	// semantics: "hidden from UI, still in model context". Actual
	// deletion uses DeleteMessage, not hidden=true.
	history, err := h.Repo.ListMessages(ctx, chatID, true)
	if err == nil {
		// M38.4 memory — drop messages already covered by the rolling
		// auto-summary so the model doesn't see them twice. Summaries
		// are loaded just below and injected via PromptInput.Summaries.
		if auto, _ := h.Repo.GetAutoSummary(ctx, chatID); auto != nil {
			history = FilterHistoryForPrompt(history, auto)
		}
	}
	if err != nil {
		writeSSEError(w, flusher, "load_history", err)
		return
	}

	// 3b. Load attached lorebooks for the character.
	worlds := loadAttachedWorlds(ctx, h, userID, charID)

	// 4. Build prompt.
	// Load summaries (auto + manual + pinned) for memory injection.
	// Best-effort: on DB error we skip memory rather than fail the
	// generation — model falls back to raw history only.
	summaries, _ := h.Repo.ListSummaries(ctx, chatID)

	promptMsgs := Build(PromptInput{
		Character:            ch,
		OtherCharacters:      others,
		History:              history,
		UserName:             userName,
		UserDesc:             personaDesc,
		SystemPromptOverride: in.SystemPromptOverride,
		Worlds:               worlds,
		AuthorsNote:          in.AuthorsNote,
		Summaries:            summaries,
		Bundle:               in.Bundle,
	})

	// Convert to the loose map[string]any that wuapi expects.
	up := make([]map[string]any, 0, len(promptMsgs))
	for _, m := range promptMsgs {
		up = append(up, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	model := in.Model
	if model == "" {
		model = defaultModel
	}

	req := wuapi.ChatCompletionRequest{
		Model:             model,
		Messages:          up,
		Temperature:       in.Temperature,
		TopP:              in.TopP,
		TopK:              in.TopK,
		MinP:              in.MinP,
		MaxTokens:         in.MaxTokens,
		FrequencyPenalty:  in.FrequencyPenalty,
		PresencePenalty:   in.PresencePenalty,
		RepetitionPenalty: in.RepetitionPenalty,
		Seed:              in.Seed,
		Stop:              in.Stop,
		Extra:             mergeReasoning(in.Overrides, in.ReasoningEnabled),
	}

	// 5. Insert empty assistant placeholder with the model chosen and the
	//    responding character attribution (nil for single-char chats →
	//    column stays NULL and the UI falls back to chat.character_id).
	placeholder, err := h.Repo.AppendMessageForCharacter(ctx, chatID, RoleAssistant, "", charID, &MessageExtras{
		Model: model,
	})
	if err != nil {
		writeSSEError(w, flusher, "save_placeholder", err)
		return
	}
	writeSSE(w, flusher, "assistant_start", map[string]any{"id": placeholder.ID, "model": model, "character_id": charID})

	started := time.Now()

	// 6. Start upstream stream. Routes to WuApi proxy by default, or
	//    direct to a BYOK-provided URL when the chat is pinned to a BYOK key.
	body, resp, err := h.openChatStream(ctx, ups, req)
	if err != nil {
		finalizeError(ctx, h, placeholder.ID, model, err)
		writeSSEError(w, flusher, "upstream_connect", err)
		return
	}
	defer body.Close()

	if resp.StatusCode >= 400 {
		// Drain the body into an error string (capped) so the UI can display it.
		b, _ := io.ReadAll(io.LimitReader(body, 4096))
		upErr := fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		finalizeError(ctx, h, placeholder.ID, model, upErr)
		writeSSEError(w, flusher, "upstream_status", upErr)
		return
	}

	// 7. Scan SSE events, pass `token` events through, accumulate text.
	var (
		accumulator strings.Builder
		finishReason string
		tokensIn, tokensOut int
	)
	scanner := bufio.NewScanner(body)
	// Upstream lines can legitimately be large (whole message in one chunk).
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "[DONE]" {
			break
		}
		// Parse into a generic event to pull delta.content.
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// Forward the raw line anyway — some providers emit non-chunk events.
			writeSSERaw(w, flusher, "raw", data)
			continue
		}

		for _, ch := range event.Choices {
			if ch.Delta.Content != "" {
				accumulator.WriteString(ch.Delta.Content)
				writeSSE(w, flusher, "token", map[string]any{"content": ch.Delta.Content})
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
		if event.Usage.PromptTokens > 0 {
			tokensIn = event.Usage.PromptTokens
		}
		if event.Usage.CompletionTokens > 0 {
			tokensOut = event.Usage.CompletionTokens
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("chats stream scanner", "err", err)
	}

	// 8. Persist final content + metadata.
	// Split <think>...</think> reasoning blocks from user-visible content so
	// the UI can render them collapsibly. If the model didn't emit any, the
	// content is unchanged and reasoning is empty.
	final := accumulator.String()
	cleanContent, reasoning := ExtractThinking(final)
	// M32: Run assistant-output regex scripts on the final content before
	// persisting. Typical use: strip HTML that jailbreak prompts request,
	// remove the unicode-invisible chars used to bypass filters, etc.
	// No-op when the bundle has no placement=2 scripts (including when
	// there's no bundle at all).
	cleanContent = ApplyRegexToAIOutput(in.Bundle, cleanContent)

	extras := &MessageExtras{
		Model:        model,
		Reasoning:    reasoning,
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
		LatencyMs:    int(time.Since(started).Milliseconds()),
		FinishReason: finishReason,
	}
	if err := h.Repo.UpdateMessageContent(ctx, placeholder.ID, cleanContent, extras); err != nil {
		slog.Error("update placeholder", "err", err, "id", placeholder.ID)
	}

	writeSSE(w, flusher, "done", map[string]any{
		"id":            placeholder.ID,
		"content":       cleanContent,
		"reasoning":     reasoning,
		"tokens_in":     tokensIn,
		"tokens_out":    tokensOut,
		"latency_ms":    extras.LatencyMs,
		"finish_reason": finishReason,
	})
}

// finalizeError writes a minimal error record to the assistant placeholder so
// the UI can render "(generation failed)" without leaving it blank forever.
func finalizeError(ctx context.Context, h *Handler, id int64, model string, reason error) {
	_ = h.Repo.UpdateMessageContent(ctx, id, "", &MessageExtras{
		Model: model,
		Error: reason.Error(),
	})
}

const defaultModel = "wu-kitsune"

// --- SSE helpers ---

func writeSSE(w http.ResponseWriter, f http.Flusher, event string, data any) {
	var buf bytes.Buffer
	buf.WriteString("event: ")
	buf.WriteString(event)
	buf.WriteString("\ndata: ")
	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		fmt.Fprintf(&buf, `{"error":"encode: %s"}`, err)
	}
	buf.WriteString("\n")
	_, _ = w.Write(buf.Bytes())
	if f != nil {
		f.Flush()
	}
}

func writeSSERaw(w http.ResponseWriter, f http.Flusher, event string, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if f != nil {
		f.Flush()
	}
}

func writeSSEError(w http.ResponseWriter, f http.Flusher, kind string, err error) {
	writeSSE(w, f, "error", map[string]string{
		"kind":    kind,
		"message": err.Error(),
	})
}

// mergeReasoning folds the ReasoningEnabled flag into the upstream Extra map
// using shapes that the most popular reasoning-model APIs accept:
//   - Anthropic (thinking): thinking: { type: "enabled" }
//   - OpenRouter / generic: reasoning: { enabled: true }
//   - OpenAI o-series: reasoning_effort: "medium" (only on true)
// We only ADD keys — we don't clobber anything the caller explicitly set.
// Providers that don't recognise a key ignore it.
func mergeReasoning(extra map[string]any, enabled *bool) map[string]any {
	if enabled == nil {
		return extra
	}
	out := make(map[string]any, len(extra)+3)
	for k, v := range extra {
		out[k] = v
	}
	if *enabled {
		if _, ok := out["thinking"]; !ok {
			out["thinking"] = map[string]any{"type": "enabled"}
		}
		if _, ok := out["reasoning"]; !ok {
			out["reasoning"] = map[string]any{"enabled": true}
		}
		if _, ok := out["reasoning_effort"]; !ok {
			out["reasoning_effort"] = "medium"
		}
	} else {
		if _, ok := out["thinking"]; !ok {
			out["thinking"] = map[string]any{"type": "disabled"}
		}
		if _, ok := out["reasoning"]; !ok {
			out["reasoning"] = map[string]any{"enabled": false}
		}
	}
	return out
}

// streamChatSwipe runs a swipe: rebuilds the prompt from history up to but
// not including the target message, streams a fresh completion, and
// finalises the new content into both `nest_messages.content` and
// `swipes[swipe_id]`. The target message row stays put — swipes accumulate
// beside the original, navigable via SelectSwipe.
func (h *Handler) streamChatSwipe(
	w http.ResponseWriter,
	r *http.Request,
	userID uuid.UUID,
	chatID uuid.UUID,
	charID *uuid.UUID,
	otherCharIDs []uuid.UUID,
	ups upstream,
	userName string,
	personaDesc string,
	targetMessageID int64,
	newSwipeID int,
	in SendMessageInput,
) {
	ctx := r.Context()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	// Announce the target id + the new swipe_id so the client can flip its
	// pagination strip before the first token arrives.
	writeSSE(w, flusher, "swipe_start", map[string]any{
		"id":       targetMessageID,
		"swipe_id": newSwipeID,
	})

	// Load speaker character for the system prompt; not fatal if missing.
	var ch *characters.Character
	if charID != nil {
		if c, err := h.Characters.Get(ctx, userID, *charID); err == nil {
			ch = c
		}
	}

	// Load other participants for group-chat manifest + history prefix.
	others := h.loadOtherCharacters(ctx, userID, otherCharIDs)

	// Load history UP TO but NOT INCLUDING the target message. The swipe
	// replaces that message's content, so it must regenerate against the
	// same context it had originally.
	// Include hidden messages in prompt — silent-message semantics.
	all, err := h.Repo.ListMessages(ctx, chatID, true)
	if err != nil {
		writeSSEError(w, flusher, "load_history", err)
		return
	}
	history := make([]Message, 0, len(all))
	for _, m := range all {
		if m.ID >= targetMessageID {
			break
		}
		history = append(history, m)
	}

	worlds := loadAttachedWorlds(ctx, h, userID, charID)

	model := in.Model
	if model == "" {
		model = defaultModel
	}

	// Load summaries (auto + manual + pinned) for memory injection.
	// Best-effort: on DB error we skip memory rather than fail the
	// generation — model falls back to raw history only.
	summaries, _ := h.Repo.ListSummaries(ctx, chatID)

	promptMsgs := Build(PromptInput{
		Character:            ch,
		OtherCharacters:      others,
		History:              history,
		UserName:             userName,
		UserDesc:             personaDesc,
		SystemPromptOverride: in.SystemPromptOverride,
		Worlds:               worlds,
		AuthorsNote:          in.AuthorsNote,
		Summaries:            summaries,
		Bundle:               in.Bundle,
	})
	up := make([]map[string]any, 0, len(promptMsgs))
	for _, m := range promptMsgs {
		up = append(up, map[string]any{"role": m.Role, "content": m.Content})
	}
	// M37: transform user messages with attachment URLs into multi-part
	// content when the model supports vision. No-op for text-only models
	// (URLs stay as-is in content string). Inlining is gated by preset's
	// image_inlining flag (defaults ON for vision models).
	up = ApplyMultimodal(up, DecideMultimodal(model, in.Bundle))

	// The "placeholder" for pipeStream is the existing message row — we
	// already cleared its content and bumped swipe_id via BeginSwipe. Pass
	// a minimal Message with the id; pipeStream's UpdateMessageContent
	// writes by id.
	placeholder := &Message{ID: targetMessageID}

	h.pipeStreamSwipe(w, flusher, ctx, chatID, placeholder, model, ups, in, up)
}

// streamChatContinue extends an existing assistant message with more
// content streamed from the model. Unlike swipe (which replaces) this
// APPENDS — useful when the original response hit max_tokens or the
// user just wants more.
//
// Implementation notes:
//   - Prompt = normal history through the target message + an assistant-role
//     message carrying the target's current content as prefill. OpenAI /
//     Anthropic / DeepSeek all continue from that prefill when the last
//     message is assistant-role.
//   - Client sees `continue_start` instead of `assistant_start` so
//     MessageBubble knows to APPEND incoming token deltas rather than
//     overwrite the displayed content.
//   - Final DB write stores existing + streamed content (full final
//     string) so the row is self-consistent even if the client
//     disconnected mid-stream.
func (h *Handler) streamChatContinue(
	w http.ResponseWriter,
	r *http.Request,
	userID uuid.UUID,
	chatID uuid.UUID,
	charID *uuid.UUID,
	otherCharIDs []uuid.UUID,
	ups upstream,
	userName string,
	personaDesc string,
	target *Message,
	in SendMessageInput,
) {
	ctx := r.Context()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	// Let the client know this is a continue — it should append tokens
	// to an existing message, not create a new row.
	writeSSE(w, flusher, "continue_start", map[string]any{
		"id":           target.ID,
		"existing_len": len(target.Content),
	})

	// Speaker character (optional — deleted-character-fallback same as others).
	var ch *characters.Character
	if charID != nil {
		if c, err := h.Characters.Get(ctx, userID, *charID); err == nil {
			ch = c
		}
	}
	others := h.loadOtherCharacters(ctx, userID, otherCharIDs)

	// History UP TO but NOT INCLUDING the target — we'll inject the
	// target's content ourselves as a prefill, so including it in
	// history would duplicate it.
	// Include hidden messages in prompt — silent-message semantics.
	all, err := h.Repo.ListMessages(ctx, chatID, true)
	if err != nil {
		writeSSEError(w, flusher, "load_history", err)
		return
	}
	history := make([]Message, 0, len(all))
	for _, m := range all {
		if m.ID >= target.ID {
			break
		}
		history = append(history, m)
	}

	worlds := loadAttachedWorlds(ctx, h, userID, charID)

	model := in.Model
	if model == "" {
		model = defaultModel
	}

	// Load summaries (auto + manual + pinned) for memory injection.
	// Best-effort: on DB error we skip memory rather than fail the
	// generation — model falls back to raw history only.
	summaries, _ := h.Repo.ListSummaries(ctx, chatID)

	promptMsgs := Build(PromptInput{
		Character:            ch,
		OtherCharacters:      others,
		History:              history,
		UserName:             userName,
		UserDesc:             personaDesc,
		SystemPromptOverride: in.SystemPromptOverride,
		Worlds:               worlds,
		AuthorsNote:          in.AuthorsNote,
		Summaries:            summaries,
		Bundle:               in.Bundle,
	})
	up := make([]map[string]any, 0, len(promptMsgs)+1)
	for _, m := range promptMsgs {
		up = append(up, map[string]any{"role": m.Role, "content": m.Content})
	}
	up = ApplyMultimodal(up, DecideMultimodal(model, in.Bundle))

	// Assistant prefill — the heart of the continue trick. Providers
	// that understand this ({OpenAI, Anthropic, DeepSeek, Gemini,
	// OpenRouter-proxied to any of the above}) resume from exactly
	// this text. Providers that don't will generate from scratch
	// (degrades to regenerate-style; the append merge below still
	// works).
	//
	// If the user's active bundle has `continue_prefill` set (a
	// "Claude style: continue from the last word" convention), we
	// append that verbatim AFTER the current content. That's where
	// ST's continue_prefill preset field feeds in.
	continuePrefill := target.Content
	// ST conventions:
	//   - ContinueNudgePrompt: short nudge inserted as a system hint before
	//     the prefill. Rarely changes behaviour much; kept optional.
	//   - ContinuePostfix: text appended to the prefill itself (some users
	//     like "..." or a newline to give the model a literal starting point).
	if in.Bundle != nil && in.Bundle.ContinuePostfix != "" {
		continuePrefill = target.Content + in.Bundle.ContinuePostfix
	}
	// In group chats the history rows were prefixed "Alice: ..." —
	// do the same for the prefill so the model sees consistent
	// attribution and continues as the right speaker.
	if len(others) > 0 && charID != nil {
		for _, c := range append([]*characters.Character{ch}, others...) {
			if c != nil && c.ID == *charID && c.Name != "" {
				continuePrefill = c.Name + ": " + continuePrefill
				break
			}
		}
	}
	up = append(up, map[string]any{
		"role":    "assistant",
		"content": continuePrefill,
	})

	h.pipeStreamContinue(w, flusher, ctx, chatID, target, model, ups, in, up)
}

// pipeStreamContinue is the continue-aware twin of pipeStream: tokens
// are appended to the target row's content (not replacing), and the
// final content saved is existing + streamed.
func (h *Handler) pipeStreamContinue(
	w http.ResponseWriter,
	flusher http.Flusher,
	ctx context.Context,
	chatID uuid.UUID,
	target *Message,
	model string,
	ups upstream,
	in SendMessageInput,
	up []map[string]any,
) {
	started := time.Now()

	req := wuapi.ChatCompletionRequest{
		Model:             model,
		Messages:          up,
		Temperature:       in.Temperature,
		TopP:              in.TopP,
		TopK:              in.TopK,
		MinP:              in.MinP,
		MaxTokens:         in.MaxTokens,
		FrequencyPenalty:  in.FrequencyPenalty,
		PresencePenalty:   in.PresencePenalty,
		RepetitionPenalty: in.RepetitionPenalty,
		Seed:              in.Seed,
		Stop:              in.Stop,
		Extra:             mergeReasoning(in.Overrides, in.ReasoningEnabled),
	}

	body, resp, err := h.openChatStream(ctx, ups, req)
	if err != nil {
		writeSSEError(w, flusher, "upstream_connect", err)
		return
	}
	defer body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(body, 4096))
		writeSSEError(w, flusher, "upstream_status",
			fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(b))))
		return
	}

	var (
		accumulator         strings.Builder
		finishReason        string
		tokensIn, tokensOut int
	)
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "[DONE]" {
			break
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			writeSSERaw(w, flusher, "raw", data)
			continue
		}
		for _, ch := range event.Choices {
			if ch.Delta.Content != "" {
				accumulator.WriteString(ch.Delta.Content)
				writeSSE(w, flusher, "token", map[string]any{"content": ch.Delta.Content})
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
		if event.Usage.PromptTokens > 0 {
			tokensIn = event.Usage.PromptTokens
		}
		if event.Usage.CompletionTokens > 0 {
			tokensOut = event.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("chats continue stream scanner", "err", err)
	}

	streamed := accumulator.String()
	// Regex transforms on the streamed portion only (existing content
	// was already transformed when originally persisted).
	cleanStreamed, newReasoning := ExtractThinking(streamed)
	cleanStreamed = ApplyRegexToAIOutput(in.Bundle, cleanStreamed)
	combined := target.Content + cleanStreamed

	extras := &MessageExtras{
		Model:        model,
		Reasoning:    newReasoning,
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
		LatencyMs:    int(time.Since(started).Milliseconds()),
		FinishReason: finishReason,
	}
	// If the target message already had a reasoning block from the
	// original generation, concatenate rather than replacing.
	var oldExtras MessageExtras
	if len(target.Extras) > 0 {
		_ = json.Unmarshal(target.Extras, &oldExtras)
	}
	if oldExtras.Reasoning != "" && newReasoning != "" {
		extras.Reasoning = oldExtras.Reasoning + "\n\n---\n\n" + newReasoning
	} else if oldExtras.Reasoning != "" {
		extras.Reasoning = oldExtras.Reasoning
	}
	if err := h.Repo.UpdateMessageContent(ctx, target.ID, combined, extras); err != nil {
		slog.Error("continue: save content", "err", err, "id", target.ID)
	}

	writeSSE(w, flusher, "done", map[string]any{
		"id":            target.ID,
		"content":       combined,
		"finish_reason": finishReason,
		"tokens_in":     tokensIn,
		"tokens_out":    tokensOut,
	})
}

// pipeStreamSwipe is the swipe-aware twin of pipeStream: on done it also
// mirrors the final content into swipes[swipe_id] via FinalizeSwipe so the
// stored array stays consistent with the visible content.
func (h *Handler) pipeStreamSwipe(
	w http.ResponseWriter,
	flusher http.Flusher,
	ctx context.Context,
	chatID uuid.UUID,
	placeholder *Message,
	model string,
	ups upstream,
	in SendMessageInput,
	up []map[string]any,
) {
	started := time.Now()

	req := wuapi.ChatCompletionRequest{
		Model:             model,
		Messages:          up,
		Temperature:       in.Temperature,
		TopP:              in.TopP,
		TopK:              in.TopK,
		MinP:              in.MinP,
		MaxTokens:         in.MaxTokens,
		FrequencyPenalty:  in.FrequencyPenalty,
		PresencePenalty:   in.PresencePenalty,
		RepetitionPenalty: in.RepetitionPenalty,
		Seed:              in.Seed,
		Stop:              in.Stop,
		Extra:             mergeReasoning(in.Overrides, in.ReasoningEnabled),
	}

	body, resp, err := h.openChatStream(ctx, ups, req)
	if err != nil {
		finalizeError(ctx, h, placeholder.ID, model, err)
		writeSSEError(w, flusher, "upstream_connect", err)
		return
	}
	defer body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(body, 4096))
		upErr := fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		finalizeError(ctx, h, placeholder.ID, model, upErr)
		writeSSEError(w, flusher, "upstream_status", upErr)
		return
	}

	var (
		accumulator         strings.Builder
		finishReason        string
		tokensIn, tokensOut int
	)
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "[DONE]" {
			break
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			writeSSERaw(w, flusher, "raw", data)
			continue
		}
		for _, ch := range event.Choices {
			if ch.Delta.Content != "" {
				accumulator.WriteString(ch.Delta.Content)
				writeSSE(w, flusher, "token", map[string]any{"content": ch.Delta.Content})
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
		if event.Usage.PromptTokens > 0 {
			tokensIn = event.Usage.PromptTokens
		}
		if event.Usage.CompletionTokens > 0 {
			tokensOut = event.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("chats swipe stream scanner", "err", err)
	}

	final := accumulator.String()
	cleanContent, reasoning := ExtractThinking(final)
	// M32: Run assistant-output regex scripts on the final content before
	// persisting. Typical use: strip HTML that jailbreak prompts request,
	// remove the unicode-invisible chars used to bypass filters, etc.
	// No-op when the bundle has no placement=2 scripts (including when
	// there's no bundle at all).
	cleanContent = ApplyRegexToAIOutput(in.Bundle, cleanContent)
	extras := &MessageExtras{
		Model:        model,
		Reasoning:    reasoning,
		TokensIn:     tokensIn,
		TokensOut:    tokensOut,
		LatencyMs:    int(time.Since(started).Milliseconds()),
		FinishReason: finishReason,
	}
	// Mirror into swipes[swipe_id] AND update extras.
	if err := h.Repo.FinalizeSwipe(ctx, chatID, placeholder.ID, cleanContent); err != nil {
		slog.Error("finalize swipe", "err", err, "id", placeholder.ID)
	}
	if err := h.Repo.UpdateMessageContent(ctx, placeholder.ID, cleanContent, extras); err != nil {
		slog.Error("update swipe extras", "err", err, "id", placeholder.ID)
	}

	writeSSE(w, flusher, "done", map[string]any{
		"id":            placeholder.ID,
		"content":       cleanContent,
		"reasoning":     reasoning,
		"tokens_in":     tokensIn,
		"tokens_out":    tokensOut,
		"latency_ms":    extras.LatencyMs,
		"finish_reason": finishReason,
	})
}

// loadAttachedWorlds fetches lorebooks attached to a character, tolerating
// loadOtherCharacters resolves the "other participants in this group"
// from their IDs into full Character objects, skipping any that are
// missing/deleted. Returns nil (not empty slice) when otherCharIDs is
// nil or empty so IsGroupChat() stays false in single-char chats.
//
// Failures are silent: a rogue id doesn't break generation, we just
// omit that character from the manifest.
func (h *Handler) loadOtherCharacters(ctx context.Context, userID uuid.UUID, otherCharIDs []uuid.UUID) []*characters.Character {
	if len(otherCharIDs) == 0 || h.Characters == nil {
		return nil
	}
	out := make([]*characters.Character, 0, len(otherCharIDs))
	for _, id := range otherCharIDs {
		c, err := h.Characters.Get(ctx, userID, id)
		if err != nil {
			slog.Warn("group chat: participant load failed", "err", err, "character_id", id)
			continue
		}
		out = append(out, c)
	}
	return out
}

// a nil character id (character-less chat) and any repository error (we log
// and skip — WI is an enhancement, not a blocker for generation).
func loadAttachedWorlds(ctx context.Context, h *Handler, userID uuid.UUID, charID *uuid.UUID) []*worldinfo.World {
	if charID == nil || h.Worlds == nil {
		return nil
	}
	books, err := h.Worlds.ListForCharacter(ctx, userID, *charID)
	if err != nil {
		slog.Warn("worldinfo: listForCharacter failed", "err", err, "character_id", *charID)
		return nil
	}
	if len(books) == 0 {
		return nil
	}
	out := make([]*worldinfo.World, len(books))
	for i := range books {
		out[i] = &books[i]
	}
	return out
}
