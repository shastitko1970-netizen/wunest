package chats

import (
	"encoding/json"
	"testing"

	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

func ptrF(v float64) *float64 { return &v }
func ptrI(v int) *int         { return &v }

// Build a "full sampler" request — every knob WuApi knows about — so each
// test case can assert which ones survive filtering for a given provider.
func fullRequest() wuapi.ChatCompletionRequest {
	return wuapi.ChatCompletionRequest{
		Model:             "some-model",
		Messages:          []map[string]any{{"role": "user", "content": "hi"}},
		Temperature:       ptrF(0.7),
		TopP:              ptrF(0.9),
		TopK:              ptrI(40),
		MinP:              ptrF(0.05),
		MaxTokens:         ptrI(1024),
		FrequencyPenalty:  ptrF(0.1),
		PresencePenalty:   ptrF(0.2),
		RepetitionPenalty: ptrF(1.1),
		Seed:              ptrI(42),
		Stop:              []string{"\n\nUser:"},
		Extra: map[string]any{
			"thinking":         map[string]any{"type": "enabled"},
			"reasoning":        map[string]any{"enabled": true},
			"reasoning_effort": "medium",
			"user_custom":      "keep-this",
		},
	}
}

func TestPrepareRequestForProvider_OpenAI(t *testing.T) {
	out := PrepareRequestForProvider("openai", fullRequest())

	// OpenAI drops top_k, min_p, repetition_penalty — has its own
	// frequency/presence/seed/stop.
	if out.TopK != nil {
		t.Errorf("openai: top_k should be dropped, got %v", *out.TopK)
	}
	if out.MinP != nil {
		t.Errorf("openai: min_p should be dropped, got %v", *out.MinP)
	}
	if out.RepetitionPenalty != nil {
		t.Errorf("openai: repetition_penalty should be dropped, got %v", *out.RepetitionPenalty)
	}
	if out.FrequencyPenalty == nil {
		t.Error("openai: frequency_penalty should be kept")
	}
	if out.Seed == nil {
		t.Error("openai: seed should be kept")
	}

	// Extra: keeps reasoning_effort + user_custom; drops thinking + reasoning.
	if _, ok := out.Extra["reasoning_effort"]; !ok {
		t.Error("openai: reasoning_effort should be kept")
	}
	if _, ok := out.Extra["thinking"]; ok {
		t.Error("openai: thinking should be dropped")
	}
	if _, ok := out.Extra["reasoning"]; ok {
		t.Error("openai: reasoning should be dropped")
	}
	if _, ok := out.Extra["user_custom"]; !ok {
		t.Error("openai: user_custom should be preserved")
	}
}

func TestPrepareRequestForProvider_Anthropic(t *testing.T) {
	out := PrepareRequestForProvider("anthropic", fullRequest())

	if out.TopK != nil || out.MinP != nil || out.RepetitionPenalty != nil {
		t.Error("anthropic: top_k/min_p/repetition_penalty should be dropped")
	}
	if out.FrequencyPenalty != nil || out.PresencePenalty != nil {
		t.Error("anthropic: frequency/presence_penalty should be dropped (native API doesn't take them)")
	}
	if out.Seed != nil {
		t.Error("anthropic: seed should be dropped")
	}
	if out.Stop == nil {
		t.Error("anthropic: stop should be kept (maps to stop_sequences)")
	}

	// Extra: only `thinking` survives.
	if _, ok := out.Extra["thinking"]; !ok {
		t.Error("anthropic: thinking should be kept")
	}
	if _, ok := out.Extra["reasoning"]; ok {
		t.Error("anthropic: reasoning should be dropped")
	}
	if _, ok := out.Extra["reasoning_effort"]; ok {
		t.Error("anthropic: reasoning_effort should be dropped")
	}
	if _, ok := out.Extra["user_custom"]; !ok {
		t.Error("anthropic: user_custom should be preserved")
	}
}

func TestPrepareRequestForProvider_Google(t *testing.T) {
	out := PrepareRequestForProvider("google", fullRequest())

	// Gemini OAI bridge: only takes temperature/top_p/max_tokens/stop.
	if out.TopK != nil {
		t.Error("google: top_k should be dropped (OAI bridge does not forward it)")
	}
	if out.FrequencyPenalty != nil || out.PresencePenalty != nil {
		t.Error("google: penalties should be dropped")
	}
	if out.Seed != nil {
		t.Error("google: seed should be dropped")
	}
	if out.Stop == nil {
		t.Error("google: stop should be kept")
	}

	// Extra: no reasoning key is passed through; Gemini uses model variant.
	for _, k := range []string{"thinking", "reasoning", "reasoning_effort"} {
		if _, ok := out.Extra[k]; ok {
			t.Errorf("google: %s should be dropped", k)
		}
	}
}

func TestPrepareRequestForProvider_OpenRouter(t *testing.T) {
	out := PrepareRequestForProvider("openrouter", fullRequest())

	// OpenRouter is a passthrough — should keep everything we sent.
	if out.TopK == nil || out.MinP == nil || out.RepetitionPenalty == nil {
		t.Error("openrouter: expected to keep top_k/min_p/repetition_penalty")
	}
	if out.FrequencyPenalty == nil || out.Seed == nil {
		t.Error("openrouter: expected to keep frequency_penalty/seed")
	}
	// Reasoning: openrouter keeps its own `reasoning` key, drops the others.
	if _, ok := out.Extra["reasoning"]; !ok {
		t.Error("openrouter: reasoning should be kept")
	}
	if _, ok := out.Extra["thinking"]; ok {
		t.Error("openrouter: thinking should be dropped (not openrouter's shape)")
	}
	if _, ok := out.Extra["reasoning_effort"]; ok {
		t.Error("openrouter: reasoning_effort should be dropped")
	}
}

func TestPrepareRequestForProvider_EmptyProvider_Passthrough(t *testing.T) {
	// Empty provider = WuApi path. We never call Prepare there, but make
	// sure it's still a safe no-op.
	req := fullRequest()
	out := PrepareRequestForProvider("", req)

	if out.TopK == nil || out.MinP == nil {
		t.Error("empty provider: should pass through top_k/min_p unchanged")
	}
	for _, k := range []string{"thinking", "reasoning", "reasoning_effort", "user_custom"} {
		if _, ok := out.Extra[k]; !ok {
			t.Errorf("empty provider: %s should be preserved", k)
		}
	}
}

func TestDirectCallHeaders(t *testing.T) {
	// Anthropic uses x-api-key + anthropic-version.
	h := DirectCallHeaders("anthropic", "sk-ant-abc")
	if h["x-api-key"] != "sk-ant-abc" {
		t.Errorf("anthropic: x-api-key mismatch, got %q", h["x-api-key"])
	}
	if h["anthropic-version"] == "" {
		t.Error("anthropic: anthropic-version header missing")
	}
	if _, ok := h["Authorization"]; ok {
		t.Error("anthropic: Authorization header should NOT be set (only x-api-key)")
	}

	// Everyone else: Bearer.
	for _, p := range []string{"openai", "google", "openrouter", "deepseek", "mistral", "custom"} {
		h := DirectCallHeaders(p, "sk-abc")
		if h["Authorization"] != "Bearer sk-abc" {
			t.Errorf("%s: expected Bearer auth, got %q", p, h["Authorization"])
		}
	}
}

func TestMergeReasoningForProvider(t *testing.T) {
	t.Run("anthropic enabled", func(t *testing.T) {
		b := true
		out := MergeReasoningForProvider("anthropic", nil, &b)
		if th, ok := out["thinking"].(map[string]any); !ok || th["type"] != "enabled" {
			t.Errorf("expected thinking={type:enabled}, got %v", out["thinking"])
		}
		if _, ok := out["reasoning_effort"]; ok {
			t.Error("anthropic should not emit reasoning_effort")
		}
	})
	t.Run("openai enabled", func(t *testing.T) {
		b := true
		out := MergeReasoningForProvider("openai", nil, &b)
		if out["reasoning_effort"] != "medium" {
			t.Errorf("expected reasoning_effort=medium, got %v", out["reasoning_effort"])
		}
		if _, ok := out["thinking"]; ok {
			t.Error("openai should not emit thinking")
		}
	})
	t.Run("openai disabled", func(t *testing.T) {
		b := false
		out := MergeReasoningForProvider("openai", nil, &b)
		if _, ok := out["reasoning_effort"]; ok {
			t.Error("openai disabled: should emit nothing (not reasoning_effort)")
		}
	})
	t.Run("google", func(t *testing.T) {
		b := true
		out := MergeReasoningForProvider("google", map[string]any{"untouched": 1}, &b)
		if out["untouched"] != 1 {
			t.Error("google: existing keys should be preserved")
		}
		for _, k := range []string{"thinking", "reasoning", "reasoning_effort"} {
			if _, ok := out[k]; ok {
				t.Errorf("google: %s should not be emitted", k)
			}
		}
	})
}

// Sanity: the Extra map round-trips through MarshalJSON correctly after
// shaping. Guards against a regression where shape would strip a key but
// the marshaller would re-include it from the named fields.
func TestPrepareRequestForProvider_JSONShape(t *testing.T) {
	out := PrepareRequestForProvider("openai", fullRequest())
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, forbidden := range []string{"top_k", "min_p", "repetition_penalty", "thinking", "reasoning"} {
		if _, ok := decoded[forbidden]; ok {
			t.Errorf("openai JSON contains forbidden key %q: %v", forbidden, decoded[forbidden])
		}
	}
	// Should still have the allowed ones.
	for _, required := range []string{"model", "messages", "temperature", "top_p", "max_tokens", "frequency_penalty", "presence_penalty", "seed", "stop", "reasoning_effort"} {
		if _, ok := decoded[required]; !ok {
			t.Errorf("openai JSON missing required key %q", required)
		}
	}
}
