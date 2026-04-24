package chats

import (
	"encoding/json"
	"strings"
	"testing"
)

// Pure-logic tests for the JSONL import/export helpers (M13). No DB, no HTTP.

func TestSplitJSONL_DropsEmptyAndCRLF(t *testing.T) {
	// Mix of LF and CRLF, blank lines, leading/trailing whitespace.
	body := []byte("line1\r\nline2\n\n  line3  \r\n\n")
	got := splitJSONL(body)
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(got), got)
	}
	if string(got[0]) != "line1" {
		t.Errorf("line 0: %q", got[0])
	}
	if string(got[2]) != "line3" {
		t.Errorf("line 2: %q", got[2])
	}
}

func TestSplitJSONL_EmptyInput(t *testing.T) {
	if got := splitJSONL(nil); len(got) != 0 {
		t.Errorf("expected 0 lines, got %+v", got)
	}
}

func TestSplitJSONL_NoTrailingNewline(t *testing.T) {
	// Common case — last line without \n should still be included.
	body := []byte(`{"a":1}`)
	got := splitJSONL(body)
	if len(got) != 1 || string(got[0]) != `{"a":1}` {
		t.Errorf("lone line: %+v", got)
	}
}

func TestDetectChatFormat(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"wunest native", `{"type":"chat_meta","name":"x"}`, "wunest"},
		{"ST with user_name", `{"user_name":"U","character_name":"C"}`, "silly-tavern"},
		{"ST with chat_metadata only", `{"chat_metadata":{}}`, "silly-tavern"},
		{"ST with create_date only", `{"create_date":"2024-1-1"}`, "silly-tavern"},
		{"neither", `{"foo":"bar"}`, "unknown"},
		// A WuNest file masquerading wouldn't match the literal "chat_meta" check.
		{"wrong type value", `{"type":"something_else","user_name":"U"}`, "silly-tavern"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.line), &m); err != nil {
				t.Fatalf("bad test fixture %q: %v", tc.line, err)
			}
			got := detectChatFormat(m)
			if got != tc.want {
				t.Errorf("line=%q: got %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// ── parseMessageExtras ──────────────────────────────────────────────

func TestParseMessageExtras_EmptyReturnsNil(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte(""), []byte("null"), []byte("{}")} {
		if got := parseMessageExtras(json.RawMessage(raw)); got != nil {
			t.Errorf("expected nil for %q, got %+v", raw, got)
		}
	}
}

func TestParseMessageExtras_ValidShape(t *testing.T) {
	raw := json.RawMessage(`{"model":"wu-kitsune","tokens_in":10,"latency_ms":250}`)
	got := parseMessageExtras(raw)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Model != "wu-kitsune" {
		t.Errorf("model: %q", got.Model)
	}
	if got.TokensIn != 10 {
		t.Errorf("tokens_in: %d", got.TokensIn)
	}
}

func TestParseMessageExtras_Garbage(t *testing.T) {
	// Malformed JSON → nil; shouldn't panic.
	got := parseMessageExtras(json.RawMessage(`not json`))
	if got != nil {
		t.Fatalf("expected nil on garbage, got %+v", got)
	}
}

// ── sanitiseFilename ────────────────────────────────────────────────

func TestSanitiseFilename_EmptyBecomesChat(t *testing.T) {
	if got := sanitiseFilename(""); got != "chat" {
		t.Errorf("empty → %q", got)
	}
}

func TestSanitiseFilename_StripsUnsafe(t *testing.T) {
	got := sanitiseFilename(`Pirate / "run" \with\ breaks`)
	if strings.ContainsAny(got, `/\"`) {
		t.Errorf("unsafe chars survived: %q", got)
	}
}

func TestSanitiseFilename_PreservesUnicode(t *testing.T) {
	// Russian text in filenames — modern browsers handle UTF-8 in
	// Content-Disposition, so we leave Cyrillic alone.
	got := sanitiseFilename("Пиратский побег")
	if !strings.Contains(got, "Пиратский") {
		t.Errorf("Cyrillic stripped: %q", got)
	}
}

func TestSanitiseFilename_TruncatesAt80(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := sanitiseFilename(long)
	if len(got) != 80 {
		t.Errorf("expected 80 chars, got %d", len(got))
	}
}
