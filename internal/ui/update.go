package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/xguot/difi/internal/tree"
	"github.com/xguot/difi/internal/vcs"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	keyHandled := false

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

	case StatsMsg:
		m.statsAdded = msg.Added
		m.statsDeleted = msg.Deleted
		if msg.ByFile != nil {
			m.fileStats = msg.ByFile
		}

	case tea.KeyMsg:
		// --- Search input mode ---
		if m.searchMode {
			switch msg.String() {
			case "enter":
				m.searchMode = false
				query := m.searchInput.Value()
				m.searchInput.Blur()
				if query == "" {
					return m, nil
				}
				m.searchQuery = query
				m.focus = FocusDiff
				m.updateTreeFocus()
				m.findMatches()
				if len(m.searchMatches) > 0 {
					m.jumpToMatch(0)
				}
				m.globalSearch()
				m.globalSearchMode = len(m.globalSearchResults) > 0
				return m, nil
			case "esc":
				m.searchMode = false
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				return m, nil
			default:
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}

		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if msg.String() == "?" {
			m.showHelp = !m.showHelp
			m.updateSizes()
			return m, nil
		}

		if len(m.fileList.Items()) == 0 {
			return m, nil
		}

		if m.pendingZ {
			m.pendingZ = false
			if m.focus == FocusDiff {
				switch msg.String() {
				case "z", ".":
					m.centerDiffCursor()
				case "t":
					m.setYOffset(m.diffCursor)
				case "b":
					m.setYOffset(m.diffCursor - m.diffViewport.Height + 1)
				}
			}
			return m, nil
		}

		if len(msg.String()) == 1 && strings.ContainsAny(msg.String(), "0123456789") {
			m.inputBuffer += msg.String()
			return m, nil
		}

		switch msg.String() {
		case "V":
			if m.focus == FocusDiff {
				m.visualMode = !m.visualMode
				if m.visualMode {
					m.visualStart = m.diffCursor
				}
			}
			m.inputBuffer = ""

		case "esc":
			m.visualMode = false
			m.searchQuery = ""
			m.searchMatches = nil
			m.globalSearchMode = false
			m.globalSearchResults = nil
			m.inputBuffer = ""

		case "tab":
			m.visualMode = false
			if m.focus == FocusTree {
				if item, ok := m.fileList.SelectedItem().(tree.TreeItem); ok && item.IsDir {
					return m, nil
				}
				m.focus = FocusDiff
			} else {
				m.focus = FocusTree
			}
			m.updateTreeFocus()
			m.inputBuffer = ""

		case "ctrl+h", "[":
			m.visualMode = false
			m.focus = FocusTree
			m.updateTreeFocus()
			m.inputBuffer = ""

		case "ctrl+l", "]":
			m.visualMode = false
			if m.focus == FocusTree {
				if item, ok := m.fileList.SelectedItem().(tree.TreeItem); ok && item.IsDir {
					return m, nil
				}
			}
			m.focus = FocusDiff
			m.updateTreeFocus()
			m.inputBuffer = ""

		case "h", "left":
			m.visualMode = false
			keyHandled = true
			m.focus = FocusTree
			m.updateTreeFocus()
			m.inputBuffer = ""

		case "l", "right":
			m.visualMode = false
			keyHandled = true
			if item, ok := m.fileList.SelectedItem().(tree.TreeItem); ok && item.IsDir {
				return m, nil
			}
			m.focus = FocusDiff
			m.updateTreeFocus()
			m.inputBuffer = ""

		case "f":
			if m.focus == FocusTree {
				m.flatMode = !m.flatMode
				m.fileList.SetItems(m.treeState.Items(m.flatMode))
				for i, item := range m.fileList.Items() {
					if ti, ok := item.(tree.TreeItem); ok && ti.FullPath == m.selectedPath {
						m.fileList.Select(i)
						break
					}
				}
				return m, nil
			}

		case "enter", "e":
			m.visualMode = false
			if m.focus == FocusTree && msg.String() == "enter" {
				if i, ok := m.fileList.SelectedItem().(tree.TreeItem); ok && i.IsDir {
					m.treeState.ToggleExpand(i.FullPath)
					m.fileList.SetItems(m.treeState.Items(m.flatMode))
					return m, nil
				}
			}

			if m.selectedPath != "" {
				if i, ok := m.fileList.SelectedItem().(tree.TreeItem); ok && i.IsDir {
					return m, nil
				}

				line := 0
				if m.focus == FocusDiff {
					line = m.vcs.CalculateFileLine(m.diffLines, m.diffCursor)
				} else {
					line = m.vcs.CalculateFileLine(m.diffLines, 0)
				}
				m.inputBuffer = ""
				return m, m.vcs.OpenEditorCmd(m.selectedPath, line, m.targetBranch, m.treeDelegate.Config.Editor)
			}

		case "r":
			// Reload: re-run DIFI_REFRESH_CMD and rebuild the view. Useful
			// when an external process (agent, editor outside difi, build
			// step) has changed files behind difi's back.
			if refresh := m.refreshPipedDiffCmd(); refresh != nil {
				m.inputBuffer = ""
				return m, refresh
			}

		case "z":
			if m.focus == FocusDiff {
				m.pendingZ = true
				return m, nil
			}

		case "H":
			if m.focus == FocusDiff {
				m.diffCursor = m.snapCursor(m.diffViewport.YOffset, 1)
			}

		case "M":
			if m.focus == FocusDiff {
				half := m.diffViewport.Height / 2
				m.diffCursor = m.snapCursor(m.diffViewport.YOffset+half, 1)
			}

		case "L":
			if m.focus == FocusDiff {
				end := m.diffViewport.YOffset + m.diffViewport.Height - 1
				m.diffCursor = m.snapCursor(end, -1)
			}

		case "ctrl+d":
			if m.focus == FocusDiff {
				target := m.diffCursor + m.diffViewport.Height/2
				m.diffCursor = m.snapCursor(target, 1)
				m.centerDiffCursor()
			}
			m.inputBuffer = ""

		case "ctrl+u":
			if m.focus == FocusDiff {
				target := m.diffCursor - m.diffViewport.Height/2
				m.diffCursor = m.snapCursor(target, -1)
				m.centerDiffCursor()
			}
			m.inputBuffer = ""

		case "j", "down":
			keyHandled = true
			for i := 0; i < m.getRepeatCount(); i++ {
				if m.focus == FocusDiff {
					m.diffCursor = m.snapCursor(m.diffCursor+1, 1)
					m.handleScrolling()
				} else {
					m.fileList.CursorDown()
				}
			}
			m.inputBuffer = ""

		case "k", "up":
			keyHandled = true
			for i := 0; i < m.getRepeatCount(); i++ {
				if m.focus == FocusDiff {
					m.diffCursor = m.snapCursor(m.diffCursor-1, -1)
					m.handleScrolling()
				} else {
					m.fileList.CursorUp()
				}
			}
			m.inputBuffer = ""

		case "g":
			if m.focus == FocusDiff {
				if m.inputBuffer == "g" {
					m.diffCursor = m.snapCursor(0, 1)
					m.setYOffset(m.diffCursor)
					m.inputBuffer = ""
				} else {
					m.inputBuffer = "g"
				}
			}

		case "G":
			if m.focus == FocusDiff {
				count, err := strconv.Atoi(m.inputBuffer)
				if err == nil && count > 0 {
					target := count - 1
					m.diffCursor = m.snapCursor(target, 1)
				} else {
					m.diffCursor = m.snapCursor(len(m.diffLines)-1, -1)
				}
				m.setYOffset(m.diffCursor - m.diffViewport.Height + 1)
				m.inputBuffer = ""
			}

		case "/":
			m.searchMode = true
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			m.inputBuffer = ""
			return m, textinput.Blink

		case "n":
			if m.globalSearchMode {
				cmd = m.globalNext()
				return m, cmd
			} else if m.searchQuery != "" {
				m.searchNext()
				m.inputBuffer = ""
				return m, nil
			}
			m.inputBuffer = ""

		case "N":
			if m.globalSearchMode {
				cmd = m.globalPrev()
				return m, cmd
			} else if m.searchQuery != "" {
				m.searchPrev()
				m.inputBuffer = ""
				return m, nil
			}
			m.inputBuffer = ""

		default:
			m.inputBuffer = ""
		}
	}

	if len(m.fileList.Items()) > 0 && m.focus == FocusTree {
		if !keyHandled {
			m.fileList, cmd = m.fileList.Update(msg)
			cmds = append(cmds, cmd)
		}

		if item, ok := m.fileList.SelectedItem().(tree.TreeItem); ok {
			if !item.IsDir && item.FullPath != m.selectedPath {
				m.selectedPath = item.FullPath
				m.diffCursor = 0
				m.visualMode = false
				m.diffViewport.GotoTop()
				if m.pipedDiff != "" {
					cmds = append(cmds, func() tea.Msg {
						return vcs.DiffMsg{Content: m.vcs.ExtractFileDiff(m.pipedDiff, m.selectedPath)}
					})
				} else {
					cmds = append(cmds, m.vcs.DiffCmd(m.targetBranch, m.selectedPath))
				}
			}
		}
	}

	switch msg := msg.(type) {
	case vcs.DiffMsg:
		fullLines := strings.Split(msg.Content, "\n")
		var cleanLines, hlLines []string
		var added, deleted int
		foundHunk := false

		ext := filepath.Ext(m.selectedPath)
		if len(ext) > 0 {
			ext = ext[1:]
		} else {
			ext = "txt"
		}

		isGitTheme := m.treeDelegate.Config.UI.Theme == "git"

		for _, line := range fullLines {
			cleanLine := stripAnsi(line)

			if strings.HasPrefix(cleanLine, "@@") {
				foundHunk = true
			}

			if !foundHunk {
				continue
			}

			cleanLines = append(cleanLines, line)

			isAdd := strings.HasPrefix(cleanLine, "+") && !strings.HasPrefix(cleanLine, "+++")
			isDel := strings.HasPrefix(cleanLine, "-") && !strings.HasPrefix(cleanLine, "---")

			if isAdd {
				added++
			} else if isDel {
				deleted++
			}

			codeContent := cleanLine
			if len(codeContent) > 0 && (isAdd || isDel || strings.HasPrefix(codeContent, " ")) {
				codeContent = codeContent[1:]
			}

			if isGitTheme {
				hlLines = append(hlLines, codeContent)
			} else {
				// Collect for batch highlighting below
				hlLines = append(hlLines, codeContent)
			}
		}

		for len(cleanLines) > 0 {
			lastLine := strings.TrimRight(stripAnsi(cleanLines[len(cleanLines)-1]), "\r")
			if lastLine != "" {
				break
			}
			cleanLines = cleanLines[:len(cleanLines)-1]
			hlLines = hlLines[:len(hlLines)-1]
		}

		// Batch-highlight all lines together so chroma tracks multi-line
		// comment state (/* ... */, docstrings, etc.)
		if !isGitTheme && len(hlLines) > 0 {
			chromaTheme := "nord"
			if !lipgloss.HasDarkBackground() {
				chromaTheme = "github"
			}
			allCode := strings.Join(hlLines, "\n")
			var buf strings.Builder
			err := quick.Highlight(&buf, allCode, ext, "terminal16m", chromaTheme)
			if err == nil && buf.String() != "" {
				highlighted := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
				if len(highlighted) == len(hlLines) {
					hlLines = highlighted
				}
			}
		}

		m.diffLines = cleanLines
		m.diffHighlighted = hlLines
		m.currentFileAdded = added
		m.currentFileDeleted = deleted
		m.diffCursor = m.snapCursor(0, 1)

		// Re-apply search matches after loading new diff
		if m.searchQuery != "" {
			m.findMatches()
			// If global search, jump to the match line in this file
			if m.globalSearchMode && len(m.globalSearchResults) > 0 {
				match := m.globalSearchResults[m.globalSearchIndex]
				if match.FilePath == m.selectedPath && match.Line < len(m.diffLines) {
					m.diffCursor = match.Line
					m.centerDiffCursor()
				} else if len(m.searchMatches) > 0 {
					m.jumpToMatch(0)
				}
			} else if len(m.searchMatches) > 0 {
				m.jumpToMatch(0)
			}
		}

	case vcs.EditorFinishedMsg:
		// Piped mode + DIFI_REFRESH_CMD: re-run the diff producer so edits
		// the user just made show up. Without DIFI_REFRESH_CMD the cached
		// pipedDiff is stale, so we just re-render from cache.
		if m.pipedDiff != "" {
			if refresh := m.refreshPipedDiffCmd(); refresh != nil {
				return m, refresh
			}
			return m, func() tea.Msg {
				return vcs.DiffMsg{Content: m.vcs.ExtractFileDiff(m.pipedDiff, m.selectedPath)}
			}
		}
		return m, m.vcs.DiffCmd(m.targetBranch, m.selectedPath)

	case RefreshedPipedDiffMsg:
		if msg.Err != nil {
			// Refresh failed — fall back to re-rendering the cached diff so
			// the user at least sees something. The error is silent; surface
			// it via a status line in a follow-up if it becomes a problem.
			return m, func() tea.Msg {
				return vcs.DiffMsg{Content: m.vcs.ExtractFileDiff(m.pipedDiff, m.selectedPath)}
			}
		}
		m.pipedDiff = msg.Content

		// Rebuild file tree — files may have appeared or disappeared between
		// runs (rare for in-place edits, common after add/delete).
		files := m.vcs.ParseFilesFromDiff(m.pipedDiff)
		t := tree.New(files)
		items := t.Items(m.flatMode)
		m.treeState = t
		m.fileList.SetItems(items)

		// Preserve selection if the file still exists; otherwise pick the
		// first file in the new tree.
		newSelection := -1
		for idx, item := range items {
			ti, ok := item.(tree.TreeItem)
			if !ok || ti.IsDir {
				continue
			}
			if ti.FullPath == m.selectedPath {
				newSelection = idx
				break
			}
			if newSelection == -1 {
				newSelection = idx
			}
		}
		if newSelection >= 0 {
			m.fileList.Select(newSelection)
			if ti, ok := items[newSelection].(tree.TreeItem); ok {
				m.selectedPath = ti.FullPath
			}
		} else {
			m.selectedPath = ""
		}

		// Reset viewport to top of the (possibly different) file diff.
		m.diffCursor = 0
		m.diffViewport.GotoTop()

		var refreshCmds []tea.Cmd
		if m.selectedPath != "" {
			selected := m.selectedPath
			refreshCmds = append(refreshCmds, func() tea.Msg {
				return vcs.DiffMsg{Content: m.vcs.ExtractFileDiff(m.pipedDiff, selected)}
			})
		}
		refreshCmds = append(refreshCmds, m.computePipedStatsCmd())
		return m, tea.Batch(refreshCmds...)
	}

	return m, tea.Batch(cmds...)
}
