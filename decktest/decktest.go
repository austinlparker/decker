// Package decktest is the test suite every deck wants: every slide renders at
// every step, size and moment without panicking, in exact-size, fully opaque
// frames; golden hashes catch unintended changes; and a per-slide benchmark
// measures frame time. A talk's test file is a few lines:
//
//	func TestSlides(t *testing.T)      { decktest.Slides(t, talk()) }
//	func TestGolden(t *testing.T)      { decktest.Golden(t, talk(), "testdata/golden.txt") }
//	func BenchmarkFrames(b *testing.B) { decktest.Frames(b, talk()) }
package decktest

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/austinlparker/decker"
)

// Slides renders every slide at every step, at three sizes and four moments
// (appearing, mid-entrance, entered, settled), failing on panics, wrong-size
// frames, and cells without a background color (a translucent terminal shows
// through them).
func Slides(t *testing.T, d decker.Deck) {
	t.Helper()
	for i, s := range d.Slides {
		for _, size := range [][2]int{{80, 24}, {120, 36}, {200, 50}} {
			for step := 0; step < d.Steps(i); step++ {
				for _, at := range []float64{0, 0.3, 1.5, decker.Settled} {
					c := decker.Ctx{W: size[0], H: size[1], T: at, Step: step, StepT: at, Theme: d.Theme}
					out := view(t, s, c)
					if n := strings.Count(out, "\n") + 1; n != c.H {
						t.Errorf("slide %d %q: %d lines at %dx%d, want %d", i+1, s.Title, n, c.W, c.H, c.H)
					}
				}
			}
		}
		const w, h = 120, 36
		frame := d.Render(i, decker.Ctx{W: w, H: h, T: 1.5, Step: d.Steps(i) - 1, StepT: 1.5})
		if x, y, ok := unpainted(frame, w, h); ok {
			t.Errorf("slide %d %q: cell (%d,%d) has no background color", i+1, s.Title, x, y)
		}
	}
}

func unpainted(frame string, w, h int) (x, y int, found bool) {
	cv := lipgloss.NewCanvas(w, h)
	uv.NewStyledString(frame).Draw(cv, cv.Bounds())
	for y := range h {
		for x := range w {
			if c := cv.CellAt(x, y); c == nil || (c.Width > 0 && c.Style.Bg == nil) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// view draws one frame straight from the slide so a panic fails the test
// instead of being drawn.
func view(t *testing.T, s decker.Slide, c decker.Ctx) string {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("slide %q panicked at %dx%d step %d t=%v: %v", s.Title, c.W, c.H, c.Step, c.T, r)
		}
	}()
	if s.View == nil {
		return strings.Repeat("\n", c.H-1)
	}
	return s.View(c)
}

// Golden compares every frame with the hashes recorded in path: one per slide
// and build step, covering eight moments at three sizes (up to 682×171). Run
// with UPDATE_GOLDEN=1 to re-record after an intentional change. Skipped with
// -short.
func Golden(t *testing.T, d decker.Deck, path string) {
	t.Helper()
	if testing.Short() {
		t.Skip("golden frames take a few seconds; skipped with -short")
	}
	got := Hashes(d)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		var b strings.Builder
		b.WriteString("# Frame hashes: decktest.Golden. Regenerate with UPDATE_GOLDEN=1 go test.\n")
		for _, h := range got {
			b.WriteString(h.Key + " " + h.Sum + "\n")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d hashes in %s", len(got), path)
		return
	}
	want, err := readHashes(path)
	if err != nil {
		t.Fatalf("%v (record them with UPDATE_GOLDEN=1)", err)
	}
	seen := map[string]bool{}
	for _, h := range got {
		seen[h.Key] = true
		switch w, ok := want[h.Key]; {
		case !ok:
			t.Errorf("%s: new (record it with UPDATE_GOLDEN=1)", h.Key)
		case w != h.Sum:
			t.Errorf("%s: frames changed", h.Key)
		}
	}
	for k := range want {
		if !seen[k] {
			t.Errorf("%s: gone", k)
		}
	}
}

// Hash is the hash of every golden frame of one build step.
type Hash struct{ Key, Sum string }

// Hashes renders the golden frames of every slide and step, keyed by slide
// number and title.
func Hashes(d decker.Deck) []Hash {
	sizes := [][2]int{{80, 24}, {240, 67}, {682, 171}}
	times := []float64{0, 0.25, 0.7, 1.5, 3, 6, 12, decker.Settled}
	var out []Hash
	for i, s := range d.Slides {
		for step := 0; step < d.Steps(i); step++ {
			h := sha256.New()
			for _, at := range times {
				for _, sz := range sizes {
					h.Write([]byte(d.Render(i, decker.Ctx{W: sz[0], H: sz[1], T: at, Step: step, StepT: at})))
				}
			}
			if step > 0 { // a settled slide whose newest step just began
				for _, sz := range sizes {
					h.Write([]byte(d.Render(i, decker.Ctx{W: sz[0], H: sz[1], T: decker.Settled, Step: step, StepT: 0.4})))
				}
			}
			key := fmt.Sprintf("%02d.%d:%s", i+1, step+1, strings.ReplaceAll(s.Title, " ", "_"))
			out = append(out, Hash{key, hex.EncodeToString(h.Sum(nil))})
		}
	}
	return out
}

func readHashes(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("%s: bad line %q", path, line)
		}
		out[k] = v
	}
	return out, sc.Err()
}

// Frames benchmarks one mid-animation frame of each slide at projector-like
// sizes, drawn into cells as the live deck does. At 60 fps a frame has ~16ms
// and the terminal needs time too, so aim for a few ms at the presenting size:
//
//	go test -bench Frames/682x171
func Frames(b *testing.B, d decker.Deck) {
	for _, size := range [][2]int{{240, 67}, {320, 90}, {682, 171}} {
		for i := range d.Slides {
			b.Run(fmt.Sprintf("%dx%d/%d", size[0], size[1], i+1), func(b *testing.B) {
				c := decker.Ctx{W: size[0], H: size[1], T: 1.3, Step: d.Steps(i) - 1, StepT: 1.3}
				for b.Loop() {
					c.T += 1.0 / 60
					c.StepT += 1.0 / 60
					d.Draw(i, c)
				}
			})
		}
	}
}
