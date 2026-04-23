package chats

import (
	"sort"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/presets"
)

// BuildBundleMessages assembles OpenAI-chat messages[] from an ST-style
// OpenAIBundleData preset. Returns nil when the bundle lacks the Prompt
// Manager fields (no prompts[] / no prompt_order[]) — the caller should
// then fall back to the legacy Build() path.
//
// Semantics (matching SillyTavern as closely as practical):
//
//   1. Walk prompt_order (pick the wildcard character_id=100001 group, or
//      the first group if wildcard is missing) in array order.
//   2. For each entry with enabled=true, look up the prompt by identifier.
//   3. Marker identifiers (charDescription, chatHistory, worldInfoBefore,
//      ...) resolve to runtime content from the chat context (character,
//      persona, lorebook). Non-marker identifiers use the preset's
//      prompt.content verbatim.
//   4. injection_position = 0 → content merged into the big system message
//      (joined with \n\n in prompt_order sequence, roles collapsed to
//      "system").
//   5. injection_position = 1 → content injected at injection_depth from
//      the end of messages[] after history is in place (Author's-Note
//      style depth).
//   6. assistant_prefill → appended as a trailing assistant message so the
//      provider continues from that prefix (Claude pattern).
//
// Things we deliberately DON'T do here (yet):
//   - squash_system_messages: optional post-pass that collapses adjacent
//     system turns. See squashSystem() helper below.
//   - impersonation / new_chat / new_example / group_nudge prompts: ST
//     uses those for UI features WuNest doesn't have (impersonate, group
//     chats). Fields preserved on round-trip, just not applied.
//   - wrap_in_quotes, names_behavior: display-layer transforms.
func BuildBundleMessages(bundle *presets.OpenAIBundleData, in PromptInput) []ChatMessage {
	if bundle == nil || len(bundle.Prompts) == 0 {
		return nil
	}
	orderEntries := pickOrderGroup(bundle.PromptOrder)
	if len(orderEntries) == 0 {
		return nil
	}

	// identifier → prompt index for O(1) lookups.
	promptsByID := make(map[string]*presets.PromptBlock, len(bundle.Prompts))
	for i := range bundle.Prompts {
		p := &bundle.Prompts[i]
		promptsByID[p.Identifier] = p
	}

	// Accumulator for the big position=0 (system) block. We keep the
	// order from prompt_order so users can reorder blocks and see the
	// effect on the output.
	var systemParts []string

	// Position=1 injections — collected and spliced into messages[]
	// after history has been appended.
	type relativeInject struct {
		msg   ChatMessage
		depth int
		order int
	}
	var relatives []relativeInject

	// "chatHistory" marker determines where history is placed. If the
	// marker is absent from prompt_order, we assume the natural ST
	// behavior: history comes right after the system block.
	_ = relatives // silence unused until we append later

	for _, entry := range orderEntries {
		if !entry.Enabled {
			continue
		}
		p, known := promptsByID[entry.Identifier]

		// Markers that only signal a position — content comes from runtime.
		if isPositionMarker(entry.Identifier) {
			// chatHistory is a pure position marker (message boundary) —
			// doesn't contribute content, handled by message ordering.
			if entry.Identifier == "chatHistory" {
				continue
			}
			resolved := resolveContentMarker(entry.Identifier, in)
			if resolved != "" {
				systemParts = append(systemParts, SubstituteMacros(resolved, in))
			}
			continue
		}

		// Non-marker identifier with no prompt block — unlikely but guard.
		if !known {
			continue
		}

		content := SubstituteMacros(p.Content, in)
		if strings.TrimSpace(content) == "" {
			continue
		}

		pos := 0
		if p.InjectionPosition != nil {
			pos = *p.InjectionPosition
		}
		role := p.Role
		if role == "" {
			role = "system"
		}

		if pos == 1 {
			depth := 0
			if p.InjectionDepth != nil {
				depth = *p.InjectionDepth
			}
			ord := 0
			if p.InjectionOrder != nil {
				ord = *p.InjectionOrder
			}
			relatives = append(relatives, relativeInject{
				msg:   ChatMessage{Role: role, Content: content},
				depth: depth,
				order: ord,
			})
			continue
		}

		// Position=0: merge into system block. Role is collapsed to
		// "system" for the merged block; separate non-system pos=0
		// prompts (rare) get their own messages appended after the
		// merged system block.
		if role == "system" {
			systemParts = append(systemParts, content)
		} else {
			// Rare: non-system prompt at position 0 — emit as its own
			// message after the merged system block. Collect here,
			// flush after systemParts.
			relatives = append(relatives, relativeInject{
				msg:   ChatMessage{Role: role, Content: content},
				depth: 0, // means "right before the reply" for pos=1 semantics
				order: -1, // sentinel: append at end instead of depth-splice
			})
		}
	}

	// ── Assemble messages[] ──────────────────────────────
	out := make([]ChatMessage, 0, 2+len(in.History)+len(relatives))

	// 1. Merged system block.
	if len(systemParts) > 0 {
		out = append(out, ChatMessage{
			Role:    "system",
			Content: strings.Join(systemParts, "\n\n"),
		})
	}

	// 2. History.
	for _, m := range in.History {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		out = append(out, ChatMessage{
			Role:    string(m.Role),
			Content: SubstituteMacros(content, in),
		})
	}

	// 3. Author's Note from the chat (independent of bundle).
	if in.AuthorsNote != nil && strings.TrimSpace(in.AuthorsNote.Content) != "" {
		role := in.AuthorsNote.Role
		if role == "" {
			role = "system"
		}
		note := ChatMessage{Role: role, Content: SubstituteMacros(in.AuthorsNote.Content, in)}
		insertAt := len(out) - in.AuthorsNote.Depth
		if insertAt < 1 {
			insertAt = 1
		}
		if insertAt > len(out) {
			insertAt = len(out)
		}
		out = append(out[:insertAt], append([]ChatMessage{note}, out[insertAt:]...)...)
	}

	// 4. Relative injections. Sort by order first (lower = earlier).
	sort.SliceStable(relatives, func(i, j int) bool {
		return relatives[i].order < relatives[j].order
	})
	for _, r := range relatives {
		if r.order == -1 {
			// pos=0 non-system: append at end.
			out = append(out, r.msg)
			continue
		}
		insertAt := len(out) - r.depth
		if insertAt < 1 {
			insertAt = 1 // never before system
		}
		if insertAt > len(out) {
			insertAt = len(out)
		}
		out = append(out[:insertAt], append([]ChatMessage{r.msg}, out[insertAt:]...)...)
	}

	// 5. Assistant prefill — last-message continuation prefix (Claude pattern).
	if prefill := strings.TrimSpace(bundle.AssistantPrefill); prefill != "" {
		out = append(out, ChatMessage{
			Role:    "assistant",
			Content: SubstituteMacros(prefill, in),
		})
	}

	// 6. Optional: squash adjacent system messages (Gemini-style providers
	//    that don't tolerate multiple `role=system`).
	if bundle.SquashSystemMessages != nil && *bundle.SquashSystemMessages {
		out = squashSystem(out)
	}

	return out
}

// pickOrderGroup returns the Order slice we should walk. ST uses
// character_id = 100001 as the "default / any character" group — prefer
// that. Otherwise return the first group we find.
func pickOrderGroup(groups []presets.PromptOrderGroup) []presets.PromptOrderEntry {
	var first []presets.PromptOrderEntry
	for i := range groups {
		g := &groups[i]
		if g.CharacterID == 100001 && len(g.Order) > 0 {
			return g.Order
		}
		if first == nil && len(g.Order) > 0 {
			first = g.Order
		}
	}
	return first
}

// isPositionMarker reports whether the identifier is one of ST's reserved
// positional placeholders. Their content — if any — is resolved from the
// live chat context, not the preset.
func isPositionMarker(id string) bool {
	switch id {
	case "chatHistory",
		"worldInfoBefore", "worldInfoAfter",
		"dialogueExamples",
		"charDescription", "charPersonality",
		"scenario", "personaDescription",
		"jailbreak", "nsfw", "enhanceDefinitions":
		return true
	}
	return false
}

// resolveContentMarker returns the runtime text for a content-producing
// marker (not chatHistory, which is purely positional). Empty string
// means "no content — skip". Called for each enabled marker entry.
func resolveContentMarker(id string, in PromptInput) string {
	switch id {
	case "charDescription":
		if in.Character != nil {
			return strings.TrimSpace(in.Character.Data.Description)
		}
	case "charPersonality":
		if in.Character != nil {
			if s := strings.TrimSpace(in.Character.Data.Personality); s != "" {
				return "Personality: " + s
			}
		}
	case "scenario":
		if in.Character != nil {
			if s := strings.TrimSpace(in.Character.Data.Scenario); s != "" {
				return "Scenario: " + s
			}
		}
	case "personaDescription":
		if s := strings.TrimSpace(in.UserDesc); s != "" {
			return "About the user: " + s
		}
	case "dialogueExamples":
		if in.Character != nil {
			return strings.TrimSpace(in.Character.Data.MesExample)
		}
	case "worldInfoBefore":
		act := activateWorlds(in)
		if len(act.BeforeChar) > 0 {
			return strings.Join(act.BeforeChar, "\n\n")
		}
	case "worldInfoAfter":
		act := activateWorlds(in)
		if len(act.AfterChar) > 0 {
			return strings.Join(act.AfterChar, "\n\n")
		}
	case "nsfw":
		// ST's default "NSFW prompt" — users rarely override. Kept
		// minimal so enabling the marker doesn't surprise people with
		// unexpected content guidance.
		return "NSFW content is allowed in this roleplay."
	case "jailbreak", "enhanceDefinitions":
		// Blank by default. Users provide their own via standalone
		// prompt blocks with these identifiers if they want.
		return ""
	}
	return ""
}

// squashSystem collapses runs of consecutive `role=system` messages into
// a single one (joined with \n\n). Some providers (notably Gemini) only
// accept one system message. The bundle flag opts in.
func squashSystem(msgs []ChatMessage) []ChatMessage {
	if len(msgs) < 2 {
		return msgs
	}
	out := make([]ChatMessage, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		if m.Role != "system" {
			out = append(out, m)
			continue
		}
		// Collect adjacent system messages.
		parts := []string{m.Content}
		for j := i + 1; j < len(msgs) && msgs[j].Role == "system"; j++ {
			parts = append(parts, msgs[j].Content)
			i = j
		}
		out = append(out, ChatMessage{
			Role:    "system",
			Content: strings.Join(parts, "\n\n"),
		})
	}
	return out
}
