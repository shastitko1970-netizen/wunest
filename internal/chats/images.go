package chats

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Image generation via OpenRouter.
//
// OpenRouter exposes image-gen models behind its standard
// /api/v1/chat/completions endpoint — you pass `modalities:["image"]`
// and get back `{choices[].message.images[].image_url.url}` with
// either a data URL (base64) or a CDN link. That way we don't need
// a separate provider integration — reuse the user's OpenRouter BYOK
// key that already goes through direct-routing infrastructure.
//
// Models that work today:
//   - google/gemini-2.5-flash-image  (fast, cheap)
//   - google/gemini-2.5-flash-image-preview
//   - black-forest-labs/flux-1.1-pro (higher quality, slower)
//
// Response can be a data: URL (when the provider returns base64) — we
// decode + upload to MinIO so the chat attachment has a stable URL
// instead of a 2MB inline blob in the message content.

// ImageGenerateRequest is what the client POSTs to /api/images/generate.
type ImageGenerateRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"` // defaults to defaultImageModel
	// ChatID, when set, attributes the generation to a chat for audit
	// + lets us persist the resulting URL directly to that chat's
	// attachments. Optional — bare-generation flow (from a future
	// dedicated page) would leave this empty.
	ChatID string `json:"chat_id,omitempty"`
}

// ImageGenerateResponse carries the resulting image URL back. The URL
// is always a WuNest nest-attachments URL — we re-host OpenRouter
// data URLs through MinIO so the chat message doesn't balloon.
type ImageGenerateResponse struct {
	URL   string `json:"url"`
	Model string `json:"model"`
}

const defaultImageModel = "google/gemini-2.5-flash-image"

// GenerateImage is the handler for POST /api/images/generate.
//
// Gated on BYOK OpenRouter key: image-gen is billed out-of-pocket,
// not via WuApi tier. If the user hasn't added an OpenRouter key,
// return 412 with a helpful message.
//
// Flow:
//   1. Validate prompt + look up user's active OpenRouter BYOK key
//   2. POST to OpenRouter /chat/completions with modalities=["image"]
//   3. Extract the image URL / base64 payload from the response
//   4. If base64 data URL → decode + upload to nest-attachments
//   5. Return the stable nest-attachments URL
func (h *Handler) GenerateImage(w http.ResponseWriter, r *http.Request) {
	user, err := h.currentUser(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req ImageGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		req.Model = defaultImageModel
	}

	// Find the user's OpenRouter BYOK key. We use BYOK.List + filter
	// by provider="openrouter" instead of adding a dedicated method —
	// same path our chat-stream resolveUpstream uses.
	if h.BYOK == nil {
		http.Error(w, "BYOK not configured — image generation requires an OpenRouter key", http.StatusPreconditionFailed)
		return
	}
	keys, err := h.BYOK.List(r.Context(), user.ID)
	if err != nil {
		slog.Error("image: list byok", "err", err, "user_id", user.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var orKeyID uuid.UUID
	for _, k := range keys {
		if k.Provider == "openrouter" {
			orKeyID = k.ID
			break
		}
	}
	if orKeyID == uuid.Nil {
		http.Error(w, "image generation requires an OpenRouter BYOK key (add one in Settings → API Keys)", http.StatusPreconditionFailed)
		return
	}
	revealed, err := h.BYOK.Reveal(r.Context(), user.ID, orKeyID)
	if err != nil {
		slog.Error("image: reveal byok", "err", err, "user_id", user.ID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	plaintext := revealed.Key
	orBaseURL := revealed.BaseURL
	if orBaseURL == "" {
		orBaseURL = "https://openrouter.ai/api/v1"
	}

	// Build the chat-completions request with modalities=image.
	// OpenRouter's extended field `modalities` opts into image output.
	body := map[string]any{
		"model": req.Model,
		"messages": []map[string]any{
			{"role": "user", "content": req.Prompt},
		},
		"modalities": []string{"image", "text"},
	}
	payload, _ := json.Marshal(body)

	reqCtx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		strings.TrimRight(orBaseURL, "/")+"/chat/completions",
		bytes.NewReader(payload))
	if err != nil {
		http.Error(w, "build request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+plaintext)

	client := &http.Client{Timeout: 125 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if resp.StatusCode >= 400 {
		slog.Warn("image gen upstream error", "status", resp.StatusCode, "body", string(raw))
		http.Error(w, fmt.Sprintf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(raw))), http.StatusBadGateway)
		return
	}

	// Parse response — OpenRouter returns images in
	// choices[].message.images[].image_url.url which may be a data:
	// URL (base64) or a remote URL.
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Images  []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"images"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		slog.Error("image gen decode", "err", err)
		http.Error(w, "bad upstream response", http.StatusBadGateway)
		return
	}

	var imageURL string
	for _, ch := range parsed.Choices {
		for _, im := range ch.Message.Images {
			if im.ImageURL.URL != "" {
				imageURL = im.ImageURL.URL
				break
			}
		}
		if imageURL != "" {
			break
		}
	}
	if imageURL == "" {
		http.Error(w, "model returned no image", http.StatusBadGateway)
		return
	}

	// If data URL, decode + upload to our attachments bucket so the
	// chat message can reference a stable https URL instead of
	// embedding kilobytes of base64.
	if strings.HasPrefix(imageURL, "data:") {
		nestURL, err := h.rehostDataURL(r.Context(), imageURL)
		if err != nil {
			slog.Error("image gen rehost", "err", err)
			http.Error(w, "rehost failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		imageURL = nestURL
	}

	writeJSON(w, http.StatusOK, ImageGenerateResponse{URL: imageURL, Model: req.Model})
}

// rehostDataURL decodes a `data:image/xxx;base64,...` URL and uploads
// it to nest-attachments via the Storage client. Returns the public
// https URL.
func (h *Handler) rehostDataURL(ctx context.Context, dataURL string) (string, error) {
	if h.Storage == nil {
		return "", errors.New("storage not configured")
	}
	// data:image/png;base64,iVBORw0K...
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return "", errors.New("malformed data URL")
	}
	header := dataURL[:comma] // data:image/png;base64
	body := dataURL[comma+1:]

	// Content type: between "data:" and ";".
	semi := strings.IndexByte(header, ';')
	if semi < 0 {
		semi = len(header)
	}
	contentType := strings.TrimPrefix(header[:semi], "data:")
	if contentType == "" {
		contentType = "image/png"
	}

	// Assume base64 encoding (that's all OpenRouter emits currently).
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	url, err := h.Storage.PutAttachment(ctx, raw, contentType)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	return url, nil
}
