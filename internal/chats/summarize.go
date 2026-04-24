package chats

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// M38.4 memory / summarisation.
//
// Strategy:
//   - One "rolling" auto-summary per chat covering everything up to
//     covered_through_message_id. Regen replaces in place.
//   - Keep the last ~keepRecent messages always-fresh in the prompt;
//     summarise everything before that.
//   - Prompt the summariser with { previous_summary, new_messages }
//     so it can extend the existing narrative rather than restart.
//
// The summariser doesn't need to be a frontier model — Gemini 2.5 Flash
// or GPT-3.5-class models do great summaries at a fraction of the cost.
// Default is `gemini-2.5-flash`; user can override per call.

// keepRecentMessages is how many recent messages stay raw in the
// prompt (not folded into the summary). Higher = better short-term
// coherence + more tokens per turn; lower = more aggressive
// compression. 30 balances cost + quality for typical RP flow.
const keepRecentMessages = 30

// defaultSummariserModel is used when the caller doesn't specify one.
// Gemini 2.5 Flash is the current sweet spot for WuApi-proxied users:
// cheap, 1M context (so old chats of any length fit), good RP
// comprehension. BYOK users fall through to a model on their active
// provider — caller picks.
const defaultSummariserModel = "gemini-2.5-flash"

// summariserSystemPrompt frames the task. Kept terse; the model gets
// the prior summary + messages verbatim.
const summariserSystemPrompt = `You are a chat summariser. You will receive:
1. (optional) A running summary of the conversation so far.
2. New messages that came after the previous summary.

Produce an UPDATED running summary that incorporates the new events
while preserving important facts from the prior summary (if any).

Rules:
- Keep it concise: 3-6 short paragraphs. Prioritise facts a character
  would reference in future turns (names, relationships, open
  questions, decisions made, emotional state, physical state,
  inventory).
- Write in third person ("Alice asked…", not "I asked…").
- Do NOT invent details. Only summarise what actually happened.
- Preserve specific numbers, names, and unique identifiers verbatim.
- Write in the same language as the chat (English / Russian / mixed
  — match the input).
- Output ONLY the summary text. No meta commentary, no headers, no
  "Here's the summary:" preamble.`

// SummariseInput packages everything needed to (re)generate a rolling
// summary for one chat.
type SummariseInput struct {
	ChatID                  string
	Model                   string // empty → defaultSummariserModel
	PreviousSummary         string // prior auto-summary content, may be empty
	Messages                []Message // messages to fold into the summary
	PromptAPIKey            string // wuapi/byok key to use for the call
	PromptBaseURL           string // empty → wuapi proxy; non-empty → direct
	PromptProvider          string // BYOK provider id; empty on wuapi path (used for per-provider request shaping)
	SpeakerNameByCharacterID map[string]string // for "Alice:" prefix on group assistant msgs
}

// SummariseResult carries the generated text + coverage marker so the
// caller can persist.
type SummariseResult struct {
	Content                 string
	Model                   string
	CoveredThroughMessageID int64
	TokenCount              int
}

// SummariseChat runs a blocking chat-completion call against the
// summariser and returns the generated text. Uses the same streaming
// upstream path as normal generation but drains to a single string.
//
// Blocks up to the caller's context — no internal timeout; handler
// should set one via context.WithTimeout.
func (h *Handler) SummariseChat(ctx context.Context, in SummariseInput) (*SummariseResult, error) {
	if len(in.Messages) == 0 {
		return nil, errors.New("summarise: no messages to fold in")
	}
	model := in.Model
	if model == "" {
		model = defaultSummariserModel
	}

	// Build the summariser user message: previous summary + new
	// messages rendered as "Role (Speaker): content".
	var body strings.Builder
	if strings.TrimSpace(in.PreviousSummary) != "" {
		body.WriteString("PREVIOUS SUMMARY:\n")
		body.WriteString(strings.TrimSpace(in.PreviousSummary))
		body.WriteString("\n\n")
	}
	body.WriteString("NEW MESSAGES:\n")
	for _, m := range in.Messages {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		speaker := string(m.Role)
		if m.Role == RoleAssistant && m.CharacterID != nil {
			if n, ok := in.SpeakerNameByCharacterID[m.CharacterID.String()]; ok && n != "" {
				speaker = "assistant (" + n + ")"
			}
		}
		body.WriteString(speaker)
		body.WriteString(": ")
		body.WriteString(content)
		body.WriteString("\n\n")
	}
	body.WriteString("UPDATED SUMMARY:")

	req := wuapi.ChatCompletionRequest{
		Model: model,
		Messages: []map[string]any{
			{"role": "system", "content": summariserSystemPrompt},
			{"role": "user", "content": body.String()},
		},
	}

	upStream := upstream{APIKey: in.PromptAPIKey, BaseURL: in.PromptBaseURL, Provider: in.PromptProvider}
	reader, resp, err := h.openChatStream(ctx, upStream, req)
	if err != nil {
		return nil, fmt.Errorf("summarise: upstream connect: %w", err)
	}
	defer reader.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(reader, 4096))
		return nil, fmt.Errorf("summarise: upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	// Drain SSE → concatenate token deltas.
	var (
		content strings.Builder
		tokenOut int
	)
	scanner := bufio.NewScanner(reader)
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
		var ev struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage struct {
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		for _, ch := range ev.Choices {
			if ch.Delta.Content != "" {
				content.WriteString(ch.Delta.Content)
			}
		}
		if ev.Usage.CompletionTokens > 0 {
			tokenOut = ev.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("summarise: scanner", "err", err)
	}
	text := strings.TrimSpace(content.String())
	if text == "" {
		return nil, errors.New("summarise: empty response from model")
	}

	// covered_through = id of the last message we folded in
	var lastID int64
	for _, m := range in.Messages {
		if m.ID > lastID {
			lastID = m.ID
		}
	}

	return &SummariseResult{
		Content:                 text,
		Model:                   model,
		CoveredThroughMessageID: lastID,
		TokenCount:              tokenOut,
	}, nil
}

// PickSummariserBounds decides which messages to fold into the summary
// given the current history + existing summary coverage.
//
// Rules:
//   - Keep the last keepRecentMessages raw (they're short-term memory
//     model always sees fresh).
//   - Fold everything older than that which isn't already covered by
//     the existing auto-summary.
//
// Returns the slice of messages to summarise (can be empty if there's
// nothing new to fold in) and the `keepFrom` index for the caller to
// re-filter history if needed.
func PickSummariserBounds(history []Message, existingCoveredThrough int64) (toSummarise []Message, keepFromIdx int) {
	if len(history) <= keepRecentMessages {
		return nil, 0
	}
	// Everything before the last keepRecent is eligible.
	cutoff := len(history) - keepRecentMessages
	keepFromIdx = cutoff
	for i := 0; i < cutoff; i++ {
		m := history[i]
		if existingCoveredThrough > 0 && m.ID <= existingCoveredThrough {
			continue // already in prior summary
		}
		if m.Role != RoleUser && m.Role != RoleAssistant {
			continue
		}
		toSummarise = append(toSummarise, m)
	}
	return toSummarise, keepFromIdx
}

// FilterHistoryForPrompt drops messages covered by the auto summary
// so the model doesn't see them twice. Called right before prompt
// build — handler loads full history, passes through this filter,
// passes filtered slice into PromptInput.History.
//
// No-op when summary is nil or coveredThrough is 0.
func FilterHistoryForPrompt(history []Message, auto *Summary) []Message {
	if auto == nil || auto.CoveredThroughMessageID == nil {
		return history
	}
	covered := *auto.CoveredThroughMessageID
	out := make([]Message, 0, len(history))
	for _, m := range history {
		if m.ID <= covered {
			continue
		}
		out = append(out, m)
	}
	return out
}
