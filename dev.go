package decker

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// devState implements -dev: it rebuilds the deck's main package on save of any
// watched Go source (every same-module package it imports, so talk and engine
// edits both reload). A good build replaces the process on the same slide; a
// failed one shows the compiler output over the deck while the old version
// runs.
type devState struct {
	events     chan struct{}
	pkg        string // the package to build
	bin        string // where rebuilt binaries go
	pending    bool   // a change arrived and hasn't been built yet
	lastChange time.Time
	building   bool
	buildErr   string
}

type fileChangedMsg struct{}

type buildDoneMsg struct {
	out string
	err error
}

// debounce waits for saves to settle (editors often write several times).
const debounce = 250 * time.Millisecond

// startDev starts watching the deck's module; rebuilt binaries go to bin.
func startDev(bin string) (*devState, error) {
	pkg, dirs := devTarget()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if err := w.Add(dir); err != nil {
			return nil, err
		}
	}
	d := &devState{events: make(chan struct{}, 1), pkg: pkg, bin: bin}
	go func() {
		for ev := range w.Events {
			name := filepath.Base(ev.Name)
			if ev.Op == fsnotify.Chmod || strings.HasPrefix(name, ".") ||
				!(strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum") {
				continue
			}
			select {
			case d.events <- struct{}{}:
			default: // a change is already queued
			}
		}
	}()
	return d, nil
}

// wait returns a command that delivers one fileChangedMsg; re-issue it after
// each.
func (d *devState) wait() tea.Cmd {
	return func() tea.Msg {
		<-d.events
		return fileChangedMsg{}
	}
}

func (d *devState) changed() tea.Cmd {
	d.pending, d.lastChange = true, time.Now()
	return d.wait()
}

func (d *devState) tick(now time.Time) tea.Cmd {
	if !d.pending || d.building || now.Sub(d.lastChange) <= debounce {
		return nil
	}
	d.pending, d.building = false, true
	return func() tea.Msg {
		out, err := exec.Command("go", "build", "-o", d.bin, d.pkg).CombinedOutput()
		return buildDoneMsg{out: string(out), err: err}
	}
}

// built records a finished build and reports whether it succeeded.
func (d *devState) built(msg buildDoneMsg) bool {
	d.building = false
	d.buildErr = ""
	if msg.err != nil {
		if d.buildErr = strings.TrimSpace(msg.out); d.buildErr == "" {
			d.buildErr = msg.err.Error()
		}
	}
	return msg.err == nil
}

// restartArgs is the command line that reopens the rebuilt deck at slide and
// step (1-based).
func (d *devState) restartArgs(slide, step, fps int) []string {
	return []string{d.bin, "-dev", "-slide", strconv.Itoa(slide), "-step", strconv.Itoa(step), "-fps", strconv.Itoa(fps)}
}

func execRestart(args []string, socket string) error {
	args = append(args, "-socket", socket)
	return syscall.Exec(args[0], args, os.Environ())
}

// devTarget returns the deck's main package and the directories of every
// same-module package it depends on; without build info it falls back to ".".
func devTarget() (pkg string, dirs []string) {
	pkg = "."
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Path != "" && bi.Path != "command-line-arguments" {
		pkg = bi.Path
	}
	out, err := exec.Command("go", "list", "-deps",
		"-f", "{{if .Module}}{{if .Module.Main}}{{.Dir}}{{end}}{{end}}", pkg).Output()
	dirs = slices.DeleteFunc(strings.Split(string(out), "\n"), func(d string) bool { return d == "" })
	if err != nil || len(dirs) == 0 {
		return ".", []string{"."}
	}
	return pkg, dirs
}

func (d *devState) status(st styles) string {
	switch {
	case d.building:
		return st.accent2.Render("⟳ rebuilding")
	case d.buildErr != "":
		return st.warn.Render("✗ build failed")
	default:
		return st.faint.Render("● dev")
	}
}
