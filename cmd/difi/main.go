package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xguot/difi/internal/config"
	"github.com/xguot/difi/internal/ui"
	"github.com/xguot/difi/internal/vcs"
)

var version = "0.2.5"

func main() {
	showVersion := flag.Bool("version", false, "Show version")
	flag.BoolVar(showVersion, "v", false, "Show version (shorthand)")

	plain := flag.Bool("plain", false, "Print a plain summary")
	flag.BoolVar(plain, "p", false, "Print a plain summary (shorthand)")

	flat := flag.Bool("flat", false, "Use one-line file navigation")
	flag.BoolVar(flat, "f", false, "Use one-line file navigation (shorthand)")

	forceVCS := flag.String("vcs", "", "Force specific VCS (git or hg)")

	// -1 means "unset" — fall back to the config value (default 3).
	contextLines := flag.Int("context", -1, "Lines of context shown around changes")
	flag.IntVar(contextLines, "U", -1, "Lines of context (shorthand)")

	diffCmd := flag.String("cmd", "", "Command that prints a git-format diff; difi runs and re-runs it instead of reading stdin (supports a {context} placeholder)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s [options] [target]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  Show version\n")
		fmt.Fprintf(os.Stderr, "  -p, --plain    Print a plain summary\n")
		fmt.Fprintf(os.Stderr, "  -f, --flat     Use one-line file navigation\n")
		fmt.Fprintf(os.Stderr, "  -U, --context  Lines of context shown around changes (default 3)\n")
		fmt.Fprintf(os.Stderr, "  --cmd string   Command that prints a git-format diff (supports {context}); re-run on +/-/r\n")
		fmt.Fprintf(os.Stderr, "  --vcs string   Force specific VCS (git or hg)\n")
		fmt.Fprintf(os.Stderr, "\nTarget:\n")
		fmt.Fprintf(os.Stderr, "  branch, commit, or tag to compare against (default: HEAD or tip)\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("difi version %s\n", version)
		os.Exit(0)
	}

	cfg := config.Load()
	if *contextLines >= 0 {
		cfg.UI.ContextLines = *contextLines
	}

	// difi's diff source, in priority order:
	//   1. --cmd: difi runs (and re-runs) the command itself — the robust path
	//      for VCSs with no native backend (e.g. jj). A {context} placeholder is
	//      substituted with the current context-line count, so '+'/'-' and 'r'
	//      re-run with the right context.
	//   2. a stdin pipe: a static diff blob (optionally re-runnable via the
	//      legacy DIFI_REFRESH_CMD env var).
	//   3. neither: native git/hg diffing.
	var pipedDiff string
	var stdinPiped bool
	refreshCmd := os.Getenv("DIFI_REFRESH_CMD")
	if *diffCmd != "" {
		refreshCmd = *diffCmd
		initial := strings.ReplaceAll(*diffCmd, "{context}", strconv.Itoa(cfg.UI.ContextLines))
		out, err := exec.Command("sh", "-c", initial).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running --cmd: %v\n", err)
			os.Exit(1)
		}
		pipedDiff = string(out)
	} else if stat, _ := os.Stdin.Stat(); (stat.Mode() & os.ModeCharDevice) == 0 {
		stdinPiped = true
		b, _ := io.ReadAll(os.Stdin)
		pipedDiff = string(b)
	}

	// Detect or force VCS type
	var vcsClient vcs.VCS
	if *forceVCS != "" {
		switch *forceVCS {
		case "git":
			vcsClient = vcs.GitVCS{}
		case "hg":
			vcsClient = vcs.HgVCS{}
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported VCS '%s'. Supported values: git, hg\n", *forceVCS)
			os.Exit(1)
		}
	} else {
		vcsClient = vcs.DetectVCS()
	}

	target := "HEAD"
	if flag.NArg() > 0 {
		target = flag.Arg(0)
	}

	// For Mercurial, use "tip" as default instead of "HEAD"
	if _, isHg := vcsClient.(vcs.HgVCS); isHg && target == "HEAD" {
		target = "tip"
	}

	if *plain && pipedDiff == "" {
		// Use VCS-specific commands for plain output
		files, err := vcsClient.ListChangedFiles(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing changed files: %v\n", err)
			os.Exit(1)
		}
		for _, file := range files {
			fmt.Println(file)
		}
		os.Exit(0)
	}

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if stdinPiped {
		if tty, err := os.Open("/dev/tty"); err == nil {
			opts = append(opts, tea.WithInput(tty))
		}
	}

	p := tea.NewProgram(ui.NewModel(cfg, target, pipedDiff, refreshCmd, vcsClient, *flat), opts...)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
