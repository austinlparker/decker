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

// devState implements --dev: it rebuilds the deck's main package on save of any
// watched Go source or embedded file (in every same-module package it
// imports, so talk and engine edits both reload, and so do notes and images). A good build replaces the process on the same slide; a
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
	restart    []string // set when a good build wants to replace this process
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
	pkg, watch := devTarget()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, dir := range watch.dirs {
		if err := w.Add(dir); err != nil {
			return nil, err
		}
	}
	d := &devState{events: make(chan struct{}, 1), pkg: pkg, bin: bin}
	go func() {
		for ev := range w.Events {
			if ev.Op == fsnotify.Chmod || !watch.rebuilds(ev.Name) {
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
	return []string{d.bin, "--dev", "--slide", strconv.Itoa(slide), "--step", strconv.Itoa(step), "--fps", strconv.Itoa(fps)}
}

func execRestart(args []string, socket string) error {
	args = append(args, "--socket", socket)
	return syscall.Exec(args[0], args, os.Environ())
}

// devWatch is what dev mode watches: directories, and the embedded files in
// them, by absolute path.
type devWatch struct {
	dirs   []string
	embeds map[string]bool
}

// rebuilds reports whether a change to path should rebuild the deck: an
// embedded file, Go source, or the module's go.mod or go.sum.
func (w devWatch) rebuilds(path string) bool {
	if w.embeds[path] {
		return true
	}
	name := filepath.Base(path)
	return !strings.HasPrefix(name, ".") && (strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum")
}

// devTarget returns the deck's main package and what to watch: the directory
// of every same-module package it depends on, and the files each embeds
// with their directories; without build info it falls back to ".".
func devTarget() (pkg string, w devWatch) {
	pkg = "."
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Path != "" && bi.Path != "command-line-arguments" {
		pkg = bi.Path
	}
	out, err := exec.Command("go", "list", "-deps",
		"-f", `{{if .Module}}{{if .Module.Main}}{{.Dir}}{{range .EmbedFiles}}{{"\t"}}{{.}}{{end}}{{end}}{{end}}`, pkg).Output()
	w = parseDevList(string(out))
	if err != nil || len(w.dirs) == 0 {
		return ".", devWatch{dirs: []string{"."}}
	}
	return pkg, w
}

// parseDevList reads devTarget's go list output: a line per package, its
// directory then the files it embeds, relative to it, separated by tabs.
// Embedded files can sit in subdirectories, which fsnotify only watches
// when asked.
func parseDevList(out string) devWatch {
	w := devWatch{embeds: map[string]bool{}}
	add := func(dir string) {
		if !slices.Contains(w.dirs, dir) {
			w.dirs = append(w.dirs, dir)
		}
	}
	for line := range strings.Lines(out) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		if fields[0] == "" {
			continue
		}
		add(fields[0])
		for _, f := range fields[1:] {
			path := filepath.Join(fields[0], filepath.FromSlash(f))
			w.embeds[path] = true
			add(filepath.Dir(path))
		}
	}
	return w
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
