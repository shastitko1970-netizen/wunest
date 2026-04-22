package worldinfo

import (
	"fmt"
	"sort"
	"strings"
)

// DefaultScanDepth is how many recent messages we scan for key matches when
// an entry doesn't specify a per-entry depth and the caller doesn't override.
const DefaultScanDepth = 4

// Activate runs every enabled entry across all provided books against the
// recent chat tail, and returns the entries grouped by position. The result
// is deterministic: constant entries come first, then key-matched entries,
// sorted within each position by (InsertionOrder, book index, entry index).
//
// v1 scope:
//   - Keys are OR-matched (any hit activates).
//   - Selective + SecondaryKeys: when true, at least one secondary key must
//     also match in the scanned window.
//   - Constant entries always fire regardless of keys.
//   - Case-insensitive substring match by default. An entry can flip to
//     case-sensitive via CaseSensitive=true. Word-boundary matching is
//     deferred to v1.1.
//   - Probability / recursion / token budgeting: deferred.
func Activate(in ActivationInput) Activated {
	depth := in.DefaultDepth
	if depth <= 0 {
		depth = DefaultScanDepth
	}

	// For each entry, pick a lowercased scan window sized to the entry's Depth.
	// Two windows cached: the global one and anything shorter an entry asks for.
	globalWin := tailJoin(in.Recent, depth)
	globalWinLower := strings.ToLower(globalWin)

	type activatedRow struct {
		e        *Entry
		bookIdx  int
		entryIdx int
		reason   string
		trace    Trace
	}
	rows := make([]activatedRow, 0)

	for bi, book := range in.Books {
		if book == nil {
			continue
		}
		for ei := range book.Entries {
			e := &book.Entries[ei]
			if !e.Enabled {
				continue
			}

			// Constant: fires unconditionally.
			if e.Constant {
				rows = append(rows, activatedRow{
					e: e, bookIdx: bi, entryIdx: ei,
					reason: "constant",
					trace: Trace{
						WorldID: book.ID,
						EntryID: e.ID,
						Reason:  "constant",
					},
				})
				continue
			}

			if len(e.Keys) == 0 {
				continue
			}

			// Choose scan window for this entry.
			var win, winLower string
			if e.Depth > 0 && e.Depth != depth {
				win = tailJoin(in.Recent, e.Depth)
				winLower = strings.ToLower(win)
			} else {
				win = globalWin
				winLower = globalWinLower
			}
			if win == "" {
				continue
			}

			caseSens := e.CaseSensitive != nil && *e.CaseSensitive

			primaryHit, primaryKey := containsAny(win, winLower, e.Keys, caseSens)
			if primaryHit == "" {
				continue
			}

			if e.Selective && len(e.SecondaryKeys) > 0 {
				secHit, secKey := containsAny(win, winLower, e.SecondaryKeys, caseSens)
				if secHit == "" {
					continue
				}
				reason := fmt.Sprintf("key: %s + secondary: %s", primaryKey, secKey)
				rows = append(rows, activatedRow{
					e: e, bookIdx: bi, entryIdx: ei,
					reason: reason,
					trace: Trace{
						WorldID: book.ID,
						EntryID: e.ID,
						Reason:  reason,
					},
				})
				continue
			}

			reason := "key: " + primaryKey
			rows = append(rows, activatedRow{
				e: e, bookIdx: bi, entryIdx: ei,
				reason: reason,
				trace: Trace{
					WorldID: book.ID,
					EntryID: e.ID,
					Reason:  reason,
				},
			})
		}
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
