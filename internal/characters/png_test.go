package characters

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"testing"
)

// buildTestPNG fabricates a minimal PNG containing a single tEXt chunk with
// the given keyword and base64-encoded JSON payload. Enough to unit-test
// ParsePNGCard without a real image library.
func buildTestPNG(t *testing.T, keyword string, cardJSON []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	buf.Write(pngSignature)

	// IHDR (required first chunk per spec; we stuff dummy data).
	writeChunk(&buf, "IHDR", make([]byte, 13))

	// tEXt chunk: keyword + 0 + base64(card json)
	text := make([]byte, 0, len(keyword)+1+len(cardJSON)*2)
	text = append(text, []byte(keyword)...)
	text = append(text, 0)
	enc := base64.StdEncoding.EncodeToString(cardJSON)
	text = append(text, []byte(enc)...)
	writeChunk(&buf, "tEXt", text)

	// IEND terminator.
	writeChunk(&buf, "IEND", nil)

	return buf.Bytes()
}

func writeChunk(buf *bytes.Buffer, chunkType string, data []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	buf.WriteString(chunkType)
	buf.Write(data)
	// CRC over type + data.
	crc := crc32.NewIEEE()
	crc.Write([]byte(chunkType))
	crc.Write(data)
	_ = binary.Write(buf, binary.BigEndian, crc.Sum32())
}

func TestParsePNGCard_V3(t *testing.T) {
	card := map[string]any{
		"spec":         "chara_card_v3",
		"spec_version": "3.0",
		"data": map[string]any{
			"name":        "Alice",
			"description": "A test character.",
			"tags":        []string{"test", "v3"},
		},
	}
	cardJSON, _ := json.Marshal(card)
	png := buildTestPNG(t, keyCCV3, cardJSON)

	data, spec, err := ParsePNGCard(png)
	if err != nil {
		t.Fatalf("ParsePNGCard: %v", err)
	}
	if data.Name != "Alice" {
		t.Errorf("name = %q, want Alice", data.Name)
	}
	if spec != "chara_card_v3" {
		t.Errorf("spec = %q, want chara_card_v3", spec)
	}
	if len(data.Tags) != 2 {
		t.Errorf("tags len = %d, want 2", len(data.Tags))
	}
}

func TestParsePNGCard_V2_UpgradesToV3Shape(t *testing.T) {
	card := map[string]any{
		"name":        "Bob",
		"description": "Legacy V2 card.",
		"first_mes":   "Hello.",
		"tags":        []string{"v2"},
	}
	cardJSON, _ := json.Marshal(card)
	png := buildTestPNG(t, keyChara, cardJSON)

	data, spec, err := ParsePNGCard(png)
	if err != nil {
		t.Fatalf("ParsePNGCard: %v", err)
	}
	if data.Name != "Bob" {
		t.Errorf("name = %q, want Bob", data.Name)
	}
	// V2 inputs are normalized to the V3 outer shape.
	if spec != "chara_card_v3" {
		t.Errorf("spec = %q, want chara_card_v3 (V2→V3 upgrade)", spec)
	}
	if data.FirstMes != "Hello." {
		t.Errorf("first_mes = %q, want Hello.", data.FirstMes)
	}
}

func TestParsePNGCard_NoMetadata(t *testing.T) {
	// Build a PNG with only IHDR+IEND (no tEXt chunk).
	var buf bytes.Buffer
	buf.Write(pngSignature)
	writeChunk(&buf, "IHDR", make([]byte, 13))
	writeChunk(&buf, "IEND", nil)

	_, _, err := ParsePNGCard(buf.Bytes())
	if err != ErrNoCardMetadata {
		t.Errorf("err = %v, want ErrNoCardMetadata", err)
	}
}

func TestParsePNGCard_NotAPNG(t *testing.T) {
	_, _, err := ParsePNGCard([]byte("definitely not a PNG"))
	if err == nil {
		t.Error("expected error for non-PNG input")
	}
}

func TestParsePNGCard_PrefersV3OverV2(t *testing.T) {
	// Build a PNG with BOTH chara and ccv3 chunks; ccv3 should win.
	var buf bytes.Buffer
	buf.Write(pngSignature)
	writeChunk(&buf, "IHDR", make([]byte, 13))

	v2 := map[string]any{"name": "V2-only"}
	v2JSON, _ := json.Marshal(v2)
	writeTextChunk(&buf, keyChara, v2JSON)

	v3 := map[string]any{
		"spec":         "chara_card_v3",
		"spec_version": "3.0",
		"data":         map[string]any{"name": "V3-winner"},
	}
	v3JSON, _ := json.Marshal(v3)
	writeTextChunk(&buf, keyCCV3, v3JSON)

	writeChunk(&buf, "IEND", nil)

	data, spec, err := ParsePNGCard(buf.Bytes())
	if err != nil {
		t.Fatalf("ParsePNGCard: %v", err)
	}
	if data.Name != "V3-winner" {
		t.Errorf("name = %q, expected V3 to win over V2", data.Name)
	}
	if spec != "chara_card_v3" {
		t.Errorf("spec = %q", spec)
	}
}

// writeTextChunk is a tEXt-specific helper reused by multi-chunk tests.
func writeTextChunk(buf *bytes.Buffer, keyword string, cardJSON []byte) {
	text := make([]byte, 0, len(keyword)+1+len(cardJSON)*2)
	text = append(text, []byte(keyword)...)
	text = append(text, 0)
	enc := base64.StdEncoding.EncodeToString(cardJSON)
	text = append(text, []byte(enc)...)
	writeChunk(buf, "tEXt", text)
}
