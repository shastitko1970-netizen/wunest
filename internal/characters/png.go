package characters

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// pngSignature is the 8-byte PNG file header.
var pngSignature = []byte{137, 80, 78, 71, 13, 10, 26, 10}

// Character-card tEXt chunk keys, in order of preference:
//   - "ccv3"   → V3 spec (chara_card_v3)
//   - "chara"  → V2 spec (legacy)
const (
	chunkTypeText = "tEXt"
	chunkTypeIEND = "IEND"
	keyCCV3       = "ccv3"
	keyChara      = "chara"
)

// ErrNoCardMetadata is returned when the PNG is valid but contains no
// character-card metadata chunk.
var ErrNoCardMetadata = errors.New("png has no character card metadata (no ccv3/chara tEXt chunk)")

// ParsePNGCard extracts and decodes the character card metadata embedded in
// a PNG file. It prefers the V3 (`ccv3`) chunk; falls back to V2 (`chara`).
//
// Returned CharacterData always matches the V3 shape — V2 cards are
// upgraded in-place. The Spec field on the outer Character holds
// "chara_card_v3" in both cases.
func ParsePNGCard(png []byte) (*CharacterData, string, error) {
	if len(png) < len(pngSignature) || !bytes.Equal(png[:len(pngSignature)], pngSignature) {
		return nil, "", errors.New("not a PNG: missing signature")
	}

	r := bytes.NewReader(png[len(pngSignature):])

	// First pass: walk chunks, collect payloads for ccv3 and chara if present.
	var v3Payload, v2Payload []byte

	for {
		var length uint32
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, "", fmt.Errorf("read chunk length: %w", err)
		}

		chunkType := make([]byte, 4)
		if _, err := io.ReadFull(r, chunkType); err != nil {
			return nil, "", fmt.Errorf("read chunk type: %w", err)
		}

		// Guard against malicious/corrupt length values.
		if length > 64*1024*1024 { // 64 MiB cap per chunk — character cards are much smaller
			return nil, "", fmt.Errorf("chunk %q too large: %d bytes", chunkType, length)
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, "", fmt.Errorf("read chunk data: %w", err)
		}

		// Skip CRC (we don't validate).
		if _, err := r.Seek(4, io.SeekCurrent); err != nil {
			return nil, "", fmt.Errorf("skip crc: %w", err)
		}

		ct := string(chunkType)
		if ct == chunkTypeText {
			nullIdx := bytes.IndexByte(data, 0)
			if nullIdx < 1 {
				continue
			}
			keyword := string(data[:nullIdx])
			text := data[nullIdx+1:]
			switch keyword {
			case keyCCV3:
				v3Payload = text
			case keyChara:
				v2Payload = text
			}
		}

		if ct == chunkTypeIEND {
			break
		}
	}

	// Prefer V3 over V2.
	if len(v3Payload) > 0 {
		return decodeV3(v3Payload)
	}
	if len(v2Payload) > 0 {
		return decodeV2(v2Payload)
	}
	return nil, "", ErrNoCardMetadata
}

// decodeV3 decodes a base64+JSON V3 payload.
func decodeV3(payload []byte) (*CharacterData, string, error) {
	raw, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		return nil, "", fmt.Errorf("v3 base64 decode: %w", err)
	}

	var wrapper struct {
		Spec        string        `json:"spec"`
		SpecVersion string        `json:"spec_version"`
		Data        CharacterData `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, "", fmt.Errorf("v3 json decode: %w", err)
	}
	if wrapper.Data.Name == "" {
		return nil, "", errors.New("v3 card has empty name")
	}
	return &wrapper.Data, "chara_card_v3", nil
}

// decodeV2 decodes a base64+JSON payload found under the legacy "chara"
// keyword. The shape is not always V2-flat — CHUB and some other modern
// exporters serve a V3-shape envelope ({"spec":"chara_card_v2","data":{...}})
// under the V2 keyword. We try the wrapper shape first and fall back to flat
// V2 if no name is found.
func decodeV2(payload []byte) (*CharacterData, string, error) {
	raw, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		return nil, "", fmt.Errorf("v2 base64 decode: %w", err)
	}

	// Wrapper shape — used by CHUB and most current exporters.
	var wrapper struct {
		Spec        string        `json:"spec"`
		SpecVersion string        `json:"spec_version"`
		Data        CharacterData `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Data.Name != "" {
		return &wrapper.Data, "chara_card_v3", nil
	}

	// Flat shape — original SillyTavern V2 export.
	var data CharacterData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, "", fmt.Errorf("v2 json decode: %w", err)
	}
	if data.Name == "" {
		return nil, "", errors.New("v2 card has empty name")
	}
	return &data, "chara_card_v3", nil
}
