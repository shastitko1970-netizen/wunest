package characters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ParseCard is the format-agnostic entry point for character card imports.
// It sniffs the magic bytes and routes to the right decoder:
//
//   - PNG signature         → ParsePNGCard (embedded ccv3/chara tEXt chunk)
//   - leading "{" or "["    → ParseJSONCard (SillyTavern .json export)
//
// SillyTavern can export cards as either a PNG (with metadata in a tEXt
// chunk) or a .json file — both shapes round-trip cleanly through this
// function so the HTTP handler doesn't have to care.
func ParseCard(raw []byte) (*CharacterData, string, error) {
	if len(raw) == 0 {
		return nil, "", errors.New("empty file")
	}
	// PNG always starts with the 8-byte signature.
	if len(raw) >= len(pngSignature) && bytes.Equal(raw[:len(pngSignature)], pngSignature) {
		return ParsePNGCard(raw)
	}
	// Trim leading BOM / whitespace so JSON with a UTF-8 BOM (Windows exports)
	// still parses.
	trimmed := bytes.TrimLeft(raw, "\ufeff \t\r\n")
	if len(trimmed) == 0 {
		return nil, "", errors.New("empty file after trim")
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return ParseJSONCard(trimmed)
	}
	return nil, "", errors.New("unsupported file format — expected PNG card or JSON export")
}

// ParseJSONCard decodes a raw .json character card exported from SillyTavern
// (or any compatible tool). Three shapes are accepted:
//
//  1. V3 wrapper:   {"spec":"chara_card_v3","data":{...}}
//  2. V2 wrapper:   {"spec":"chara_card_v2","data":{...}}
//  3. V2 flat:      {"name":"...","description":"...", ...}
//
// Always normalises to the V3 CharacterData shape; the returned spec
// string is "chara_card_v3" either way so downstream code doesn't branch.
func ParseJSONCard(raw []byte) (*CharacterData, string, error) {
	// (1) and (2) — wrapper shape. Try it first so a flat card with an
	// accidental `data` field doesn't get misrouted.
	var wrapper struct {
		Spec        string        `json:"spec"`
		SpecVersion string        `json:"spec_version"`
		Data        CharacterData `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Data.Name != "" {
		return &wrapper.Data, "chara_card_v3", nil
	}

	// (3) flat V2 shape.
	var data CharacterData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, "", fmt.Errorf("json decode: %w", err)
	}
	if data.Name == "" {
		return nil, "", errors.New("card has empty name")
	}
	return &data, "chara_card_v3", nil
}
