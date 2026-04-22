package presets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidate_EmptyIsOK(t *testing.T) {
	if err := validatePresetData(TypeSampler, nil); err != nil {
		t.Fatal(err)
	}
	if err := validatePresetData(TypeSampler, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_RejectsNonObject(t *testing.T) {
	for _, raw := range []string{`"just a string"`, `42`, `[1,2]`, `null`, `true`} {
		if err := validatePresetData(TypeSampler, json.RawMessage(raw)); err == nil {
			t.Errorf("should reject %q", raw)
		}
	}
}

func TestValidate_SamplerNumberFields(t *testing.T) {
	// Valid.
	ok := json.RawMessage(`{"temperature":0.9,"top_k":null,"stop":["</s>"]}`)
	if err := validatePresetData(TypeSampler, ok); err != nil {
		t.Errorf("valid rejected: %v", err)
	}
	// Invalid: temperature as string.
	bad := json.RawMessage(`{"temperature":"hot"}`)
	err := validatePresetData(TypeSampler, bad)
	if err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Errorf("should mention temperature, got %v", err)
	}
}

func TestValidate_SamplerStopAsString(t *testing.T) {
	// Some ST exports use a single string instead of []string.
	ok := json.RawMessage(`{"stop":"END"}`)
	if err := validatePresetData(TypeSampler, ok); err != nil {
		t.Errorf("single-string stop rejected: %v", err)
	}
}

func TestValidate_ContextBoolFields(t *testing.T) {
	ok := json.RawMessage(`{"trim_sentences":true,"single_line":false}`)
	if err := validatePresetData(TypeContext, ok); err != nil {
		t.Errorf("valid rejected: %v", err)
	}
	bad := json.RawMessage(`{"trim_sentences":"yes"}`)
	if err := validatePresetData(TypeContext, bad); err == nil {
		t.Error("string for bool field should be rejected")
	}
}

func TestValidate_SyspromptStringFields(t *testing.T) {
	// Accept both post_history and post_history_instructions.
	ok1 := json.RawMessage(`{"content":"x","post_history":"y"}`)
	ok2 := json.RawMessage(`{"content":"x","post_history_instructions":"y"}`)
	for _, raw := range []json.RawMessage{ok1, ok2} {
		if err := validatePresetData(TypeSysprompt, raw); err != nil {
			t.Errorf("rejected %s: %v", raw, err)
		}
	}
	bad := json.RawMessage(`{"content":42}`)
	if err := validatePresetData(TypeSysprompt, bad); err == nil {
		t.Error("numeric content should be rejected")
	}
}

func TestValidate_ReasoningStrings(t *testing.T) {
	ok := json.RawMessage(`{"prefix":"<think>","suffix":"</think>","separator":"\n\n"}`)
	if err := validatePresetData(TypeReasoning, ok); err != nil {
		t.Fatal(err)
	}
	bad := json.RawMessage(`{"prefix":{}}`)
	if err := validatePresetData(TypeReasoning, bad); err == nil {
		t.Error("object prefix should be rejected")
	}
}

func TestValidate_UnknownTypePasses(t *testing.T) {
	// Unknown types fall through — handler rejects them before reaching here.
	if err := validatePresetData(PresetType("made-up"), json.RawMessage(`{}`)); err != nil {
		t.Errorf("unknown type should pass-through, got %v", err)
	}
}
