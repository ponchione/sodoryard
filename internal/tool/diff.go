package tool

import (
	"fmt"
	"strings"
)

// unifiedDiff generates a minimal unified diff between two strings.
// The output includes --- / +++ headers and @@ hunk markers with
// contextLines lines of surrounding context.
func unifiedDiff(oldName, newName, oldText, newText string, contextLines int) string {
	if contextLines < 0 {
		contextLines = 3
	}

	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	// Simple Myers-like diff using the longest common subsequence approach.
	// For tool result display this doesn't need to be optimal — just correct.
	edits := computeEdits(oldLines, newLines)

	// Check if there are any actual changes (not just keeps).
	hasChanges := false
	for _, e := range edits {
		if e.op != editKeep {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return "" // no changes
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n", oldName))
	sb.WriteString(fmt.Sprintf("+++ %s\n", newName))

	// Group edits into hunks with context.
	hunks := groupHunks(edits, len(oldLines), len(newLines), contextLines)
	for _, hunk := range hunks {
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
			hunk.oldStart+1, hunk.oldCount,
			hunk.newStart+1, hunk.newCount))
		for _, line := range hunk.lines {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

type editOp int

const (
	editKeep   editOp = iota
	editDelete        // line exists in old, not in new
	editInsert        // line exists in new, not in old
)

type edit struct {
	op      editOp
	oldIdx  int // index in oldLines (-1 for insert)
	newIdx  int // index in newLines (-1 for delete)
	content string
}

// computeEdits computes the edit sequence between oldLines and newLines
// using a simple LCS-based approach.
func computeEdits(oldLines, newLines []string) []edit {
	m, n := len(oldLines), len(newLines)

	// Build LCS table.
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	// Walk the LCS table to produce edits.
	var edits []edit
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			edits = append(edits, edit{op: editKeep, oldIdx: i, newIdx: j, content: oldLines[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			edits = append(edits, edit{op: editDelete, oldIdx: i, newIdx: -1, content: oldLines[i]})
			i++
		} else {
			edits = append(edits, edit{op: editInsert, oldIdx: -1, newIdx: j, content: newLines[j]})
			j++
		}
	}
	for ; i < m; i++ {
		edits = append(edits, edit{op: editDelete, oldIdx: i, newIdx: -1, content: oldLines[i]})
	}
	for ; j < n; j++ {
		edits = append(edits, edit{op: editInsert, oldIdx: -1, newIdx: j, content: newLines[j]})
	}

	return edits
}

type hunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	lines    []string
}

// groupHunks groups edits into hunks with context lines.
func groupHunks(edits []edit, oldLen, newLen, contextLines int) []hunk {
	// Find runs of changes and merge nearby ones.
	type changeRange struct {
		start, end int // indices into edits
	}

	var ranges []changeRange
	inChange := false
	start := 0
	for i, e := range edits {
		if e.op != editKeep {
			if !inChange {
				start = i
				inChange = true
			}
		} else {
			if inChange {
				ranges = append(ranges, changeRange{start, i})
				inChange = false
			}
		}
	}
	if inChange {
		ranges = append(ranges, changeRange{start, len(edits)})
	}

	if len(ranges) == 0 {
		return nil
	}

	// Merge ranges that are close together (within 2*contextLines).
	var merged []changeRange
	cur := ranges[0]
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start-cur.end <= 2*contextLines {
			cur.end = ranges[i].end
		} else {
			merged = append(merged, cur)
			cur = ranges[i]
		}
	}
	merged = append(merged, cur)

	// Build hunks with context.
	var hunks []hunk
	for _, r := range merged {
		// Expand range to include context.
		hunkStart := r.start - contextLines
		if hunkStart < 0 {
			hunkStart = 0
		}
		hunkEnd := r.end + contextLines
		if hunkEnd > len(edits) {
			hunkEnd = len(edits)
		}

		var h hunk
		h.oldStart = -1
		h.newStart = -1

		for i := hunkStart; i < hunkEnd; i++ {
			e := edits[i]
			switch e.op {
			case editKeep:
				h.lines = append(h.lines, " "+e.content)
				h.oldCount++
				h.newCount++
				if h.oldStart == -1 {
					h.oldStart = e.oldIdx
				}
				if h.newStart == -1 {
					h.newStart = e.newIdx
				}
			case editDelete:
				h.lines = append(h.lines, "-"+e.content)
				h.oldCount++
				if h.oldStart == -1 {
					h.oldStart = e.oldIdx
				}
				if h.newStart == -1 {
					// Use the corresponding new line position.
					// Find the next keep or insert to determine position.
					for j := i + 1; j < hunkEnd; j++ {
						if edits[j].newIdx >= 0 {
							h.newStart = edits[j].newIdx
							break
						}
					}
					if h.newStart == -1 {
						h.newStart = newLen
					}
				}
			case editInsert:
				h.lines = append(h.lines, "+"+e.content)
				h.newCount++
				if h.newStart == -1 {
					h.newStart = e.newIdx
				}
				if h.oldStart == -1 {
					// Find next keep or delete position.
					for j := i + 1; j < hunkEnd; j++ {
						if edits[j].oldIdx >= 0 {
							h.oldStart = edits[j].oldIdx
							break
						}
					}
					if h.oldStart == -1 {
						h.oldStart = oldLen
					}
				}
			}
		}

		if h.oldStart == -1 {
			h.oldStart = 0
		}
		if h.newStart == -1 {
			h.newStart = 0
		}

		hunks = append(hunks, h)
	}

	return hunks
}

// splitLines splits text into lines. An empty string returns an empty slice.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	// Remove trailing empty element from trailing newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
