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
