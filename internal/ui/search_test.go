package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
)

func newTestModel(lines []string) Model {
	ti := textinput.New()
	ti.Prompt = "/"
	m := Model{
		diffLines:    lines,
		diffViewport: viewport.New(80, 20),
		searchInput:  ti,
	}
	return m
}

func TestFindMatches(t *testing.T) {
	m := newTestModel([]string{
		"@@ -1,3 +1,4 @@",
		" func main() {",
		"+\tfmt.Println(\"hello\")",
		"-\tfmt.Println(\"world\")",
		" }",
	})

	m.searchQuery = "Println"
	m.findMatches()

	if len(m.searchMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.searchMatches))
	}
	if m.searchMatches[0] != 2 || m.searchMatches[1] != 3 {
		t.Fatalf("expected matches at [2,3], got %v", m.searchMatches)
	}
}

func TestFindMatchesCaseInsensitive(t *testing.T) {
	m := newTestModel([]string{
		" Hello World",
		" hello world",
		" HELLO WORLD",
	})

	m.searchQuery = "hello"
	m.findMatches()

	if len(m.searchMatches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(m.searchMatches))
	}
}

func TestFindMatchesNoResults(t *testing.T) {
	m := newTestModel([]string{
		" func main() {",
		" }",
	})

	m.searchQuery = "notfound"
	m.findMatches()

	if len(m.searchMatches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(m.searchMatches))
	}
}

func TestSearchNextWraps(t *testing.T) {
	m := newTestModel([]string{
		" line0",
		" match1",
		" line2",
		" match3",
	})

	m.searchQuery = "match"
	m.findMatches()

	if len(m.searchMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.searchMatches))
	}

	// Start before first match
	m.diffCursor = 0
	m.searchNext()
	if m.searchIndex != 0 || m.diffCursor != 1 {
		t.Fatalf("first next: expected index=0 cursor=1, got index=%d cursor=%d", m.searchIndex, m.diffCursor)
	}

	m.searchNext()
	if m.searchIndex != 1 || m.diffCursor != 3 {
		t.Fatalf("second next: expected index=1 cursor=3, got index=%d cursor=%d", m.searchIndex, m.diffCursor)
	}

	// Should wrap
	m.searchNext()
	if m.searchIndex != 0 || m.diffCursor != 1 {
		t.Fatalf("wrap next: expected index=0 cursor=1, got index=%d cursor=%d", m.searchIndex, m.diffCursor)
	}
}

func TestSearchPrevWraps(t *testing.T) {
	m := newTestModel([]string{
		" match0",
		" line1",
		" match2",
	})

	m.searchQuery = "match"
	m.findMatches()

	// Start at end
	m.diffCursor = 2
	m.searchPrev()
	if m.searchIndex != 0 || m.diffCursor != 0 {
		t.Fatalf("first prev: expected index=0 cursor=0, got index=%d cursor=%d", m.searchIndex, m.diffCursor)
	}

	// Should wrap to last
	m.searchPrev()
	if m.searchIndex != 1 || m.diffCursor != 2 {
		t.Fatalf("wrap prev: expected index=1 cursor=2, got index=%d cursor=%d", m.searchIndex, m.diffCursor)
	}
}

func TestIsSearchMatch(t *testing.T) {
	m := newTestModel([]string{" a", " b", " c"})
	m.searchQuery = "b"
	m.findMatches()

	if m.isSearchMatch(0) {
		t.Fatal("line 0 should not match")
	}
	if !m.isSearchMatch(1) {
		t.Fatal("line 1 should match")
	}
}
