package worldinfo

import (
	"testing"

	"github.com/google/uuid"
)

func ptrBool(b bool) *bool { return &b }

func TestActivate_ConstantFiresWithoutKeys(t *testing.T) {
	w := &World{
		ID: uuid.New(),
		Entries: []Entry{
			{ID: 1, Content: "Always here.", Enabled: true, Constant: true},
		},
	}
	got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"hi"}})
	if len(got.BeforeChar) != 1 || got.BeforeChar[0] != "Always here." {
		t.Fatalf("constant entry did not fire: %+v", got)
	}
	if len(got.Trace) != 1 || got.Trace[0].Reason != "constant" {
		t.Fatalf("trace missing constant reason: %+v", got.Trace)
	}
}

func TestActivate_KeyMatchCaseInsensitive(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"Dragon"}, Content: "Dragons hoard gold.", Enabled: true},
			{ID: 2, Keys: []string{"rabbit"}, Content: "Rabbits eat carrots.", Enabled: true},
		},
	}
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"I saw a DRAGON flying above."},
	})
	if len(got.BeforeChar) != 1 || got.BeforeChar[0] != "Dragons hoard gold." {
		t.Fatalf("expected dragon entry only, got: %+v", got)
	}
}

func TestActivate_Depth_IgnoresOlderMessages(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"kingdom"}, Content: "Kingdom stuff.", Enabled: true, Depth: 2},
		},
	}
	// "kingdom" only appears in the 4th-from-last message; depth=2 must miss it.
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"the kingdom", "msg 2", "msg 3", "msg 4"},
	})
	if len(got.BeforeChar) != 0 {
		t.Fatalf("depth constraint ignored: %+v", got)
	}
	// Depth=4 should see it.
	w.Entries[0].Depth = 4
	got = Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"the kingdom", "msg 2", "msg 3", "msg 4"},
	})
	if len(got.BeforeChar) != 1 {
		t.Fatalf("wider depth missed match: %+v", got)
	}
}

func TestActivate_Selective_NeedsSecondary(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{
				ID:            1,
				Keys:          []string{"king"},
				SecondaryKeys: []string{"crown"},
				Selective:     true,
				Content:       "Royal lore.",
				Enabled:       true,
			},
		},
	}
	// Only primary matches — should NOT fire.
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"the king said hi"},
	})
	if len(got.BeforeChar) != 0 {
		t.Fatalf("selective fired without secondary: %+v", got)
	}
	// Both match — should fire.
	got = Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"the king wore his crown"},
	})
	if len(got.BeforeChar) != 1 {
		t.Fatalf("selective didn't fire with both keys: %+v", got)
	}
}

func TestActivate_Position_GroupsOutput(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"a"}, Content: "A-before", Enabled: true, Position: "before_char"},
			{ID: 2, Keys: []string{"a"}, Content: "A-after", Enabled: true, Position: "after_char"},
		},
	}
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"a"},
	})
	if len(got.BeforeChar) != 1 || got.BeforeChar[0] != "A-before" {
		t.Fatalf("before group wrong: %+v", got.BeforeChar)
	}
	if len(got.AfterChar) != 1 || got.AfterChar[0] != "A-after" {
		t.Fatalf("after group wrong: %+v", got.AfterChar)
	}
}

func TestActivate_InsertionOrder(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"a"}, Content: "second", Enabled: true, InsertionOrder: 200},
			{ID: 2, Keys: []string{"a"}, Content: "first", Enabled: true, InsertionOrder: 100},
		},
	}
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"a"},
	})
	if len(got.BeforeChar) != 2 || got.BeforeChar[0] != "first" || got.BeforeChar[1] != "second" {
		t.Fatalf("order wrong: %+v", got.BeforeChar)
	}
}

func TestActivate_Disabled_SkipsEntry(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"dragon"}, Content: "never", Enabled: false},
		},
	}
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"dragon"},
	})
	if len(got.BeforeChar) != 0 {
		t.Fatalf("disabled fired: %+v", got)
	}
}

func TestActivate_CaseSensitive_Respected(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{
				ID:            1,
				Keys:          []string{"API"},
				Content:       "acronym note",
				Enabled:       true,
				CaseSensitive: ptrBool(true),
			},
		},
	}
	got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"api docs"}})
	if len(got.BeforeChar) != 0 {
		t.Fatalf("case-sensitive matched lowercase: %+v", got)
	}
	got = Activate(ActivationInput{Books: []*World{w}, Recent: []string{"check the API"}})
	if len(got.BeforeChar) != 1 {
		t.Fatalf("case-sensitive missed exact case: %+v", got)
	}
}

func TestActivate_DedupIdenticalContent(t *testing.T) {
	a := &World{Entries: []Entry{{ID: 1, Keys: []string{"x"}, Content: "same", Enabled: true}}}
	b := &World{Entries: []Entry{{ID: 2, Keys: []string{"x"}, Content: "same", Enabled: true}}}
	got := Activate(ActivationInput{Books: []*World{a, b}, Recent: []string{"x"}})
	if len(got.BeforeChar) != 1 {
		t.Fatalf("duplicate content not deduped: %+v", got.BeforeChar)
	}
}

// Primary pass fires "dragon"; the dragon entry's content mentions "hoard",
// which should pull in the hoard entry on the next recursion pass even
// though "hoard" never appeared in the user's message.
func TestActivate_Recursion_ChainsActivations(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"dragon"}, Content: "Dragons sleep on their hoard.", Enabled: true},
			{ID: 2, Keys: []string{"hoard"}, Content: "Hoards are piles of gold.", Enabled: true},
			{ID: 3, Keys: []string{"volcano"}, Content: "Never read.", Enabled: true},
		},
	}
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"I saw a dragon."},
	})
	if len(got.BeforeChar) != 2 {
		t.Fatalf("recursion did not chain: %+v", got.BeforeChar)
	}
}

// Recursion depth is bounded — a long chain shouldn't loop forever.
func TestActivate_Recursion_RespectsDepth(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"one"}, Content: "two", Enabled: true},   // primary
			{ID: 2, Keys: []string{"two"}, Content: "three", Enabled: true}, // 1st recursion
			{ID: 3, Keys: []string{"three"}, Content: "four", Enabled: true}, // 2nd recursion
			{ID: 4, Keys: []string{"four"}, Content: "five", Enabled: true},  // 3rd recursion (cap hit)
			{ID: 5, Keys: []string{"five"}, Content: "six", Enabled: true},   // should NOT fire
		},
	}
	got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"one"}})
	if len(got.BeforeChar) != 4 {
		t.Fatalf("expected 4 activations within MaxRecursionDepth, got %d: %+v", len(got.BeforeChar), got.BeforeChar)
	}
}

// ExcludeRecursion: entry fires, but its content is not fed into the next
// pass — so downstream entries that depend on it don't activate.
func TestActivate_Recursion_ExcludeRecursionBlocksChain(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"dragon"}, Content: "The dragon hoards.", Enabled: true, ExcludeRecursion: true},
			{ID: 2, Keys: []string{"hoard"}, Content: "Hoards are piles of gold.", Enabled: true},
		},
	}
	got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"I see a dragon"}})
	if len(got.BeforeChar) != 1 {
		t.Fatalf("excluded entry should have blocked the chain: %+v", got.BeforeChar)
	}
	if got.BeforeChar[0] != "The dragon hoards." {
		t.Fatalf("wrong surviving entry: %+v", got.BeforeChar)
	}
}

// PreventRecursion: entry can only be activated by the primary pass. If its
// keys only appear in an already-activated entry's content, it doesn't fire.
func TestActivate_Recursion_PreventRecursionIsolates(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"dragon"}, Content: "Dragons and hoards.", Enabled: true},
			{ID: 2, Keys: []string{"hoard"}, Content: "Protected lore.", Enabled: true, PreventRecursion: true},
		},
	}
	got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"dragon"}})
	if len(got.BeforeChar) != 1 {
		t.Fatalf("prevent-recursion entry shouldn't have activated: %+v", got.BeforeChar)
	}
	// But the primary pass still activates it when the key is in the user's turn.
	got2 := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"dragon with a hoard"}})
	if len(got2.BeforeChar) != 2 {
		t.Fatalf("prevent-recursion must still fire on primary hit: %+v", got2.BeforeChar)
	}
}
