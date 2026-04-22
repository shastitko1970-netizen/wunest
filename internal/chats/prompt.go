package chats

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/worldinfo"
)

// ChatMessage is the shape we send upstream to WuApi (OpenAI-compatible).
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// PromptInput bundles everything needed to assemble the outbound messages[].
type PromptInput struct {
	Character            *characters.Character // may be nil for character-less chats
	History              []Message             // chronological, hidden already filtered
	UserName             string                // persona or WuApi first_name fallback
	UserDesc             string                // persona description, may be empty
	SystemPromptOverride string                // if non-empty, replaces the character-derived system message entirely
	Worlds               []*worldinfo.World    // lorebooks attached to this chat/character
	AuthorsNote          *AuthorsNote          // optional mid-history injection
}

// AuthorsNote is a block of prose injected into the prompt at a specific
// `Depth` from the end of history. Depth 0 = after the last message (so
// right before the model's next reply). Depth 1 = before the last message.
// Mirrors SillyTavern's semantics. Role defaults to "system" when empty.
type AuthorsNote struct {
	Content string `json:"content"`
	Depth   int    `json:"depth"`
	Role    string `json:"role,omitempty"` // "system" | "user" | "assistant"
}

// Build returns the OpenAI-compatible messages[] to send to WuApi.
//
// Assembly order (simple V1):
//  1. System message: character description / personality / scenario
//     (with macros {{char}}, {{user}}, {{random::a,b}} substituted).
//  2. For each history entry: map Role → "user" | "assistant" and copy content.
//     Macros are substituted in every turn, so inserts from the UI still work.
//
// Intentionally omitted in this version (will land later):
//   - Author's Note depth injection
//   - Regex input rules
//   - Mes example few-shot priming
//   - Instruct-mode wrapping (for text-completion backends)
//
// World Info / Lorebook activation runs inside buildSystem.
func Build(in PromptInput) []ChatMessage {
	out := make([]ChatMessage, 0, len(in.History)+1)

	// SystemPromptOverride wins over everything else. Comes from the chat's
	// sampler preset, for when a user wants to wipe the character's default
	// system message (e.g. strict-style or stripped-down prompts).
	if override := strings.TrimSpace(in.SystemPromptOverride); override != "" {
		out = append(out, ChatMessage{
			Role:    "system",
			Content: SubstituteMacros(override, in),
		})
	} else if sys := buildSystem(in); sys != "" {
		out = append(out, ChatMessage{Role: "system", Content: sys})
	}

	for _, m := range in.History {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		content = SubstituteMacros(content, in)
		out = append(out, ChatMessage{
			Role:    string(m.Role),
			Content: content,
		})
	}

	// Author's Note — inject at `Depth` positions from the end. Depth 0 means
	// append after the last turn; depth 1 means "one turn back"; depth ≥ len
	// clamps to right after the initial system message. We splice rather than
	// prepend so the injection is actually mid-history, matching ST's model.
	if in.AuthorsNote != nil && strings.TrimSpace(in.AuthorsNote.Content) != "" {
		role := in.AuthorsNote.Role
		if role == "" {
			role = "system"
		}
		note := ChatMessage{
			Role:    role,
			Content: SubstituteMacros(in.AuthorsNote.Content, in),
		}
		// Insert position is counted from the END of the messages slice. With
		// a leading system turn, guarantee the note lands at or after index 1
		// (never before the sys prompt). Clamp to [0, len(out)].
		insertAt := len(out) - in.AuthorsNote.Depth
		if insertAt < 0 {
			insertAt = 0
		}
		if insertAt > len(out) {
			insertAt = len(out)
		}
		if len(out) > 0 && out[0].Role == "system" && insertAt < 1 {
			insertAt = 1
		}
		out = append(out[:insertAt], append([]ChatMessage{note}, out[insertAt:]...)...)
	}

	return out
}

// buildSystem concatenates the character's description, personality,
// scenario, and any activated Lorebook entries into a single system prompt.
// Layout:
//
//	[ activated WI entries with position=before_char ]
//	[ character's own system_prompt override ]
//	[ description ]
//	Personality: [personality]
//	Scenario: [scenario]
//	About the user: [user desc]
//	[ activated WI entries with position=after_char ]
//
// Empty fields are skipped.
func buildSystem(in PromptInput) string {
	// Lorebook activation runs before we touch the character so both branches
	// (character present / absent) benefit. Recent user/assistant texts drive
	// keyword matches.
	act := activateWorlds(in)

	var b strings.Builder
	for _, block := range act.BeforeChar {
		b.WriteString(SubstituteMacros(block, in))
		b.WriteString("\n\n")
	}

	if in.Character != nil {
		data := in.Character.Data
		// Character-provided override goes first verbatim.
		if s := strings.TrimSpace(data.SystemPrompt); s != "" {
			b.WriteString(SubstituteMacros(s, in))
			b.WriteString("\n\n")
		}
		if s := strings.TrimSpace(data.Description); s != "" {
			b.WriteString(SubstituteMacros(s, in))
			b.WriteString("\n\n")
		}
		if s := strings.TrimSpace(data.Personality); s != "" {
			b.WriteString("Personality: ")
			b.WriteString(SubstituteMacros(s, in))
			b.WriteString("\n\n")
		}
		if s := strings.TrimSpace(data.Scenario); s != "" {
			b.WriteString("Scenario: ")
			b.WriteString(SubstituteMacros(s, in))
			b.WriteString("\n\n")
		}
	}

	if ud := strings.TrimSpace(in.UserDesc); ud != "" {
		b.WriteString("About the user: ")
		b.WriteString(ud)
		b.WriteString("\n\n")
	}

	for _, block := range act.AfterChar {
		b.WriteString(SubstituteMacros(block, in))
		b.WriteString("\n\n")
	}

	return strings.TrimSpace(b.String())
}

// activateWorlds runs the WI engine across attached lorebooks.
// Returns a zero-value Activated when no worlds are attached.
func activateWorlds(in PromptInput) worldinfo.Activated {
	if len(in.Worlds) == 0 {
		return worldinfo.Activated{}
	}
	// Feed the tail of history (user + assistant turns only) into the scanner.
	recent := make([]string, 0, len(in.History))
	for _, m := range in.History {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			continue
		}
		if s := strings.TrimSpace(m.Content); s != "" {
			recent = append(recent, s)
		}
	}
	return worldinfo.Activate(worldinfo.ActivationInput{
		Books:  in.Worlds,
		Recent: recent,
	})
}

// SubstituteMacros replaces the basic set of {{...}} macros.
//
// Supported in V1:
//
//	{{user}}            — PromptInput.UserName
//	{{char}}            — Character.Name (or "character")
//	{{random::a,b,c}}   — uniformly random choice
//	{{pick::a,b,c}}     — alias of random
//	{{roll::NdM}}       — sum of N rolls of dM (e.g. {{roll::2d6}}); also {{roll::d20}}
//
// More macros ({{getvar}}, {{setvar}}, {{time}}, {{date}}) land in M4 together
// with the variables scope plumbing. Unsupported macros are left as literals
// so users see why they didn't expand.
func SubstituteMacros(s string, in PromptInput) string {
	s = strings.ReplaceAll(s, "{{user}}", displayName(in.UserName, "User"))
	s = strings.ReplaceAll(s, "{{User}}", displayName(in.UserName, "User"))
	s = strings.ReplaceAll(s, "{{USER}}", strings.ToUpper(displayName(in.UserName, "User")))

	charName := "character"
	if in.Character != nil && in.Character.Name != "" {
		charName = in.Character.Name
	}
	s = strings.ReplaceAll(s, "{{char}}", charName)
	s = strings.ReplaceAll(s, "{{Char}}", charName)
	s = strings.ReplaceAll(s, "{{CHAR}}", strings.ToUpper(charName))

	s = macroRandom.ReplaceAllStringFunc(s, expandRandom)
	s = macroPick.ReplaceAllStringFunc(s, expandRandom)
	s = macroRoll.ReplaceAllStringFunc(s, expandRoll)

	return s
}

var (
	macroRandom = regexp.MustCompile(`\{\{random::([^}]*)\}\}`)
	macroPick   = regexp.MustCompile(`\{\{pick::([^}]*)\}\}`)
	macroRoll   = regexp.MustCompile(`\{\{roll::([^}]*)\}\}`)

	// thinkBlock matches OpenAI o1 / Claude thinking / DeepSeek-R1 style
	// reasoning blocks. Dotall-ish via (?s). Captures are greedy within
	// a single <think>…</think> pair but non-greedy across multiple pairs.
	thinkBlock = regexp.MustCompile(`(?s)<think>(.*?)</think>`)
)

// ExtractThinking separates <think>...</think> blocks from visible content.
// Returns (cleanContent, concatenatedReasoning).
//
// Handles multiple blocks (concatenated with a newline), unbalanced open tag
// (unclosed <think>... through end of text → reasoning), and empty input.
// Surrounding whitespace from removed blocks is collapsed to one newline.
func ExtractThinking(raw string) (content, reasoning string) {
	if raw == "" {
		return "", ""
	}

	// Collect all <think>...</think> payloads in order, then strip them.
	matches := thinkBlock.FindAllStringSubmatchIndex(raw, -1)
	if len(matches) > 0 {
		var reasons []string
		var b strings.Builder
		prev := 0
		for _, m := range matches {
			// m[0]:m[1] is the whole <think>...</think>, m[2]:m[3] is the inner.
			b.WriteString(raw[prev:m[0]])
			reasons = append(reasons, strings.TrimSpace(raw[m[2]:m[3]]))
			prev = m[1]
		}
		b.WriteString(raw[prev:])
		content = collapseGaps(b.String())
		reasoning = strings.Join(reasons, "\n\n")
	} else {
		content = raw
	}

	// Handle unclosed <think> that extends to end-of-text (happens with
	// truncated streams or models that forget the closing tag).
	if open := strings.Index(content, "<think>"); open >= 0 {
		unclosed := strings.TrimSpace(content[open+len("<think>"):])
		if unclosed != "" {
			if reasoning != "" {
				reasoning += "\n\n"
			}
			reasoning += unclosed
		}
		content = strings.TrimSpace(content[:open])
	}

	return content, reasoning
}

// collapseGaps replaces runs of 3+ newlines (which happen after stripping a
// block surrounded by blank lines) with just two newlines. Cosmetic only.
func collapseGaps(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

func expandRandom(match string) string {
	// Strip "{{random::" or "{{pick::" prefix and "}}" suffix.
	inner := match
	if strings.HasPrefix(inner, "{{random::") {
		inner = strings.TrimPrefix(inner, "{{random::")
	} else {
		inner = strings.TrimPrefix(inner, "{{pick::")
	}
	inner = strings.TrimSuffix(inner, "}}")

	options := strings.Split(inner, ",")
	// Trim each option.
	for i := range options {
		options[i] = strings.TrimSpace(options[i])
	}
	if len(options) == 0 {
		return match
	}
	idx := randIntN(len(options))
	return options[idx]
}

// expandRoll supports NdM and dM syntax — N is optional count, defaults to 1.
// Example: "{{roll::2d6}}" → "7", "{{roll::d20}}" → "13".
func expandRoll(match string) string {
	inner := strings.TrimPrefix(match, "{{roll::")
	inner = strings.TrimSuffix(inner, "}}")
	inner = strings.ToLower(strings.TrimSpace(inner))

	parts := strings.SplitN(inner, "d", 2)
	if len(parts) != 2 {
		return match
	}
	n := 1
	if parts[0] != "" {
		v, err := strconv.Atoi(parts[0])
		if err != nil || v <= 0 || v > 100 {
			return match
		}
		n = v
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m <= 0 || m > 10000 {
		return match
	}

	total := 0
	for i := 0; i < n; i++ {
		total += randIntN(m) + 1
	}
	return strconv.Itoa(total)
}

func randIntN(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func displayName(primary, fallback string) string {
	if s := strings.TrimSpace(primary); s != "" {
		return s
	}
	return fallback
}
