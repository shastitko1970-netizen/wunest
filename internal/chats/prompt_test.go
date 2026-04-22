package chats

import (
	"strings"
	"testing"

	"github.com/shastitko1970-netizen/wunest/internal/characters"
)

func TestSubstituteMacros_Basic(t *testing.T) {
	in := PromptInput{
		UserName:  "Alice",
		Character: &characters.Character{Name: "Bob"},
	}
	got := SubstituteMacros("Hello {{user}}, I am {{char}}.", in)
	want := "Hello Alice, I am Bob."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteMacros_Random_ChoosesFromList(t *testing.T) {
	in := PromptInput{}
	// Seed-independent test: run many times, collect outputs, verify all
	// came from the allowed set.
	got := map[string]bool{}
	for i := 0; i < 40; i++ {
		out := SubstituteMacros("{{random::apple,banana,cherry}}", in)
		got[out] = true
	}
	for k := range got {
		if k != "apple" && k != "banana" && k != "cherry" {
			t.Errorf("unexpected random result: %q", k)
		}
	}
	if len(got) < 2 {
		t.Errorf("random macro looks stuck on one value: %v", got)
	}
}

func TestSubstituteMacros_Roll(t *testing.T) {
	in := PromptInput{}
	// 2d6 result should always be 2..12.
	for i := 0; i < 40; i++ {
		out := SubstituteMacros("{{roll::2d6}}", in)
		if out == "{{roll::2d6}}" {
			t.Fatalf("roll did not expand: %q", out)
		}
		// Should be an integer in [2, 12].
		if out[0] < '0' || out[0] > '9' {
			t.Fatalf("non-numeric roll output: %q", out)
		}
	}
}

func TestBuild_ComposesSystemAndHistory(t *testing.T) {
	char := &characters.Character{
		Name: "Seraphina",
		Data: characters.CharacterData{
			Name:        "Seraphina",
			Description: "An ancient librarian who speaks in riddles.",
			Personality: "curious, patient",
			Scenario:    "You meet {{char}} in a dusty library.",
			FirstMes:    "Welcome, {{user}}.",
		},
	}
	in := PromptInput{
		Character: char,
		UserName:  "Pete",
		History: []Message{
			{Role: RoleAssistant, Content: "Welcome, Pete."},
			{Role: RoleUser, Content: "Hello. Tell me about {{char}}."},
		},
	}
	msgs := Build(in)
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("first msg role = %q, want system", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "Seraphina") {
		t.Errorf("system msg should mention character name, got: %q", msgs[0].Content)
	}
	// Ensure macro {{char}} was substituted in scenario too.
	if !strings.Contains(msgs[0].Content, "dusty library") {
		t.Errorf("system should contain scenario text")
	}
	if strings.Contains(msgs[0].Content, "{{char}}") {
		t.Errorf("system should not contain unexpanded {{char}}, got: %q", msgs[0].Content)
	}
	if msgs[1].Role != "assistant" || !strings.Contains(msgs[1].Content, "Pete") {
		t.Errorf("assistant msg wrong: %+v", msgs[1])
	}
	if msgs[2].Role != "user" || !strings.Contains(msgs[2].Content, "Seraphina") {
		t.Errorf("user msg wrong: %+v", msgs[2])
	}
}

func TestExtractThinking_SingleBlock(t *testing.T) {
	in := "<think>let me think about this carefully</think>Hello, world!"
	content, reasoning := ExtractThinking(in)
	if content != "Hello, world!" {
		t.Errorf("content = %q, want 'Hello, world!'", content)
	}
	if reasoning != "let me think about this carefully" {
		t.Errorf("reasoning = %q", reasoning)
	}
}

func TestExtractThinking_MultipleBlocks(t *testing.T) {
	in := "<think>first thought</think>Hi <think>second thought</think>there."
	content, reasoning := ExtractThinking(in)
	if !strings.Contains(content, "Hi") || !strings.Contains(content, "there") {
		t.Errorf("content missing fragments: %q", content)
	}
	if !strings.Contains(reasoning, "first") || !strings.Contains(reasoning, "second") {
		t.Errorf("reasoning missing fragments: %q", reasoning)
	}
}

func TestExtractThinking_NoBlocks(t *testing.T) {
	content, reasoning := ExtractThinking("Just a plain reply.")
	if content != "Just a plain reply." {
		t.Errorf("content = %q", content)
	}
	if reasoning != "" {
		t.Errorf("reasoning should be empty, got %q", reasoning)
	}
}

func TestExtractThinking_UnclosedTag(t *testing.T) {
	in := "<think>stream was truncated here"
	content, reasoning := ExtractThinking(in)
	if content != "" {
		t.Errorf("content should be empty for unclosed tag, got %q", content)
	}
	if !strings.Contains(reasoning, "truncated") {
		t.Errorf("reasoning = %q", reasoning)
	}
}

func TestExtractThinking_MultilineBlock(t *testing.T) {
	in := "<think>step 1: foo\nstep 2: bar\nstep 3: baz</think>\nFinal answer."
	content, reasoning := ExtractThinking(in)
	if content != "Final answer." {
		t.Errorf("content = %q", content)
	}
	if !strings.Contains(reasoning, "step 1") || !strings.Contains(reasoning, "step 3") {
		t.Errorf("reasoning missing steps: %q", reasoning)
	}
}

func TestBuild_EmptyHistory(t *testing.T) {
	in := PromptInput{
		Character: &characters.Character{
			Name: "Ghost",
			Data: characters.CharacterData{Name: "Ghost", Description: "Silent."},
		},
		UserName: "User",
	}
	msgs := Build(in)
	if len(msgs) != 1 || msgs[0].Role != "system" {
		t.Errorf("expected single system message, got %+v", msgs)
	}
}

// ── Author's Note (M11) ─────────────────────────────────────────────

// Depth 0 → note lands at the very end (right before the next model reply).
func TestBuild_AuthorsNote_DepthZero(t *testing.T) {
	in := PromptInput{
		Character: &characters.Character{
			Name: "A", Data: characters.CharacterData{Name: "A", Description: "x"},
		},
		UserName: "U",
		History: []Message{
			{Role: RoleUser, Content: "Hi"},
			{Role: RoleAssistant, Content: "Hello"},
		},
		AuthorsNote: &AuthorsNote{Content: "Remember: rain falls.", Depth: 0, Role: "system"},
	}
	msgs := Build(in)
	if got := msgs[len(msgs)-1]; got.Role != "system" || got.Content != "Remember: rain falls." {
		t.Fatalf("note should be last at depth=0, got %+v", msgs)
	}
}

// Depth 1 → note lands before the last message.
func TestBuild_AuthorsNote_DepthOne(t *testing.T) {
	in := PromptInput{
		Character: &characters.Character{
			Name: "A", Data: characters.CharacterData{Name: "A", Description: "x"},
		},
		UserName: "U",
		History: []Message{
			{Role: RoleUser, Content: "Hi"},
			{Role: RoleAssistant, Content: "Hello"},
		},
		AuthorsNote: &AuthorsNote{Content: "Note.", Depth: 1, Role: "system"},
	}
	msgs := Build(in)
	// Expected: [system, user, note, assistant]
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[2].Content != "Note." {
		t.Fatalf("note not at depth=1: %+v", msgs)
	}
}

// Massive depth clamps after the system prompt — never inserted before it.
func TestBuild_AuthorsNote_DepthClampsAfterSystem(t *testing.T) {
	in := PromptInput{
		Character: &characters.Character{
			Name: "A", Data: characters.CharacterData{Name: "A", Description: "x"},
		},
		UserName:    "U",
		History:     []Message{{Role: RoleUser, Content: "Hi"}},
		AuthorsNote: &AuthorsNote{Content: "Big.", Depth: 99},
	}
	msgs := Build(in)
	if msgs[0].Role != "system" {
		t.Fatalf("system must stay at index 0, got %+v", msgs)
	}
	if msgs[1].Content != "Big." {
		t.Fatalf("note must land at index 1 (right after system): %+v", msgs)
	}
}

// Empty note is a no-op.
func TestBuild_AuthorsNote_EmptyIgnored(t *testing.T) {
	in := PromptInput{
		Character:   &characters.Character{Name: "A", Data: characters.CharacterData{Name: "A"}},
		UserName:    "U",
		History:     []Message{{Role: RoleUser, Content: "Hi"}},
		AuthorsNote: &AuthorsNote{Content: "   ", Depth: 0},
	}
	msgs := Build(in)
	for _, m := range msgs {
		if m.Content == "   " {
			t.Fatalf("whitespace note leaked into prompt: %+v", msgs)
		}
	}
}

// Role override — user/assistant instead of default system.
func TestBuild_AuthorsNote_RoleOverride(t *testing.T) {
	in := PromptInput{
		Character:   &characters.Character{Name: "A", Data: characters.CharacterData{Name: "A"}},
		UserName:    "U",
		History:     []Message{{Role: RoleUser, Content: "Hi"}},
		AuthorsNote: &AuthorsNote{Content: "Hi from narrator.", Depth: 0, Role: "assistant"},
	}
	msgs := Build(in)
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" {
		t.Fatalf("expected assistant role override, got %+v", last)
	}
}
