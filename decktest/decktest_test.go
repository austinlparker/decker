package decktest

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/austinlparker/decker"
)

var theme = &decker.Theme{
	Display: decker.StockFont("SpaceGrotesk-Bold"),
	Body:    decker.StockFont("SpaceGrotesk-Medium"),
	Mono:    decker.StockFont("JetBrainsMono-ExtraBold"),
}

// boomDeck is a one-slide deck that panics in part ("View", "a placed
// element" or "Theme.Overlay"), or nowhere for "".
func boomDeck(part string) decker.Deck {
	boom := func() { panic(errBoom) }
	th := *theme
	s := decker.Slide{Title: "Boom", View: func(decker.Ctx, *decker.Scene) {}}
	switch part {
	case "View":
		s.View = func(decker.Ctx, *decker.Scene) { boom() }
	case "a placed element":
		s.View = func(c decker.Ctx, sc *decker.Scene) {
			sc.Place("k", c.Frame(), func(*decker.Pixels, decker.Rect) { boom() })
		}
	case "Theme.Overlay":
		th.Overlay = func(decker.Ctx, *decker.Pixels) { boom() }
	}
	return decker.Deck{Name: "boom", Theme: &th, Slides: []decker.Slide{s}}
}

var errBoom = errors.New("boom")

var parts = []string{"View", "a placed element", "Theme.Overlay"}

// TestDrawReportsEveryPart checks Deck.Draw, which Slides fails on, returns
// a panic wherever the engine catches one: in View, in a placed element, or
// in the overlay.
func TestDrawReportsEveryPart(t *testing.T) {
	d := boomDeck("")
	if err := d.Draw(0, decker.Ctx{W: 40, H: 12}); err != nil {
		t.Errorf("no panic: got %v", err)
	}
	Slides(t, d)
	for _, part := range parts {
		d := boomDeck(part)
		err := d.Draw(0, decker.Ctx{W: 40, H: 12})
		if want := `slide 1 "Boom": ` + part + " panicked: boom"; err == nil || err.Error() != want {
			t.Errorf("%s: got %v, want %s", part, err, want)
		}
		if !errors.Is(err, errBoom) {
			t.Errorf("%s: %v does not wrap the panic's error", part, err)
		}
	}
}

// TestSlidesFailsOnEveryPart runs Slides on a deck that panics in each part,
// in a child test process, and checks it fails naming the part.
func TestSlidesFailsOnEveryPart(t *testing.T) {
	if part := os.Getenv("DECKTEST_BOOM"); part != "" {
		Slides(t, boomDeck(part))
		return
	}
	for _, part := range parts {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSlidesFailsOnEveryPart$")
		cmd.Env = append(os.Environ(), "DECKTEST_BOOM="+part)
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), part+" panicked: boom (at 80x24, build 1, t=0)") {
			t.Errorf("%s: Slides passed or said something else: %v\n%s", part, err, out)
		}
	}
}

func TestReviewPassesACleanDeck(t *testing.T) {
	d := decker.Deck{Name: "clean", Theme: theme, Slides: []decker.Slide{{
		Title: "Hello",
		View: func(c decker.Ctx, sc *decker.Scene) {
			decker.Text{Font: c.Theme.Display, Size: c.Size(0.15)}.Draw(sc.Px, "Hello", c.X(0.1), c.Y(0.2))
		},
	}}}
	Review(t, d)
	if is := d.Review(); len(is) != 0 {
		t.Errorf("issues in a clean deck: %v", is)
	}
	d.Slides[0].View = func(c decker.Ctx, sc *decker.Scene) {
		decker.Text{Font: c.Theme.Display, Size: c.Size(0.15)}.Draw(sc.Px, "Hello, off the edge", c.X(0.8), c.Y(0.2))
	}
	is := d.Review()
	if len(is) != 3 || is[0].Severity != decker.SeverityError || !strings.Contains(is[0].Msg, "past the right edge") {
		t.Errorf("off-edge deck: %v", is)
	}
}
