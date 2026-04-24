package worldinfo

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"unicode"
)

// DefaultScanDepth is how many recent messages we scan for key matches when
// an entry doesn't specify a per-entry depth and the caller doesn't override.
const DefaultScanDepth = 4

// MaxRecursionDepth caps how many recursive passes Activate runs after the
// primary pass. Three is the SillyTavern default and plenty for any
// reasonable lore pack — past that it's almost always a cycle.
const MaxRecursionDepth = 3

// activatedRow captures one fired entry. Lives at package scope so the
// recursion helper can accept a slice of it without anonymous-struct
// type mismatches.
type activatedRow struct {
	e        *Entry
	bookIdx  int
	entryIdx int
	reason   string
	trace    Trace
}

// entryKey is the (book, entry) tuple used to deduplicate activations
// across recursion passes.
type entryKey struct{ b, e int }

// Activate runs every enabled entry across all provided books against the
// recent chat tail, and returns the entries grouped by position. The result
// is deterministic: constant entries come first, then key-matched entries,
// sorted within each position by (InsertionOrder, book index, entry index).
//
// Scope:
//   - Keys are OR-matched (any hit activates).
//   - Selective + SecondaryKeys: when true, at least one secondary key must
//     also match in the scanned window.
//   - Constant entries always fire regardless of keys.
//   - Case-insensitive substring match by default; `CaseSensitive=true` flips.
//   - Recursive scanning: after the primary pass, activated entries'
//     content becomes a secondary scan window. Non-activated entries whose
//     keys hit that window fire too. Repeats up to MaxRecursionDepth (3)
//     iterations or until a pass activates nothing new — whichever comes
//     first. Per-entry opt-outs: `ExcludeRecursion` (entry's content is
//     omitted from the next pass's window) and `PreventRecursion` (entry
//     cannot be activated by recursion, only by the primary history scan).
//   - Probability / token budgeting: deferred.
func Activate(in ActivationInput) Activated {
	depth := in.DefaultDepth
	if depth <= 0 {
		depth = DefaultScanDepth
	}

	globalWin := tailJoin(in.Recent, depth)
	globalWinLower := strings.ToLower(globalWin)

	rows := make([]activatedRow, 0)
	// Track (bookIdx, entryIdx) → already activated so recursion doesn't
	// double-fire entries.
	activated := make(map[entryKey]struct{})

	// Primary pass — scan the history window.
	for bi, book := range in.Books {
		if book == nil {
			continue
		}
		for ei := range book.Entries {
			e := &book.Entries[ei]
			if !e.Enabled {
				continue
			}

			// Constant: fires unconditionally, no scan.
			if e.Constant {
				rows = append(rows, activatedRow{
					e: e, bookIdx: bi, entryIdx: ei,
					reason: "constant",
					trace: Trace{
						WorldID: book.ID, EntryID: e.ID, Reason: "constant",
					},
				})
				activated[entryKey{bi, ei}] = struct{}{}
				continue
			}

			if len(e.Keys) == 0 {
				continue
			}

			// Choose scan window for this entry — global or per-entry.
			win, winLower := globalWin, globalWinLower
			if e.Depth > 0 && e.Depth != depth {
				win = tailJoin(in.Recent, e.Depth)
				winLower = strings.ToLower(win)
			}
			if win == "" {
				continue
			}

			if row, ok := matchEntry(e, win, winLower, "key: %s", "key: %s + secondary: %s"); ok {
				rows = append(rows, activatedRow{
					e: e, bookIdx: bi, entryIdx: ei,
					reason: row.reason,
					trace: Trace{
						WorldID: book.ID, EntryID: e.ID, Reason: row.reason,
					},
				})
				activated[entryKey{bi, ei}] = struct{}{}
			}
		}
	}

	// Recursive passes — feed the just-activated content back in as a new
	// scan window; pick up entries whose keys hit that text. Stops early
	// when a pass adds nothing, or at MaxRecursionDepth iterations.
	lastPassContents := collectRecursionText(rows, activated, in.Books, 0 /* all primary-pass rows qualify */)
	for iter := 0; iter < MaxRecursionDepth && lastPassContents != ""; iter++ {
		lower := strings.ToLower(lastPassContents)
		addedThisIter := 0
		// Track the starting index so we only collect recursion text from
		// rows added in THIS iteration for the next one.
		startLen := len(rows)

		for bi, book := range in.Books {
			if book == nil {
				continue
			}
			for ei := range book.Entries {
				e := &book.Entries[ei]
				if !e.Enabled || e.Constant || e.PreventRecursion {
					continue
				}
				if _, already := activated[entryKey{bi, ei}]; already {
					continue
				}
				if len(e.Keys) == 0 {
					continue
				}

				reasonFmt := fmt.Sprintf("key: %%s (recursion #%d)", iter+1)
				secFmt := fmt.Sprintf("key: %%s + secondary: %%s (recursion #%d)", iter+1)
				if row, ok := matchEntry(e, lastPassContents, lower, reasonFmt, secFmt); ok {
					rows = append(rows, activatedRow{
						e: e, bookIdx: bi, entryIdx: ei,
						reason: row.reason,
						trace: Trace{
							WorldID: book.ID, EntryID: e.ID, Reason: row.reason,
						},
					})
					activated[entryKey{bi, ei}] = struct{}{}
					addedThisIter++
				}
			}
		}

		if addedThisIter == 0 {
			break
		}
		lastPassContents = collectRecursionText(rows, activated, in.Books, startLen)
	}

	// Sort: lower InsertionOrder first, then book order, then entry order.
	sort.SliceStable(rows, func(a, b int) bool {
		if rows[a].e.InsertionOrder != rows[b].e.InsertionOrder {
			return rows[a].e.InsertionOrder < rows[b].e.InsertionOrder
		}
		if rows[a].bookIdx != rows[b].bookIdx {
			return rows[a].bookIdx < rows[b].bookIdx
		}
		return rows[a].entryIdx < rows[b].entryIdx
	})

	// Group gate: within a non-empty Group, only the first-sorted entry
	// survives — except for entries flagged GroupOverride. This is the
	// "mutually-exclusive trigger group" semantic from ST (e.g. multiple
	// greetings in one group, pick one).
	rows = filterByGroup(rows)

	out := Activated{
		BeforeChar: make([]string, 0, len(rows)),
		AfterChar:  make([]string, 0, len(rows)),
		BeforeAN:   make([]string, 0),
		AfterAN:    make([]string, 0),
		AtDepth:    make([]AtDepthEntry, 0),
		Trace:      make([]Trace, 0, len(rows)),
	}
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		content := strings.TrimSpace(r.e.Content)
		if content == "" {
			continue
		}
		// Deduplicate identical content across books — common when two books
		// import the same entry.
		if _, dup := seen[content]; dup {
			continue
		}
		seen[content] = struct{}{}

		pos := r.e.Position
		if pos == "" {
			pos = PositionBeforeChar
		}
		switch pos {
		case PositionAfterChar:
			out.AfterChar = append(out.AfterChar, content)
		case PositionBeforeAN:
			out.BeforeAN = append(out.BeforeAN, content)
		case PositionAfterAN:
			out.AfterAN = append(out.AfterAN, content)
		case PositionAtDepth:
			role := r.e.Role
			if role == "" {
				role = "system"
			}
			out.AtDepth = append(out.AtDepth, AtDepthEntry{
				Content: content,
				Depth:   r.e.Depth,
				Role:    role,
				Order:   r.e.InsertionOrder,
			})
		default:
			out.BeforeChar = append(out.BeforeChar, content)
		}
		out.Trace = append(out.Trace, r.trace)
	}
	return out
}

// matchEntry reports whether a single entry matches a given scan window.
// Handles primary key OR-match, optional selective+secondary gate, and
// the probability gate. `match_whole_words` flips the key-scan to word
// boundary mode so "cat" doesn't fire on "concatenate".
type matchRow struct{ reason string }

func matchEntry(e *Entry, win, winLower, primaryFmt, secondaryFmt string) (matchRow, bool) {
	caseSens := e.CaseSensitive != nil && *e.CaseSensitive
	wholeWords := e.MatchWholeWords != nil && *e.MatchWholeWords

	primaryHit, primaryKey := matchAny(win, winLower, e.Keys, caseSens, wholeWords)
	if primaryHit == "" {
		return matchRow{}, false
	}

	if e.Selective && len(e.SecondaryKeys) > 0 {
		secHit, secKey := matchAny(win, winLower, e.SecondaryKeys, caseSens, wholeWords)
		if secHit == "" {
			return matchRow{}, false
		}
		// Probability gate — apply after we've confirmed the keys match so
		// the random roll is rare (cheap to not-roll when keys don't hit).
		if !rollProbability(e.Probability) {
			return matchRow{}, false
		}
		return matchRow{reason: fmt.Sprintf(secondaryFmt, primaryKey, secKey)}, true
	}
	if !rollProbability(e.Probability) {
		return matchRow{}, false
	}
	return matchRow{reason: fmt.Sprintf(primaryFmt, primaryKey)}, true
}

// rollProbability returns true if an entry passes its probability gate.
// Zero or unset = 100% (always fires). 1..99 = random roll. 100 = always.
// Negative / >100 clamp to always-fire.
func rollProbability(p int) bool {
	if p <= 0 || p >= 100 {
		return true
	}
	return rand.IntN(100) < p
}

// filterByGroup collapses activated rows so that within each non-empty
// Group only the first-sorted row survives, unless GroupOverride is set.
// Must be called AFTER the rows are sorted by InsertionOrder — we rely on
// that order to pick the "winner" deterministically.
func filterByGroup(rows []activatedRow) []activatedRow {
	out := make([]activatedRow, 0, len(rows))
	claimed := make(map[string]struct{})
	for _, r := range rows {
		if r.e == nil {
			continue
		}
		g := r.e.Group
		if g == "" || r.e.GroupOverride {
			out = append(out, r)
			continue
		}
		if _, taken := claimed[g]; taken {
			continue
		}
		claimed[g] = struct{}{}
		out = append(out, r)
	}
	return out
}

// collectRecursionText concatenates the content of recently-activated rows
// (starting at index `fromIdx` in `rows`) for feeding into the next
// recursion pass. Entries flagged `ExcludeRecursion` are skipped — they
// contribute to the prompt but not to the recursion window.
//
// This indirection keeps the main loop readable and lets us tune the
// exclusion rules without rewriting the primary flow.
func collectRecursionText(rows []activatedRow, _ map[entryKey]struct{}, _ []*World, fromIdx int) string {
	if fromIdx >= len(rows) {
		return ""
	}
	var b strings.Builder
	for _, r := range rows[fromIdx:] {
		if r.e == nil || r.e.ExcludeRecursion {
			continue
		}
		content := strings.TrimSpace(r.e.Content)
		if content == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(content)
	}
	return b.String()
}

// tailJoin returns the last N non-empty messages joined by newlines.
// Empty string if nothing qualifies.
func tailJoin(msgs []string, n int) string {
	if n <= 0 || len(msgs) == 0 {
		return ""
	}
	start := len(msgs) - n
	if start < 0 {
		start = 0
	}
	picked := make([]string, 0, len(msgs)-start)
	for _, m := range msgs[start:] {
		m = strings.TrimSpace(m)
		if m != "" {
			picked = append(picked, m)
		}
	}
	return strings.Join(picked, "\n")
}

// matchAny returns (key-that-hit, key-literal) when any of keys appears in
// the window, or ("", "") when none do. Both raw and lowercased windows are
// passed so we only lowercase once per window. `wholeWords` flips to a
// word-boundary match — letters/digits/underscore on either side of a key
// break the match, so "cat" won't fire on "concatenate" or "cats".
//
// The word-boundary implementation is regexp-free for speed and to avoid
// ReDoS surface from user-authored keys.
func matchAny(win, winLower string, keys []string, caseSens, wholeWords bool) (string, string) {
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		var hay, needle string
		if caseSens {
			hay = win
			needle = k
		} else {
			hay = winLower
			needle = strings.ToLower(k)
		}
		if wholeWords {
			if containsWordBoundary(hay, needle) {
				return k, k
			}
		} else if strings.Contains(hay, needle) {
			return k, k
		}
	}
	return "", ""
}

// containsWordBoundary reports whether needle appears in hay bordered by
// word-break characters (or string start/end) on BOTH sides. Treats
// letters / digits / underscore as word characters, matching JS's \b
// semantics closely enough for lorebook use.
//
// For needle n at position p in hay h: n is a "whole word" iff
//   (p == 0  OR h[p-1] is non-word) AND
//   (p+len(n) == len(h)  OR h[p+len(n)] is non-word)
func containsWordBoundary(hay, needle string) bool {
	if needle == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(hay[start:], needle)
		if idx < 0 {
			return false
		}
		abs := start + idx
		leftOK := abs == 0 || !isWordByte(hay[abs-1])
		rightOK := abs+len(needle) == len(hay) || !isWordByte(hay[abs+len(needle)])
		if leftOK && rightOK {
			return true
		}
		start = abs + 1
		if start >= len(hay) {
			return false
		}
	}
}

// isWordByte reports whether a single byte counts as a "word" character
// under JS \b semantics: letters, digits, underscore. For lorebook keys
// (mostly ASCII proper nouns) byte-level is faster and accurate enough;
// a non-ASCII byte is conservatively treated as a word char via unicode
// fallback so Cyrillic words don't accidentally get split.
func isWordByte(b byte) bool {
	r := rune(b)
	if r < 128 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
