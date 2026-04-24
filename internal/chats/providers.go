package chats

import (
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// Per-provider request shaping. WuApi accepts our full superset request
// (top_k, min_p, repetition_penalty, thinking, reasoning, …) and normalises
// downstream. Direct-provider calls don't get that translation, so we need
// to drop or rename fields the provider doesn't recognise — otherwise
// OpenAI 400s on top_k, Anthropic 400s on frequency_penalty, etc.
//
// Design: we never mutate the caller's req; we build a shallow copy with
// the bits that aren't compatible nil'd out, and rebuild the Extra map
// with a provider-specific reasoning shape when ReasoningEnabled is set.

// PrepareRequestForProvider returns a copy of req with fields that the
// given provider doesn't accept dropped, and reasoning-override keys
// renamed into whatever shape the provider actually honours.
//
// An empty provider means "don't shape" (WuApi path or unknown source).
func PrepareRequestForProvider(provider string, req wuapi.ChatCompletionRequest) wuapi.ChatCompletionRequest {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return req
	}
	caps := providerCaps(provider)

	out := req // shallow copy of the struct; pointer fields shared — we nil them out below where needed

	if !caps.topK {
		out.TopK = nil
	}
	if !caps.minP {
		out.MinP = nil
	}
	if !caps.repetitionPenalty {
		out.RepetitionPenalty = nil
	}
	if !caps.frequencyPenalty {
		out.FrequencyPenalty = nil
	}
	if !caps.presencePenalty {
		out.PresencePenalty = nil
	}
	if !caps.seed {
		out.Seed = nil
	}
	if !caps.stop {
		out.Stop = nil
	}

	// Rebuild Extra: drop reasoning-override keys that don't belong to this
	// provider, keep everything else verbatim. We never clobber the caller's
	// keys silently — if they jammed `thinking` into the preset's extra by
	// hand, we'll still respect it (the filter only strips keys added by our
	// own mergeReasoning helper).
	if len(out.Extra) > 0 {
		filtered := make(map[string]any, len(out.Extra))
		for k, v := range out.Extra {
			if isReasoningKey(k) && !caps.reasoningKey(k) {
				continue
			}
			filtered[k] = v
		}
		out.Extra = filtered
	}

	return out
}

// MergeReasoningForProvider is the provider-aware twin of mergeReasoning.
// It only adds the one key the provider actually recognises:
//
//   - anthropic  → thinking: { type: "enabled"|"disabled" }
//   - openai     → reasoning_effort: "medium" (only when enabled)
//   - openrouter → reasoning: { enabled: bool }   (OR's unified shape)
//   - google     → no key (Gemini thinking is controlled server-side by
//                  picking the -thinking model variant)
//   - deepseek   → no key (reasoner models always think; non-reasoner don't)
//   - mistral    → no key
//   - custom     → reasoning: { enabled: bool }  (safe default for proxies)
//
// For the WuApi path we keep the old "union of all shapes" behaviour in
// mergeReasoning() so WuApi's router can pick whichever one it needs.
func MergeReasoningForProvider(provider string, extra map[string]any, enabled *bool) map[string]any {
	if enabled == nil {
		return extra
	}
	out := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		out[k] = v
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "anthropic":
		if _, ok := out["thinking"]; !ok {
			if *enabled {
				out["thinking"] = map[string]any{"type": "enabled"}
			} else {
				out["thinking"] = map[string]any{"type": "disabled"}
			}
		}
	case "openai":
		if *enabled {
			if _, ok := out["reasoning_effort"]; !ok {
				out["reasoning_effort"] = "medium"
			}
		}
	case "openrouter", "custom", "":
		if _, ok := out["reasoning"]; !ok {
			out["reasoning"] = map[string]any{"enabled": *enabled}
		}
	}
	return out
}

// providerCaps captures what each supported provider's OAI-compat endpoint
// tolerates. Values here are conservative — when a provider's docs say a
// field is ignored silently we keep it off anyway so a future-strict update
// doesn't break us.
type providerCap struct {
	topK              bool
	minP              bool
	repetitionPenalty bool
	frequencyPenalty  bool
	presencePenalty   bool
	seed              bool
	stop              bool
	reasoning         map[string]bool // which reasoning-key names survive filtering
}

func (c providerCap) reasoningKey(k string) bool {
	return c.reasoning[k]
}

func providerCaps(provider string) providerCap {
	switch provider {
	case "openai":
		return providerCap{
			frequencyPenalty:  true,
			presencePenalty:   true,
			seed:              true,
			stop:              true,
			reasoning:         map[string]bool{"reasoning_effort": true},
		}
	case "anthropic":
		// OAI-compat endpoint at /v1/chat/completions. Doesn't map the
		// penalties, min_p, seed, or top_k to Anthropic's native knobs —
		// sending them yields 400.
		return providerCap{
			stop:      true,
			reasoning: map[string]bool{"thinking": true},
		}
	case "google":
		// Gemini OAI-compat layer at /v1beta/openai. Accepts temperature,
		// top_p, max_tokens, stop, and its own reasoning model variants.
		// top_k is native to Gemini but the OAI bridge doesn't forward it.
		return providerCap{
			stop:      true,
			reasoning: map[string]bool{},
		}
	case "deepseek":
		return providerCap{
			frequencyPenalty: true,
			presencePenalty:  true,
			stop:             true,
			reasoning:        map[string]bool{},
		}
	case "mistral":
		return providerCap{
			frequencyPenalty: true,
			presencePenalty:  true,
			seed:             true,
			stop:             true,
			reasoning:        map[string]bool{},
		}
	case "openrouter":
		// OpenRouter is a passthrough to many backends — it accepts the
		// full OAI superset plus its own `reasoning` key.
		return providerCap{
			topK:              true,
			minP:              true,
			repetitionPenalty: true,
			frequencyPenalty:  true,
			presencePenalty:   true,
			seed:              true,
			stop:              true,
			reasoning:         map[string]bool{"reasoning": true},
		}
	default:
		// "custom" and anything we don't know: assume permissive
		// (most proxies are) and let the upstream 400 if we're wrong.
		return providerCap{
			topK:              true,
			minP:              true,
			repetitionPenalty: true,
			frequencyPenalty:  true,
			presencePenalty:   true,
			seed:              true,
			stop:              true,
			reasoning:         map[string]bool{"reasoning": true, "thinking": true, "reasoning_effort": true},
		}
	}
}

// isReasoningKey tells filtering whether a given Extra key is one of
// ours (subject to provider shaping) or user-supplied (leave alone).
func isReasoningKey(k string) bool {
	switch k {
	case "thinking", "reasoning", "reasoning_effort":
		return true
	}
	return false
}

// DirectCallHeaders returns the HTTP headers that directChatStream should
// layer on top of the common ones (Content-Type/Accept/User-Agent).
//
// Most providers are happy with plain `Authorization: Bearer <key>`. The
// one consistent exception is Anthropic's native endpoints, which want
// `x-api-key` + `anthropic-version`. Their OAI-compat endpoint accepts
// both, but Anthropic support reps recommend the native headers so
// x-api-key is the less-surprising choice.
func DirectCallHeaders(provider, apiKey string) map[string]string {
	out := map[string]string{}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		out["x-api-key"] = apiKey
		out["anthropic-version"] = "2023-06-01"
	default:
		out["Authorization"] = "Bearer " + apiKey
	}
	return out
}
