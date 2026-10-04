package decker

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
	defer srv.close()

	// A state published before anyone connects arrives on connect.
	srv.publish(linkState{Slide: 3, Step: 1, Notes: "hello"})
	var cl linkClient
	if err := cl.dial(path); err != nil {
		t.Fatal(err)
	}
	defer cl.close()
	st, err := cl.next()
	if err != nil || st.Slide != 3 || st.Step != 1 || st.Notes != "hello" {
		t.Fatalf("first state = %+v, %v", st, err)
	}

	srv.publish(linkState{Slide: 4})
	if st, err = cl.next(); err != nil || st.Slide != 4 {
		t.Fatalf("second state = %+v, %v", st, err)
	}

	cl.send(linkCmd{Slide: 12})
	select {
	case c := <-cmds:
		if c.Slide != 12 {
			t.Fatalf("command = %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("command never arrived")
	}

	// Closing the deck's end ends the presenter's read, so it can reconnect.
	srv.close()
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
	srv.close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Close left the socket file behind")
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
			p := newPresenter(&Deck{Slides: slides, Theme: testTheme}, "/tmp/x.sock", 30*time.Minute)
			p.w, p.h, p.linked = size[0], size[1], at.linked
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
				if err := writePNG(parseGrid(view, size[0], size[1], testTheme), name); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
