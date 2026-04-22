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
