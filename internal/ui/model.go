package ui

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alecthomas/chroma/v2/quick"
	osc52 "github.com/aymanbagabas/go-osc52/v2"

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

// RefreshedPipedDiffMsg carries the output of re-running DIFI_REFRESH_CMD
// after $EDITOR exits. The Update loop replaces m.pipedDiff with Content
// and rebuilds the file tree, current diff, and stats.
type RefreshedPipedDiffMsg struct {
	Content string
	Err     error
}

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

	// copyStatus holds a transient "Copied <path>" notice shown in the status
	// bar after the 'c' key. Cleared on the next keypress.
	copyStatus string

	width, height int

	pipedDiff  string
	refreshCmd string // shell command run via `sh -c` to refresh pipedDiff

	// wrap mirrors cfg.UI.Wrap but is mutable at runtime via the 'w' key.
	// When true, long lines render on multiple visual rows with a
	// continuation gutter; when false, they're truncated with "…".
	wrap bool

	// Cursor restore across editor invocations. Set when launching $EDITOR
	// and consumed by the next vcs.DiffMsg so the user lands back on the
	// line they were editing instead of being yanked to the top.
	preEditPath string
	preEditLine int
	vcs        vcs.VCS
}

func NewModel(cfg config.Config, targetBranch string, pipedDiff string, refreshCmd string, vcsClient vcs.VCS, flatMode bool) Model {
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
		refreshCmd:    refreshCmd,
		vcs:           vcsClient,
		visualMode:    false,
		visualStart:   0,
		wrap:          cfg.UI.Wrap,
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

// refreshPipedDiffCmd runs DIFI_REFRESH_CMD via `sh -c` and returns the
// captured stdout as a RefreshedPipedDiffMsg. Returns nil if no refresh
// command is configured.
func (m Model) refreshPipedDiffCmd() tea.Cmd {
	if m.refreshCmd == "" {
		return nil
	}
	cmd := m.refreshCmd
	return func() tea.Msg {
		out, err := exec.Command("sh", "-c", cmd).Output()
		return RefreshedPipedDiffMsg{Content: string(out), Err: err}
	}
}

// copyPath returns the value the 'c' key copies to the clipboard: the path of
// the highlighted tree item when the tree is focused (a directory's path, or a
// file's path), otherwise the file whose diff is shown in the right pane.
func (m Model) copyPath() string {
	if m.focus == FocusTree {
		if item, ok := m.fileList.SelectedItem().(tree.TreeItem); ok {
			return item.FullPath
		}
	}
	return m.selectedPath
}

// copyToClipboardCmd copies s to the system clipboard via an OSC52 escape
// sequence, which works on local terminals, through tmux, and over SSH without
// shelling out to xclip/pbcopy. The sequence is written to /dev/tty so it
// reaches the terminal directly rather than racing Bubble Tea's renderer on
// stdout; stdout is the fallback when /dev/tty can't be opened.
func copyToClipboardCmd(s string) tea.Cmd {
	return func() tea.Msg {
		seq := osc52.New(s)
		if os.Getenv("TMUX") != "" {
			seq = seq.Tmux()
		}
		if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
			seq.WriteTo(tty)
			tty.Close()
		} else {
			seq.WriteTo(os.Stdout)
		}
		return nil
	}
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

// findDiffIndexForFileLine returns the diff-line index whose new-file line
// number is the closest match (<=) to fileLine within the same hunk. Returns
// -1 if no hunk covers fileLine.
func findDiffIndexForFileLine(diffLines []string, fileLine int) int {
	if fileLine < 1 {
		return -1
	}
	newLineNum := 0
	bestIdx := -1
	for i, raw := range diffLines {
		clean := strings.TrimRight(stripAnsi(raw), "\r")
		if strings.HasPrefix(clean, "@@") {
			newLineNum = parseHunkNewStart(clean)
			continue
		}
		if newLineNum == 0 {
			continue
		}
		isPlus := strings.HasPrefix(clean, "+") && !strings.HasPrefix(clean, "+++")
		isCtx := strings.HasPrefix(clean, " ")
		if !(isPlus || isCtx) {
			continue
		}
		if newLineNum == fileLine {
			return i
		}
		if newLineNum < fileLine {
			bestIdx = i
		}
		newLineNum++
	}
	return bestIdx
}

// parseHunkNewStart returns the starting line in the new file from a unified
// diff hunk header like "@@ -10,7 +12,8 @@". Returns 0 if it can't parse.
func parseHunkNewStart(hdr string) int {
	for _, p := range strings.Fields(hdr) {
		if !strings.HasPrefix(p, "+") {
			continue
		}
		s := p[1:]
		if comma := strings.Index(s, ","); comma > 0 {
			s = s[:comma]
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// highlightFile reads path and runs chroma over the whole file, returning the
// per-line highlighted output. ok=false if the file can't be read or chroma
// fails.
func highlightFile(path, ext, theme string) ([]string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var buf strings.Builder
	if err := quick.Highlight(&buf, string(data), ext, "terminal16m", theme); err != nil {
		return nil, false
	}
	return strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n"), true
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

// visualLinesFor returns the number of viewport rows source line `i` will
// occupy at the current diff-pane width. Returns 1 when wrap is off, when
// the index is out of range, or for diff metadata. The cursor/scroll math
// uses this to keep multi-row source lines fully visible.
func (m *Model) visualLinesFor(i int) int {
	if !m.wrap || i < 0 || i >= len(m.diffLines) {
		return 1
	}
	cleanLine := stripAnsi(m.diffLines[i])
	if isDiffMetadata(cleanLine) {
		return 1
	}
	chunkWidth := m.diffViewport.Width - 7 - 4 // gutter(7) + lineno-pane(4) headroom
	if chunkWidth < 1 {
		return 1
	}
	codeContent := cleanLine
	if len(codeContent) > 0 && (strings.HasPrefix(codeContent, "+") ||
		strings.HasPrefix(codeContent, "-") ||
		strings.HasPrefix(codeContent, " ")) {
		codeContent = codeContent[1:]
	}
	visWidth := lipgloss.Width(codeContent)
	rows := (visWidth + chunkWidth - 1) / chunkWidth
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *Model) setYOffset(offset int) {
	// With wrap on, a source line can occupy multiple visual rows, so the
	// "fits exactly N source lines" math no longer applies. Bound only at
	// the lower end and at the last source line; the renderer will stop
	// drawing once it fills the viewport. The user can never scroll past
	// the last source line being at the top of the pane.
	maxOffset := len(m.diffLines) - 1
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
		return
	}

	// With wrap off this is the original "cursor − height + 1" math. With
	// wrap on we sum visual rows from YOffset through the cursor; if that
	// exceeds the viewport we advance YOffset until the cursor's source
	// line fits at the bottom (or scroll cursor to top if it can't).
	height := m.diffViewport.Height
	if height < 1 {
		height = 1
	}

	if !m.wrap {
		if m.diffCursor >= m.diffViewport.YOffset+height {
			m.setYOffset(m.diffCursor - height + 1)
		}
		return
	}

	// Loop is bounded by (cursor - YOffset), so worst case O(viewport rows).
	for m.diffViewport.YOffset < m.diffCursor {
		visRows := 0
		for i := m.diffViewport.YOffset; i <= m.diffCursor; i++ {
			visRows += m.visualLinesFor(i)
		}
		if visRows <= height {
			return
		}
		m.diffViewport.YOffset++
	}
}

func (m *Model) centerDiffCursor() {
	// In wrap mode "half the viewport" is in visual rows but YOffset is in
	// source lines, so naive subtraction overshoots when the cursor's line
	// (and surrounding lines) wrap. Walk backwards from the cursor summing
	// visual heights until we've consumed half the viewport.
	if !m.wrap {
		targetOffset := m.diffCursor - (m.diffViewport.Height / 2)
		m.setYOffset(targetOffset)
		return
	}

	half := m.diffViewport.Height / 2
	used := 0
	target := m.diffCursor
	for target > 0 {
		used += m.visualLinesFor(target - 1)
		if used > half {
			break
		}
		target--
	}
	m.setYOffset(target)
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

	for _, item := range m.fileList.Items() {
		ti, ok := item.(tree.TreeItem)
		if !ok || ti.IsDir {
			continue
		}

		var diffContent string
		if m.pipedDiff != "" {
			diffContent = m.vcs.ExtractFileDiff(m.pipedDiff, ti.FullPath)
		} else {
			// Non-piped: fetch diff synchronously (safe — just runs git/hg diff)
			diffContent = m.vcs.DiffSync(m.targetBranch, ti.FullPath)
		}
		if diffContent == "" {
			continue
		}

		// Only search lines from @@ onward (matching diffLines in the viewer)
		foundHunk := false
		lineIdx := 0
		for _, line := range strings.Split(diffContent, "\n") {
			clean := stripAnsi(line)
			if strings.HasPrefix(clean, "@@") {
				foundHunk = true
			}
			if !foundHunk {
				continue
			}
			if strings.Contains(strings.ToLower(clean), q) {
				m.globalSearchResults = append(m.globalSearchResults, GlobalMatch{
					FilePath: ti.FullPath,
					Line:     lineIdx,
					Text:     clean,
				})
			}
			lineIdx++
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

// highlightMatchesInRendered injects background-color ANSI codes around every
// occurrence of query in the already-rendered (chroma-highlighted) line.
// Existing chroma foreground colors are preserved; chroma background writes
// inside a match span are suppressed so the match bg shows through. After
// each match span, restoreBg (the line's own background, e.g. add/del bg, or
// "" for context lines) is re-applied so the surrounding styling continues.
func highlightMatchesInRendered(rendered, plain, query, restoreBg string) string {
	if query == "" || plain == "" {
		return rendered
	}
	lq := strings.ToLower(query)
	lp := strings.ToLower(plain)

	type span struct{ start, end int }
	var spans []span
	i := 0
	for i <= len(lp)-len(lq) {
		idx := strings.Index(lp[i:], lq)
		if idx < 0 {
			break
		}
		s := i + idx
		e := s + len(lq)
		spans = append(spans, span{s, e})
		i = e
	}
	if len(spans) == 0 {
		return rendered
	}

	matchBg := MatchHighlightBgAnsi
	matchEnd := "\x1b[49m" + restoreBg

	var out strings.Builder
	out.Grow(len(rendered) + len(spans)*32)
	vis := 0
	sIdx := 0
	inMatch := false
	j := 0
	for j < len(rendered) {
		if rendered[j] == 0x1b && j+1 < len(rendered) && rendered[j+1] == '[' {
			k := j + 2
			for k < len(rendered) && rendered[k] != 'm' {
				k++
			}
			if k < len(rendered) {
				k++
			}
			esc := rendered[j:k]
			if inMatch && bgAnsiRe.MatchString(esc) {
				// Suppress chroma/line bg writes while inside a match span.
			} else {
				out.WriteString(esc)
				if inMatch && esc == "\x1b[0m" {
					out.WriteString(matchBg)
				}
			}
			j = k
			continue
		}
		for sIdx < len(spans) && vis == spans[sIdx].end {
			out.WriteString(matchEnd)
			inMatch = false
			sIdx++
		}
		if sIdx < len(spans) && vis == spans[sIdx].start {
			out.WriteString(matchBg)
			inMatch = true
		}
		out.WriteByte(rendered[j])
		j++
		vis++
	}
	if inMatch {
		out.WriteString(matchEnd)
	}
	return out.String()
}
