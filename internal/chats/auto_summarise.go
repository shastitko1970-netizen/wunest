package chats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
)

// M44 — auto-summary fire hook.
//
// After each successful stream `done` event the pipeStream caller
// invokes maybeFireAutoSummarise with the chat, the user and the
// assistant message's tokens_in. If the chat opted in (chat_metadata.
// auto_summarise.enabled == true) AND tokens_in >= threshold_tokens,
// we fire a SummariseChat in a background goroutine, using the model
// (and optional BYOK key) the user picked.
//
// Design choices:
//
//   - **Per-chat mutex**: one background summarise per chat at a
//     time. Repeat triggers from a rapid-fire session are dropped
//     silently — the one already running will catch up to the new
//     tail in its own turn.
//   - **Retry once + silent fail**: if the first attempt errors
//     (upstream 500, rate-limit, network blip), we back off 5 s and
//     retry. Persistent failure just logs to slog. The user opted in;
//     we don't spam them with "auto-summary failed" toasts, and we
//     DO NOT auto-disable the feature — they asked for it, we keep
//     trying on the next qualifying turn.
//   - **Detached context**: the browser may close the tab during a
//     long summarise call. We use context.Background() with a 120 s
//     timeout so the summarise completes and saves even if the
//     triggering HTTP request is long gone.
//
// The helper is a method on Handler because it needs Handler.SummariseChat
// + Handler.BYOK + Handler.Repo access — reusing all the infrastructure
// the manual Summariser button already uses.

// autoSummariseLocks: one sync.Mutex per chatID, created on demand.
// Held for the lifetime of a single background summarise. TryLock
// semantics via LoadOrStore — if the key is already there, we skip
// and trust the in-flight run.
type chatMutexMap struct {
	m sync.Map // map[uuid.UUID]*sync.Mutex
}

func (c *chatMutexMap) tryLock(chatID uuid.UUID) (func(), bool) {
	// LoadOrStore returns the existing mutex if present. The second
	// return reports whether the value came from store (new) or load.
	mu := &sync.Mutex{}
	actual, loaded := c.m.LoadOrStore(chatID, mu)
	real := actual.(*sync.Mutex)
	if !real.TryLock() {
		return nil, false
	}
	_ = loaded
	return func() { real.Unlock() }, true
}

// autoSummariseState lives on Handler (see handler.go near field
// declarations). Initialised lazily the first time the hook fires.
var globalAutoSummariseLocks chatMutexMap

// maybeFireAutoSummarise inspects the chat's auto_summarise config and
// the most recent assistant message's prompt size, firing an async
// background SummariseChat when both conditions are met.
//
// Called after each successful `done` SSE event from every streaming
// path (initial send, regenerate, swipe, continue). Never blocks the
// caller — all work happens in a goroutine with its own context.
//
// The config read + the LLM call + the DB upsert all happen inside
// the goroutine, not here — caller passes just the cheap primitives.
func (h *Handler) maybeFireAutoSummarise(
	userID, chatID uuid.UUID,
	wuapiKey string,
	lastPromptTokens int,
	model string,
) {
	// Off-hot-path — copy the params and spin up a goroutine right
	// away. Heavy work (config read, mutex grab, LLM call) is async.
	go h.runAutoSummariseIfNeeded(userID, chatID, wuapiKey, lastPromptTokens, model)
}

// ecoSummaryThreshold is the input-token threshold at which we trigger
// auto-summary on `:lite` chats (M55.3). Picked to match the eco-mode
// 30k input cap with a comfortable buffer — the pipeline starts
// compressing well before the cap fires server-side.
const ecoSummaryThreshold = 4000

// isEcoModel returns true when the model id ends with `:lite`. The
// cheap suffix check stays in sync with WuApi's catalog without us
// having to refetch the catalog from this hot path.
func isEcoModel(model string) bool {
	return strings.HasSuffix(model, ":lite")
}

// fireAutoSummariseFromSSE — convenience wrapper called by the four
// stream paths (pipeStream, pipeSwipe, pipeContinue, regenerate pipe).
// Pulls userID + wuapiKey from the session on the context. When the
// session is missing (shouldn't happen for an authenticated SSE but
// possible during teardown), no-op.
//
// `model` is the model id used for the request that just finished
// streaming. M55.3 — when it's a `:lite` (eco-mode) variant, the
// summary trigger threshold is clamped down to 4000 tokens so the
// pipeline keeps eco chats compact without forcing the user to
// reconfigure their per-chat auto-summary setting.
func (h *Handler) fireAutoSummariseFromSSE(
	ctx context.Context,
	chatID uuid.UUID,
	lastPromptTokens int,
	model string,
) {
	session := auth.FromContext(ctx)
	if session == nil {
		return
	}
	user, err := h.Users.Resolve(ctx, session.WuApi.ID)
	if err != nil {
		return
	}
	h.maybeFireAutoSummarise(user.ID, chatID, session.WuApi.APIKey, lastPromptTokens, model)
}

func (h *Handler) runAutoSummariseIfNeeded(
	userID, chatID uuid.UUID,
	wuapiKey string,
	lastPromptTokens int,
	model string,
) {
	// Fresh context — triggering HTTP call may have finished already.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	chat, err := h.Repo.GetChat(ctx, userID, chatID)
	if err != nil {
		// Chat deleted between the turn ending and the goroutine
		// waking up — nothing to do, not an error worth logging.
		return
	}
	cfg := readAutoSummarise(chat.Metadata)
	if cfg == nil || !cfg.Enabled {
		return
	}
	// M55.3 — eco-mode threshold clamp. When the active model is a
	// `:lite` variant, force the trigger threshold down to 4k so the
	// pipeline keeps the input small enough to fit eco-mode's 30k
	// server-side cap. Users can still set their own threshold lower
	// than 4k via the chat-settings drawer; we just refuse to go
	// HIGHER on eco chats.
	threshold := cfg.ThresholdTokens
	if isEcoModel(model) && threshold > ecoSummaryThreshold {
		threshold = ecoSummaryThreshold
	}
	// Zero / negative threshold is treated as "every turn" — respects
	// user intent, even if unusual.
	if lastPromptTokens < threshold {
		return
	}

	// Acquire the per-chat lock. If someone else is already running
	// a summarise for this chat, skip — they'll cover the tail.
	unlock, ok := globalAutoSummariseLocks.tryLock(chatID)
	if !ok {
		return
	}
	defer unlock()

	// Delegate to the same pipeline the manual Summariser uses.
	// Retry once on first-attempt error; silent log on permanent fail.
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				lastErr = ctx.Err()
				break
			case <-time.After(5 * time.Second):
			}
		}
		err := h.runSummarisePipeline(ctx, chat, userID, wuapiKey, cfg)
		if err == nil {
			return
		}
		lastErr = err
	}
	slog.Warn("auto-summarise: failed after retry",
		"err", lastErr, "chat_id", chatID, "user_id", userID)
}

// runSummarisePipeline: load history + existing summaries, pick bounds
// (same keepRecent logic as manual path), build SummariseInput, call
// SummariseChat, upsert the auto summary. Mirrors handler.go's
// `summarize` HTTP handler almost 1:1; extracted because the async
// path can't reuse the HTTP handler's wiring directly (no ResponseWriter,
// fresh context, different error surfacing).
func (h *Handler) runSummarisePipeline(
	ctx context.Context,
	chat *Chat,
	userID uuid.UUID,
	wuapiKey string,
	cfg *AutoSummariseConfig,
) error {
	history, err := h.Repo.ListMessages(ctx, chat.ID, true)
	if err != nil {
		return err
	}
	existing, _ := h.Repo.GetAutoSummary(ctx, chat.ID)
	var existingCovered int64
	previousSummary := ""
	if existing != nil {
		previousSummary = existing.Content
		if existing.CoveredThroughMessageID != nil {
			existingCovered = *existing.CoveredThroughMessageID
		}
	}
	// PickSummariserBounds returns (toFold, _keepIdx). The second value is
	// an index into history used by the manual handler's callers that
	// want to know where raw-history starts; we don't need it here — the
	// SummariseChat result itself carries CoveredThroughMessageID set
	// from the last folded message's id.
	// Auto-trigger — honor keepRecentMessages window (short-term memory).
	toFold, _ := PickSummariserBounds(history, existingCovered, false)
	if len(toFold) == 0 {
		// Threshold hit but nothing new to fold — the last keepRecent
		// messages dominated tokens. No-op; next qualifying turn will
		// try again.
		return nil
	}

	// Upstream routing — config's BYOK override wins, else chat's pinned
	// BYOK, else WuApi pool. Lets user pin cheap-model Gemini Flash for
	// summaries while their main chat runs on Claude Sonnet.
	ups := h.pickAutoSummariseUpstream(ctx, userID, chat, cfg, wuapiKey)
	if ups.APIKey == "" {
		return errAutoMissingAPIKey
	}

	// Group-chat speaker-name map — same inline style as the manual
	// handler (line ~747). Duplicated rather than extracted because
	// the handler's loop has slightly different error handling.
	speakerNames := map[string]string{}
	if chat.IsGroupChat && h.Characters != nil {
		for _, cid := range chat.CharacterIDs {
			if c, err := h.Characters.Get(ctx, userID, cid); err == nil {
				speakerNames[cid.String()] = c.Name
			}
		}
	}

	res, err := h.SummariseChat(ctx, SummariseInput{
		ChatID:                   chat.ID.String(),
		Model:                    cfg.Model,
		PreviousSummary:          previousSummary,
		Messages:                 toFold,
		PromptAPIKey:             ups.APIKey,
		PromptBaseURL:            ups.BaseURL,
		PromptProvider:           ups.Provider,
		SpeakerNameByCharacterID: speakerNames,
	})
	if err != nil {
		return err
	}
	if _, err := h.Repo.UpsertAutoSummary(
		ctx, chat.ID,
		res.Content, res.CoveredThroughMessageID,
		res.TokenCount, res.Model,
	); err != nil {
		return err
	}

	// M48 — user-visible notification. Append a system-role message to
	// the chat so the user sees что auto-summary fired без того чтобы
	// им открывать Memory drawer. Content stays short + human-friendly.
	// `extras.AutoSummariseMarker = true` позволит фронту потом точно
	// идентифицировать это как system-event (сейчас frontend рендерит
	// всё `role=system` одинаково — отдельный marker не обязателен,
	// зато будущий «dismiss this notice» мог бы его использовать).
	folded := len(toFold)
	notice := fmt.Sprintf("📝 Автосаммари: сжато %d %s в сводку (модель %s)",
		folded, pluralizeMessages(folded), res.Model)
	if _, mErr := h.Repo.AppendMessage(ctx, chat.ID, RoleSystem, notice, &MessageExtras{
		Model:              res.Model,
		AutoSummariseEvent: true,
	}); mErr != nil {
		// Non-fatal — summary uже сохранён. Просто логируем что
		// система-уведомление не получилось добавить.
		slog.Warn("auto-summarise: append system notice failed",
			"err", mErr, "chat_id", chat.ID)
	}
	return nil
}

// pluralizeMessages — простой ru-morphology helper для «1 сообщение /
// 2-4 сообщения / 5+ сообщений». Не тянем libs, inline-решение.
func pluralizeMessages(n int) string {
	// Убираем знак.
	if n < 0 {
		n = -n
	}
	// 11-14 — всегда «сообщений».
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return "сообщений"
	}
	switch n % 10 {
	case 1:
		return "сообщение"
	case 2, 3, 4:
		return "сообщения"
	default:
		return "сообщений"
	}
}

// errAutoMissingAPIKey — cfg picked a BYOK that couldn't be revealed
// AND session has no WuApi key (e.g. expired). Logged once per run
// via the retry loop; user sees nothing.
var errAutoMissingAPIKey = errors.New("auto-summarise: no api key")

// pickAutoSummariseUpstream resolves which upstream (WuApi or BYOK)
// handles the async summarise call. Priority:
//  1. cfg.BYOKID explicit (user picked a key specifically for summaries)
//  2. chat's pinned BYOK (the one the current chat uses for generation)
//  3. WuApi with the user's session key
//
// The explicit cfg.BYOKID may belong to the same user but point to a
// different provider than the chat — e.g. Claude Sonnet for chat,
// Gemini Flash for cheap summaries. Intentional separation.
func (h *Handler) pickAutoSummariseUpstream(
	ctx context.Context,
	userID uuid.UUID,
	chat *Chat,
	cfg *AutoSummariseConfig,
	wuapiKey string,
) upstream {
	if cfg.BYOKID != nil && *cfg.BYOKID != uuid.Nil && h.BYOK != nil {
		if rev, err := h.BYOK.Reveal(ctx, userID, *cfg.BYOKID); err == nil {
			return upstream{APIKey: rev.Key, BaseURL: rev.BaseURL, Provider: rev.Provider}
		}
	}
	return h.resolveUpstream(ctx, userID, chat.Metadata, wuapiKey)
}
