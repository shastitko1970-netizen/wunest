package converter

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/byok"
	"github.com/shastitko1970-netizen/wunest/internal/chats"
	"github.com/shastitko1970-netizen/wunest/internal/outboundproxy"
	"github.com/shastitko1970-netizen/wunest/internal/users"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// Limits hard-coded to match the spec:
//
//   - MaxInputBytes: 500 KB. Reasonable: median ST theme.json < 50 KB,
//     even aggressive bloated ones < 300 KB. 500 KB gives a comfortable
//     head-room without letting someone shove a whole DB dump as input.
//   - RateWindow / RateLimit: 3 conversions per rolling hour per user.
//   - JobTTL: 24h — conversations are scrap-paper, users can re-run.
const (
	MaxInputBytes = 500 * 1024
	RateLimit     = 3
	RateWindow    = 1 * time.Hour
	JobTTL        = 24 * time.Hour
)

// Handler is the HTTP front for the converter. Wired in router.go.
//
// Dependencies mirror what chats.Handler needs because we delegate the
// LLM call path to the same shape — BYOK (direct provider) or WuApi
// (proxy). We intentionally DO NOT call chats.Handler methods directly
// since those handle chat-specific concerns (prompt building,
// persistence, SSE pass-through); the converter rolls its own consumer.
type Handler struct {
	Repo      *Repository
	Users     *users.Resolver
	BYOK      *byok.Repository // nil allowed — falls back to WuApi-only
	WuApi     *wuapi.Client
	ProxyPool *outboundproxy.Pool
	Logger    *slog.Logger
}

// Register hooks the four endpoints. Caller supplies authRequired middleware.
func (h *Handler) Register(mux *http.ServeMux, authRequired func(http.Handler) http.Handler) {
	mux.Handle("POST /api/convert/theme", authRequired(http.HandlerFunc(h.convert)))
	mux.Handle("GET /api/convert/jobs", authRequired(http.HandlerFunc(h.listJobs)))
	mux.Handle("GET /api/convert/{id}", authRequired(http.HandlerFunc(h.getJob)))
	mux.Handle("GET /api/convert/{id}/download", authRequired(http.HandlerFunc(h.download)))
}

// ── POST /api/convert/theme ───────────────────────────────────────────
//
// Body (multipart/form-data):
//
//	file      — .json file (ST theme export). Max 500KB.
//	model     — string, model id (required).
//	byok_id   — UUID of a BYOK row to bill (optional; empty → WuApi pool).
//
// Response (200 OK, JSON):
//
//	{ job: Job, output_url: "/api/convert/{id}" }
//
// On rate-limit: 429 with {error:"rate_limited", resets_at: ISO8601}.
// On validation error: 400.
// On LLM failure: 502 with the model's error text.
func (h *Handler) convert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := auth.FromContext(ctx)
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userRow, err := h.Users.Resolve(ctx, session.WuApi.ID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// ── 1. Parse multipart form ─────────────────────────────────
	// Cap parse at MaxInputBytes+64KB so a 501KB upload doesn't OOM us
	// on a burst. Go's multipart reader streams up to the cap; beyond
	// that, ParseMultipartForm errors out cleanly.
	if err := r.ParseMultipartForm(int64(MaxInputBytes) + 64*1024); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	model := strings.TrimSpace(r.FormValue("model"))
	if model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	var byokID *uuid.UUID
	if s := strings.TrimSpace(r.FormValue("byok_id")); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			http.Error(w, "invalid byok_id", http.StatusBadRequest)
			return
		}
		byokID = &id
	}
	file, fh, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if fh.Size > MaxInputBytes {
		http.Error(w, fmt.Sprintf("file too large: max %d bytes", MaxInputBytes), http.StatusRequestEntityTooLarge)
		return
	}
	inputBytes, err := io.ReadAll(io.LimitReader(file, int64(MaxInputBytes)))
	if err != nil {
		http.Error(w, "read file", http.StatusBadRequest)
		return
	}
	if len(inputBytes) == 0 {
		http.Error(w, "empty file", http.StatusBadRequest)
		return
	}

	// ── 2. Rate limit ───────────────────────────────────────────
	n, err := h.Repo.RecentCount(ctx, userRow.ID, RateWindow)
	if err != nil {
		h.logWarn("recent count", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if n >= RateLimit {
		// Compute reset hint = earliest-pending-job.created + RateWindow.
		// Good-enough precision; client just shows a countdown.
		resetsAt := time.Now().Add(RateWindow).Format(time.RFC3339)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":      "rate_limited",
			"limit":      RateLimit,
			"window":     "1h",
			"resets_at":  resetsAt,
		})
		return
	}

	// ── 3. Resolve upstream (BYOK vs WuApi) ─────────────────────
	ups, err := h.resolveUpstream(ctx, userRow.ID, byokID, session.WuApi.APIKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// ── 4. Create pending job row ───────────────────────────────
	sum := sha256.Sum256(inputBytes)
	inputHash := hex.EncodeToString(sum[:])
	job, err := h.Repo.Create(ctx, CreateInput{
		UserID:      userRow.ID,
		Model:       model,
		BYOKID:      byokID,
		InputSHA256: inputHash,
		InputSize:   len(inputBytes),
	})
	if err != nil {
		h.logWarn("create job", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_ = h.Repo.MarkRunning(ctx, job.ID)

	// ── 5. Call the LLM ──────────────────────────────────────────
	// Detached context with a ceiling — user may close the tab, but we
	// still want to finish so the 24h output is usable for them later.
	// 180s is generous for a 500 KB theme + extensive rewrite; longer
	// would keep a pool slot busy unnecessarily.
	llmCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	systemPrompt, userPrompt := BuildPrompt(inputBytes)
	req := wuapi.ChatCompletionRequest{
		Model: model,
		Messages: []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		// No explicit max_tokens — per spec ("не ограничиваем ничего по
		// инпут/оутпут"). Providers will cap at their own model-level
		// ceiling (Anthropic 8K-64K, OpenAI 16K-128K depending on model).
	}

	content, usage, err := h.callLLM(llmCtx, ups, req)
	if err != nil {
		_ = h.Repo.Finish(ctx, job.ID, FinishInput{
			Status:       StatusError,
			ErrorMessage: err.Error(),
		})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// ── 6. Parse model output → WuNest theme JSON ───────────────
	out, err := parseOutput(content)
	if err != nil {
		_ = h.Repo.Finish(ctx, job.ID, FinishInput{
			Status:       StatusError,
			ErrorMessage: "model output not valid JSON: " + err.Error(),
			TokensIn:     usage.In,
			TokensOut:    usage.Out,
		})
		http.Error(w, "converter: model returned non-JSON — try a better model", http.StatusBadGateway)
		return
	}
	// Normalise — re-marshal so we store compact JSON regardless of
	// how the model formatted it.
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(out)); err != nil {
		compact.Write(out) // non-fatal — fall back to raw
	}

	// ── 7. Finish row → 200 response ────────────────────────────
	if err := h.Repo.Finish(ctx, job.ID, FinishInput{
		Status:     StatusDone,
		OutputJSON: compact.Bytes(),
		TokensIn:   usage.In,
		TokensOut:  usage.Out,
	}); err != nil {
		h.logWarn("finish job", err)
	}
	// Re-fetch so created_at/finished_at/etc. are server-canonical.
	fresh, err := h.Repo.Get(ctx, job.ID)
	if err != nil {
		// Degenerate: row was reaped between finish + fetch. Build a
		// synthesised response using what we know.
		fresh = &Job{
			ID:        job.ID,
			UserID:    userRow.ID,
			Status:    StatusDone,
			Model:     model,
			BYOKID:    byokID,
			TokensIn:  usage.In,
			TokensOut: usage.Out,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job":           fresh,
		"output":        json.RawMessage(compact.Bytes()),
		"output_url":    "/api/convert/" + job.ID.String(),
		"download_url":  "/api/convert/" + job.ID.String() + "/download",
	})
}

// ── GET /api/convert/jobs ─────────────────────────────────────────────
// List user's recent jobs (last 24h) so the UI can show a history strip.
func (h *Handler) listJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := auth.FromContext(ctx)
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userRow, err := h.Users.Resolve(ctx, session.WuApi.ID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	items, err := h.Repo.ListForUser(ctx, userRow.ID, 20)
	if err != nil {
		h.logWarn("list jobs", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ── GET /api/convert/{id} ─────────────────────────────────────────────
// Owner-only fetch — returns the job metadata + output JSON. We keep
// this owner-gated (not shareable) because a random URL is a weak
// security property; dedicated share would be a separate endpoint.
func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := auth.FromContext(ctx)
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userRow, err := h.Users.Resolve(ctx, session.WuApi.ID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	job, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found or expired", http.StatusGone)
			return
		}
		h.logWarn("get job", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if job.UserID != userRow.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job":    job,
		"output": json.RawMessage(job.OutputJSON),
	})
}

// ── GET /api/convert/{id}/download ────────────────────────────────────
// Sends the output as a .json file with Content-Disposition so the
// browser triggers a download dialog.
func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := auth.FromContext(ctx)
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userRow, err := h.Users.Resolve(ctx, session.WuApi.ID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	job, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found or expired", http.StatusGone)
			return
		}
		h.logWarn("get job for download", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if job.UserID != userRow.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if job.Status != StatusDone || len(job.OutputJSON) == 0 {
		http.Error(w, "job not completed", http.StatusConflict)
		return
	}
	// Filename from theme name if we can pull it — else a generic fallback.
	filename := "wunest-theme-" + id.String()[:8] + ".json"
	var probe struct{ Name string `json:"name"` }
	if err := json.Unmarshal(job.OutputJSON, &probe); err == nil && probe.Name != "" {
		safe := sanitiseFilename(probe.Name)
		if safe != "" {
			filename = safe + ".json"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write(job.OutputJSON)
}

// ── helpers ────────────────────────────────────────────────────────────

// resolveUpstream picks (APIKey, BaseURL, Provider) for the LLM call.
// Returns err if user asked for a BYOK id that doesn't belong to them
// or can't be decrypted.
type callUpstream struct {
	APIKey   string
	BaseURL  string // empty → WuApi proxy
	Provider string
}

func (h *Handler) resolveUpstream(ctx context.Context, userID uuid.UUID, byokID *uuid.UUID, wuapiKey string) (callUpstream, error) {
	if byokID == nil || h.BYOK == nil {
		return callUpstream{APIKey: wuapiKey}, nil
	}
	rev, err := h.BYOK.Reveal(ctx, userID, *byokID)
	if err != nil {
		// Return explicit error rather than silent WuApi fallback — the
		// user asked for a specific BYOK, hiding the failure would bill
		// WuApi quota without their consent.
		return callUpstream{}, fmt.Errorf("byok: %w", err)
	}
	return callUpstream{APIKey: rev.Key, BaseURL: rev.BaseURL, Provider: rev.Provider}, nil
}

// callLLM runs one non-streamed chat completion by consuming the SSE
// response ourselves. Returns accumulated `content` and usage counters.
//
// We don't return tokens via SSE to the user (the conversion is a
// single JSON blob, no need for progressive UI); the stream is only an
// implementation detail of the upstream API.
type usageCounts struct{ In, Out int }

func (h *Handler) callLLM(ctx context.Context, ups callUpstream, req wuapi.ChatCompletionRequest) (string, usageCounts, error) {
	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &wuapi.StreamOptions{IncludeUsage: true}
	}

	var (
		body io.ReadCloser
		resp *http.Response
		err  error
	)
	if ups.BaseURL == "" {
		body, resp, err = h.WuApi.ChatCompletionsStream(ctx, ups.APIKey, req)
	} else {
		// Direct BYOK call via the proxy pool. We inline the minimal
		// HTTP plumbing to avoid exporting chats.directChatStream.
		body, resp, err = h.directCall(ctx, ups, req)
	}
	if err != nil {
		return "", usageCounts{}, fmt.Errorf("upstream connect: %w", err)
	}
	defer body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(body, 4096))
		return "", usageCounts{}, fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var (
		acc strings.Builder
		u   usageCounts
	)
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
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
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		for _, ch := range ev.Choices {
			if ch.Delta.Content != "" {
				acc.WriteString(ch.Delta.Content)
			}
		}
		if ev.Usage.PromptTokens > 0 {
			u.In = ev.Usage.PromptTokens
		}
		if ev.Usage.CompletionTokens > 0 {
			u.Out = ev.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		h.logWarn("scanner", err)
	}
	text := strings.TrimSpace(acc.String())
	if text == "" {
		return "", u, errors.New("model returned empty response")
	}
	return text, u, nil
}

// directCall is a thinned-down BYOK direct HTTP call — uses the same
// PrepareRequestForProvider + DirectCallHeaders helpers as the chat
// stream, but kept local so the converter package doesn't pull the
// entire `chats` dependency graph into its own build.
func (h *Handler) directCall(ctx context.Context, ups callUpstream, req wuapi.ChatCompletionRequest) (io.ReadCloser, *http.Response, error) {
	req = chats.PrepareRequestForProvider(ups.Provider, req)
	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &wuapi.StreamOptions{IncludeUsage: true}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal: %w", err)
	}
	url := strings.TrimRight(ups.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("User-Agent", "WuNest-Converter/0.1 (+https://nest.wusphere.ru)")
	for k, v := range chats.DirectCallHeaders(ups.Provider, ups.APIKey) {
		httpReq.Header.Set(k, v)
	}
	client := &http.Client{Transport: h.ProxyPool.Transport()}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("do: %w", err)
	}
	return resp.Body, resp, nil
}

func (h *Handler) logWarn(msg string, err error) {
	if h.Logger != nil {
		h.Logger.Warn("converter: "+msg, "err", err)
	} else {
		slog.Warn("converter: "+msg, "err", err)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// sanitiseFilename — strip anything that's not [A-Za-z0-9_.-] so the
// resulting filename is safe across OSes. Truncate to 48 chars.
func sanitiseFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.' || r == ' ':
			b.WriteRune('_')
		}
		if b.Len() >= 48 {
			break
		}
	}
	return strings.Trim(b.String(), "_.")
}
