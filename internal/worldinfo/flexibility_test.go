package worldinfo

import (
	"testing"

	"github.com/google/uuid"
)

// match_whole_words should suppress substring hits inside longer words.
func TestActivate_MatchWholeWords(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{
				ID:              1,
				Keys:            []string{"cat"},
				Content:         "A cat entry.",
				Enabled:         true,
				MatchWholeWords: ptrBool(true),
			},
		},
	}
	// "concatenate" contains "cat" as substring but not as whole word.
	got := Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"we should concatenate these strings"},
	})
	if len(got.BeforeChar) != 0 {
		t.Fatalf("whole-words let substring fire: %+v", got)
	}
	// Whole word hit: fire.
	got = Activate(ActivationInput{
		Books:  []*World{w},
		Recent: []string{"there is a cat here"},
	})
	if len(got.BeforeChar) != 1 {
		t.Fatalf("whole-words missed explicit hit: %+v", got)
	}
}

// match_whole_words must work at string edges (start / end of message).
func TestActivate_MatchWholeWords_Edges(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"dog"}, Content: "dog", Enabled: true, MatchWholeWords: ptrBool(true)},
		},
	}
	for _, msg := range []string{"dog runs", "run dog", "dog", "a dog!"} {
		got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{msg}})
		if len(got.BeforeChar) != 1 {
			t.Errorf("whole-words missed edge case %q: %+v", msg, got)
		}
	}
}

// probability = 0 (unset) fires unconditionally.
func TestActivate_Probability_ZeroMeansAlways(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"foo"}, Content: "hit", Enabled: true, Probability: 0},
		},
	}
	for i := 0; i < 10; i++ {
		got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"foo"}})
		if len(got.BeforeChar) != 1 {
			t.Fatalf("probability=0 skipped: %+v", got)
		}
	}
}

// probability = 100 always fires; probability = 1 almost never fires over
// a reasonable sample. We're deterministic about the extremes, tolerant
// in the middle.
func TestActivate_Probability_Extremes(t *testing.T) {
	always := &World{
		Entries: []Entry{{ID: 1, Keys: []string{"x"}, Content: "c", Enabled: true, Probability: 100}},
	}
	for i := 0; i < 20; i++ {
		got := Activate(ActivationInput{Books: []*World{always}, Recent: []string{"x"}})
		if len(got.BeforeChar) != 1 {
			t.Fatalf("probability=100 skipped: %+v", got)
		}
	}

	// probability=1 — over 500 rolls we expect ~5 hits. Just verify at least
	// one pass produces 0 hits AND at most one pass produces hits out of a
	// sample (fuzz-friendly but not flaky).
	low := &World{
		Entries: []Entry{{ID: 1, Keys: []string{"x"}, Content: "c", Enabled: true, Probability: 1}},
	}
	misses := 0
	for i := 0; i < 50; i++ {
		got := Activate(ActivationInput{Books: []*World{low}, Recent: []string{"x"}})
		if len(got.BeforeChar) == 0 {
			misses++
		}
	}
	// With probability=1, over 50 rolls we expect ~49 misses. Anything less
	// than 40 is suspicious.
	if misses < 40 {
		t.Fatalf("probability=1 fired too often: only %d misses out of 50", misses)
	}
}

// Within a group, exactly one entry fires — the first in the sort order
// (lowest InsertionOrder). GroupOverride bypasses the cap.
func TestActivate_Group_PicksFirstWinner(t *testing.T) {
	w := &World{
		ID: uuid.New(),
		Entries: []Entry{
			{ID: 1, Keys: []string{"x"}, Content: "first",  Enabled: true, Group: "greet", InsertionOrder: 10},
			{ID: 2, Keys: []string{"x"}, Content: "second", Enabled: true, Group: "greet", InsertionOrder: 20},
			{ID: 3, Keys: []string{"x"}, Content: "third",  Enabled: true, Group: "greet", InsertionOrder: 30},
			// Different group — not affected.
			{ID: 4, Keys: []string{"x"}, Content: "other", Enabled: true, Group: "bye"},
			// Override — always included.
			{ID: 5, Keys: []string{"x"}, Content: "over",  Enabled: true, Group: "greet", GroupOverride: true, InsertionOrder: 40},
		},
	}
	got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"x"}})

	// Expected: first + other + over. Second/third are suppressed by the group.
	if len(got.BeforeChar) != 3 {
		t.Fatalf("group cap wrong: %+v", got)
	}
	// Order-wise: first (io=10) comes before other (io=0, but book order — actually no, io=0 sorts first globally).
	// We just check set membership rather than exact order.
	set := map[string]bool{}
	for _, c := range got.BeforeChar {
		set[c] = true
	}
	for _, want := range []string{"first", "other", "over"} {
		if !set[want] {
			t.Errorf("expected %q, missing: %+v", want, got.BeforeChar)
		}
	}
	if set["second"] || set["third"] {
		t.Errorf("group members leaked: %+v", got.BeforeChar)
	}
}

// Empty group = no cap. Two untagged entries with different content both fire.
func TestActivate_Group_EmptyMeansNoCap(t *testing.T) {
	w := &World{
		Entries: []Entry{
			{ID: 1, Keys: []string{"x"}, Content: "a", Enabled: true},
			{ID: 2, Keys: []string{"x"}, Content: "b", Enabled: true},
		},
	}
	got := Activate(ActivationInput{Books: []*World{w}, Recent: []string{"x"}})
	if len(got.BeforeChar) != 2 {
		t.Fatalf("untagged entries were group-suppressed: %+v", got)
	}
}
