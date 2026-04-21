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
	apiKey string,
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

	// Load character (optional).
	var ch *characters.Character
	if charID != nil {
		if c, err := h.Characters.Get(ctx, userID, *charID); err == nil {
			ch = c
		}
	}

	// Load history (already without the deleted assistant message).
	history, err := h.Repo.ListMessages(ctx, chatID, false)
	if err != nil {
		writeSSEError(w, flusher, "load_history", err)
		return
	}

	model := in.Model
	if model == "" {
		model = defaultModel
	}

	// Inline the pipeline: prompt → placeholder → upstream → pipe.
	promptMsgs := Build(PromptInput{
		Character: ch,
		History:   history,
		UserName:  userName,
		UserDesc:  personaDesc,
	})
	up := make([]map[string]any, 0, len(promptMsgs))
	for _, m := range promptMsgs {
		up = append(up, map[string]any{"role": m.Role, "content": m.Content})
	}

	placeholder, err := h.Repo.AppendMessage(ctx, chatID, RoleAssistant, "", &MessageExtras{Model: model})
	if err != nil {
		writeSSEError(w, flusher, "save_placeholder", err)
		return
	}
	writeSSE(w, flusher, "assistant_start", map[string]any{"id": placeholder.ID, "model": model})

	h.pipeStream(w, flusher, ctx, placeholder, model, apiKey, in, up)
}

// pipeStream runs the upstream WuApi call + SSE pass-through + persistence.
// Extracted so streamChat and streamChatRegen share the same hot loop.
func (h *Handler) pipeStream(
	w http.ResponseWriter,
	flusher http.Flusher,
	ctx context.Context,
	placeholder *Message,
	model string,
	apiKey string,
	in SendMessageInput,
	up []map[string]any,
) {
	started := time.Now()

	req := wuapi.ChatCompletionRequest{
		Model:       model,
		Messages:    up,
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
		Extra:       in.Overrides,
	}

	body, resp, err := h.WuApi.ChatCompletionsStream(ctx, apiKey, req)
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
	apiKey string,
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

	// 1. Persist user message.
	userMsg, err := h.Repo.AppendMessage(ctx, chatID, RoleUser, in.Content, &MessageExtras{
		Model: in.Model,
	})
	if err != nil {
		writeSSEError(w, flusher, "save_user_message", err)
		return
	}
	writeSSE(w, flusher, "user_message", userMsg)

	// 2. Load character (if any).
	var ch *characters.Character
	if charID != nil {
		c, err := h.Characters.Get(ctx, userID, *charID)
		// If the character was deleted after the chat was created, fall
		// through silently with no character — the system prompt becomes empty.
		if err == nil {
			ch = c
		}
	}

	// 3. Load history (visible only).
	history, err := h.Repo.ListMessages(ctx, chatID, false)
	if err != nil {
		writeSSEError(w, flusher, "load_history", err)
		return
	}

	// 4. Build prompt.
	promptMsgs := Build(PromptInput{
		Character: ch,
		History:   history,
		UserName:  userName,
		UserDesc:  personaDesc,
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
		Model:       model,
		Messages:    up,
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
		Extra:       in.Overrides,
	}

	// 5. Insert empty assistant placeholder with the model chosen.
	placeholder, err := h.Repo.AppendMessage(ctx, chatID, RoleAssistant, "", &MessageExtras{
		Model: model,
	})
	if err != nil {
		writeSSEError(w, flusher, "save_placeholder", err)
		return
	}
	writeSSE(w, flusher, "assistant_start", map[string]any{"id": placeholder.ID, "model": model})

	started := time.Now()

	// 6. Start upstream stream.
	body, resp, err := h.WuApi.ChatCompletionsStream(ctx, apiKey, req)
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
