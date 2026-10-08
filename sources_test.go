package decker

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const otelURL = "https://opentelemetry.io/docs/specs/otel/logs/data-model/"

// citedDeck has a slide with no sources, one with two, and one citing the
// same two again: the first by its URL under another label, the second by
// its label alone.
func citedDeck() *Deck {
	return &Deck{Name: "cited", Theme: testTheme, Slides: []Slide{
		{Title: "Opening", Section: "Intro",
			Sources: []Source{{Label: "Go (language)", URL: "https://en.wikipedia.org/wiki/Go_(programming_language)"}}},
		{Title: "Logs *and* traces", Steps: 2, Notes: "Say why logs matter.\n\nThen the model.\n",
			Sources: []Source{{Label: "OpenTelemetry Logs Data Model, v1.40", URL: otelURL}, {Label: "A book, ch. 3"}}},
		{Title: "Again", Section: "Detail",
			Sources: []Source{{Label: "OTel logs", URL: otelURL}, {Label: "A book, ch. 3"}}},
		{Title: "Plain"},
	}}
}

// Ctx stays comparable: apidiff counts losing == as a break.
var _ = Ctx{} == Ctx{}

func TestCtxSources(t *testing.T) {
	d := citedDeck()
	var got [][]Source
	for i := range d.Slides {
		d.Slides[i].View = func(c Ctx, _ *Scene) { got = append(got, c.Sources()) }
	}
	for i := range d.Slides {
		d.Draw(i, Ctx{W: 20, H: 5})
	}
	for i, s := range d.Slides {
		if !slices.Equal(got[i], s.Sources) {
			t.Errorf("slide %d: Sources() = %v, want %v", i, got[i], s.Sources)
		}
	}
	if src := (Ctx{W: 20, H: 5, Index: 1, Count: 4}).Sources(); src != nil {
		t.Errorf("a hand-built Ctx has sources %v", src)
	}
	c := d.At(1, Ctx{W: 20, H: 5})
	if c.Index != 1 || c.Count != len(d.Slides) || c.Theme != d.Theme || c.Section != d.Section(1) || !slices.Equal(c.Sources(), d.Slides[1].Sources) {
		t.Errorf("At(1) = %+v", c)
	}
}

func TestOutlineJSON(t *testing.T) {
	var b bytes.Buffer
	if err := writeOutline(&b, citedDeck()); err != nil {
		t.Fatal(err)
	}
	var got []outlineSlide
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("%v in %s", err, b.String())
	}
	d := citedDeck()
	if len(got) != len(d.Slides) {
		t.Fatalf("%d entries, want %d", len(got), len(d.Slides))
	}
	for i, s := range d.Slides {
		want := outlineSlide{i + 1, s.Title, s.steps(), d.Section(i), s.Notes, s.Sources}
		if want.Sources == nil {
			want.Sources = []Source{}
		}
		g := got[i]
		if g.Slide != want.Slide || g.Title != want.Title || g.Steps != want.Steps || g.Section != want.Section ||
			g.Notes != want.Notes || !slices.Equal(g.Sources, want.Sources) {
			t.Errorf("entry %d = %+v, want %+v", i, g, want)
		}
	}
	out := b.String()
	for _, want := range []string{
		`"slide": 1,`,
		`"section": "Intro",`,
		`"sources": []`,                // an empty list, not null
		`"title": "Logs *and* traces"`, // no HTML escaping either
	} {
		if !strings.Contains(out, want) {
			t.Errorf("outline lacks %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, `"url": ""`) {
		t.Errorf("a source with no URL printed one:\n%s", out)
	}
}

func TestLinkStateSources(t *testing.T) {
	d := citedDeck()
	m := testModel(d, 1, 0, 80, 24, Settled, nil)
	st := m.linkState()
	if !slices.Equal(st.Sources, d.Slides[1].Sources) {
		t.Errorf("linkState sources = %v", st.Sources)
	}
	b, _ := json.Marshal(st)
	if !strings.Contains(string(b), `"sources":[{"label":"OpenTelemetry`) {
		t.Errorf("state JSON lacks the sources: %s", b)
	}
	b, _ = json.Marshal(testModel(d, 3, 0, 80, 24, Settled, nil).linkState())
	if strings.Contains(string(b), "sources") {
		t.Errorf("a slide without sources sent them: %s", b)
	}
}

func TestNotesPanelShowsSources(t *testing.T) {
	d := citedDeck()
	// At 30 lines the panel holds 7: the notes' trailing newline mustn't
	// cost a source its line.
	m := press(testModel(d, 1, 0, 100, 30, Settled, nil), "n")
	screen := ansi.Strip(view(m))
	for _, want := range []string{"sources:", "OpenTelemetry Logs Data Model, v1.40 — " + otelURL, "A book, ch. 3"} {
		if !strings.Contains(screen, want) {
			t.Errorf("notes panel lacks %q", want)
		}
	}
	m = press(testModel(d, 3, 0, 120, 36, Settled, nil), "n")
	if strings.Contains(ansi.Strip(view(m)), "sources:") {
		t.Error("notes panel lists sources for a slide without any")
	}
}

// presenterAt is the presenter view of d at slide i, as the deck would
// report it.
func presenterAt(d *Deck, i, w, h int) presenter {
	outline := make([]linkOutline, len(d.Slides))
	for j, s := range d.Slides {
		outline[j] = linkOutline{Title: s.Title, Steps: s.steps(), Section: d.Section(j)}
	}
	p := newPresenter(d, "/tmp/x.sock", 30*time.Minute)
	p.w, p.h, p.linked = w, h, true
	s := d.Slides[i]
	p.st = linkState{Slide: i, Outline: outline, Notes: s.Notes, Sources: s.Sources, W: 682, H: 171}
	return p
}

func TestPresenterShowsSources(t *testing.T) {
	d := citedDeck()
	view := ansi.Strip(presenterAt(d, 1, 160, 40).View().Content)
	notes, sources := strings.Index(view, "NOTES"), strings.Index(view, "SOURCES")
	if notes < 0 || sources < notes {
		t.Fatalf("want NOTES, then SOURCES:\n%s", view)
	}
	for _, want := range []string{"OpenTelemetry Logs Data Model, v1.40 — " + otelURL, "\n  A book, ch. 3\n"} {
		if !strings.Contains(view[sources:], want) {
			t.Errorf("sources lack %q:\n%s", want, view)
		}
	}
	if view := ansi.Strip(presenterAt(d, 3, 160, 40).View().Content); strings.Contains(view, "SOURCES") {
		t.Errorf("a slide without sources shows SOURCES:\n%s", view)
	}

	// Long notes and long sources share the room, and still fit the window.
	// At 80x24 the previews leave 7 lines: room for the labels and a little
	// notes, but not a source.
	d.Slides[1].Notes = strings.Repeat("a long line of speaker notes, which wraps over several lines\n", 20)
	for range 8 {
		d.Slides[1].Sources = append(d.Slides[1].Sources, Source{Label: strings.Repeat("long label ", 20), URL: otelURL})
	}
	for _, size := range [][3]int{{60, 15, 2}, {80, 24, 0}, {120, 36, 4}, {200, 50, 6}} {
		view := presenterAt(d, 1, size[0], size[1]).View().Content
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w > size[0] {
				t.Errorf("%dx%d: line %d is %d wide", size[0], size[1], i+1, w)
			}
		}
		plain := ansi.Strip(view)
		if !strings.Contains(plain, "SOURCES") || !strings.Contains(plain, "a long line") {
			t.Errorf("%dx%d: notes or sources crowded out:\n%s", size[0], size[1], plain)
		}
		shown := 0
		for _, l := range strings.Split(plain, "\n") {
			for _, start := range []string{"OpenTelemetry", "A book", "long label"} {
				if strings.HasPrefix(strings.TrimSpace(l), start) {
					shown++
				}
			}
		}
		if shown != size[2] {
			t.Errorf("%dx%d: %d sources shown, want %d:\n%s", size[0], size[1], shown, size[2], plain)
		}
	}
}

func TestShare(t *testing.T) {
	for _, c := range []struct{ n, a, b, wantA, wantB int }{
		{10, 3, 4, 3, 4},   // both fit
		{10, 20, 0, 10, 0}, // no sources: the notes take it all
		{10, 20, 3, 7, 3},  // short sources keep theirs
		{10, 2, 20, 2, 8},  // short notes keep theirs
		{10, 20, 20, 5, 5}, // both long: halves
		{9, 20, 20, 5, 4},
		{-3, 4, 4, 0, 0},
	} {
		if a, b := share(c.n, c.a, c.b); a != c.wantA || b != c.wantB {
			t.Errorf("share(%d, %d, %d) = %d, %d, want %d, %d", c.n, c.a, c.b, a, b, c.wantA, c.wantB)
		}
	}
}
