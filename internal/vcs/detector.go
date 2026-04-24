package vcs

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xguot/difi/internal/git"
	"github.com/xguot/difi/internal/hg"
)

type GitVCS struct{}
type HgVCS struct{}

func (g GitVCS) GetCurrentBranch() string { return git.GetCurrentBranch() }
func (g GitVCS) GetRepoName() string      { return git.GetRepoName() }
func (g GitVCS) ListChangedFiles(targetBranch string) ([]string, error) {
	return git.ListChangedFiles(targetBranch)
}
func (g GitVCS) DiffCmd(targetBranch, path string) tea.Cmd {
	gitCmd := git.DiffCmd(targetBranch, path)
	return func() tea.Msg {
		msg := gitCmd()
		if gitMsg, ok := msg.(git.DiffMsg); ok {
			return DiffMsg{Content: gitMsg.Content}
		}
		return msg
	}
}
func (g GitVCS) DiffSync(targetBranch, path string) string {
	return git.DiffSync(targetBranch, path)
}
func (g GitVCS) OpenEditorCmd(path string, lineNumber int, targetBranch string, editor string) tea.Cmd {
	// tea.ExecProcess wraps the *exec.Cmd in an internal execMsg whose
	// callback fires only after the editor actually exits. The outer
	// `func() tea.Msg` wrapper pattern (used elsewhere in this file) can't
	// rewrap that callback's result, so the editor's finish message would
	// arrive as git.EditorFinishedMsg and Update would silently drop it.
	// Build the *exec.Cmd here and call tea.ExecProcess ourselves so the
	// callback returns vcs.EditorFinishedMsg directly.
	c := git.BuildEditorCmd(path, lineNumber, targetBranch, editor)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}
func (g GitVCS) DiffStats(targetBranch string) (added int, deleted int, err error) {
	return git.DiffStats(targetBranch)
}
func (g GitVCS) DiffStatsByFile(targetBranch string) (map[string][2]int, error) {
	return git.DiffStatsByFile(targetBranch)
}
func (g GitVCS) CalculateFileLine(diffLines []string, visualLineIndex int) int {
	return git.CalculateFileLine(diffLines, visualLineIndex)
}
func (g GitVCS) ParseFilesFromDiff(diffText string) []string { return git.ParseFilesFromDiff(diffText) }
func (g GitVCS) ExtractFileDiff(diffText, targetPath string) string {
	return git.ExtractFileDiff(diffText, targetPath)
}

func (h HgVCS) GetCurrentBranch() string { return hg.GetCurrentBranch() }
func (h HgVCS) GetRepoName() string      { return hg.GetRepoName() }
func (h HgVCS) ListChangedFiles(targetBranch string) ([]string, error) {
	return hg.ListChangedFiles(targetBranch)
}
func (h HgVCS) DiffCmd(targetBranch, path string) tea.Cmd {
	hgCmd := hg.DiffCmd(targetBranch, path)
	return func() tea.Msg {
		msg := hgCmd()
		if hgMsg, ok := msg.(hg.DiffMsg); ok {
			return DiffMsg{Content: hgMsg.Content}
		}
		return msg
	}
}
func (h HgVCS) DiffSync(targetBranch, path string) string {
	return hg.DiffSync(targetBranch, path)
}
func (h HgVCS) OpenEditorCmd(path string, lineNumber int, targetBranch string, editor string) tea.Cmd {
	// See GitVCS.OpenEditorCmd for the rationale.
	c := hg.BuildEditorCmd(path, lineNumber, targetBranch, editor)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}
func (h HgVCS) DiffStats(targetBranch string) (added int, deleted int, err error) {
	return hg.DiffStats(targetBranch)
}
func (h HgVCS) DiffStatsByFile(targetBranch string) (map[string][2]int, error) {
	return hg.DiffStatsByFile(targetBranch)
}
func (h HgVCS) CalculateFileLine(diffLines []string, visualLineIndex int) int {
	return hg.CalculateFileLine(diffLines, visualLineIndex)
}
func (h HgVCS) ParseFilesFromDiff(diffText string) []string { return hg.ParseFilesFromDiff(diffText) }
func (h HgVCS) ExtractFileDiff(diffText, targetPath string) string {
	return hg.ExtractFileDiff(diffText, targetPath)
}

func DetectVCS() VCS {
	dir, err := os.Getwd()
	if err != nil {
		return GitVCS{}
	}

	checkDir := dir
	for {
		if _, err := os.Stat(filepath.Join(checkDir, ".git")); err == nil {
			return GitVCS{}
		}
		parent := filepath.Dir(checkDir)
		if parent == checkDir {
			break
		}
		checkDir = parent
	}

	checkDir = dir
	for {
		if _, err := os.Stat(filepath.Join(checkDir, ".hg")); err == nil {
			return HgVCS{}
		}
		parent := filepath.Dir(checkDir)
		if parent == checkDir {
			break
		}
		checkDir = parent
	}

	return GitVCS{}
}
