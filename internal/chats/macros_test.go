package chats

import (
	"strings"
	"testing"
	"time"
)

// Focused tests for M39.1 extended macros — getvar/setvar, time/date,
// last-message references, idle_duration. Existing user/char/random/
// roll macros covered implicitly via older tests.

func TestMacroGetSetVar(t *testing.T) {
	vars := map[string]string{"location": "Tavern"}
	in := PromptInput{Variables: vars}

	if got := SubstituteMacros("You're at {{getvar::location}}.", in); got != "You're at Tavern." {
		t.Errorf("getvar existing: %q", got)
	}
	if got := SubstituteMacros("You're at {{getvar::missing}}.", in); got != "You're at ." {
		t.Errorf("getvar missing: %q", got)
	}

	// Setvar side-effect: mutates the map + expands to empty.
	out := SubstituteMacros("{{setvar::mood::happy}}Mood registered.", in)
	if out != "Mood registered." {
		t.Errorf("setvar should expand to empty, got %q", out)
	}
	if vars["mood"] != "happy" {
		t.Errorf("setvar did not persist: %+v", vars)
	}

	// Nil map — setvar is no-op, doesn't panic.
	out = SubstituteMacros("{{setvar::x::y}}ok", PromptInput{})
	if out != "ok" {
		t.Errorf("nil-map setvar: %q", out)
	}
}

func TestMacroTimeDate(t *testing.T) {
	// Fix Now so the test doesn't depend on wall clock.
	fixed := time.Date(2026, 4, 24, 15, 30, 0, 0, time.UTC)
	in := PromptInput{Now: fixed}
	got := SubstituteMacros("It's {{time}} on {{date}} ({{weekday}}).", in)
	want := "It's 15:30 on 2026-04-24 (Friday)."
	if got != want {
		t.Errorf("time/date/weekday: got %q, want %q", got, want)
	}
}

func TestMacroLastMessage(t *testing.T) {
	now := time.Date(2026, 4, 24, 15, 30, 0, 0, time.UTC)
	history := []Message{
		{Role: RoleUser, Content: "hello", CreatedAt: now.Add(-2 * time.Hour)},
		{Role: RoleAssistant, Content: "hi!", CreatedAt: now.Add(-2 * time.Hour)},
		{Role: RoleUser, Content: "how are you?", CreatedAt: now.Add(-30 * time.Minute)},
		{Role: RoleAssistant, Content: "great", CreatedAt: now.Add(-29 * time.Minute)},
	}
	in := PromptInput{History: history, Now: now}
	got := SubstituteMacros("U={{lastUserMessage}}, A={{lastCharMessage}}", in)
	if !strings.Contains(got, "U=how are you?") || !strings.Contains(got, "A=great") {
		t.Errorf("lastMessage: %q", got)
	}
}

func TestMacroIdleDuration(t *testing.T) {
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"30 seconds", 30 * time.Second, "moments ago"},
		{"3 minutes", 3 * time.Minute, "a few minutes ago"},
		{"15 minutes", 15 * time.Minute, "15 minutes ago"},
		{"1 hour", 65 * time.Minute, "an hour ago"},
		{"3 hours", 3 * time.Hour, "3 hours ago"},
		{"yesterday", 30 * time.Hour, "yesterday"},
		{"3 days", 3 * 24 * time.Hour, "3 days ago"},
		{"1 week", 7 * 24 * time.Hour, "a week ago"},
		{"3 weeks", 21 * 24 * time.Hour, "3 weeks ago"},
		{"long", 90 * 24 * time.Hour, "a long time ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []Message{
				{Role: RoleUser, Content: "hi", CreatedAt: now.Add(-tc.ago)},
			}
			in := PromptInput{History: history, Now: now}
			got := SubstituteMacros("{{idle_duration}}", in)
			if got != tc.want {
				t.Errorf("idle ago=%v: got %q, want %q", tc.ago, got, tc.want)
			}
		})
	}
	// Empty history → empty string.
	got := SubstituteMacros("[{{idle_duration}}]", PromptInput{Now: now})
	if got != "[]" {
		t.Errorf("empty history: %q", got)
	}
}

func TestMacroRound_TripBasic(t *testing.T) {
	// Regression: ensure existing user/char/random/roll macros still work
	// alongside the new ones.
	in := PromptInput{
		UserName: "Alice",
		History:  []Message{{Role: RoleUser, Content: "hi"}},
		Variables: map[string]string{"q": "quest"},
		Now:       time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
	}
	got := SubstituteMacros("{{user}} asked about the {{getvar::q}} at {{time}}.", in)
	want := "Alice asked about the quest at 09:00."
	if got != want {
		t.Errorf("combined: got %q, want %q", got, want)
	}
}
