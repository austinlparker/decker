package decker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func frameLines(s string) []string { return strings.Split(ansi.Strip(s), "\n") }

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

// Every slide should render at every step, at several sizes and times,
// without panicking and at exactly the requested size.
func TestAllSlidesRender(t *testing.T) {
	for i, s := range testDeck().Slides {
		for _, size := range [][2]int{{80, 24}, {120, 36}, {200, 50}} {
			for step := 0; step < s.steps(); step++ {
				for _, at := range []float64{0, 0.3, 1.5, Settled} {
					c := Ctx{W: size[0], H: size[1], T: at, Step: step, StepT: at, Theme: testTheme}
					out := renderSlideStrict(t, s, c)
					if n := len(strings.Split(out, "\n")); n != c.H {
						t.Errorf("slide %d %q: %d lines at %dx%d, want %d", i+1, s.Title, n, c.W, c.H, c.H)
					}
				}
			}
		}
	}
}

func renderSlideStrict(t *testing.T, s Slide, c Ctx) string {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("slide %q panicked at %dx%d step %d t=%v: %v", s.Title, c.W, c.H, c.Step, c.T, r)
		}
	}()
	return fit(s.View(c), c.W, c.H)
}

func TestTransitionsKeepSize(t *testing.T) {
	a := strings.Repeat(strings.Repeat("a", 20)+"\n", 4) + strings.Repeat("a", 20)
	b := strings.Repeat(strings.Repeat("b", 20)+"\n", 4) + strings.Repeat("b", 20)
	for _, k := range []Transition{TransitionPush, TransitionDissolve, TransitionWipe} {
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
