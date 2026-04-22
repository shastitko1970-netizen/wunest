package chats

import (
	"encoding/json"
	"testing"
)

// Pure-logic tests for the chat metadata helpers:
//   readSampler / readAuthorsNote / readPersonaID — JSONB parsing safety.
//   applyChatSampler               — merge semantics between request and chat.
//   mergeReasoning                 — upstream Extra field folding.
//
// None of these touch the DB; they're the critical bits that would rot
// silently if a shape changes. Covers both Author's Note (M11) and the
// sampler/reasoning pieces shipped in M8b.

// ── readSampler / readAuthorsNote / readPersonaID ───────────────────

func TestReadSampler_EmptyMetadataReturnsZero(t *testing.T) {
	got := readSampler(nil)
	if got.Temperature != nil || got.TopP != nil || len(got.Stop) != 0 {
		t.Fatalf("zero expected, got %+v", got)
	}
}

func TestReadSampler_ExtractsKnownFields(t *testing.T) {
	raw := json.RawMessage(`{"sampler":{"temperature":0.9,"top_p":0.95,"stop":["</s>"]}}`)
	got := readSampler(raw)
	if got.Temperature == nil || *got.Temperature != 0.9 {
		t.Errorf("temperature mis-read: %+v", got.Temperature)
	}
	if got.TopP == nil || *got.TopP != 0.95 {
		t.Errorf("top_p mis-read: %+v", got.TopP)
	}
	if len(got.Stop) != 1 || got.Stop[0] != "</s>" {
		t.Errorf("stop mis-read: %+v", got.Stop)
	}
}

func TestReadSampler_ToleratesGarbage(t *testing.T) {
	// Not an object — should zero-value not panic.
	got := readSampler(json.RawMessage(`"this is not an envelope"`))
	if got.Temperature != nil {
		t.Fatalf("expected zero sampler on malformed, got %+v", got)
	}
}

func TestReadAuthorsNote_RoundTrip(t *testing.T) {
	raw := json.RawMessage(`{"authors_note":{"content":"x","depth":3,"role":"user"}}`)
	got := readAuthorsNote(raw)
	if got == nil {
		t.Fatal("expected non-nil note")
	}
	if got.Content != "x" || got.Depth != 3 || got.Role != "user" {
		t.Fatalf("fields mis-read: %+v", got)
	}
}

func TestReadAuthorsNote_MissingKeyReturnsNil(t *testing.T) {
	raw := json.RawMessage(`{"sampler":{}}`) // no authors_note field
	if got := readAuthorsNote(raw); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestReadPersonaID_Missing(t *testing.T) {
	if got := readPersonaID(nil); got.String() != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("expected nil uuid, got %s", got)
	}
}

func TestReadPersonaID_Present(t *testing.T) {
	raw := json.RawMessage(`{"persona_id":"11111111-1111-1111-1111-111111111111"}`)
	got := readPersonaID(raw)
	if got.String() != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("wrong uuid: %s", got)
	}
}

func TestReadPersonaID_InvalidString(t *testing.T) {
	raw := json.RawMessage(`{"persona_id":"not-a-uuid"}`)
	// Should not panic; returns nil uuid.
	if got := readPersonaID(raw); got.String() != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("expected nil on invalid, got %s", got)
	}
}

// ── applyChatSampler ────────────────────────────────────────────────

func TestApplyChatSampler_RequestWins(t *testing.T) {
	reqTemp := 0.5
	chatTemp := 0.9
	in := SendMessageInput{Temperature: &reqTemp}
	chat := ChatSamplerMetadata{Temperature: &chatTemp}
	out := applyChatSampler(in, chat)
	if *out.Temperature != 0.5 {
		t.Fatalf("request-level temperature should win, got %v", *out.Temperature)
	}
}

func TestApplyChatSampler_ChatFillsGaps(t *testing.T) {
	chatTemp := 0.7
	in := SendMessageInput{} // nothing set
	chat := ChatSamplerMetadata{Temperature: &chatTemp}
	out := applyChatSampler(in, chat)
	if out.Temperature == nil || *out.Temperature != 0.7 {
		t.Fatalf("chat default should fill, got %+v", out.Temperature)
	}
}

func TestApplyChatSampler_StopOnlyIfUnset(t *testing.T) {
	in := SendMessageInput{Stop: []string{"from-req"}}
	chat := ChatSamplerMetadata{Stop: []string{"from-chat"}}
	out := applyChatSampler(in, chat)
	if len(out.Stop) != 1 || out.Stop[0] != "from-req" {
		t.Fatalf("explicit Stop should win: %+v", out.Stop)
	}
}

func TestApplyChatSampler_SystemPromptOnlyIfEmpty(t *testing.T) {
	in := SendMessageInput{SystemPromptOverride: ""}
	chat := ChatSamplerMetadata{SystemPromptOverride: "from chat"}
	out := applyChatSampler(in, chat)
	if out.SystemPromptOverride != "from chat" {
		t.Fatalf("chat sysprompt should fill, got %q", out.SystemPromptOverride)
	}
}

// ── mergeReasoning ──────────────────────────────────────────────────

func TestMergeReasoning_Nil_NoChanges(t *testing.T) {
	extra := map[string]any{"existing": 1}
	got := mergeReasoning(extra, nil)
	// Nil reasoning flag → untouched (same map, no additions).
	if _, has := got["thinking"]; has {
		t.Fatalf("thinking should not be set when reasoning is nil, got %+v", got)
	}
}

func TestMergeReasoning_EnabledAddsAllThreeFormats(t *testing.T) {
	enabled := true
	got := mergeReasoning(nil, &enabled)
	for _, k := range []string{"thinking", "reasoning", "reasoning_effort"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("expected %q in Extra, got %+v", k, got)
		}
	}
	if got["reasoning_effort"] != "medium" {
		t.Fatalf("reasoning_effort should default to 'medium', got %+v", got["reasoning_effort"])
	}
}

func TestMergeReasoning_DisabledSetsDisabledFormats(t *testing.T) {
	enabled := false
	got := mergeReasoning(nil, &enabled)
	thinking, _ := got["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("thinking.type should be 'disabled', got %+v", thinking)
	}
}

func TestMergeReasoning_CallerKeysWin(t *testing.T) {
	enabled := true
	extra := map[string]any{"thinking": "custom-shape"}
	got := mergeReasoning(extra, &enabled)
	if got["thinking"] != "custom-shape" {
		t.Fatalf("caller-supplied thinking should not be overwritten, got %+v", got["thinking"])
	}
	// Other keys should still be filled.
	if _, ok := got["reasoning"]; !ok {
		t.Fatalf("reasoning should still be added: %+v", got)
	}
}
