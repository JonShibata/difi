package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/xguot/difi/internal/config"
)

var (
	// Adaptive colors: first value = light bg, second = dark bg
	borderDim    = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#4C566A"}
	borderFocus  = lipgloss.AdaptiveColor{Light: "#2266AA", Dark: "#81A1C1"}
	barBg        = lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#2E3440"}
	barFg        = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#D8DEE9"}
	addColor     = lipgloss.AdaptiveColor{Light: "#22863A", Dark: "#A3BE8C"}
	delColor     = lipgloss.AdaptiveColor{Light: "#CB2431", Dark: "#BF616A"}
	accentColor  = lipgloss.AdaptiveColor{Light: "#2266AA", Dark: "#81A1C1"}
	dimText      = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}
	fileText     = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"}

	PaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderDim).
			Padding(0, 1)

	FocusedPaneStyle = PaneStyle.Copy().
				BorderForeground(borderFocus)

	TopBarStyle = lipgloss.NewStyle().
			Background(barBg).
			Foreground(barFg).
			Height(1)

	TopInfoStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	TopStatsAddedStyle = lipgloss.NewStyle().
				Foreground(addColor).
				PaddingLeft(1)

	TopStatsDeletedStyle = lipgloss.NewStyle().
				Foreground(delColor).
				PaddingLeft(1).
				PaddingRight(1)

	DirectoryStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6F42C1", Dark: "#9999FF"})
	FileStyle      = lipgloss.NewStyle().Foreground(fileText)

	DiffStyle       = lipgloss.NewStyle().Padding(0, 0)
	LineNumberStyle = lipgloss.NewStyle().Foreground(dimText).Width(4).Align(lipgloss.Right).MarginRight(1)

	DiffAddGutter = lipgloss.NewStyle().Foreground(addColor).Bold(true)
	DiffDelGutter = lipgloss.NewStyle().Foreground(delColor).Bold(true)
	DiffCtxGutter = lipgloss.NewStyle().Foreground(dimText)

	HunkHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#ECEFF4"}).
			Background(lipgloss.AdaptiveColor{Light: "#2266AA", Dark: "#5E81AC"}).
			Bold(true)

	DiffAddLineStyle lipgloss.Style
	DiffDelLineStyle lipgloss.Style

	// Dynamic full-line cursor styles
	CursorNormalStyle lipgloss.Style
	CursorAddStyle    lipgloss.Style
	CursorDelStyle    lipgloss.Style

	EmptyLogoStyle   = lipgloss.NewStyle().Foreground(accentColor).Bold(true).MarginBottom(1)
	EmptyDescStyle   = lipgloss.NewStyle().Foreground(dimText).MarginBottom(1)
	EmptyStatusStyle = lipgloss.NewStyle().Foreground(dimText).MarginBottom(2)
	EmptyHeaderStyle = lipgloss.NewStyle().Foreground(dimText).Bold(true).MarginBottom(1)
	EmptyCodeStyle   = lipgloss.NewStyle().Foreground(dimText)

	HelpDrawerStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, false, false).BorderForeground(borderDim).Padding(1, 2)
	HelpTextStyle   = lipgloss.NewStyle().Foreground(dimText).MarginRight(2)

	StatusBarStyle     = lipgloss.NewStyle().Background(barBg).Foreground(barFg).Height(1)
	StatusKeyStyle     = lipgloss.NewStyle().Foreground(dimText).Padding(0, 1)
	StatusRepoStyle    = lipgloss.NewStyle().Bold(true).Foreground(accentColor).Padding(0, 1)
	StatusBranchStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6F42C1", Dark: "#bb9af7"}).Padding(0, 1)
	StatusAddedStyle   = lipgloss.NewStyle().Foreground(addColor).Padding(0, 1)
	StatusDeletedStyle = lipgloss.NewStyle().Foreground(delColor).Padding(0, 1)
	StatusDividerStyle = lipgloss.NewStyle().Foreground(borderDim).Padding(0, 1)

	ColorText = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"}
)

func InitStyles(cfg config.Config) {
	addBg := cfg.UI.DiffAddBg
	if addBg == "" {
		if lipgloss.HasDarkBackground() {
			addBg = "#1A251E"
		} else {
			addBg = "#DAFBE1"
		}
	}

	delBg := cfg.UI.DiffDelBg
	if delBg == "" {
		if lipgloss.HasDarkBackground() {
			delBg = "#2D1A1A"
		} else {
			delBg = "#FFEEF0"
		}
	}

	DiffAddLineStyle = lipgloss.NewStyle().Background(lipgloss.Color(addBg))
	DiffDelLineStyle = lipgloss.NewStyle().Background(lipgloss.Color(delBg))

	if lipgloss.HasDarkBackground() {
		CursorNormalStyle = lipgloss.NewStyle().Background(lipgloss.Color("#434C5E")).Foreground(lipgloss.Color("#ECEFF4"))
		CursorAddStyle = lipgloss.NewStyle().Background(lipgloss.Color("#A3E4D7")).Foreground(lipgloss.Color("#1A251E"))
		CursorDelStyle = lipgloss.NewStyle().Background(lipgloss.Color("#F5B7B1")).Foreground(lipgloss.Color("#2D1A1A"))
	} else {
		CursorNormalStyle = lipgloss.NewStyle().Background(lipgloss.Color("#D0D0D0")).Foreground(lipgloss.Color("#1A1A1A"))
		CursorAddStyle = lipgloss.NewStyle().Background(lipgloss.Color("#ACE2B8")).Foreground(lipgloss.Color("#1A251E"))
		CursorDelStyle = lipgloss.NewStyle().Background(lipgloss.Color("#F5B7B1")).Foreground(lipgloss.Color("#2D1A1A"))
	}
}
