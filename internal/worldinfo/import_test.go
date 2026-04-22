package worldinfo

import (
	"encoding/json"
	"testing"
)

// Pure-logic tests for the SillyTavern lorebook import normaliser.
// parseSTEntries + convertSTEntries + stringifyPosition live in
// handler.go but are small and side-effect free; we call them directly
// via the exported shape (parseSTEntries is package-level).

func TestParseSTEntries_ArrayShape(t *testing.T) {
	raw := json.RawMessage(`[
		{"key":["dragon"],"content":"Dragons hoard gold.","enabled":true,"order":10,"position":0}
	]`)
	got, err := parseSTEntries(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	e := got[0]
	if e.Content != "Dragons hoard gold." {
		t.Errorf("content: %q", e.Content)
	}
	if len(e.Keys) != 1 || e.Keys[0] != "dragon" {
		t.Errorf("keys: %+v", e.Keys)
	}
	if e.InsertionOrder != 10 {
		t.Errorf("order→insertion_order: %d", e.InsertionOrder)
	}
	if e.Position != PositionBeforeChar {
		t.Errorf("position 0 should map to before_char, got %q", e.Position)
	}
}

func TestParseSTEntries_ObjectShape(t *testing.T) {
	// ST's classic export — entries is an object keyed by numeric strings.
	raw := json.RawMessage(`{
		"0": {"keys":["a"],"content":"A.","enabled":true},
		"1": {"keys":["b"],"content":"B.","enabled":true}
	}`)
	got, err := parseSTEntries(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
}

func TestParseSTEntries_EmptyInputs(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte(""), []byte("null")} {
		got, err := parseSTEntries(json.RawMessage(raw))
		if err != nil {
			t.Errorf("raw=%q: unexpected err %v", raw, err)
		}
		if len(got) != 0 {
			t.Errorf("raw=%q: expected empty, got %d", raw, len(got))
		}
	}
}

func TestParseSTEntries_DisablePromotedToEnabledFalse(t *testing.T) {
	// ST uses `disable: true` to mean "off"; newer exports use `enabled`.
	raw := json.RawMessage(`[{"key":["x"],"content":"x","disable":true}]`)
	got, _ := parseSTEntries(raw)
	if got[0].Enabled {
		t.Fatalf("disable:true should produce enabled:false, got %+v", got[0])
	}
}

func TestParseSTEntries_KeyvsKeys(t *testing.T) {
	// Legacy export used `key`, modern uses `keys`. We accept either.
	legacy := json.RawMessage(`[{"key":["a","b"],"content":"x","enabled":true}]`)
	modern := json.RawMessage(`[{"keys":["a","b"],"content":"x","enabled":true}]`)

	for name, raw := range map[string]json.RawMessage{
		"legacy": legacy, "modern": modern,
	} {
		got, err := parseSTEntries(raw)
		if err != nil || len(got) != 1 || len(got[0].Keys) != 2 {
			t.Errorf("%s failed: %+v err=%v", name, got, err)
		}
	}
}

// ── stringifyPosition ───────────────────────────────────────────────

func TestStringifyPosition_IntMapping(t *testing.T) {
	cases := map[int]string{
		0: PositionBeforeChar, // ST: before character
		1: PositionBeforeChar, // ST: also before — merged
		2: PositionAfterChar,  // ST: after character
		3: PositionAfterChar,
		4: PositionAfterChar, // ST: at depth; treated as after for v1
	}
	for in, want := range cases {
		got := stringifyPosition(float64(in))
		if got != want {
			t.Errorf("position %d → %q, want %q", in, got, want)
		}
	}
}

func TestStringifyPosition_StringPassthrough(t *testing.T) {
	if stringifyPosition("before_char") != PositionBeforeChar {
		t.Error("string before_char not preserved")
	}
	if stringifyPosition("unknown") != "" {
		t.Error("unknown string should zero-out, not leak")
	}
}

func TestStringifyPosition_Nil(t *testing.T) {
	if stringifyPosition(nil) != "" {
		t.Error("nil should zero-out")
	}
}
