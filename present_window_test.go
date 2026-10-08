package decker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The deck window p opens runs the way the presenter's deck flags say, and
// nothing else from the presenter's command line comes with it.
func TestPresentationArgs(t *testing.T) {
	_, ctx, err := parseArgs(testDeck(), []string{"present", "--dev", "--fps=30", "--length=45m",
		"--presentation-font-size=4.5", "--previews=image", "--socket=relative.sock", "--slide=3", "--step=2"})
	if err != nil {
		t.Fatal(err)
	}
	c := ctx.Selected().Target.Interface().(presentCmd)
	if got, want := c.args(), []string{"--fps=30", "--dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("launch arguments = %q, want %q", got, want)
	}
	if c.Socket != "relative.sock" || c.Slide != 3 || c.Step != 2 || c.Length != 45*time.Minute {
		t.Fatalf("parsed %+v", c)
	}
}

// Ghostty's macOS surface API wraps the command in "exec -l" itself. Paths,
// flag values and positional arguments must survive that wrapper unchanged.
func TestPresentationCommandQuoting(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "unexpected")
	executable := filepath.Join(dir, "deck's executable")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\0' \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	w := presentationWindow{executable: executable, socket: filepath.Join(dir, "a socket's name"),
		args: []string{"-theme=$(touch " + marker + ");`touch " + marker + "` ' quoted \n text", "file with spaces"}}
	out, err := exec.Command("bash", "--noprofile", "--norc", "-c", "exec -l "+w.command(7, 3)).CombinedOutput()
	if err != nil {
		t.Fatalf("Ghostty shell wrapper: %v\n%s", err, out)
	}
	got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	want := append([]string{"--socket=" + w.socket, "--slide=7", "--step=3"}, w.args...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("shell received %q, want %q", got, want)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("argument contents executed: %v", err)
	}
}

func TestPresentationSurfaceCommand(t *testing.T) {
	// Unlike the command config option, the surface API treats "shell:" as
	// literal command text. Pass the quoted command argv through unchanged.
	if !strings.Contains(presentationWindowScript, "set command of cfg to item 1 of argv\n") {
		t.Fatal("surface command must receive argv directly, without config prefixes")
	}
}

func TestPresenterLaunchLifecycle(t *testing.T) {
	d := testDeck()
	p := newPresenter(d, "/tmp/talk.sock", 30*time.Minute)
	p.w, p.h = 100, 30
	var launches [][2]int
	launchErr := errors.New("Automation permission denied")
	p.launch = func(slide, step int) error {
		launches = append(launches, [2]int{slide, step})
		return launchErr
	}
	if !strings.Contains(p.View().Content, "Press p") {
		t.Fatal("waiting view does not explain how to open the deck")
	}
	next, cmd := p.handleKey("p")
	p = next.(presenter)
	if cmd == nil || !p.launching {
		t.Fatal("p did not start a launch")
	}
	if _, again := p.handleKey("p"); again != nil {
		t.Fatal("repeated p started a duplicate launch")
	}
	next, _ = p.Update(cmd())
	p = next.(presenter)
	if p.launching || p.launchBusy || !strings.Contains(p.View().Content, launchErr.Error()) {
		t.Fatal("launch failure was not displayed or did not allow retry")
	}
	launchErr = nil
	p.st.Slide, p.st.Step = 6, 2
	next, cmd = p.handleKey("p")
	p = next.(presenter)
	next, _ = p.Update(cmd())
	p = next.(presenter)
	if !p.launching || p.launchBusy {
		t.Fatal("successful launch did not wait for a connection")
	}
	next, _ = p.Update(presentationWaitMsg(p.launchAttempt))
	p = next.(presenter)
	if p.launching || !strings.Contains(p.launchErr, "not connected") {
		t.Fatal("connection timeout did not allow retry")
	}
	next, cmd = p.handleKey("p")
	p = next.(presenter)
	if cmd == nil {
		t.Fatal("timeout prevented retry")
	}
	// A connection can arrive before the AppleScript command returns. Keep
	// blocking launches until that command finishes, even if the link drops.
	next, _ = p.Update(linkUpMsg{})
	p = next.(presenter)
	next, _ = p.Update(linkDownMsg{})
	p = next.(presenter)
	if _, again := p.handleKey("p"); again != nil {
		t.Fatal("reconnecting started a concurrent launch")
	}
	next, _ = p.Update(cmd())
	p = next.(presenter)
	next, _ = p.Update(linkUpMsg{})
	p = next.(presenter)
	if p.launching || p.launchErr != "" {
		t.Fatal("connection did not clear launch feedback")
	}
	if _, again := p.handleKey("p"); again != nil {
		t.Fatal("connected presenter started another deck")
	}
	want := [][2]int{{1, 1}, {7, 3}, {7, 3}}
	if !reflect.DeepEqual(launches, want) {
		t.Fatalf("launched positions %v, want %v", launches, want)
	}
}

func TestPresenterIgnoresOldLaunchResults(t *testing.T) {
	d := testDeck()
	p := newPresenter(d, "", time.Minute)
	p.launchAttempt, p.launching, p.launchBusy = 2, true, true
	for _, msg := range []tea.Msg{presentationOpenedMsg{1, errors.New("old failure")}, presentationWaitMsg(1)} {
		next, _ := p.Update(msg)
		p = next.(presenter)
		if !p.launching || !p.launchBusy || p.launchErr != "" {
			t.Fatalf("old result %T changed the new launch", msg)
		}
	}
}
