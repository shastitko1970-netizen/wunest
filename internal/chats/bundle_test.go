package chats

import (
	"strings"
	"testing"

	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/presets"
)

// Helpers ──────────────────────────────────────────────────────────

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func simpleBundle() *presets.OpenAIBundleData {
	return &presets.OpenAIBundleData{
		Prompts: []presets.PromptBlock{
			{Identifier: "main", Name: "Main", Role: "system", Content: "You are {{char}}."},
			{Identifier: "rules", Name: "Rules", Role: "system", Content: "Be consistent."},
			{Identifier: "chatHistory"},
			{Identifier: "prefill", Name: "Prefill", Role: "assistant", Content: "Okay,", InjectionPosition: intPtr(1), InjectionDepth: intPtr(0)},
		},
		PromptOrder: []presets.PromptOrderGroup{
			{CharacterID: 100001, Order: []presets.PromptOrderEntry{
				{Identifier: "main", Enabled: true},
				{Identifier: "rules", Enabled: true},
				{Identifier: "chatHistory", Enabled: true},
				{Identifier: "prefill", Enabled: true},
			}},
		},
	}
}

// Bundle fallback ──────────────────────────────────────────────────

func TestBuild_NoBundle_UsesLegacyPath(t *testing.T) {
	in := PromptInput{
		Character: &characters.Character{Name: "Ghost", Data: characters.CharacterData{Name: "Ghost", Description: "Silent."}},
		History:   []Message{{Role: RoleUser, Content: "Hi"}},
	}
	out := Build(in)
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if out[0].Role != "system" {
		t.Errorf("legacy path should emit system first, got role=%s", out[0].Role)
	}
}

// Bundle basic assembly ───────────────────────────────────────────

func TestBuildBundleMessages_Basic(t *testing.T) {
	in := PromptInput{
		Character: &characters.Character{Name: "Ghost"},
		UserName:  "Pete",
		History: []Message{
			{Role: RoleUser, Content: "Hello"},
		},
		Bundle: simpleBundle(),
	}
	out := Build(in)
	if len(out) < 3 {
		t.Fatalf("expected at least 3 messages (system + user + prefill), got %d", len(out))
	}
	if out[0].Role != "system" {
		t.Errorf("first message should be system, got %s", out[0].Role)
	}
	if !strings.Contains(out[0].Content, "You are Ghost.") {
		t.Errorf("{{char}} macro not substituted: %q", out[0].Content)
	}
	if !strings.Contains(out[0].Content, "Be consistent.") {
		t.Errorf("rules block missing: %q", out[0].Content)
	}
	// Prefill injected at depth 0 = at the very end.
	if out[len(out)-1].Role != "assistant" {
		t.Errorf("last message should be assistant prefill, got %s", out[len(out)-1].Role)
	}
}

// Disabled prompts don't appear ────────────────────────────────────

func TestBuildBundleMessages_DisabledSkipped(t *testing.T) {
	b := simpleBundle()
	b.PromptOrder[0].Order[1].Enabled = false // disable "rules"
	in := PromptInput{
		Character: &characters.Character{Name: "G"},
		History:   []Message{{Role: RoleUser, Content: "Hi"}},
		Bundle:    b,
	}
	out := Build(in)
	if strings.Contains(out[0].Content, "Be consistent.") {
		t.Errorf("disabled prompt should not appear in system: %q", out[0].Content)
	}
}

// Marker resolution: charDescription pulls from character ─────────

func TestBuildBundleMessages_CharDescriptionMarker(t *testing.T) {
	b := &presets.OpenAIBundleData{
		Prompts: []presets.PromptBlock{
			{Identifier: "main", Role: "system", Content: "Intro."},
			{Identifier: "charDescription"}, // marker — no content
			{Identifier: "chatHistory"},
		},
		PromptOrder: []presets.PromptOrderGroup{
			{CharacterID: 100001, Order: []presets.PromptOrderEntry{
				{Identifier: "main", Enabled: true},
				{Identifier: "charDescription", Enabled: true},
				{Identifier: "chatHistory", Enabled: true},
			}},
		},
	}
	in := PromptInput{
		Character: &characters.Character{
			Name: "Rune",
			Data: characters.CharacterData{Name: "Rune", Description: "A masked wanderer."},
		},
		History: []Message{{Role: RoleUser, Content: "Hi"}},
		Bundle:  b,
	}
	out := Build(in)
	if !strings.Contains(out[0].Content, "A masked wanderer.") {
		t.Errorf("charDescription marker should inject character.description: %q", out[0].Content)
	}
}

// Squash system merges adjacent system turns ──────────────────────

func TestSquashSystem_MergesRun(t *testing.T) {
	in := []ChatMessage{
		{Role: "system", Content: "A"},
		{Role: "system", Content: "B"},
		{Role: "user", Content: "hi"},
		{Role: "system", Content: "C"},
	}
	out := squashSystem(in)
	// Expect: merged A+B system, then user, then standalone system C.
	if len(out) != 3 {
		t.Fatalf("expected 3 messages after squash, got %d", len(out))
	}
	if out[0].Content != "A\n\nB" {
		t.Errorf("adjacent system not merged: %q", out[0].Content)
	}
	if out[1].Role != "user" {
		t.Errorf("user should be preserved in second slot")
	}
	if out[2].Content != "C" {
		t.Errorf("trailing system should remain")
	}
}

// Pick order group prefers character_id=100001 wildcard ───────────

func TestPickOrderGroup_PrefersWildcard(t *testing.T) {
	groups := []presets.PromptOrderGroup{
		{CharacterID: 42, Order: []presets.PromptOrderEntry{{Identifier: "perChar"}}},
		{CharacterID: 100001, Order: []presets.PromptOrderEntry{{Identifier: "wildcard"}}},
	}
	picked := pickOrderGroup(groups)
	if len(picked) != 1 || picked[0].Identifier != "wildcard" {
		t.Errorf("wildcard group should win, got %+v", picked)
	}
}

// Regex tests ─────────────────────────────────────────────────────

func TestApplyRegexToUserInput_SpaceInvisibleSwap(t *testing.T) {
	bundle := &presets.OpenAIBundleData{
		Extensions: presets.ExtensionsBundle{
			RegexScripts: []presets.RegexScript{
				{
					ScriptName:    "jailbreakuser",
					FindRegex:     "/ /g",
					ReplaceString: "⠀",
					Placement:     []int{1},
				},
			},
		},
	}
	out := ApplyRegexToUserInput(bundle, "hi there")
	if !strings.Contains(out, "⠀") {
		t.Errorf("regex should replace spaces with unicode invisible: %q", out)
	}
}

func TestApplyRegexToAIOutput_StripHTML(t *testing.T) {
	bundle := &presets.OpenAIBundleData{
		Extensions: presets.ExtensionsBundle{
			RegexScripts: []presets.RegexScript{
				{
					ScriptName:    "Hide html",
					FindRegex:     "/<[^>]+>/g",
					ReplaceString: "",
					Placement:     []int{2},
				},
			},
		},
	}
	out := ApplyRegexToAIOutput(bundle, "hello <b>world</b>!")
	if strings.Contains(out, "<") {
		t.Errorf("HTML should be stripped: %q", out)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Errorf("text content lost: %q", out)
	}
}

func TestApplyRegex_DisabledScriptSkipped(t *testing.T) {
	bundle := &presets.OpenAIBundleData{
		Extensions: presets.ExtensionsBundle{
			RegexScripts: []presets.RegexScript{
				{FindRegex: "/./g", ReplaceString: "X", Placement: []int{1}, Disabled: true},
			},
		},
	}
	out := ApplyRegexToUserInput(bundle, "hello")
	if out != "hello" {
		t.Errorf("disabled script should not run: %q", out)
	}
}

func TestApplyRegex_WrongPlacementSkipped(t *testing.T) {
	bundle := &presets.OpenAIBundleData{
		Extensions: presets.ExtensionsBundle{
			RegexScripts: []presets.RegexScript{
				{FindRegex: "/./g", ReplaceString: "X", Placement: []int{2}}, // only AI output
			},
		},
	}
	out := ApplyRegexToUserInput(bundle, "hello")
	if out != "hello" {
		t.Errorf("wrong-placement script should not run on user input: %q", out)
	}
}

// compileSTRegex translates JS-style flags correctly ──────────────

func TestCompileSTRegex_CaseInsensitive(t *testing.T) {
	re, err := compileSTRegex("/HELLO/i")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("hello") {
		t.Errorf("case-insensitive flag should match mixed case")
	}
}

func TestCompileSTRegex_PlainPattern(t *testing.T) {
	re, err := compileSTRegex(`\d+`)
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("abc123") {
		t.Errorf("plain pattern should match")
	}
}
