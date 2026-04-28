package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

// M51 Sprint 3 wave 3 — regression coverage on parseOutput.
//
// The model can wrap its JSON in ```json fences, prepend "Here's the theme:",
// emit malformed JSON, or worse — and we have to handle each case
// gracefully. Each test below codifies a real failure mode we've seen
// (or proactively guarded against) so future model upgrades don't
// silently regress.

func TestParseOutput_BareJSON(t *testing.T) {
	in := `{"name":"x","custom_css":""}`
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != in {
		t.Fatalf("expected pass-through, got %q", string(out))
	}
}

func TestParseOutput_FencedJSON(t *testing.T) {
	in := "```json\n{\"name\":\"x\",\"custom_css\":\"body{color:red}\"}\n```"
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Validate it parses as JSON — we don't care about exact whitespace.
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("output should be valid JSON: %v\nraw: %s", err, string(out))
	}
	if probe["name"] != "x" {
		t.Fatalf("expected name=x, got %v", probe["name"])
	}
}

func TestParseOutput_UppercaseFence(t *testing.T) {
	// Some models emit ```JSON uppercase. parseOutput strips both.
	in := "```JSON\n{\"a\":1}\n```"
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
}

func TestParseOutput_BareTripleBacktick(t *testing.T) {
	// Model emits ``` without a language tag.
	in := "```\n{\"a\":1}\n```"
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `"a":1`) {
		t.Fatalf("expected stripped JSON, got %q", string(out))
	}
}

func TestParseOutput_LeadingProse(t *testing.T) {
	// The fallback path scans for the outermost {...}. Common
	// failure mode: model says "Sure! Here's the theme: { ... }".
	in := `Sure! Here's the theme:
{"name":"x"}
Hope this helps!`
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("output should be valid JSON: %v\nraw: %s", err, string(out))
	}
	if probe["name"] != "x" {
		t.Fatalf("expected name=x, got %v", probe["name"])
	}
}

func TestParseOutput_NestedObjectInProse(t *testing.T) {
	// The outermost {...} matcher must be greedy enough to handle
	// nested objects. LastIndexByte('}') gets us the last `}`,
	// which envelopes the whole object.
	in := `Sure!
{
  "name": "x",
  "nested": {"foo": "bar"}
}
Done.`
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("output should be valid JSON: %v\nraw: %s", err, string(out))
	}
	nested, ok := probe["nested"].(map[string]any)
	if !ok || nested["foo"] != "bar" {
		t.Fatalf("expected nested.foo=bar, got %v", probe["nested"])
	}
}

func TestParseOutput_NoJSONAtAll(t *testing.T) {
	in := "I cannot help with that."
	_, err := parseOutput(in)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no JSON object") {
		t.Fatalf("expected 'no JSON object' error, got: %v", err)
	}
}

func TestParseOutput_MalformedJSON(t *testing.T) {
	// Missing close brace — both bare-attempt and fallback fail.
	in := `{"name": "x"`
	_, err := parseOutput(in)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Bare-attempt fails first; fallback can't find `}` → "no JSON object".
	if !strings.Contains(err.Error(), "no JSON object") {
		t.Fatalf("expected 'no JSON object' error, got: %v", err)
	}
}

func TestParseOutput_MalformedInsideOutermostBrackets(t *testing.T) {
	// Brackets present, but content between them is junk. Both
	// the bare-attempt and the {...} fallback should fail. The
	// fallback's specific error path is "malformed JSON".
	in := `{not json at all}`
	_, err := parseOutput(in)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "malformed JSON") {
		t.Fatalf("expected 'malformed JSON' error, got: %v", err)
	}
}

func TestParseOutput_EmptyString(t *testing.T) {
	_, err := parseOutput("")
	if err == nil {
		t.Fatal("expected error on empty input, got nil")
	}
}

func TestParseOutput_OnlyWhitespace(t *testing.T) {
	_, err := parseOutput("   \n\t  ")
	if err == nil {
		t.Fatal("expected error on whitespace-only, got nil")
	}
}

func TestParseOutput_FencesWithExtraText(t *testing.T) {
	// Fenced JSON but with prose AFTER the closing fence. parseOutput
	// strips one trailing fence; remaining prose makes JSON.Unmarshal
	// fail, but the fallback finds the {...}.
	in := "```json\n{\"a\":1}\n```\n\nPretty cool, right?"
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v\nraw: %s", err, in)
	}
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
}

func TestParseOutput_ConverterNotesArrayShape(t *testing.T) {
	// Real-world output has `_converter_notes: string[]`. Make sure
	// parseOutput passes through arrays correctly inside the JSON.
	in := `{
  "name": "x",
  "custom_css": "body{}",
  "_converter_notes": [
    "Replaced .mes → .nest-msg in 12 places",
    "Removed .drawer-content (ST-only)"
  ]
}`
	out, err := parseOutput(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var probe struct {
		ConverterNotes []string `json:"_converter_notes"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("decode notes: %v", err)
	}
	if len(probe.ConverterNotes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(probe.ConverterNotes))
	}
}
