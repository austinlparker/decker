package decker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestBlankKeysToggle(t *testing.T) {
	m := testModel(testDeck(), 1, 0, 80, 24, Settled, &devState{}) // dev mode: with a footer
	m.showNotes, m.showHelp = true, true
	for _, c := range []struct {
		key  string
		want blankMode
	}{
		{"b", blankBlack}, {"b", blankNone}, {".", blankBlack}, {".", blankNone},
		{"w", blankWhite}, {"w", blankNone}, {",", blankWhite}, {",", blankNone},
		{"b", blankBlack}, {"w", blankWhite}, // the other color switches
		{"x", blankWhite}, {"ctrl+l", blankWhite}, {"5", blankWhite}, // keys that don't navigate leave it up
		{"w", blankNone},
	} {
		m = press(m, c.key)
		if m.blank != c.want {
			t.Fatalf("after %q: blank = %v, want %v", c.key, m.blank, c.want)
		}
		if m.blank != blankNone && (m.showNotes || m.showHelp) {
			t.Fatalf("after %q: overlays still open", c.key)
		}
	}
	if m.idx != 1 {
		t.Fatalf("blank keys moved the deck to slide %d", m.idx+1)
	}
}

func TestBlankFrameCoversScreen(t *testing.T) {
	m := testModel(testDeck(), 1, 0, 80, 24, Settled, &devState{})
	for _, c := range []struct {
		key  string
		want [3]uint8
	}{{"b", [3]uint8{0, 0, 0}}, {"w", [3]uint8{255, 255, 255}}} {
		m = press(m, c.key)
		g := m.frame()
		if g.W != 80 || g.H != 24 {
			t.Fatalf("%s: frame is %dx%d", c.key, g.W, g.H)
		}
		for i, cell := range g.Cells {
			if cell.ch != " " || cell.bg != c.want || cell.fg != c.want {
				t.Fatalf("%s: cell %d = %+v, want a blank %v cell (footer included)", c.key, i, cell, c.want)
			}
		}
		g.release()
	}
}

func TestBlankKeepsClockRunning(t *testing.T) {
	m := testModel(testDeck(), 1, 0, 80, 24, 0, nil)
	m = press(m, "b")
	m.advance(goldenBase.Add(3 * time.Second))
	if m.blank != blankBlack || m.ctx(24).T != 3 {
		t.Fatalf("blank = %v, T = %v after 3s blanked", m.blank, m.ctx(24).T)
	}
}

func TestBlankEndsOnNavigation(t *testing.T) {
	last := len(testDeck().Slides) - 1
	for _, c := range []struct {
		key string
		idx int // slide afterwards, from slide 2 (index 1) at step 0
	}{
		{"]", 2}, {"[", 0}, {"g", 0}, {"G", last}, {"r", 1}, {"n", 1}, {"?", 1}, {"esc", 1},
	} {
		m := testModel(testDeck(), 1, 0, 80, 24, Settled, nil)
		m = press(press(m, "w"), c.key)
		if m.blank != blankNone {
			t.Errorf("%q: still blank", c.key)
		}
		if m.idx != c.idx {
			t.Errorf("%q: slide %d, want %d", c.key, m.idx+1, c.idx+1)
		}
	}

	// Next restores the slide and then navigates as usual.
	plain := press(testModel(testDeck(), 1, 0, 80, 24, Settled, nil), "right")
	m := press(press(testModel(testDeck(), 1, 0, 80, 24, Settled, nil), "b"), "right")
	if m.blank != blankNone || m.idx != plain.idx || m.step != plain.step {
		t.Errorf("right while blank: blank %v, s%d.%d, want s%d.%d", m.blank, m.idx, m.step, plain.idx, plain.step)
	}

	// A jump by number ends it too, but typing the digits does not.
	m = press(testModel(testDeck(), 1, 0, 80, 24, Settled, nil), "b")
	if m = press(m, "3"); m.blank == blankNone {
		t.Fatal("a digit ended the blank")
	}
	if m = press(m, "enter"); m.blank != blankNone || m.idx != 2 {
		t.Fatalf("3 enter: blank %v, slide %d", m.blank, m.idx+1)
	}
}

func TestBlankFromPresenterLink(t *testing.T) {
	d := testDeck()
	path := shortSocket(t)
	cmds := make(chan linkCmd, 4)
	srv, err := listenLink(path, func(c linkCmd) { cmds <- c })
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()

	// The presenter view forwards the blank keys, since they are nav keys...
	p := newPresenter(d, path, 30*time.Minute)
	if err := p.link.dial(path); err != nil {
		t.Fatal(err)
	}
	defer p.link.close()
	keys := []string{"b", ".", "w", ",", "w", "right"}
	for _, k := range keys {
		p.handleKey(k)
	}

	// ...and the deck applies them as if they were pressed on it.
	m := testModel(d, 1, 0, 80, 24, Settled, nil)
	want := []blankMode{blankBlack, blankNone, blankWhite, blankNone, blankWhite, blankNone}
	for i, k := range keys {
		select {
		case c := <-cmds:
			if c.Key != k {
				t.Fatalf("command %d = %+v, want key %q", i, c, k)
			}
			next, _ := m.Update(c)
			m = next.(model)
			if m.blank != want[i] {
				t.Fatalf("after %q: blank = %v, want %v", k, m.blank, want[i])
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("command %d (%q) never arrived", i, k)
		}
	}
	if m.idx == 1 && m.step == 0 {
		t.Fatal("right after unblanking did not navigate")
	}
}

func TestBlankIsReportedOverLink(t *testing.T) {
	m := testModel(testDeck(), 1, 0, 80, 24, Settled, nil)
	path := shortSocket(t)
	srv, err := listenLink(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	m.link = srv
	var cl linkClient
	if err := cl.dial(path); err != nil {
		t.Fatal(err)
	}
	defer cl.close()

	next, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(model)
	if st, err := cl.next(); err != nil || st.Blank != "white" || st.Slide != 1 {
		t.Fatalf("state after w = %+v, %v", st, err)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = next.(model)
	if st, err := cl.next(); err != nil || st.Blank != "" || st.Slide != 1 {
		t.Fatalf("state after w again = %+v, %v", st, err)
	}

	// An unblanked state is the JSON it was before the field existed.
	b, _ := json.Marshal(m.linkState())
	if strings.Contains(string(b), "blank") {
		t.Fatalf("unblanked state mentions blank: %s", b)
	}
}

func TestPresenterShowsBlank(t *testing.T) {
	slides := testDeck().Slides
	outline := make([]linkOutline, len(slides))
	for i, s := range slides {
		outline[i] = linkOutline{Title: s.Title, Steps: s.steps()}
	}
	p := newPresenter(&Deck{Slides: slides, Theme: testTheme}, "/tmp/x.sock", 30*time.Minute)
	p.w, p.h, p.linked = 100, 30, true
	p.st = linkState{Slide: 1, Outline: outline, Notes: "my notes", W: 682, H: 171}
	if strings.Contains(p.View().Content, "BLANK") {
		t.Fatal("BLANK shown on a live slide")
	}
	p.st.Blank = "white"
	v := p.View().Content
	for _, want := range []string{"BLANK", "white", "my notes", "NOW", "NEXT"} {
		if !strings.Contains(v, want) {
			t.Errorf("blanked presenter view has no %q:\n%s", want, v)
		}
	}
}
