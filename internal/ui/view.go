package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/xguot/difi/internal/tree"
	"github.com/xguot/difi/internal/vcs"
)

func (m Model) isSearchMatch(lineIdx int) bool {
	for _, idx := range m.searchMatches {
		if idx == lineIdx {
			return true
		}
	}
	return false
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	topBar := m.renderTopBar()

	var bottomBar string
	if m.showHelp {
		bottomBar = m.renderHelpDrawer()
	} else {
		bottomBar = m.viewStatusBar()
	}

	contentHeight := m.height - lipgloss.Height(topBar) - lipgloss.Height(bottomBar)
	if contentHeight < 0 {
		contentHeight = 0
	}

	var mainContent string
	if len(m.fileList.Items()) == 0 {
		mainContent = m.renderEmptyState(m.width, contentHeight, "No changes found against "+m.targetBranch)
	} else {
		treeStyle := PaneStyle
		if m.focus == FocusTree {
			treeStyle = FocusedPaneStyle
		}

		treeView := treeStyle.Copy().
			Width(m.fileList.Width()).
			Height(contentHeight).
			MaxHeight(contentHeight).
			Render(m.fileList.View())

		var rightPaneView string
		selectedItem, ok := m.fileList.SelectedItem().(tree.TreeItem)

		if ok && selectedItem.IsDir {
			rightPaneView = m.renderEmptyState(m.diffViewport.Width, contentHeight, "Directory: "+selectedItem.Name)
		} else {
			var renderedDiff strings.Builder

			viewportHeight := contentHeight
			start := m.diffViewport.YOffset

			maxLineWidth := m.diffViewport.Width - 7
			if maxLineWidth < 1 {
				maxLineWidth = 1
			}

			isGitTheme := m.treeDelegate.Config.UI.Theme == "git"

			// chunkWidth is the visible width available for code (after the
			// 4-char gutter). When wrap is enabled, ansi.Hardwrap splits the
			// highlighted line into at-most-chunkWidth-wide segments, each
			// rendered as its own viewport row with a continuation gutter.
			chunkWidth := maxLineWidth - 4
			if chunkWidth < 1 {
				chunkWidth = 1
			}

			// visRows counts viewport rows actually emitted. When wrap is on
			// a single source line can produce multiple viewport rows, so we
			// stop iterating source lines as soon as visRows fills the viewport.
			visRows := 0

			for i := start; i < len(m.diffLines) && visRows < viewportHeight; i++ {
				rawLine := m.diffLines[i]
				cleanLine := stripAnsi(rawLine)

				if isDiffMetadata(cleanLine) {
					if strings.HasPrefix(cleanLine, "@@") {
						// Render hunk header as a visible blue separator
						hunkText := "  " + cleanLine + strings.Repeat(" ", maxLineWidth)
						hunkText = ansi.Truncate(hunkText, maxLineWidth, "")
						line := HunkHeaderStyle.Copy().Width(maxLineWidth).Render(hunkText)
						renderedDiff.WriteString(line + "\n")
						visRows++
					}
					// Non-hunk metadata (--- / +++ / index lines) is skipped
					// silently — it doesn't consume a viewport row, so we don't
					// extend the loop bound.
					continue
				}

				isAdd := strings.HasPrefix(cleanLine, "+")
				isDel := strings.HasPrefix(cleanLine, "-")

				codeContent := cleanLine
				if len(codeContent) > 0 && (isAdd || isDel || strings.HasPrefix(codeContent, " ")) {
					codeContent = codeContent[1:]
				}

				isCursor := false
				if m.focus == FocusDiff {
					if m.visualMode {
						minIdx, maxIdx := m.visualStart, m.diffCursor
						if minIdx > maxIdx {
							minIdx, maxIdx = maxIdx, minIdx
						}
						isCursor = (i >= minIdx && i <= maxIdx)
					} else {
						isCursor = (i == m.diffCursor)
					}
				}

				isMatch := m.searchQuery != "" && m.isSearchMatch(i)

				separator := "│"
				if isCursor {
					separator = "┃"
				} else if isMatch {
					separator = "▶"
				}

				var primaryGutter string
				if isAdd {
					primaryGutter = "+ " + separator + " "
				} else if isDel {
					primaryGutter = "- " + separator + " "
				} else {
					primaryGutter = "  " + separator + " "
				}
				// Continuation gutter shown on wrap chunks 2+; the ↪ marker
				// makes it visually obvious that this row is a continuation
				// of the line above rather than a fresh diff line.
				continuationGutter := "  ↪ "

				var numStr string
				mode := "relative"

				if mode != "hidden" {
					if isCursor && mode == "hybrid" {
						realLine := m.vcs.CalculateFileLine(m.diffLines, m.diffCursor)
						numStr = fmt.Sprintf("%d", realLine)
					} else if isCursor && mode == "relative" {
						numStr = "0"
					} else if mode == "absolute" {
						numStr = fmt.Sprintf("%d", i+1)
					} else {
						dist := int(math.Abs(float64(i - m.diffCursor)))
						numStr = fmt.Sprintf("%d", dist)
					}
				}

				primaryLineNum := ""
				if numStr != "" {
					primaryLineNum = LineNumberStyle.Render(numStr)
				}
				// Continuation rows get a blank line-number cell of the same
				// width so the diff content stays vertically aligned.
				continuationLineNum := LineNumberStyle.Render("")

				// Build the full (un-truncated) highlighted code for this
				// source line, then split into chunks based on m.wrap. When
				// wrap is off, chunks has exactly one entry (the truncated
				// line) — preserving pre-wrap behavior.
				var fullHlCode string
				if isCursor {
					if isGitTheme {
						switch {
						case isAdd:
							fullHlCode = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(codeContent)
						case isDel:
							fullHlCode = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(codeContent)
						default:
							fullHlCode = codeContent
						}
					} else if i < len(m.diffHighlighted) {
						fullHlCode = m.diffHighlighted[i]
						fullHlCode = bgAnsiRe.ReplaceAllString(fullHlCode, "")
					} else {
						fullHlCode = codeContent
					}
				} else {
					if isGitTheme {
						if isAdd {
							fullHlCode = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(codeContent)
						} else if isDel {
							fullHlCode = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(codeContent)
						} else {
							fullHlCode = codeContent
						}
					} else if i < len(m.diffHighlighted) {
						fullHlCode = m.diffHighlighted[i]
						fullHlCode = bgAnsiRe.ReplaceAllString(fullHlCode, "")
					} else {
						fullHlCode = codeContent
					}
				}

				var hlChunks, plainChunks []string
				if m.wrap {
					// ansi.Hardwrap preserves ANSI styles across chunk
					// boundaries so syntax highlighting survives the wrap.
					hlChunks = strings.Split(ansi.Hardwrap(fullHlCode, chunkWidth, true), "\n")
					plainChunks = strings.Split(ansi.Hardwrap(codeContent, chunkWidth, true), "\n")
				} else {
					hlChunks = []string{ansi.Truncate(fullHlCode, chunkWidth, "")}
					plainChunks = []string{ansi.Truncate(codeContent, chunkWidth, "")}
				}
				// Defensive: keep hlChunks and plainChunks the same length so
				// the per-chunk loop below can index both safely.
				for len(plainChunks) < len(hlChunks) {
					plainChunks = append(plainChunks, "")
				}

				for chunkIdx, hlChunk := range hlChunks {
					if visRows >= viewportHeight {
						break
					}

					var gutterStr, lineNumRendered string
					if chunkIdx == 0 {
						gutterStr = primaryGutter
						lineNumRendered = primaryLineNum
					} else {
						gutterStr = continuationGutter
						lineNumRendered = continuationLineNum
					}
					plainChunk := plainChunks[chunkIdx]

					var line string
					if isCursor {
						var cursorBg string
						if isAdd {
							cursorBg = CursorAddBgAnsi
						} else if isDel {
							cursorBg = CursorDelBgAnsi
						} else {
							cursorBg = CursorNormalBgAnsi
						}

						code := resetAnsiRe.ReplaceAllString(hlChunk, "\x1b[0m"+cursorBg)

						fullLine := cursorBg + gutterStr + code
						visibleLen := lipgloss.Width(fullLine)
						padLen := maxLineWidth - visibleLen
						if padLen > 0 {
							fullLine += cursorBg + strings.Repeat(" ", padLen)
						}
						fullLine += "\x1b[0m"

						if isMatch {
							plain := gutterStr + plainChunk
							if padLen > 0 {
								plain += strings.Repeat(" ", padLen)
							}
							fullLine = highlightMatchesInRendered(fullLine, plain, m.searchQuery, cursorBg)
						}
						line = fullLine
					} else {
						var gutter string
						if isGitTheme {
							if isAdd {
								gutter = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(gutterStr)
							} else if isDel {
								gutter = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(gutterStr)
							} else {
								gutter = DiffCtxGutter.Render(gutterStr)
							}
						} else {
							if isAdd {
								gutter = DiffAddGutter.Render(gutterStr)
							} else if isDel {
								gutter = DiffDelGutter.Render(gutterStr)
							} else {
								gutter = DiffCtxGutter.Render(gutterStr)
							}
						}

						code := hlChunk
						var bgAnsi string
						if isAdd || isDel {
							if isAdd {
								r, g, b, _ := DiffAddLineStyle.GetBackground().RGBA()
								bgAnsi = fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
							} else {
								r, g, b, _ := DiffDelLineStyle.GetBackground().RGBA()
								bgAnsi = fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
							}
							code = resetAnsiRe.ReplaceAllString(code, "\x1b[0m"+bgAnsi)
							code = bgAnsi + code + "\x1b[0m"
						}

						if isMatch {
							code = highlightMatchesInRendered(code, plainChunk, m.searchQuery, bgAnsi)
						}

						if isAdd || isDel {
							fullLine := gutter + code
							visibleLen := lipgloss.Width(fullLine)
							padLen := maxLineWidth - visibleLen
							if padLen > 0 {
								fullLine += bgAnsi + strings.Repeat(" ", padLen) + "\x1b[0m"
							}
							line = fullLine
						} else {
							line = gutter + code
						}
					}

					renderedDiff.WriteString(lineNumRendered + line + "\n")
					visRows++
				}
			}

			diffContentStr := "\n" + strings.TrimRight(renderedDiff.String(), "\n")

			rightPaneView = DiffStyle.Copy().
				Width(m.diffViewport.Width).
				Height(contentHeight).
				MaxHeight(contentHeight).
				Render(diffContentStr)
		}

		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, treeView, rightPaneView)
	}

	return lipgloss.JoinVertical(lipgloss.Top, topBar, mainContent, bottomBar)
}

func (m Model) renderTopBar() string {
	vcsType := "git"
	if _, isHg := m.vcs.(vcs.HgVCS); isHg {
		vcsType = "hg"
	}

	repoStats := ""
	if m.statsAdded > 0 || m.statsDeleted > 0 {
		repoStats = fmt.Sprintf(" +%d -%d", m.statsAdded, m.statsDeleted)
	}

	info := fmt.Sprintf(" %s:%s  %s ➜ %s%s", m.repoName, vcsType, m.currentBranch, m.targetBranch, repoStats)
	leftSide := TopInfoStyle.Render(info)

	rightSide := ""
	if selectedItem, ok := m.fileList.SelectedItem().(tree.TreeItem); ok {
		var displayPath string
		var statsAdded, statsDeleted int

		if selectedItem.IsDir {
			displayPath = selectedItem.FullPath + "/"
			prefix := selectedItem.FullPath + "/"
			for filePath, stats := range m.fileStats {
				if strings.HasPrefix(filePath, prefix) {
					statsAdded += stats[0]
					statsDeleted += stats[1]
				}
			}
		} else {
			displayPath = selectedItem.FullPath
			if fs, ok := m.fileStats[selectedItem.FullPath]; ok {
				statsAdded = fs[0]
				statsDeleted = fs[1]
			} else {
				statsAdded = m.currentFileAdded
				statsDeleted = m.currentFileDeleted
			}
		}

		fileStats := ""
		if statsAdded > 0 || statsDeleted > 0 {
			added := TopStatsAddedStyle.Render(fmt.Sprintf("+%d", statsAdded))
			deleted := TopStatsDeletedStyle.Render(fmt.Sprintf("-%d", statsDeleted))
			fileStats = lipgloss.JoinHorizontal(lipgloss.Center, added, deleted)
		}

		fileStatsWidth := lipgloss.Width(fileStats)
		maxPathWidth := m.width - lipgloss.Width(leftSide) - fileStatsWidth - 4
		if maxPathWidth < 10 {
			maxPathWidth = 10
		}

		truncPath := ansi.Truncate(displayPath, maxPathWidth, "…")
		if fileStats != "" {
			rightSide = truncPath + " " + fileStats
		} else {
			rightSide = truncPath
		}
	}

	availWidth := m.width - lipgloss.Width(leftSide) - lipgloss.Width(rightSide)
	if availWidth < 0 {
		availWidth = 0
	}

	padding := strings.Repeat(" ", availWidth)
	finalBar := lipgloss.JoinHorizontal(lipgloss.Top, leftSide, padding, rightSide)

	return TopBarStyle.Width(m.width).Render(finalBar)
}

func (m Model) viewStatusBar() string {
	if m.searchMode {
		input := m.searchInput.View()
		availWidth := m.width - lipgloss.Width(input)
		if availWidth < 0 {
			availWidth = 0
		}
		paddingStyle := lipgloss.NewStyle().Background(barBg)
		padding := paddingStyle.Render(strings.Repeat(" ", availWidth))
		return lipgloss.JoinHorizontal(lipgloss.Top, input, padding)
	}

	shortcutsStyle := StatusKeyStyle.Copy().Background(barBg)
	shortcuts := shortcutsStyle.Render("? Help  / Search  q Quit  Tab Switch  V Visual  f Flat")

	rightInfo := ""
	if m.copyStatus != "" {
		// Truncate so a long path can't push the shortcuts off-screen.
		max := m.width - lipgloss.Width(shortcuts) - 2
		if max < 8 {
			max = 8
		}
		rightInfo = " " + ansi.Truncate(m.copyStatus, max, "…") + " "
	} else if m.searchQuery != "" {
		matchCount := len(m.searchMatches)
		if m.globalSearchMode {
			rightInfo = fmt.Sprintf(" [%d/%d global] ", m.globalSearchIndex+1, len(m.globalSearchResults))
		} else if matchCount > 0 {
			rightInfo = fmt.Sprintf(" [%d/%d] ", m.searchIndex+1, matchCount)
		} else {
			rightInfo = " [no match] "
		}
	}

	rightRendered := shortcutsStyle.Render(rightInfo)
	availWidth := m.width - lipgloss.Width(shortcuts) - lipgloss.Width(rightRendered)
	if availWidth < 0 {
		availWidth = 0
	}

	paddingStyle := lipgloss.NewStyle().Background(barBg)
	padding := paddingStyle.Render(strings.Repeat(" ", availWidth))

	return lipgloss.JoinHorizontal(lipgloss.Top, shortcuts, padding, rightRendered)
}

func (m Model) renderHelpDrawer() string {
	col1 := lipgloss.JoinVertical(lipgloss.Left,
		HelpTextStyle.Render("↑/k   Move Up"),
		HelpTextStyle.Render("↓/j   Move Down"),
	)
	col2 := lipgloss.JoinVertical(lipgloss.Left,
		HelpTextStyle.Render("←/h   Left Panel"),
		HelpTextStyle.Render("→/l   Right Panel"),
	)
	col3 := lipgloss.JoinVertical(lipgloss.Left,
		HelpTextStyle.Render("C-d/u Page Dn/Up"),
		HelpTextStyle.Render("zz/zt Scroll View"),
	)
	col4 := lipgloss.JoinVertical(lipgloss.Left,
		HelpTextStyle.Render("H/M/L Move Cursor"),
		HelpTextStyle.Render("e     Edit File"),
		HelpTextStyle.Render("c     Copy Path"),
		HelpTextStyle.Render("y     Yank Lines"),
	)
	col5 := lipgloss.JoinVertical(lipgloss.Left,
		HelpTextStyle.Render("V     Visual Mode"),
		HelpTextStyle.Render("f     Flat Mode"),
		HelpTextStyle.Render("esc   Cancel"),
	)
	col6 := lipgloss.JoinVertical(lipgloss.Left,
		HelpTextStyle.Render("/     Search"),
		HelpTextStyle.Render("n/N   Next/Prev"),
	)
	return HelpDrawerStyle.Copy().
		Width(m.width).
		Render(lipgloss.JoinHorizontal(lipgloss.Top,
			col1, lipgloss.NewStyle().Width(4).Render(""),
			col2, lipgloss.NewStyle().Width(4).Render(""),
			col3, lipgloss.NewStyle().Width(4).Render(""),
			col4, lipgloss.NewStyle().Width(4).Render(""),
			col5, lipgloss.NewStyle().Width(4).Render(""),
			col6,
		))
}

func (m Model) renderEmptyState(w, h int, statusMsg string) string {
	logo := EmptyLogoStyle.Render("difi")
	desc := EmptyDescStyle.Render("A calm, focused way to review Git & Mercurial diffs.")
	status := EmptyStatusStyle.Render(statusMsg)

	usageHeader := EmptyHeaderStyle.Render("Usage Patterns")
	cmd1 := lipgloss.NewStyle().Foreground(ColorText).Render("difi")
	desc1 := EmptyCodeStyle.Render("Auto-detect VCS, diff against main/tip")
	cmd2 := lipgloss.NewStyle().Foreground(ColorText).Render("difi --vcs git")
	desc2 := EmptyCodeStyle.Render("Force Git mode")
	cmd3 := lipgloss.NewStyle().Foreground(ColorText).Render("difi --vcs hg")
	desc3 := EmptyCodeStyle.Render("Force Mercurial mode")

	usageBlock := lipgloss.JoinVertical(lipgloss.Left,
		usageHeader,
		lipgloss.JoinHorizontal(lipgloss.Left, cmd1, "    ", desc1),
		lipgloss.JoinHorizontal(lipgloss.Left, cmd2, "    ", desc2),
		lipgloss.JoinHorizontal(lipgloss.Left, cmd3, "    ", desc3),
	)

	navHeader := EmptyHeaderStyle.Render("Navigation")
	key1 := lipgloss.NewStyle().Foreground(ColorText).Render("Tab")
	key2 := lipgloss.NewStyle().Foreground(ColorText).Render("j/k")
	keyDesc1 := EmptyCodeStyle.Render("Switch panels")
	keyDesc2 := EmptyCodeStyle.Render("Move cursor")

	navBlock := lipgloss.JoinVertical(lipgloss.Left,
		navHeader,
		lipgloss.JoinHorizontal(lipgloss.Left, key1, "    ", keyDesc1),
		lipgloss.JoinHorizontal(lipgloss.Left, key2, "    ", keyDesc2),
	)

	nvimHeader := EmptyHeaderStyle.Render("Neovim Integration")
	nvim1 := lipgloss.NewStyle().Foreground(ColorText).Render("xguot/difi.nvim")
	nvimDesc1 := EmptyCodeStyle.Render("Install plugin")
	nvim2 := lipgloss.NewStyle().Foreground(ColorText).Render("Press 'e'")
	nvimDesc2 := EmptyCodeStyle.Render("Edit with context")

	nvimBlock := lipgloss.JoinVertical(lipgloss.Left,
		nvimHeader,
		lipgloss.JoinHorizontal(lipgloss.Left, nvim1, "  ", nvimDesc1),
		lipgloss.JoinHorizontal(lipgloss.Left, nvim2, "          ", nvimDesc2),
	)

	var guides string
	if w > 80 {
		guides = lipgloss.JoinHorizontal(lipgloss.Top,
			usageBlock, lipgloss.NewStyle().Width(6).Render(""),
			navBlock, lipgloss.NewStyle().Width(6).Render(""),
			nvimBlock,
		)
	} else {
		topRow := lipgloss.JoinHorizontal(lipgloss.Top, usageBlock, lipgloss.NewStyle().Width(4).Render(""), navBlock)
		guides = lipgloss.JoinVertical(lipgloss.Left,
			topRow,
			lipgloss.NewStyle().Height(1).Render(""),
			nvimBlock,
		)
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		logo,
		desc,
		status,
		lipgloss.NewStyle().Height(1).Render(""),
		guides,
	)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}
