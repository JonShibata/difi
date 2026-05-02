package ui

import (
	"strings"
	"testing"
)

// TestVisualLinesForWrapOff verifies the helper is a no-op when wrap is
// disabled — the rest of the cursor/scroll math depends on this returning
// 1 so it stays compatible with pre-wrap behavior.
func TestVisualLinesForWrapOff(t *testing.T) {
	m := newTestModel([]string{
		"+" + strings.Repeat("A", 500),
	})
	m.wrap = false
	if got := m.visualLinesFor(0); got != 1 {
		t.Fatalf("wrap off: visualLinesFor=%d, want 1", got)
	}
}

// TestVisualLinesForWrapOn verifies the row-count math: a 500-char content
// line at width 80 (chunkWidth = 80-7-4 = 69) should produce ceil(500/69) = 8
// rows. The first column of the diff (+/-/space) is stripped before measure.
func TestVisualLinesForWrapOn(t *testing.T) {
	m := newTestModel([]string{
		"+" + strings.Repeat("A", 500),
	})
	m.wrap = true
	got := m.visualLinesFor(0)
	chunkWidth := 80 - 7 - 4
	want := (500 + chunkWidth - 1) / chunkWidth
	if got != want {
		t.Fatalf("wrap on, 500 chars, w=80: visualLinesFor=%d, want %d", got, want)
	}
}

// TestVisualLinesForMetadata ensures @@/--- lines never report wrap rows;
// the renderer treats them as exactly one row.
func TestVisualLinesForMetadata(t *testing.T) {
	m := newTestModel([]string{
		"@@ -1,3 +1," + strings.Repeat("9", 200) + " @@",
		"--- a/" + strings.Repeat("x", 200),
	})
	m.wrap = true
	for i := range m.diffLines {
		if got := m.visualLinesFor(i); got != 1 {
			t.Fatalf("metadata line %d: visualLinesFor=%d, want 1", i, got)
		}
	}
}

// TestVisualLinesForOutOfRange — defensive: callers (handleScrolling,
// centerDiffCursor) walk source-line ranges and would crash if this didn't
// guard against bad indices.
func TestVisualLinesForOutOfRange(t *testing.T) {
	m := newTestModel([]string{" line 1"})
	m.wrap = true
	if got := m.visualLinesFor(-1); got != 1 {
		t.Fatalf("idx=-1: got %d, want 1", got)
	}
	if got := m.visualLinesFor(100); got != 1 {
		t.Fatalf("idx=100 (out of range): got %d, want 1", got)
	}
}

// TestHandleScrollingWrapKeepsCursorVisible is the integration check: a
// wrapping source line that would push the cursor past the bottom should
// trigger a scroll. With wrap off, the cursor at line 19 fits in a 20-row
// viewport at YOffset=0; with wrap on (and a 500-char line at YOffset that
// occupies many rows), it doesn't, so YOffset should advance.
func TestHandleScrollingWrapAdvancesOffset(t *testing.T) {
	lines := []string{"+" + strings.Repeat("A", 500)}
	for i := 0; i < 19; i++ {
		lines = append(lines, " line "+string(rune('a'+i)))
	}
	m := newTestModel(lines)
	m.wrap = true
	m.diffCursor = 19
	m.handleScrolling()
	if m.diffViewport.YOffset == 0 {
		t.Fatalf("wrap on with long line at top: YOffset stayed 0; expected scroll forward")
	}
}
