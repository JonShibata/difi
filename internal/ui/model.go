package ui

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/xguot/difi/internal/config"
	"github.com/xguot/difi/internal/tree"
	"github.com/xguot/difi/internal/vcs"
)

type GlobalMatch struct {
	FilePath string
	Line     int    // line index within that file's diff
	Text     string // the matching line text (stripped)
}

type Focus int

const (
	FocusTree Focus = iota
	FocusDiff
)

var ansiRe = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
var bgAnsiRe = regexp.MustCompile(`\x1b\[48;2;\d+;\d+;\d+m|\x1b\[4[0-9]m`)
var resetAnsiRe = regexp.MustCompile(`\x1b\[0m`)

type StatsMsg struct {
	Added   int
	Deleted int
	ByFile  map[string][2]int
}

type Model struct {
	fileList     list.Model
	treeState    *tree.FileTree
	treeDelegate TreeDelegate
	diffViewport viewport.Model

	selectedPath  string
	currentBranch string
	targetBranch  string
	repoName      string

	statsAdded   int
	statsDeleted int

	currentFileAdded   int
	currentFileDeleted int

	fileStats map[string][2]int

	diffContent     string
	diffLines       []string
	diffHighlighted []string
	diffCursor      int
	visualMode      bool // Visual selection mode
	visualStart     int  // Anchor for visual selection

	inputBuffer string
	pendingZ    bool

	// Search state
	searchMode    bool
	searchInput   textinput.Model
	searchQuery   string
	searchMatches []int // line indices in diffLines that match
	searchIndex   int   // current position in searchMatches

	// Cross-file search
	globalSearchMode    bool
	globalSearchResults []GlobalMatch
	globalSearchIndex   int

	focus    Focus
	showHelp bool
	flatMode bool

	width, height int

	pipedDiff string
	vcs       vcs.VCS
}

func NewModel(cfg config.Config, targetBranch string, pipedDiff string, vcsClient vcs.VCS, flatMode bool) Model {
	InitStyles(cfg)

	var files []string
	if pipedDiff != "" {
		files = vcsClient.ParseFilesFromDiff(pipedDiff)
	} else {
		files, _ = vcsClient.ListChangedFiles(targetBranch)
	}
	t := tree.New(files)
	items := t.Items(flatMode)

	delegate := TreeDelegate{
		Config:  cfg,
		Focused: true,
	}

	l := list.New(items, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(false)
	l.DisableQuitKeybindings()

	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 256

	m := Model{
		fileList:      l,
		treeState:     t,
		treeDelegate:  delegate,
		diffViewport:  viewport.New(0, 0),
		focus:         FocusTree,
		currentBranch: vcsClient.GetCurrentBranch(),
		targetBranch:  targetBranch,
		repoName:      vcsClient.GetRepoName(),
		showHelp:      false,
		flatMode:      flatMode,
		inputBuffer:   "",
		pendingZ:      false,
		searchInput:   ti,
		pipedDiff:     pipedDiff,
		vcs:           vcsClient,
		visualMode:    false,
		visualStart:   0,
	}

	for idx, item := range items {
		if ti, ok := item.(tree.TreeItem); ok && !ti.IsDir {
			m.selectedPath = ti.FullPath
			m.fileList.Select(idx)
			break
		}
	}
	return m
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.selectedPath != "" {
		if m.pipedDiff != "" {
			cmds = append(cmds, func() tea.Msg {
				return vcs.DiffMsg{Content: m.vcs.ExtractFileDiff(m.pipedDiff, m.selectedPath)}
			})
		} else {
			cmds = append(cmds, m.vcs.DiffCmd(m.targetBranch, m.selectedPath))
		}
	}

	if m.pipedDiff == "" {
		cmds = append(cmds, m.fetchStatsCmd(m.targetBranch))
	} else {
		cmds = append(cmds, m.computePipedStatsCmd())
	}

	return tea.Batch(cmds...)
}

func (m Model) fetchStatsCmd(target string) tea.Cmd {
	return func() tea.Msg {
		added, deleted, err := m.vcs.DiffStats(target)
		if err != nil {
			return nil
		}
		byFile, _ := m.vcs.DiffStatsByFile(target)
		return StatsMsg{Added: added, Deleted: deleted, ByFile: byFile}
	}
}

func (m Model) computePipedStatsCmd() tea.Cmd {
	return func() tea.Msg {
		byFile := make(map[string][2]int)
		var totalAdded, totalDeleted int
		var currentFile string

		for _, line := range strings.Split(m.pipedDiff, "\n") {
			clean := stripAnsi(line)
			if strings.HasPrefix(clean, "diff --git ") {
				parts := strings.Fields(clean)
				if len(parts) >= 4 {
					currentFile = strings.TrimPrefix(parts[3], "b/")
				}
			} else if strings.HasPrefix(clean, "diff -r ") {
				parts := strings.Fields(clean)
				if len(parts) >= 3 {
					currentFile = parts[len(parts)-1]
				}
			} else if currentFile != "" {
				if strings.HasPrefix(clean, "+") && !strings.HasPrefix(clean, "+++") {
					s := byFile[currentFile]
					s[0]++
					byFile[currentFile] = s
					totalAdded++
				} else if strings.HasPrefix(clean, "-") && !strings.HasPrefix(clean, "---") {
					s := byFile[currentFile]
					s[1]++
					byFile[currentFile] = s
					totalDeleted++
				}
			}
		}
		return StatsMsg{Added: totalAdded, Deleted: totalDeleted, ByFile: byFile}
	}
}

func (m *Model) getRepeatCount() int {
	if m.inputBuffer == "" {
		return 1
	}
	count, err := strconv.Atoi(m.inputBuffer)
	if err != nil {
		return 1
	}
	m.inputBuffer = ""
	return count
}

func stripAnsi(str string) string {
	return ansiRe.ReplaceAllString(str, "")
}

func isDiffMetadata(cleanLine string) bool {
	return strings.HasPrefix(cleanLine, "diff --git") ||
		strings.HasPrefix(cleanLine, "diff -r ") ||
		strings.HasPrefix(cleanLine, "index ") ||
		strings.HasPrefix(cleanLine, "new file mode") ||
		strings.HasPrefix(cleanLine, "old mode") ||
		strings.HasPrefix(cleanLine, "--- a/") ||
		strings.HasPrefix(cleanLine, "--- /dev/") ||
		strings.HasPrefix(cleanLine, "+++ b/") ||
		strings.HasPrefix(cleanLine, "+++ /dev/") ||
		strings.HasPrefix(cleanLine, "@@")
}

func isDiffContentLine(cleanLine string) bool {
	cleanLine = strings.TrimRight(cleanLine, "\r")
	return strings.HasPrefix(cleanLine, " ") ||
		strings.HasPrefix(cleanLine, "+") ||
		strings.HasPrefix(cleanLine, "-")
}

func (m *Model) setYOffset(offset int) {
	maxOffset := len(m.diffLines) - m.diffViewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}

	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}

	m.diffViewport.YOffset = offset
}

func (m *Model) snapCursor(idx int, dir int) int {
	if len(m.diffLines) == 0 {
		return 0
	}

	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.diffLines) {
		idx = len(m.diffLines) - 1
	}

	curr := idx
	for curr >= 0 && curr < len(m.diffLines) {
		cleanLine := stripAnsi(m.diffLines[curr])
		if isDiffContentLine(cleanLine) {
			return curr
		}
		curr += dir
	}

	curr = idx
	for curr >= 0 && curr < len(m.diffLines) {
		cleanLine := stripAnsi(m.diffLines[curr])
		if isDiffContentLine(cleanLine) {
			return curr
		}
		curr -= dir
	}

	return m.diffCursor
}

func (m *Model) handleScrolling() {
	if m.diffCursor < m.diffViewport.YOffset {
		m.setYOffset(m.diffCursor)
	} else if m.diffCursor >= m.diffViewport.YOffset+m.diffViewport.Height {
		m.setYOffset(m.diffCursor - m.diffViewport.Height + 1)
	}
}

func (m *Model) centerDiffCursor() {
	targetOffset := m.diffCursor - (m.diffViewport.Height / 2)
	m.setYOffset(targetOffset)
}

func (m *Model) updateSizes() {
	reservedHeight := 2
	if m.showHelp {
		reservedHeight += 6
	}

	contentHeight := m.height - reservedHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	treeWidth := int(float64(m.width) * 0.20)
	if treeWidth < 20 {
		treeWidth = 20
	}

	treePaneOverhead := 4
	treeInnerWidth := treeWidth - treePaneOverhead
	if treeInnerWidth < 10 {
		treeInnerWidth = 10
	}

	listHeight := contentHeight - 2
	if listHeight < 1 {
		listHeight = 1
	}
	m.fileList.SetSize(treeInnerWidth, listHeight)

	m.diffViewport.Width = m.width - treeWidth
	m.diffViewport.Height = listHeight
}

func (m *Model) updateTreeFocus() {
	m.treeDelegate.Focused = (m.focus == FocusTree)
	m.fileList.SetDelegate(m.treeDelegate)
}

// --- Search ---

func (m *Model) findMatches() {
	m.searchMatches = nil
	if m.searchQuery == "" {
		return
	}
	q := strings.ToLower(m.searchQuery)
	for i, line := range m.diffLines {
		if strings.Contains(strings.ToLower(stripAnsi(line)), q) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
}

func (m *Model) jumpToMatch(idx int) {
	if len(m.searchMatches) == 0 {
		return
	}
	if idx < 0 {
		idx = len(m.searchMatches) - 1
	} else if idx >= len(m.searchMatches) {
		idx = 0
	}
	m.searchIndex = idx
	m.diffCursor = m.searchMatches[idx]
	m.centerDiffCursor()
}

func (m *Model) searchNext() {
	if len(m.searchMatches) == 0 {
		return
	}
	// Find next match after current cursor
	for i, lineIdx := range m.searchMatches {
		if lineIdx > m.diffCursor {
			m.jumpToMatch(i)
			return
		}
	}
	// Wrap around
	m.jumpToMatch(0)
}

func (m *Model) searchPrev() {
	if len(m.searchMatches) == 0 {
		return
	}
	// Find previous match before current cursor
	for i := len(m.searchMatches) - 1; i >= 0; i-- {
		if m.searchMatches[i] < m.diffCursor {
			m.jumpToMatch(i)
			return
		}
	}
	// Wrap around
	m.jumpToMatch(len(m.searchMatches) - 1)
}

// --- Global (cross-file) search ---

func (m *Model) globalSearch() {
	m.globalSearchResults = nil
	if m.searchQuery == "" {
		return
	}
	q := strings.ToLower(m.searchQuery)

	// Get all file paths from the tree
	for _, item := range m.fileList.Items() {
		ti, ok := item.(tree.TreeItem)
		if !ok || ti.IsDir {
			continue
		}

		var diffContent string
		if m.pipedDiff != "" {
			diffContent = m.vcs.ExtractFileDiff(m.pipedDiff, ti.FullPath)
		} else {
			diffContent = m.vcs.DiffSync(m.targetBranch, ti.FullPath)
		}

		for i, line := range strings.Split(diffContent, "\n") {
			clean := stripAnsi(line)
			if strings.Contains(strings.ToLower(clean), q) {
				m.globalSearchResults = append(m.globalSearchResults, GlobalMatch{
					FilePath: ti.FullPath,
					Line:     i,
					Text:     clean,
				})
			}
		}
	}
	m.globalSearchIndex = 0
}

func (m *Model) globalNext() tea.Cmd {
	if len(m.globalSearchResults) == 0 {
		return nil
	}
	m.globalSearchIndex++
	if m.globalSearchIndex >= len(m.globalSearchResults) {
		m.globalSearchIndex = 0
	}
	return m.jumpToGlobalMatch()
}

func (m *Model) globalPrev() tea.Cmd {
	if len(m.globalSearchResults) == 0 {
		return nil
	}
	m.globalSearchIndex--
	if m.globalSearchIndex < 0 {
		m.globalSearchIndex = len(m.globalSearchResults) - 1
	}
	return m.jumpToGlobalMatch()
}

func (m *Model) jumpToGlobalMatch() tea.Cmd {
	if len(m.globalSearchResults) == 0 {
		return nil
	}
	match := m.globalSearchResults[m.globalSearchIndex]

	// Switch to the file if needed
	if match.FilePath != m.selectedPath {
		// Find and select the file in the tree
		for idx, item := range m.fileList.Items() {
			if ti, ok := item.(tree.TreeItem); ok && ti.FullPath == match.FilePath {
				m.fileList.Select(idx)
				m.selectedPath = match.FilePath
				m.diffCursor = 0
				m.diffViewport.GotoTop()

				// Load the diff, then jump to line on receipt
				if m.pipedDiff != "" {
					return func() tea.Msg {
						return vcs.DiffMsg{Content: m.vcs.ExtractFileDiff(m.pipedDiff, match.FilePath)}
					}
				}
				return m.vcs.DiffCmd(m.targetBranch, match.FilePath)
			}
		}
	}

	// Same file — just jump to the line
	if match.Line < len(m.diffLines) {
		m.diffCursor = match.Line
		m.centerDiffCursor()
	}
	return nil
}
