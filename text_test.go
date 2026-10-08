package decker

import (
	"slices"
	"strings"
	"testing"
)

func assertFits(t *testing.T, f *Font, text string, size int, maxW float64) {
	t.Helper()
	for _, l := range strings.Split(text, "\n") {
		if w := f.Measure(l, size); w > maxW {
			t.Errorf("line %q is %.1fpx wide at size %d, want <= %.1f", l, w, size, maxW)
		}
	}
}

func TestWrapIsBalanced(t *testing.T) {
	const s = "What Your MCP Server Does"
	size := 20
	full := testTheme.Display.Measure(s, size)
	lines := testTheme.Display.Wrap(s, size, full*0.85) // too narrow for one line
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), lines)
	}
	for _, l := range lines {
		if len(strings.Fields(l)) < 2 {
			t.Errorf("unbalanced wrap left a lonely word: %q", lines)
		}
	}
}

func TestWrapKeepsBreaksAndReturnsACopy(t *testing.T) {
	f := testTheme.Body
	lines := f.Wrap("one\ntwo three", 20, 1000)
	if want := []string{"one", "two three"}; !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
	lines[0] = "scribble"
	if again := f.Wrap("one\ntwo three", 20, 1000); again[0] != "one" {
		t.Errorf("Wrap handed out its cached slice: %q", again)
	}
}

func TestFitRespectsBox(t *testing.T) {
	size, text := testTheme.Body.Fit("of weekly query volume comes from agents", 100, 60, 40, 0)
	assertFits(t, testTheme.Body, text, size, 100)
	if h := float64(len(strings.Split(text, "\n"))) * float64(size) * DefaultLeading; h > 60 {
		t.Errorf("text is %.1fpx tall, want <= 60", h)
	}
}

func TestFitWrapsRatherThanOverflows(t *testing.T) {
	// When nothing fits the box, Fit wraps to the width and runs taller.
	size, text := testTheme.Body.Fit("a line that is far too long to fit on one row", 60, 10, 20, 0)
	assertFits(t, testTheme.Body, text, size, 60)
}

func TestFitAllFitsTogetherAndReturnsACopy(t *testing.T) {
	parts := []string{"first part of it", "second part"}
	size, lines := FitAll(testTheme.Body, parts, 100, 80, 40)
	if size > 40 || len(lines) < len(parts) {
		t.Fatalf("size %d, lines %q", size, lines)
	}
	assertFits(t, testTheme.Body, strings.Join(lines, "\n"), size, 100)
	if h := float64(len(lines)) * float64(size) * DefaultLeading; h > 80 {
		t.Errorf("lines are %.1fpx tall, want <= 80", h)
	}
	lines[0] = "scribble"
	if _, again := FitAll(testTheme.Body, parts, 100, 80, 40); again[0] == "scribble" {
		t.Error("FitAll handed out its cached slice")
	}
}

func TestTextMeasureMatchesDraw(t *testing.T) {
	p := NewPixels(120, 80, testTheme.Background)
	texts := []string{
		"", "Hi", "two\nlines", "\n\n", "trailing \n", "界 é",
		"   spaced   out  ", "a long sentence that wraps across several lines of a box",
		"What Your MCP Server Does", "overlongwordthatcannotbreak and more\nafter a break",
	}
	for _, font := range []*Font{testTheme.Display, testTheme.Body, testTheme.Mono} {
		for _, size := range []int{0, 3, 9, 13, 20, 48} {
			for _, maxW := range []float64{0, -5, 30, 80, 300} {
				for _, leading := range []float64{0, 1, 1.4} {
					for i, s := range texts {
						tx := Text{Font: font, Size: size, MaxW: maxW, Leading: leading, Align: Align(i % 3)}
						dw, dh := tx.Draw(p, s, 10, 5)
						if mw, mh := tx.Measure(s); mw != dw || mh != dh {
							t.Errorf("size %d maxW %v leading %v %q: Measure = %v×%v, Draw = %v×%v", size, maxW, leading, s, mw, mh, dw, dh)
						}
					}
				}
			}
		}
	}
	// Font.Small takes over below SmallBelow, in Measure as in Draw.
	small := StockFont("SpaceGrotesk-Medium")
	small.Small, small.SmallBelow = testTheme.Mono, 14
	for _, size := range []int{10, 13, 14} {
		tx := Text{Font: small, Size: size, MaxW: 50}
		dw, dh := tx.Draw(p, "small type wraps", 0, 0)
		if mw, mh := tx.Measure("small type wraps"); mw != dw || mh != dh {
			t.Errorf("size %d with Small: Measure = %v×%v, Draw = %v×%v", size, mw, mh, dw, dh)
		}
	}
}

func TestTextMeasureAllocatesNothing(t *testing.T) {
	for _, maxW := range []float64{0, 60} {
		tx := Text{Font: testTheme.Body, Size: 14, MaxW: maxW}
		s := "a few words to wrap\nand a second paragraph"
		tx.Measure(s) // fills the glyph and wrap caches
		if n := testing.AllocsPerRun(50, func() { tx.Measure(s) }); n != 0 {
			t.Errorf("Measure with MaxW %v allocated %v times", maxW, n)
		}
	}
}

func TestTextDrawsPixels(t *testing.T) {
	p := NewPixels(80, 40, testTheme.Background)
	Text{Font: testTheme.Display, Size: 16, Color: testTheme.Text}.Draw(p, "Hi", 10, 10)
	lit := 0
	for _, px := range p.Pix {
		if px != testTheme.Background {
			lit++
		}
	}
	if lit == 0 {
		t.Fatal("drawing text changed no pixels")
	}
}
