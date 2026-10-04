package decker

import (
	"strings"
	"testing"
)

func TestSceneLayering(t *testing.T) {
	sc := NewScene(10, 4, testTheme)
	sc.Text(0, 0, "..........", testTheme.Muted.Color())
	sc.Put(2, 0, "AB")                            // opaque
	sc.Overlay(5, 0, "C D")                       // transparent space keeps the dot underneath
	sc.Put(-1, 1, "XYZ")                          // clipped on the left
	sc.Sprite(8, 3, Sprite{Art: []string{"# #"}}) // clipped on the right

	got := frameLines(sc.Render())
	want := []string{"..AB.C.D..", "YZ", "", "        #"}
	if len(got) != 4 {
		t.Fatalf("got %d lines, want 4: %q", len(got), got)
	}
	for i := range want {
		if strings.TrimRight(got[i], " ") != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTransitionsKeepSize(t *testing.T) {
	a := strings.Repeat(strings.Repeat("a", 20)+"\n", 4) + strings.Repeat("a", 20)
	b := strings.Repeat(strings.Repeat("b", 20)+"\n", 4) + strings.Repeat("b", 20)
	for k := range transitions {
		for _, p := range []float64{0, 0.5, 1} {
			for _, fwd := range []bool{true, false} {
				out := frameLines(composeTransition(k, a, b, 20, 5, p, fwd, testTheme))
				if len(out) != 5 {
					t.Errorf("transition %d p=%v: %d lines", k, p, len(out))
				}
			}
		}
		if got := frameLines(composeTransition(k, a, b, 20, 5, 1, true, testTheme)); got[0] != strings.Repeat("b", 20) {
			t.Errorf("transition %d at p=1 should be fully new, got %q", k, got[0])
		}
	}
}

// A slide may render off-screen scenes to strings and Put them on its own.
func TestOffscreenSceneRenders(t *testing.T) {
	s := Slide{Title: "x", View: func(c Ctx, sc *Scene) {
		off := NewScene(c.W, c.H, c.Theme)
		off.Text(0, 0, "hi", c.Theme.Text.Color())
		sc.Put(0, 0, off.Render())
	}}
	g := renderSlideGrid(s, Ctx{W: 10, H: 2, Theme: testTheme})
	defer g.release()
	if g.at(0, 0).ch != "h" || g.at(1, 0).ch != "i" {
		t.Fatalf("off-screen scene was lost: %q %q", g.at(0, 0).ch, g.at(1, 0).ch)
	}
}
