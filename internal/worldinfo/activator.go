package worldinfo

import (
	"fmt"
	"sort"
	"strings"
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

	out := Activated{
		BeforeChar: make([]string, 0, len(rows)),
		AfterChar:  make([]string, 0, len(rows)),
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
		default:
			out.BeforeChar = append(out.BeforeChar, content)
		}
		out.Trace = append(out.Trace, r.trace)
	}
	return out
}

// matchEntry reports whether a single entry matches a given scan window.
// Handles primary key OR-match and the optional selective+secondary gate.
// Both regexp-free — `containsAny` is a simple substring scan.
type matchRow struct{ reason string }

func matchEntry(e *Entry, win, winLower, primaryFmt, secondaryFmt string) (matchRow, bool) {
	caseSens := e.CaseSensitive != nil && *e.CaseSensitive

	primaryHit, primaryKey := containsAny(win, winLower, e.Keys, caseSens)
	if primaryHit == "" {
		return matchRow{}, false
	}

	if e.Selective && len(e.SecondaryKeys) > 0 {
		secHit, secKey := containsAny(win, winLower, e.SecondaryKeys, caseSens)
		if secHit == "" {
			return matchRow{}, false
		}
		return matchRow{reason: fmt.Sprintf(secondaryFmt, primaryKey, secKey)}, true
	}
	return matchRow{reason: fmt.Sprintf(primaryFmt, primaryKey)}, true
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

// containsAny returns (key-that-hit, key-literal) when any of keys appears in
// the window, or ("", "") when none do. Both raw and lowercased windows are
// passed so we only lowercase once per window.
func containsAny(win, winLower string, keys []string, caseSens bool) (string, string) {
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if caseSens {
			if strings.Contains(win, k) {
				return k, k
			}
		} else {
			if strings.Contains(winLower, strings.ToLower(k)) {
				return k, k
			}
		}
	}
	return "", ""
}
