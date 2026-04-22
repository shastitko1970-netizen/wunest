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
