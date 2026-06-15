package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"github.com/xguot/difi/internal/tree"
)

func TestCopyPathDiffFocus(t *testing.T) {
	m := newTestModel(nil)
	m.focus = FocusDiff
	m.selectedPath = "internal/ui/model.go"

	if got := m.copyPath(); got != "internal/ui/model.go" {
		t.Fatalf("diff focus: expected file path, got %q", got)
	}
}

func TestCopyPathTreeFocus(t *testing.T) {
	items := []list.Item{
		tree.TreeItem{Name: "ui", FullPath: "internal/ui", IsDir: true},
		tree.TreeItem{Name: "model.go", FullPath: "internal/ui/model.go"},
	}
	m := newTestModel(nil)
	m.fileList = list.New(items, TreeDelegate{}, 20, 10)
	m.focus = FocusTree
	// selectedPath points at the file, but tree focus copies the highlighted
	// item — here a directory.
	m.selectedPath = "internal/ui/model.go"

	m.fileList.Select(0)
	if got := m.copyPath(); got != "internal/ui" {
		t.Fatalf("tree focus on dir: expected 'internal/ui', got %q", got)
	}

	m.fileList.Select(1)
	if got := m.copyPath(); got != "internal/ui/model.go" {
		t.Fatalf("tree focus on file: expected file path, got %q", got)
	}
}

func TestCopySelectionSingleLine(t *testing.T) {
	m := newTestModel([]string{
		"@@ -1,3 +1,4 @@",
		" context",
		"+added line",
		"-removed line",
	})
	m.diffCursor = 2 // "+added line"

	text, n := m.copySelection()
	if n != 1 || text != "added line" {
		t.Fatalf("single line: expected (\"added line\", 1), got (%q, %d)", text, n)
	}
}

func TestCopySelectionVisualRange(t *testing.T) {
	m := newTestModel([]string{
		"@@ -1,3 +1,4 @@",
		" context",
		"+added line",
		"-removed line",
	})
	m.visualMode = true
	m.visualStart = 1
	m.diffCursor = 3

	text, n := m.copySelection()
	want := "context\nadded line\nremoved line"
	if n != 3 || text != want {
		t.Fatalf("visual range: expected (%q, 3), got (%q, %d)", want, text, n)
	}
}

func TestCopySelectionSkipsHunkHeader(t *testing.T) {
	m := newTestModel([]string{
		"@@ -1,3 +1,4 @@",
		"+a",
		"+b",
	})
	m.visualMode = true
	m.visualStart = 0 // anchored on the @@ hunk header
	m.diffCursor = 2

	text, n := m.copySelection()
	if n != 2 || text != "a\nb" {
		t.Fatalf("skip hunk header: expected (\"a\\nb\", 2), got (%q, %d)", text, n)
	}
}
