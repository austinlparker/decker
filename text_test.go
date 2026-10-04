package decker

import (
	"strings"
	"testing"
)

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

func TestFitRespectsBox(t *testing.T) {
	size, text := testTheme.Body.Fit("of weekly query volume comes from agents", 100, 60, 40, 0)
	for _, l := range strings.Split(text, "\n") {
		if w := testTheme.Body.Measure(l, size); w > 100 {
			t.Errorf("line %q is %.1fpx wide at size %d, want <= 100", l, w, size)
		}
	}
	if h := float64(len(strings.Split(text, "\n"))) * float64(size) * DefaultLeading; h > 60 {
		t.Errorf("text is %.1fpx tall, want <= 60", h)
	}
}

func TestFitWrapsRatherThanOverflows(t *testing.T) {
	// When nothing fits the box, Fit wraps to the width and runs taller.
	size, text := testTheme.Body.Fit("a line that is far too long to fit on one row", 60, 10, 20, 0)
	for _, l := range strings.Split(text, "\n") {
		if w := testTheme.Body.Measure(l, size); w > 60 {
			t.Errorf("line %q is %.0fpx, want <= 60", l, w)
		}
	}
}

func TestTextDrawsInsideCanvas(t *testing.T) {
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
