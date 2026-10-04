package decker

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// shortSocket returns a socket path short enough for macOS (104 bytes),
// which t.TempDir's paths can exceed.
func shortSocket(t *testing.T) string {
	dir, err := os.MkdirTemp("", "lk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

func TestLinkRoundTrip(t *testing.T) {
	path := shortSocket(t)
	cmds := make(chan linkCmd, 4)
	srv, err := listenLink(path, func(c linkCmd) { cmds <- c })
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// A state published before anyone connects arrives on connect.
	srv.Publish(linkState{Slide: 3, Step: 1, Notes: "hello"})
	var cl linkClient
	if err := cl.dial(path); err != nil {
		t.Fatal(err)
	}
	defer cl.close()
	st, err := cl.next()
	if err != nil || st.Slide != 3 || st.Step != 1 || st.Notes != "hello" {
		t.Fatalf("first state = %+v, %v", st, err)
	}

	srv.Publish(linkState{Slide: 4})
	if st, err = cl.next(); err != nil || st.Slide != 4 {
		t.Fatalf("second state = %+v, %v", st, err)
	}

	cl.send(linkCmd{Cmd: "goto", Slide: 12})
	select {
	case c := <-cmds:
		if c.Cmd != "goto" || c.Slide != 12 {
			t.Fatalf("command = %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("command never arrived")
	}

	// Closing the deck's end ends the presenter's read, so it can reconnect.
	srv.Close()
	if _, err := cl.next(); err == nil {
		t.Fatal("read after close succeeded")
	}
}

func TestLinkSocketFiles(t *testing.T) {
	path := shortSocket(t)
	// A file left behind by a deck that exited is replaced.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := listenLink(path, nil)
	if err != nil {
		t.Fatalf("stale socket file: %v", err)
	}
	// A second deck on a live socket is refused.
	if _, err := listenLink(path, nil); err == nil {
		t.Fatal("second deck on a live socket was allowed")
	}
	srv.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Close left the socket file behind")
	}
}

func TestDeckFollowsLinkCommands(t *testing.T) {
	d := testDeck()
	slides := d.Slides
	var m tea.Model = newModel(d, 0, 0, 30, nil)
	do := func(c linkCmd) model {
		m, _ = m.Update(c)
		return m.(model)
	}
	if got := do(linkCmd{Cmd: "next"}); got.idx != 1 || got.step != 0 {
		t.Fatalf("next from slide 1: slide %d step %d", got.idx+1, got.step+1)
	}
	if got := do(linkCmd{Cmd: "goto", Slide: 3}); got.idx != 2 {
		t.Fatalf("goto 3: slide %d", got.idx+1)
	}
	if got := do(linkCmd{Cmd: "last"}); got.idx != len(slides)-1 {
		t.Fatalf("last: slide %d", got.idx+1)
	}
	if got := do(linkCmd{Cmd: "first"}); got.idx != 0 {
		t.Fatalf("first: slide %d", got.idx+1)
	}
	if got := do(linkCmd{Cmd: "bogus"}); got.idx != 0 {
		t.Fatalf("unknown command moved the deck to slide %d", got.idx+1)
	}
}

func TestPace(t *testing.T) {
	outline := []linkOutline{{"a", 1}, {"b", 4}, {"c", 1}} // 6 steps
	const talk = 30 * time.Minute
	cases := []struct {
		slide, step int
		elapsed     time.Duration
		want        time.Duration
	}{
		{0, 0, 0, 0},                              // the start
		{1, 2, 15 * time.Minute, 0},               // half the steps, half the time
		{1, 2, 10 * time.Minute, 5 * time.Minute}, // ahead
		{0, 0, 5 * time.Minute, -5 * time.Minute}, // behind
	}
	for _, c := range cases {
		if got := pace(outline, c.slide, c.step, c.elapsed, talk); got != c.want {
			t.Errorf("pace(slide %d, step %d, %v) = %v, want %v", c.slide, c.step, c.elapsed, got, c.want)
		}
	}
}

// TestPresenterViewFits draws the presenter view at several terminal sizes
// and checks every frame is exactly the terminal's size. Set PRESENTER_PNG
// to a directory to also save each frame as an image.
func TestPresenterViewFits(t *testing.T) {
	slides := testDeck().Slides
	outline := make([]linkOutline, len(slides))
	for i, s := range slides {
		outline[i] = linkOutline{s.Title, s.steps()}
	}
	out := os.Getenv("PRESENTER_PNG")
	for _, size := range [][2]int{{60, 15}, {80, 24}, {120, 36}, {200, 50}} {
		for _, at := range []struct {
			name        string
			slide, step int
			linked      bool
		}{{"waiting", -1, 0, false}, {"s2", 1, 1, true}, {"s3", 2, 0, true}, {"last", len(slides) - 1, 0, false}} {
			p := presenter{slides: slides, theme: testTheme, sty: testTheme.styles(), socket: "/tmp/x.sock", length: 30 * time.Minute,
				link: &linkClient{}, previews: map[previewKey]string{}, now: time.Now(),
				w: size[0], h: size[1], linked: at.linked}
			if at.slide >= 0 {
				p.st = linkState{Slide: at.slide, Step: at.step, Outline: outline, Notes: slides[at.slide].Notes, W: 682, H: 171}
				p.running, p.since, p.banked = true, p.now.Add(-7*time.Minute), 0
			}
			view := p.View().Content
			lines := strings.Split(view, "\n")
			if len(lines) != size[1] {
				t.Errorf("%dx%d %s: %d lines", size[0], size[1], at.name, len(lines))
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w > size[0] {
					t.Errorf("%dx%d %s: line %d is %d wide", size[0], size[1], at.name, i+1, w)
				}
			}
			if out != "" {
				name := filepath.Join(out, at.name+"-"+strconv.Itoa(size[0])+"x"+strconv.Itoa(size[1])+".png")
				if err := writePNG(view, size[0], size[1], name, testTheme); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
