package chats

import (
	"regexp"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/presets"
)

// Vision / multimodal prompt assembly.
//
// SillyTavern (and the OpenAI-compat API generally) supports two shapes for
// a user message's `content` field:
//
//  1. Plain string — the default, all models accept this.
//  2. Array of parts — `[{"type":"text","text":"..."},{"type":"image_url",
//     "image_url":{"url":"https://..."}}]`. Only vision-capable models
//     accept this; non-vision models 400 the request.
//
// Our users drag-drop images into MessageInput (M33 follow-up) and the
// upload handler returns a public URL; the frontend inserts the URL as
// a Markdown image tag `![name](url)`. Up to this point we shipped it
// to the model as a literal text URL — vision models saw the string
// "https://nest.wusphere.ru/images/attachments/abc.png" instead of
// actually looking at the image.
//
// This file transforms user messages just before they hit the wire:
//   1. Detect our attachment URLs with the pattern defined below.
//   2. If the model supports vision AND image_inlining isn't disabled
//      in the active preset bundle → split the message into a text
//      part (URLs stripped) plus one image_url part per URL found.
//   3. Otherwise leave the message as plain string (non-vision models
//      treat the URL as text, which is the pre-M37 behaviour).

// attachmentURLRegex matches full http(s) URLs pointing at our
// /images/attachments/ bucket. Kept strict so we don't accidentally
// transform arbitrary URLs in user messages — only OUR storage gets
// the image_url treatment.
var attachmentURLRegex = regexp.MustCompile(
	`https?://[^\s)"<>]+/images/attachments/[a-f0-9]{24}\.[a-z]{2,5}`,
)

// markdownImageRegex strips the full Markdown image markup `![alt](url)`
// so the stripped text part reads naturally. Two-pass: first replace
// markdown-image syntax, then any bare URLs that slipped through.
var markdownImageRegex = regexp.MustCompile(
	`!\[[^\]]*\]\(\s*(https?://[^\s)"<>]+/images/attachments/[a-f0-9]{24}\.[a-z]{2,5})\s*\)`,
)

// VisionCapableModel returns true when we know the given model id can
// accept image_url parts. Conservative: unknown models return false,
// so worst case the image is sent as text (same as pre-M37).
//
// Names are matched case-insensitively against a prefix list. When WuApi
// adds a new wu-* alias to a vision family, add it here.
func VisionCapableModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	// Frontier vision models — safe-to-assume list.
	prefixes := []string{
		// OpenAI
		"gpt-4o", "gpt-4.1", "gpt-4-turbo", "gpt-4-vision", "chatgpt-4o",
		"o1-", "o1.", "o3-", "o3.", "o4-",
		// Anthropic — Claude 3+ all vision. 3.5/3.7/4 are current.
		"claude-3", "claude-4", "claude-5", "claude-opus", "claude-sonnet",
		"claude-haiku", "claude-instant",
		// Google
		"gemini-1.5", "gemini-2", "gemini-exp", "gemini-pro", "gemini-flash",
		"gemini-ultra",
		// Mistral
		"pixtral-", "mistral-large-2", "mistral-medium-2",
		// WuApi aliases — mostly reroute to the above; err on permissive
		// side, the proxy will validate if the underlying model rejects.
		"wu-",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

// MultimodalOptions controls the transformation.
type MultimodalOptions struct {
	// Enabled toggles the whole thing off. True by default when the
	// model supports vision and the preset's image_inlining flag isn't
	// explicitly false.
	Enabled bool

	// ImageQuality maps to OpenAI's `detail` field on image_url parts:
	// "auto" | "low" | "high". Passed through verbatim; empty means
	// "auto" (OpenAI default).
	ImageQuality string
}

// DecideMultimodal reads the preset bundle + model to decide whether
// to transform user messages into multi-part content. Centralising
// the decision here keeps handler.go tidy.
//
// Behaviour:
//   - Model must pass VisionCapableModel
//   - bundle.image_inlining == true → enabled (user opted in at preset level)
//   - bundle.image_inlining == false → disabled (user opted out)
//   - bundle.image_inlining unset → default ON when model is vision-capable
//     (most users' expectation: "if I drop an image, the model sees it")
func DecideMultimodal(model string, bundle *presets.OpenAIBundleData) MultimodalOptions {
	opts := MultimodalOptions{}
	if !VisionCapableModel(model) {
		return opts
	}
	if bundle != nil && bundle.ImageInlining != nil && !*bundle.ImageInlining {
		return opts // explicit opt-out wins
	}
	opts.Enabled = true
	if bundle != nil {
		opts.ImageQuality = bundle.InlineImageQuality
	}
	return opts
}

// ApplyMultimodal transforms user-role messages in `messages` when
// multimodal is enabled. Other roles (assistant/system) are never
// transformed — only the user's outgoing turn can carry images.
//
// The input is the already-built outbound []map[string]any (what the
// wuapi client will ship). We mutate in place and return the same
// slice for call-site brevity.
//
// Per-message flow:
//   1. read content as string (no-op if already a list)
//   2. find attachment URLs
//   3. build parts: text (URLs stripped) + image_url for each
//   4. empty-text edge case: models vary; we emit an "[image]" filler
//      so Claude (which rejects empty text) doesn't 400.
func ApplyMultimodal(messages []map[string]any, opts MultimodalOptions) []map[string]any {
	if !opts.Enabled || len(messages) == 0 {
		return messages
	}
	for i, m := range messages {
		role, _ := m["role"].(string)
		if role != "user" {
			continue
		}
		content, _ := m["content"].(string)
		if content == "" {
			continue
		}
		urls := attachmentURLRegex.FindAllString(content, -1)
		if len(urls) == 0 {
			continue
		}
		// Strip markdown image markup first; leftover bare URLs go too.
		text := markdownImageRegex.ReplaceAllString(content, "")
		text = attachmentURLRegex.ReplaceAllString(text, "")
		text = strings.TrimSpace(text)
		if text == "" {
			// Keep non-empty for strict providers (Anthropic rejects
			// empty text parts when vision images accompany them).
			text = "[image]"
		}

		// De-dup URLs — same image dropped twice collapses to one part.
		seen := make(map[string]struct{}, len(urls))
		parts := make([]map[string]any, 0, 1+len(urls))
		parts = append(parts, map[string]any{"type": "text", "text": text})
		for _, u := range urls {
			if _, dup := seen[u]; dup {
				continue
			}
			seen[u] = struct{}{}
			imgPart := map[string]any{"url": u}
			if opts.ImageQuality != "" {
				imgPart["detail"] = opts.ImageQuality
			}
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": imgPart,
			})
		}
		messages[i]["content"] = parts
	}
	return messages
}
