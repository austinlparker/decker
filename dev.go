package decker

import (
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// Dev mode (-dev): watch the Go sources, rebuild on save, and if the build
// succeeds, replace this process with the new binary on the same slide.
// If the build fails, the compiler output is shown on top of the deck and
// the old version keeps running.
//
// It rebuilds the running deck's own main package, and watches every
// package of the same module that the deck imports, so editing the talk or
// the engine both reload.

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
			if strings.HasPrefix(name, ".") || !(strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum") {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
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

func (d *devState) wait() tea.Cmd {
	return func() tea.Msg {
		<-d.events
		return fileChangedMsg{}
	}
}

func (d *devState) build() tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("go", "build", "-o", d.bin, d.pkg).CombinedOutput()
		return buildDoneMsg{out: string(out), err: err}
	}
}

// devTarget returns the running deck's main package and the directories of
// every package in its module that it depends on. Without build info (or
// outside the module) it falls back to the current directory.
func devTarget() (pkg string, dirs []string) {
	pkg = "."
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Path != "" && bi.Path != "command-line-arguments" {
		pkg = bi.Path
	}
	out, err := exec.Command("go", "list", "-deps",
		"-f", "{{if .Module}}{{if .Module.Main}}{{.Dir}}{{end}}{{end}}", pkg).Output()
	if err != nil {
		return ".", []string{"."}
	}
	for _, dir := range strings.Split(string(out), "\n") {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) == 0 {
		return ".", []string{"."}
	}
	return pkg, dirs
}

// status is a short footer label.
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
