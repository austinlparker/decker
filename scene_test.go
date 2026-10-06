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

// TestTransitionsMoveChars checks the character layer goes with its frame:
// each output cell shows the old or the new frame's character, or none (in a
// wipe's band), never both, and the new frame's alone at the end. A morph
// switches all of them half-way.
func TestTransitionsMoveChars(t *testing.T) {
	const w, h = 20, 5
	text := func(ch string) *Scene {
		sc := NewScene(w, h, testTheme)
		sc.Put(0, 0, strings.TrimSuffix(strings.Repeat(strings.Repeat(ch, w)+"\n", h), "\n"))
		return sc
	}
	for kind := range transitions {
		k := Transition{kind: kind}
		for _, p := range []float64{0, 0.5, 1} {
			for _, fwd := range []bool{true, false} {
				a, b := text("a"), text("b")
				mixTransition(k, a, b, p, fwd, testTheme)
				out := frameLines(b.Render())
				a.Release()
				if len(out) != h {
					t.Fatalf("transition %v p=%v: %d lines", k, p, len(out))
				}
				if n := strings.Count(strings.Join(out, ""), "a") + strings.Count(strings.Join(out, ""), "b"); n > w*h {
					t.Errorf("transition %v p=%v: %d characters in %d cells", k, p, n, w*h)
				}
				if p == 1 && out[0] != strings.Repeat("b", w) {
					t.Errorf("transition %v at p=1 should be fully new, got %q", k, out[0])
				}
				if p == 0.5 && k != TransitionWipe && k != TransitionMorph && !strings.Contains(out[0], "a") {
					t.Errorf("transition %v at p=0.5 lost the old frame's characters: %q", k, out[0])
				}
			}
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
