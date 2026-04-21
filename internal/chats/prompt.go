package chats

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/shastitko1970-netizen/wunest/internal/characters"
)

// ChatMessage is the shape we send upstream to WuApi (OpenAI-compatible).
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// PromptInput bundles everything needed to assemble the outbound messages[].
type PromptInput struct {
	Character     *characters.Character // may be nil for character-less chats
	History       []Message             // chronological, hidden already filtered
	UserName      string                // persona or WuApi first_name fallback
	UserDesc      string                // persona description, may be empty
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
//   - World Info / Lorebook activation
//   - Author's Note depth injection
//   - Regex input rules
//   - Mes example few-shot priming
//   - Instruct-mode wrapping (for text-completion backends)
//
// This is enough to get a working chat loop going; M4 will add WI/presets.
func Build(in PromptInput) []ChatMessage {
	out := make([]ChatMessage, 0, len(in.History)+1)

	if sys := buildSystem(in); sys != "" {
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

	return out
}

// buildSystem concatenates the character's description, personality, and
// scenario fields into a single system prompt. Empty fields are skipped,
// and a `system_prompt` override (if set on the card) wraps everything.
func buildSystem(in PromptInput) string {
	if in.Character == nil {
		return ""
	}
	data := in.Character.Data
	var b strings.Builder

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
	if ud := strings.TrimSpace(in.UserDesc); ud != "" {
		b.WriteString("About the user: ")
		b.WriteString(ud)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
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
)

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
