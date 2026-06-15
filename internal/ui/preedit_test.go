package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xguot/difi/internal/vcs"
)

type fakeVCS struct{}

func (fakeVCS) GetCurrentBranch() string                          { return "" }
func (fakeVCS) GetRepoName() string                               { return "" }
func (fakeVCS) ListChangedFiles(string) ([]string, error)         { return nil, nil }
func (fakeVCS) DiffCmd(string, string, int) tea.Cmd               { return nil }
func (fakeVCS) DiffSync(string, string, int) string               { return "" }
func (fakeVCS) OpenEditorCmd(string, int, string, string) tea.Cmd { return nil }
func (fakeVCS) DiffStats(string) (int, int, error)                { return 0, 0, nil }
func (fakeVCS) DiffStatsByFile(string) (map[string][2]int, error) { return nil, nil }
func (fakeVCS) ParseFilesFromDiff(string) []string                { return nil }
func (fakeVCS) ExtractFileDiff(string, string) string             { return "" }
func (fakeVCS) CalculateFileLine(diffLines []string, idx int) int {
	// Mirror the real algorithm enough for the test: walk hunks, count + and ctx.
	cur, mapped := 1, 1
	inHunk := false
	if idx >= len(diffLines) {
		idx = len(diffLines) - 1
	}
	for i := 0; i <= idx; i++ {
		l := strings.TrimRight(diffLines[i], "\r")
		if strings.HasPrefix(l, "@@") {
			// parse @@ -a,b +c,d @@
			for _, p := range strings.Fields(l) {
				if strings.HasPrefix(p, "+") {
					s := p[1:]
					if c := strings.Index(s, ","); c > 0 {
						s = s[:c]
					}
					n := 0
					for _, ch := range s {
						if ch < '0' || ch > '9' {
							break
						}
						n = n*10 + int(ch-'0')
					}
					cur = n
					mapped = cur
					break
				}
			}
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "+") {
			mapped = cur
			cur++
		}
	}
	return mapped
}

func TestFindDiffIndexForFileLine(t *testing.T) {
	// Hunk starts at new-file line 10. Diff layout:
	//   idx 0: "@@ -10,5 +10,6 @@"
	//   idx 1: " ctx10"   -> new line 10
	//   idx 2: " ctx11"   -> new line 11
	//   idx 3: "-old12"   -> (removed; no new line)
	//   idx 4: "+new12"   -> new line 12
	//   idx 5: "+new13"   -> new line 13
	//   idx 6: " ctx14"   -> new line 14
	diff := []string{
		"@@ -10,5 +10,6 @@",
		" ctx10",
		" ctx11",
		"-old12",
		"+new12",
		"+new13",
		" ctx14",
	}

	cases := []struct {
		fileLine int
		want     int
	}{
		{10, 1},
		{11, 2},
		{12, 4},
		{13, 5},
		{14, 6},
		// Beyond the hunk: should return the closest preceding match.
		{99, 6},
		// Before the hunk: nothing.
		{1, -1},
	}
	for _, c := range cases {
		got := findDiffIndexForFileLine(diff, c.fileLine)
		if got != c.want {
			t.Errorf("fileLine=%d: got %d, want %d", c.fileLine, got, c.want)
		}
	}
}

func TestFindDiffIndexForFileLineMultipleHunks(t *testing.T) {
	diff := []string{
		"@@ -10,2 +10,2 @@",
		" a10",
		" a11",
		"@@ -50,3 +60,3 @@",
		" b60",
		"+b61",
		" b62",
	}
	cases := []struct {
		fileLine int
		want     int
	}{
		{10, 1},
		{11, 2},
		{60, 4},
		{61, 5},
		{62, 6},
	}
	for _, c := range cases {
		got := findDiffIndexForFileLine(diff, c.fileLine)
		if got != c.want {
			t.Errorf("fileLine=%d: got %d, want %d", c.fileLine, got, c.want)
		}
	}
}

func newDiffMsgTestModel() Model {
	m := newTestModel(nil)
	m.vcs = fakeVCS{}
	m.selectedPath = "foo.go"
	return m
}

// TestPreEditCursorRestore drives the full Update loop with a vcs.DiffMsg
// after preEdit info is set, verifying the cursor lands on the edited line.
func TestPreEditCursorRestore(t *testing.T) {
	m := newDiffMsgTestModel()

	// Diff: hunk @ new line 50, three context, one + at new line 53.
	diff := strings.Join([]string{
		"@@ -50,4 +50,5 @@",
		" line50",
		" line51",
		" line52",
		"+line53",
		" line54",
	}, "\n")

	// Simulate the user having opened the editor at file line 53.
	m.preEditPath = "foo.go"
	m.preEditLine = 53

	updated, _ := m.Update(vcs.DiffMsg{Content: diff})
	got := updated.(Model).diffCursor

	// Expected diff index for new-file line 53:
	//   0: @@
	//   1: " line50"  (50)
	//   2: " line51"  (51)
	//   3: " line52"  (52)
	//   4: "+line53"  (53)
	want := 4
	if got != want {
		t.Fatalf("diffCursor: got %d want %d", got, want)
	}
	mUpdated := updated.(Model)
	if mUpdated.preEditPath != "" || mUpdated.preEditLine != 0 {
		t.Errorf("preEdit fields not cleared: path=%q line=%d", mUpdated.preEditPath, mUpdated.preEditLine)
	}
}

// TestPreEditCursorRestoreWithSearch ensures a stale search query doesn't
// override the editor cursor restore.
func TestPreEditCursorRestoreWithSearch(t *testing.T) {
	m := newDiffMsgTestModel()
	diff := strings.Join([]string{
		"@@ -1,5 +1,6 @@",
		" alpha",
		" beta",
		" gamma",
		"+delta",
		" epsilon",
	}, "\n")
	m.preEditPath = "foo.go"
	m.preEditLine = 4 // -> diff index 4 ("+delta")
	m.searchQuery = "alpha"

	updated, _ := m.Update(vcs.DiffMsg{Content: diff})
	got := updated.(Model).diffCursor
	if got != 4 {
		t.Fatalf("with stale search query, cursor jumped to %d (want 4 = edited line)", got)
	}
}
