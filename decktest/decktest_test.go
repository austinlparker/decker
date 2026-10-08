package decktest

import (
	"strings"
	"testing"

	"github.com/austinlparker/decker"
)

var theme = &decker.Theme{
	Display: decker.StockFont("SpaceGrotesk-Bold"),
	Body:    decker.StockFont("SpaceGrotesk-Medium"),
	Mono:    decker.StockFont("JetBrainsMono-ExtraBold"),
}

// TestDrawFrameCatchesEveryPart checks Slides sees a panic wherever the
// engine would draw it: in View, in a placed element, or in the overlay.
func TestDrawFrameCatchesEveryPart(t *testing.T) {
	boom := func() { panic("boom") }
	overlay := *theme
	overlay.Overlay = func(decker.Ctx, *decker.Pixels) { boom() }
	for _, tc := range []struct {
		part  string
		theme *decker.Theme
		view  func(decker.Ctx, *decker.Scene)
	}{
		{"", theme, func(decker.Ctx, *decker.Scene) {}},
		{"View", theme, func(decker.Ctx, *decker.Scene) { boom() }},
		{"a placed element", theme, func(c decker.Ctx, sc *decker.Scene) {
			sc.Place("k", c.Frame(), func(*decker.Pixels, decker.Rect) { boom() })
		}},
		{"Theme.Overlay", &overlay, func(decker.Ctx, *decker.Scene) {}},
	} {
		part, r := drawFrame(decker.Slide{View: tc.view}, decker.Ctx{W: 40, H: 12, Theme: tc.theme})
		if part != tc.part || (tc.part != "") != (r != nil) {
			t.Errorf("%q: got part %q, recovered %v", tc.part, part, r)
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
	if is := d.Review(decker.ReviewOptions{}); len(is) != 0 {
		t.Errorf("issues in a clean deck: %v", is)
	}
	d.Slides[0].View = func(c decker.Ctx, sc *decker.Scene) {
		decker.Text{Font: c.Theme.Display, Size: c.Size(0.15)}.Draw(sc.Px, "Hello, off the edge", c.X(0.8), c.Y(0.2))
	}
	is := d.Review(decker.ReviewOptions{})
	if len(is) != 3 || is[0].Severity != decker.SeverityError || !strings.Contains(is[0].Msg, "past the right edge") {
		t.Errorf("off-edge deck: %v", is)
	}
}
