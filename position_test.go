package decker

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// pos is the position a slide was drawn with.
type pos struct {
	index, count int
	section      string
}

func (p pos) of(c Ctx) bool {
	return c.Index == p.index && c.Count == p.count && c.Section == p.section
}

// positionDeck has five slides in two sections, the first slide before any
// section. rec collects the Ctx every View and Overlay call saw.
func positionDeck(rec *[]Ctx) *Deck {
	th := *testTheme
	th.Overlay = func(c Ctx, _ *Pixels) { *rec = append(*rec, c) }
	slide := func(title, section string) Slide {
		return Slide{Title: title, Section: section, Transition: TransitionDissolve,
			View: func(c Ctx, _ *Scene) { *rec = append(*rec, c) }}
	}
	return &Deck{Name: "pos", Theme: &th, Slides: []Slide{
		slide("a", ""), slide("b", "One"), slide("c", ""), slide("d", "Two"), slide("e", ""),
	}}
}

func TestSectionInheritance(t *testing.T) {
	d := positionDeck(new([]Ctx))
	want := []string{"", "One", "One", "Two", "Two"}
	for i, w := range want {
		if got := d.Section(i); got != w {
			t.Errorf("Section(%d) = %q, want %q", i, got, w)
		}
	}
	if got := sectionAt(nil, -1); got != "" {
		t.Errorf("sectionAt before the first slide = %q", got)
	}
}

// Every path that draws a slide hands its View and the theme's Overlay the
// slide's own position.
func TestCtxPosition(t *testing.T) {
	var rec []Ctx
	d := positionDeck(&rec)
	check := func(name string, i int) {
		t.Helper()
		if len(rec) == 0 {
			t.Fatalf("%s: nothing drawn", name)
		}
		for _, c := range rec {
			if !(pos{i, 5, d.Section(i)}).of(c) {
				t.Errorf("%s: drew slide %d with Index %d Count %d Section %q", name, i, c.Index, c.Count, c.Section)
			}
		}
		rec = rec[:0]
	}

	// Deck.Render and Deck.Draw set the position from i, whatever the caller passed.
	d.Render(3, Ctx{W: 20, H: 5, Index: 9, Count: 1, Section: "stale"})
	check("Render", 3)
	d.Draw(2, Ctx{W: 20, H: 5})
	check("Draw", 2)

	frameFlags{Width: 20, Height: 5, Time: 1}.still(d, 4, 0).release()
	check("frameFlags.still", 4)

	renderPreview(d.Slides, previewKey{slide: 1, pw: 20, ph: 6, dw: 240, dh: 67}, d.Theme)
	check("renderPreview", 1)
}

func TestCtxPositionLive(t *testing.T) {
	var rec []Ctx
	d := positionDeck(&rec)
	m := newModel(d, 1, 0, 30, nil)
	m.w, m.h = 40, 12
	if c := m.ctx(10); !(pos{1, 5, "One"}).of(c) {
		t.Fatalf("model ctx: %+v", c)
	}
	m.frame().release()
	for _, c := range rec {
		if !(pos{1, 5, "One"}).of(c) {
			t.Errorf("live frame drew position %d/%d %q", c.Index, c.Count, c.Section)
		}
	}

	// Mid-transition both slides are drawn, each with its own position.
	rec = rec[:0]
	m.now = m.now.Add(10 * time.Millisecond)
	m.goTo(3, 0, true)
	m.now = m.now.Add(100 * time.Millisecond)
	m.frame().release()
	seen := map[int]bool{}
	for _, c := range rec {
		if !(pos{c.Index, 5, d.Section(c.Index)}).of(c) {
			t.Errorf("transition drew slide %d as %d/%d %q", c.Index, c.Index, c.Count, c.Section)
		}
		seen[c.Index] = true
	}
	if !seen[1] || !seen[3] {
		t.Errorf("transition drew slides %v, want both 1 and 3", seen)
	}
	if st := m.linkState(); st.Outline[2].Section != "One" || st.Outline[0].Section != "" {
		t.Errorf("outline sections: %+v", st.Outline)
	}
}

func TestCtxPositionVideo(t *testing.T) {
	var rec []Ctx
	d := positionDeck(&rec)
	o := videoOptions{width: 80, height: 40, fps: 10, hold: 0.3, first: 1, last: 3}
	err := writeVideoFrames(d, o, writerFunc(func(b []byte) (int, error) { return len(b), nil }))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, c := range rec {
		if !(pos{c.Index, 5, d.Section(c.Index)}).of(c) {
			t.Fatalf("video drew slide %d as %d/%d %q", c.Index, c.Index, c.Count, c.Section)
		}
		seen[c.Index] = true
	}
	if !seen[1] || !seen[2] || !seen[3] || seen[0] || seen[4] {
		t.Errorf("video drew slides %v, want 1 to 3", seen)
	}
}

func TestListShowsSections(t *testing.T) {
	d := positionDeck(new([]Ctx))
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	listSlides(d)
	os.Stdout = stdout
	w.Close()
	b, _ := io.ReadAll(r)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 5 {
		t.Fatalf("%d lines: %q", len(lines), b)
	}
	for i, want := range []string{"", "[One]", "[One]", "[Two]", "[Two]"} {
		if want == "" {
			if strings.Contains(lines[i], "[") {
				t.Errorf("line %d = %q, want no section", i+1, lines[i])
			}
		} else if !strings.HasSuffix(lines[i], want) {
			t.Errorf("line %d = %q, want section %q", i+1, lines[i], want)
		}
	}
}

func TestPresenterHeaderShowsSection(t *testing.T) {
	d := positionDeck(new([]Ctx))
	outline := make([]linkOutline, len(d.Slides))
	for i, s := range d.Slides {
		outline[i] = linkOutline{Title: s.Title, Steps: s.steps(), Section: d.Section(i)}
	}
	p := newPresenter(d, "/tmp/x.sock", 30*time.Minute)
	p.w, p.h, p.linked = 100, 30, true
	p.st = linkState{Slide: 2, Outline: outline, W: 240, H: 67}
	p.running, p.since = true, p.now
	header := strings.Split(ansi.Strip(p.View().Content), "\n")[0]
	if !strings.Contains(header, "3/5") || !strings.Contains(header, "One") {
		t.Errorf("header %q lacks the position or section", header)
	}
	p.st.Slide = 0
	if header := strings.Split(ansi.Strip(p.View().Content), "\n")[0]; strings.Contains(header, "One") {
		t.Errorf("header %q shows a section for a slide before the first", header)
	}
}

func TestPageNumberAndProgressBar(t *testing.T) {
	c := Ctx{W: 100, H: 20, Theme: testTheme, Index: 11, Count: 40}
	sc := NewScene(c.W, c.H, testTheme)
	defer sc.Release()
	if w, h := PageNumber(c, sc.Px, 5, 5, Left, testTheme.Text); w <= 0 || h <= 0 {
		t.Errorf("PageNumber size %v x %v", w, h)
	}
	if w, h := PageNumber(Ctx{W: 100, H: 20, Theme: testTheme}, sc.Px, 5, 5, Left, testTheme.Text); w != 0 || h != 0 {
		t.Errorf("PageNumber drew %v x %v without a position", w, h)
	}

	track, fill := Hex("#000000"), Hex("#FFFFFF")
	bar := NewRect(0, 30, 100, 6)
	filled := func(index int) (n int) {
		c.Index = index
		b := NewScene(c.W, c.H, testTheme)
		defer b.Release()
		ProgressBar(c, b.Px, bar, fill, track)
		for x := 0; x < b.Px.W; x++ {
			if b.Px.Pix[33*b.Px.W+x] == fill {
				n++
			}
		}
		return n
	}
	if got := filled(39); got < 98 {
		t.Errorf("last slide fills %d of 100 px", got)
	}
	if lo, hi := filled(0), filled(19); !(lo < hi && hi >= 48 && hi <= 51) {
		t.Errorf("fill widths %d, %d: want growing, halfway at 20/40", lo, hi)
	}
}
