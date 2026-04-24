package chats

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/presets"
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
	Character            *characters.Character // the SPEAKER — whose voice will generate the next turn
	History              []Message             // chronological, hidden already filtered
	UserName             string                // persona or WuApi first_name fallback
	UserDesc             string                // persona description, may be empty
	SystemPromptOverride string                // if non-empty, replaces the character-derived system message entirely
	Worlds               []*worldinfo.World    // lorebooks attached to this chat/character
	AuthorsNote          *AuthorsNote          // optional mid-history injection

	// Summaries are rolling/manual/pinned memory blocks injected into
	// the system prompt before character description. Auto summaries
	// cover messages up to their CoveredThroughMessageID — the handler
	// filters History to drop anything already summarised so the model
	// doesn't see the same content twice.
	Summaries []Summary

	// Variables is a per-chat key→value store used by the
	// {{getvar::name}} / {{setvar::name::value}} macros. Snapshotted
	// from chat_metadata.variables at handler time; SubstituteMacros
	// may MUTATE this map (setvar is a side-effectful macro) — the
	// handler persists the mutated map back to chat_metadata after
	// the generation completes. Nil means "no variables support on
	// this request" — macros still parse but getvar returns empty
	// and setvar is a no-op.
	Variables map[string]string

	// Now is the wall-clock at prompt build time. Injected by the
	// handler so {{time}} / {{date}} / {{weekday}} / {{idle_duration}}
	// macros use a consistent point-in-time even if the build drags
	// on for a few ms. Zero value ⇒ macros fall back to time.Now()
	// at each call.
	Now time.Time

	// OtherCharacters are the non-speaking participants of a group chat.
	// When non-empty:
	//   - A "scene manifest" block is injected into the system prompt
	//     listing them, so the speaker knows who else is in the room.
	//   - Assistant messages in History are prefixed with "{name}: " when
	//     their character_id matches one of these (or the speaker) — helps
	//     the model keep voice attribution straight.
	// Single-char chats leave this nil/empty; behaviour is unchanged.
	OtherCharacters []*characters.Character

	// Bundle is the ST-style full preset (prompts + prompt_order + regex
	// + per-provider flags) if the user's active sampler preset carries
	// those fields. When non-nil with a populated prompts/prompt_order,
	// Build() delegates to BuildBundleMessages for full ST-semantics
	// prompt assembly. Falls back to the legacy path otherwise so old
	// skinny presets keep working unchanged.
	Bundle *presets.OpenAIBundleData
}

// IsGroupChat is shorthand for "this prompt build is happening inside a
// chat with 2+ characters". Prompt assembly uses it to enable name
// prefixing in history + inject the scene manifest.
func (in PromptInput) IsGroupChat() bool {
	return len(in.OtherCharacters) > 0
}

// characterNameByID returns the name of the character with this id if
// it's among the speaker or other participants. Used to prefix history
// messages with a speaker label in group chats.
func (in PromptInput) characterNameByID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	if in.Character != nil && in.Character.ID == *id {
		return in.Character.Name
	}
	for _, c := range in.OtherCharacters {
		if c != nil && c.ID == *id {
			return c.Name
		}
	}
	return ""
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
// Two paths:
//
//   1. Bundle present (M32) — the active sampler preset has prompts[] +
//      prompt_order[] (imported ST-style preset like DarkNet V3). Delegate
//      to BuildBundleMessages which walks prompt_order, resolves marker
//      prompts against character/persona/lorebook, and assembles messages
//      with full ST semantics (injection_position, depth, squash, prefill).
//
//   2. Legacy — the skinny sampler preset or no preset. Build a single
//      system message from character description + persona + lorebook
//      (original behavior).
//
// World Info / Lorebook activation runs inside buildSystem for path 2, and
// inside resolveContentMarker for path 1 (via the worldInfoBefore/After
// markers).
func Build(in PromptInput) []ChatMessage {
	// Bundle path — applies when the preset has a Prompt Manager config.
	if msgs := BuildBundleMessages(in.Bundle, in); msgs != nil {
		return msgs
	}

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

	isGroup := in.IsGroupChat()
	for _, m := range in.History {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		content = SubstituteMacros(content, in)
		// In group chats we prefix assistant messages with the speaker's
		// name so the model keeps attribution straight across many turns.
		// User messages and single-char chats stay bare. If the speaker
		// can't be resolved (e.g. deleted character), fall back to no
		// prefix rather than an awkward "unknown: ...".
		if isGroup && m.Role == RoleAssistant {
			if name := in.characterNameByID(m.CharacterID); name != "" {
				content = name + ": " + content
			}
		}
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

	// Group-chat scene manifest — the speaker learns who else is in the
	// room + short description of each. Without this each character
	// writes as if alone, and the group chat illusion falls apart.
	if manifest := buildGroupManifest(in); manifest != "" {
		b.WriteString(manifest)
		b.WriteString("\n\n")
	}

	// Memory block — rolling auto summary first, then manual notes,
	// then pinned facts. The handler has already filtered `History`
	// to exclude messages covered by the auto summary, so there's no
	// content duplication.
	if memBlock := buildMemoryBlock(in); memBlock != "" {
		b.WriteString(memBlock)
		b.WriteString("\n\n")
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

// buildMemoryBlock emits the "[Memory]" system block for a chat that
// has summaries attached. No-op when no summaries exist.
//
// Layout:
//
//	[Memory]
//	## Rolling summary (through turn N)
//	<auto summary content>
//
//	## Notes
//	- <manual 1>
//	- <manual 2>
//
//	## Key facts (pinned)
//	- <pinned 1>
//
// Roles are stable in that order (rolling → manual → pinned). Macros
// expand inside each — so users can parameterise summaries with
// {{user}} / {{char}} / variables.
func buildMemoryBlock(in PromptInput) string {
	if len(in.Summaries) == 0 {
		return ""
	}
	var auto *Summary
	var manual []Summary
	var pinned []Summary
	for i := range in.Summaries {
		s := &in.Summaries[i]
		switch s.Role {
		case "auto":
			auto = s
		case "manual":
			manual = append(manual, *s)
		case "pinned":
			pinned = append(pinned, *s)
		}
	}
	if auto == nil && len(manual) == 0 && len(pinned) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Memory]\n")
	if auto != nil && strings.TrimSpace(auto.Content) != "" {
		b.WriteString("## Rolling summary of earlier events\n")
		b.WriteString(SubstituteMacros(auto.Content, in))
		b.WriteString("\n\n")
	}
	if len(manual) > 0 {
		b.WriteString("## Notes\n")
		for _, m := range manual {
			b.WriteString("- ")
			b.WriteString(SubstituteMacros(m.Content, in))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(pinned) > 0 {
		b.WriteString("## Key facts (always-on)\n")
		for _, p := range pinned {
			b.WriteString("- ")
			b.WriteString(SubstituteMacros(p.Content, in))
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// buildGroupManifest emits the "other characters in the scene" block for
// group-chat prompts. Returns "" for single-character chats so legacy
// behaviour is byte-identical.
//
// By design we include the FULL description/personality/scenario of each
// other participant. Our users burn WuApi tokens, where even heavy models
// are fine with 2-5× description injection; the coherence gain from
// "Alice knows Bob exists AND remembers his personality" is worth the
// tokens. If a user wants to trim, they can leave short descriptions
// on their character cards.
func buildGroupManifest(in PromptInput) string {
	if !in.IsGroupChat() {
		return ""
	}
	speakerName := ""
	if in.Character != nil {
		speakerName = in.Character.Name
	}
	var b strings.Builder
	b.WriteString("--- Scene participants ---\n")
	b.WriteString("You are currently speaking as ")
	if speakerName != "" {
		b.WriteString(speakerName)
	} else {
		b.WriteString("the current character")
	}
	b.WriteString(". Other characters present in this scene:\n\n")

	for _, c := range in.OtherCharacters {
		if c == nil {
			continue
		}
		b.WriteString("### ")
		b.WriteString(c.Name)
		b.WriteString("\n")
		if desc := strings.TrimSpace(c.Data.Description); desc != "" {
			b.WriteString(SubstituteMacros(desc, in))
			b.WriteString("\n")
		}
		if pers := strings.TrimSpace(c.Data.Personality); pers != "" {
			b.WriteString("Personality: ")
			b.WriteString(SubstituteMacros(pers, in))
			b.WriteString("\n")
		}
		if scen := strings.TrimSpace(c.Data.Scenario); scen != "" {
			b.WriteString("Scenario: ")
			b.WriteString(SubstituteMacros(scen, in))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(`Stay in character as `)
	if speakerName != "" {
		b.WriteString(speakerName)
	} else {
		b.WriteString("your character")
	}
	b.WriteString(`. You may address the user or any other character directly. When the history shows "<Name>: <content>", that's who said what — do not impersonate other characters.`)
	return b.String()
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

// SubstituteMacros replaces the {{...}} macro set.
//
// Pure replacements:
//
//	{{user}}            — PromptInput.UserName
//	{{char}}            — PromptInput.Character.Name (or "character")
//	{{random::a,b,c}}   — uniformly random choice
//	{{pick::a,b,c}}     — alias of random
//	{{roll::NdM}}       — sum of N rolls of dM (e.g. {{roll::2d6}}); also {{roll::d20}}
//	{{time}}            — HH:MM of PromptInput.Now (falls back to time.Now)
//	{{date}}            — YYYY-MM-DD of PromptInput.Now
//	{{weekday}}         — "Monday" / "Tuesday" / …
//	{{idle_duration}}   — pretty gap since the last user message ("3 hours",
//	                      "moments ago"); empty string if no prior user msg
//	{{lastUserMessage}} — most recent user message content, empty if none
//	{{lastCharMessage}} — most recent assistant message content, empty if none
//	{{getvar::name}}    — per-chat variable value; "" when unset
//
// Side-effectful:
//
//	{{setvar::name::value}} — writes to PromptInput.Variables (if non-nil)
//	                          and expands to empty string. Handler persists
//	                          the mutated map after generation.
//
// Unknown macros are left as literals so authors spot typos.
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

	// Choice / dice macros.
	s = macroRandom.ReplaceAllStringFunc(s, expandRandom)
	s = macroPick.ReplaceAllStringFunc(s, expandRandom)
	s = macroRoll.ReplaceAllStringFunc(s, expandRoll)

	// Time macros — snap to PromptInput.Now for build-consistency; fall
	// back to wall-clock when handler didn't set one.
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	s = strings.ReplaceAll(s, "{{time}}", now.Format("15:04"))
	s = strings.ReplaceAll(s, "{{date}}", now.Format("2006-01-02"))
	s = strings.ReplaceAll(s, "{{weekday}}", now.Weekday().String())

	// History-reference macros.
	s = strings.ReplaceAll(s, "{{lastUserMessage}}", lastMessageContent(in.History, RoleUser))
	s = strings.ReplaceAll(s, "{{lastCharMessage}}", lastMessageContent(in.History, RoleAssistant))
	s = strings.ReplaceAll(s, "{{idle_duration}}", formatIdleDuration(in.History, now))

	// Variable macros — getvar pure read, setvar writes to in.Variables
	// then expands to the stored value.
	s = macroGetVar.ReplaceAllStringFunc(s, func(m string) string {
		groups := macroGetVar.FindStringSubmatch(m)
		if len(groups) < 2 {
			return m
		}
		name := strings.TrimSpace(groups[1])
		if in.Variables == nil {
			return ""
		}
		return in.Variables[name]
	})
	s = macroSetVar.ReplaceAllStringFunc(s, func(m string) string {
		groups := macroSetVar.FindStringSubmatch(m)
		if len(groups) < 3 {
			return m
		}
		name := strings.TrimSpace(groups[1])
		value := strings.TrimSpace(groups[2])
		if name == "" {
			return ""
		}
		// Mutate the map in place so the handler can detect + persist.
		// Nil map ⇒ no-op (we can't lazy-allocate because the handler
		// uses the `== nil` sentinel to skip persist).
		if in.Variables != nil {
			in.Variables[name] = value
		}
		// ST convention: setvar expands to empty string (not to the
		// value) so users can use it as a pure side effect without
		// polluting the sentence.
		return ""
	})

	return s
}

// lastMessageContent finds the most recent message in history with the
// given role and returns its content (empty when none exist).
func lastMessageContent(history []Message, role Role) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == role {
			return history[i].Content
		}
	}
	return ""
}

// formatIdleDuration renders the gap between now and the last user
// message in human-friendly prose. Returns "" when there's no prior
// user message (fresh chat). Buckets chosen to read naturally in both
// English and Russian narrative.
func formatIdleDuration(history []Message, now time.Time) string {
	var lastUser *Message
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == RoleUser {
			m := history[i]
			lastUser = &m
			break
		}
	}
	if lastUser == nil {
		return ""
	}
	gap := now.Sub(lastUser.CreatedAt)
	switch {
	case gap < 60*time.Second:
		return "moments ago"
	case gap < 5*time.Minute:
		return "a few minutes ago"
	case gap < time.Hour:
		mins := int(gap.Minutes())
		return fmt.Sprintf("%d minutes ago", mins)
	case gap < 24*time.Hour:
		hrs := int(gap.Hours())
		if hrs == 1 {
			return "an hour ago"
		}
		return fmt.Sprintf("%d hours ago", hrs)
	case gap < 48*time.Hour:
		return "yesterday"
	case gap < 7*24*time.Hour:
		days := int(gap.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	default:
		weeks := int(gap.Hours() / (24 * 7))
		if weeks == 1 {
			return "a week ago"
		}
		if weeks < 8 {
			return fmt.Sprintf("%d weeks ago", weeks)
		}
		return "a long time ago"
	}
}

var (
	macroRandom = regexp.MustCompile(`\{\{random::([^}]*)\}\}`)
	macroPick   = regexp.MustCompile(`\{\{pick::([^}]*)\}\}`)
	macroRoll   = regexp.MustCompile(`\{\{roll::([^}]*)\}\}`)
	// getvar/setvar use `::` separators matching ST convention.
	// Name is [A-Za-z0-9_-]+ (validated loosely — we don't want to
	// over-restrict international users); value in setvar is the
	// rest of the payload up to `}}`.
	macroGetVar = regexp.MustCompile(`\{\{getvar::([^}]+)\}\}`)
	macroSetVar = regexp.MustCompile(`\{\{setvar::([^:}]+)::([^}]*)\}\}`)

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
