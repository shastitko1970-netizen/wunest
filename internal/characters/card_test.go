package characters

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// ParseCard must dispatch on magic bytes: PNG → ParsePNGCard, { or [ → ParseJSONCard.
func TestParseCard_RoutesByMagic(t *testing.T) {
	// PNG path — reuse the test builder from png_test.go.
	cardJSON, _ := json.Marshal(map[string]any{
		"name":        "Png Bob",
		"description": "from png",
	})
	png := buildTestPNG(t, keyChara, cardJSON)

	gotPng, _, err := ParseCard(png)
	if err != nil {
		t.Fatalf("ParseCard(png): %v", err)
	}
	if gotPng.Name != "Png Bob" {
		t.Errorf("png name: want Png Bob, got %q", gotPng.Name)
	}

	// JSON (flat V2) path.
	flat := []byte(`{"name":"Flat Alice","description":"flat"}`)
	gotFlat, _, err := ParseCard(flat)
	if err != nil {
		t.Fatalf("ParseCard(flat json): %v", err)
	}
	if gotFlat.Name != "Flat Alice" {
		t.Errorf("flat name: want Flat Alice, got %q", gotFlat.Name)
	}

	// JSON (V3 wrapper) path.
	wrapper := []byte(`{"spec":"chara_card_v3","spec_version":"3.0","data":{"name":"Wrap Cara","description":"wrap"}}`)
	gotWrap, spec, err := ParseCard(wrapper)
	if err != nil {
		t.Fatalf("ParseCard(wrapper): %v", err)
	}
	if gotWrap.Name != "Wrap Cara" {
		t.Errorf("wrapper name: want Wrap Cara, got %q", gotWrap.Name)
	}
	if spec != "chara_card_v3" {
		t.Errorf("spec: want chara_card_v3, got %q", spec)
	}
}

// BOM-prefixed JSON (Windows text editors save this way) must still parse.
func TestParseCard_JSONWithBOM(t *testing.T) {
	bom := []byte("\ufeff")
	body := []byte(`{"name":"BOMboy","description":"with bom"}`)
	raw := append(bom, body...)

	got, _, err := ParseCard(raw)
	if err != nil {
		t.Fatalf("ParseCard(bom+json): %v", err)
	}
	if got.Name != "BOMboy" {
		t.Errorf("bom name: want BOMboy, got %q", got.Name)
	}
}

// Binary that isn't PNG and doesn't start with `{` should error with a
// hint, not silently try the wrong parser.
func TestParseCard_UnknownFormat(t *testing.T) {
	_, _, err := ParseCard([]byte("this is not a card"))
	if err == nil {
		t.Fatal("expected error on unknown format")
	}
	if !strings.Contains(err.Error(), "unsupported file format") {
		t.Errorf("error should mention unsupported format, got: %v", err)
	}
}

// Empty body must error explicitly.
func TestParseCard_Empty(t *testing.T) {
	if _, _, err := ParseCard(nil); err == nil {
		t.Fatal("expected error on nil")
	}
	if _, _, err := ParseCard([]byte{}); err == nil {
		t.Fatal("expected error on empty slice")
	}
}

// Regression guard: a V3 wrapper with missing `data.name` should error, not
// return a zero-name card.
func TestParseCard_WrapperWithoutName(t *testing.T) {
	raw := []byte(`{"spec":"chara_card_v3","data":{"description":"nameless"}}`)
	if _, _, err := ParseCard(raw); err == nil {
		t.Fatal("expected error: wrapper has no name, flat parse also empty")
	}
}

// Ensure the PNG decoder still works through the routing layer end-to-end.
// Uses a real base64 encode round-trip like the existing png_test harness.
func TestParseCard_PngWithCcv3Chunk(t *testing.T) {
	cardJSON := []byte(`{"spec":"chara_card_v3","spec_version":"3.0","data":{"name":"CCV3 Dan","description":"ccv3"}}`)
	// Use ccv3 (base64-encoded wrapper in payload).
	png := buildTestPNG(t, keyCCV3, []byte(mustB64(cardJSON)))

	// buildTestPNG base64-encodes once more internally. For ccv3 we want the
	// inner raw JSON to survive decoding — revert: call buildTestPNG with the
	// raw wrapper so it base64's exactly once.
	_ = png
	pngRaw := buildTestPNG(t, keyCCV3, cardJSON)
	got, spec, err := ParseCard(pngRaw)
	if err != nil {
		t.Fatalf("ParseCard(ccv3 png): %v", err)
	}
	if got.Name != "CCV3 Dan" {
		t.Errorf("ccv3 name: %q", got.Name)
	}
	if spec != "chara_card_v3" {
		t.Errorf("spec: %q", spec)
	}
}

// tiny helper: standard base64 of input.
func mustB64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
