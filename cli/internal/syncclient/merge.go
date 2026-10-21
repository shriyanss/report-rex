package syncclient

import (
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const (
	conflictMarkerStart = "<<<<<<< LOCAL"
	conflictMarkerMid   = "======="
	conflictMarkerEnd   = ">>>>>>> REMOTE"
)

// interval is a half-open [start, end) range of BASE line indices that one
// side (local or remote) replaced with newLines. A pure insertion is a
// zero-width interval (start == end); a pure deletion has empty newLines.
type interval struct {
	start, end int
	newLines   []string
}

// ThreeWayMerge merges local's and remote's independent edits relative to
// base using a line-based diff3-style algorithm: BASE, LOCAL, and REMOTE
// are each split into lines; base->local and base->remote are diffed
// exactly (Myers' algorithm via diffmatchpatch's line-mode diff, not fuzzy
// context matching); non-overlapping edit regions are merged
// automatically; any base line range both sides touched differently is
// left as an explicit, git-style conflict block rather than guessed.
func ThreeWayMerge(base, local, remote []byte) (merged []byte, clean bool) {
	baseStr, localStr, remoteStr := string(base), string(local), string(remote)

	if remoteStr == baseStr {
		return local, true
	}
	if localStr == baseStr {
		return remote, true
	}
	if localStr == remoteStr {
		return local, true
	}

	baseLines := splitLines(baseStr)
	localIntervals := replaceIntervals(baseStr, localStr)
	remoteIntervals := replaceIntervals(baseStr, remoteStr)

	var out []string
	conflict := false
	pos := 0
	li, ri := 0, 0

	emitBase := func(from, to int) {
		if from < to {
			out = append(out, baseLines[from:to]...)
		}
	}

	for li < len(localIntervals) || ri < len(remoteIntervals) {
		var loc, rem *interval
		if li < len(localIntervals) {
			loc = &localIntervals[li]
		}
		if ri < len(remoteIntervals) {
			rem = &remoteIntervals[ri]
		}

		switch {
		case loc != nil && rem != nil && intervalsOverlap(*loc, *rem):
			start := min(loc.start, rem.start)
			emitBase(pos, start)
			end := max(loc.end, rem.end)
			if loc.start == rem.start && loc.end == rem.end && sameLines(loc.newLines, rem.newLines) {
				out = append(out, loc.newLines...)
			} else {
				conflict = true
				out = append(out, conflictMarkerStart)
				out = append(out, loc.newLines...)
				out = append(out, conflictMarkerMid)
				out = append(out, rem.newLines...)
				out = append(out, conflictMarkerEnd)
			}
			pos = end
			li++
			ri++

		case rem == nil || (loc != nil && loc.start <= rem.start):
			emitBase(pos, loc.start)
			out = append(out, loc.newLines...)
			pos = loc.end
			li++

		default:
			emitBase(pos, rem.start)
			out = append(out, rem.newLines...)
			pos = rem.end
			ri++
		}
	}
	emitBase(pos, len(baseLines))

	result := strings.Join(out, "\n") + "\n"
	return []byte(result), !conflict
}

func intervalsOverlap(a, b interval) bool {
	if a.start < b.end && b.start < a.end {
		return true
	}
	// Two pure insertions (zero-width intervals) at the exact same base
	// position both want to add content there; treat that as a conflict
	// too rather than silently picking an order.
	if a.start == a.end && a.start == b.start {
		return true
	}
	if b.start == b.end && b.start == a.start {
		return true
	}
	return false
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// replaceIntervals diffs base against other at line granularity (exact
// Myers diff, not fuzzy) and returns the maximal runs of base lines that
// other changed, each with the lines that replace them.
func replaceIntervals(base, other string) []interval {
	dmp := diffmatchpatch.New()
	aChars, bChars, lineArray := dmp.DiffLinesToChars(base, other)
	diffs := dmp.DiffMain(aChars, bChars, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	var intervals []interval
	baseIdx := 0
	i := 0
	for i < len(diffs) {
		if diffs[i].Type == diffmatchpatch.DiffEqual {
			baseIdx += lineCount(diffs[i].Text)
			i++
			continue
		}
		start := baseIdx
		var newLines []string
		for i < len(diffs) && diffs[i].Type != diffmatchpatch.DiffEqual {
			if diffs[i].Type == diffmatchpatch.DiffDelete {
				baseIdx += lineCount(diffs[i].Text)
			} else {
				newLines = append(newLines, splitLines(diffs[i].Text)...)
			}
			i++
		}
		intervals = append(intervals, interval{start: start, end: baseIdx, newLines: newLines})
	}
	return intervals
}

// lineCount counts the number of lines represented by a diff-match-patch
// line-mode text chunk (each "line" retains its trailing \n except
// possibly the final one).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func conflictMarkers(local, remote []byte) []byte {
	return []byte(conflictMarkerStart + "\n" + string(local) + "\n" + conflictMarkerMid + "\n" + string(remote) + "\n" + conflictMarkerEnd + "\n")
}

// HasConflictMarkers reports whether content still contains unresolved
// conflict markers written by a failed ThreeWayMerge, used by `report-rex
// resolve --merge` to detect whether the user has finished hand-editing.
func HasConflictMarkers(content []byte) bool {
	s := string(content)
	return strings.Contains(s, conflictMarkerStart) &&
		strings.Contains(s, conflictMarkerMid) &&
		strings.Contains(s, conflictMarkerEnd)
}
