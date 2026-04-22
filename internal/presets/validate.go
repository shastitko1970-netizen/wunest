package presets

import (
	"encoding/json"
	"errors"
	"fmt"
)

// validatePresetData does soft validation on the raw JSON blob destined for
// nest_presets.data. "Soft" = we accept unknown fields (presets have grown
// organically and ST pushes extra knobs all the time), but reject obvious
// garbage so a client bug doesn't corrupt a row that future loads can't
// parse.
//
// Rules:
//   - Must be a JSON object (not array / scalar / null). Empty {} is fine.
//   - For known fields on each type, check the JSON value kind matches the
//     Go type (number / string / array / boolean). Don't enforce value
//     ranges — editor does that.
func validatePresetData(t PresetType, data json.RawMessage) error {
	if len(data) == 0 {
		return nil // caller will default to "{}"
	}
	if isNull(data) {
		return errors.New("must be a JSON object, got null")
	}
	// Step 1: must be a JSON object.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("must be a JSON object: %w", err)
	}
	if probe == nil {
		return errors.New("must be a JSON object, got null")
	}

	switch t {
	case TypeSampler, TypeOpenAI:
		return validateSamplerLike(probe)
	case TypeInstruct:
		return validateStringFields(probe, "input_sequence", "output_sequence", "system_sequence",
			"first_input_sequence", "last_input_sequence", "first_output_sequence",
			"last_output_sequence", "stop_sequence", "activation_regex", "user_alignment_message")
	case TypeContext:
		if err := validateStringFields(probe, "story_string", "example_separator", "chat_start"); err != nil {
			return err
		}
		return validateBoolFields(probe, "use_stop_strings", "names_as_stop_strings", "single_line",
			"trim_sentences", "always_force_name2")
	case TypeSysprompt:
		return validateStringFields(probe, "content", "post_history", "post_history_instructions")
	case TypeReasoning:
		return validateStringFields(probe, "prefix", "suffix", "separator")
	}
	// Unknown type — let it pass; server already rejected unknown types
	// earlier in the handler.
	return nil
}

// validateSamplerLike covers both "sampler" and legacy "openai" types.
// Number fields must be numbers (including null). Stop must be an array of
// strings. Booleans must be booleans. Strings must be strings.
func validateSamplerLike(m map[string]json.RawMessage) error {
	numberFields := []string{
		"temperature", "top_p", "top_k", "min_p",
		"max_tokens", "openai_max_tokens",
		"frequency_penalty", "presence_penalty", "repetition_penalty",
		"seed",
	}
	for _, f := range numberFields {
		if err := assertNumberOrNull(m, f); err != nil {
			return err
		}
	}
	if raw, ok := m["stop"]; ok && !isNull(raw) {
		var stop []string
		if err := json.Unmarshal(raw, &stop); err != nil {
			// Accept a single string too — some ST exports do that.
			var s string
			if err2 := json.Unmarshal(raw, &s); err2 != nil {
				return fmt.Errorf("stop must be string[] or string, got %s", shortPreview(raw))
			}
		}
	}
	if err := assertBoolOrNull(m, "reasoning_enabled"); err != nil {
		return err
	}
	if err := validateStringFields(m, "system_prompt"); err != nil {
		return err
	}
	return nil
}

func validateStringFields(m map[string]json.RawMessage, fields ...string) error {
	for _, f := range fields {
		raw, ok := m[f]
		if !ok || isNull(raw) {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("%s must be a string, got %s", f, shortPreview(raw))
		}
	}
	return nil
}

func validateBoolFields(m map[string]json.RawMessage, fields ...string) error {
	for _, f := range fields {
		raw, ok := m[f]
		if !ok || isNull(raw) {
			continue
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return fmt.Errorf("%s must be a boolean, got %s", f, shortPreview(raw))
		}
	}
	return nil
}

func assertNumberOrNull(m map[string]json.RawMessage, f string) error {
	raw, ok := m[f]
	if !ok || isNull(raw) {
		return nil
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("%s must be a number, got %s", f, shortPreview(raw))
	}
	return nil
}

func assertBoolOrNull(m map[string]json.RawMessage, f string) error {
	raw, ok := m[f]
	if !ok || isNull(raw) {
		return nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return fmt.Errorf("%s must be boolean, got %s", f, shortPreview(raw))
	}
	return nil
}

func isNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}

// shortPreview returns the raw value truncated to 32 chars for error
// messages, so a user sending a giant blob doesn't get the whole thing
// echoed back in a 400 response.
func shortPreview(raw json.RawMessage) string {
	s := string(raw)
	if len(s) > 32 {
		s = s[:32] + "…"
	}
	return s
}

// Sentinel to help tests check shape — not used by handlers.
var ErrInvalidPresetData = errors.New("invalid preset data")
