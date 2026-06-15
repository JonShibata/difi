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
