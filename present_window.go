package decker

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// presentationWindow keeps the launched surface's ID so retries cannot open
// another window while the first is still starting or reconnecting.
type presentationWindow struct {
	ctx                     context.Context
	executable, dir, socket string
	args                    []string
	fontSize                float64
	terminal                string
}

func newPresentationWindow(socket string, fontSize float64) (*presentationWindow, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	socket, err = filepath.Abs(socket)
	if err != nil {
		return nil, err
	}
	return &presentationWindow{
		ctx:        context.Background(),
		executable: executable, dir: dir, socket: socket, fontSize: fontSize,
		args: presentationArgs(flag.CommandLine),
	}, nil
}

// presentationArgs preserves the talk's own flags without reopening a
// presenter or an export mode. Position and socket are supplied at launch.
func presentationArgs(fs *flag.FlagSet) []string {
	var args []string
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "presenter", "length", "presentation-font-size", "previews",
			"slide", "step", "socket", "list", "snapshot", "sheet", "video", "png":
			return
		}
		args = append(args, "-"+f.Name+"="+f.Value.String())
	})
	return append(args, fs.Args()...)
}

func (w *presentationWindow) command(slide, step int) string {
	args := []string{w.executable, "-socket=" + w.socket,
		"-slide=" + strconv.Itoa(slide), "-step=" + strconv.Itoa(step)}
	args = append(args, w.args...)
	for i := range args {
		args[i] = shellQuote(args[i])
	}
	return strings.Join(args, " ")
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func (w *presentationWindow) open(slide, step int) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("opening a window requires Ghostty 1.3+ on macOS; run the deck in another terminal")
	}
	ctx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-", w.command(slide, step), w.dir,
		strconv.FormatFloat(w.fontSize, 'f', -1, 64), w.terminal, os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader(presentationWindowScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("Ghostty could not open the presentation: %s", detail)
	}
	w.terminal = strings.TrimSpace(string(out))
	if w.terminal == "" {
		return fmt.Errorf("Ghostty did not return a presentation window ID")
	}
	return nil
}

// Values travel as argv, never as AppleScript source. The deck command is
// shell-quoted separately because Ghostty executes it through a shell. The
// surface API supplies its own exec wrapper and does not parse config prefixes.
const presentationWindowScript = `on run argv
    tell application "Ghostty"
        set previousID to item 4 of argv
        if previousID is not "" then
            try
                set existingTerm to terminal id previousID
                return id of existingTerm
            end try
        end if
        set origin to missing value
        try
            set origin to focused terminal of selected tab of front window
        end try
        set cfg to new surface configuration
        set font size of cfg to (item 3 of argv) as real
        set initial working directory of cfg to item 2 of argv
        set command of cfg to item 1 of argv
        set environment variables of cfg to {"PATH=" & (item 5 of argv)}
        set wait after command of cfg to false
        set win to new window with configuration cfg
        set deckTerm to terminal 1 of selected tab of win
        set deckID to id of deckTerm
        if origin is not missing value then
            try
                focus origin
            end try
        end if
        return deckID
    end tell
end run
`
