// Package decktest is the test suite every deck wants: every slide renders at
// every step, size and moment without panicking; every build shows all it
// means to (nothing clipped, dropped or off the canvas); golden hashes catch
// unintended changes; and a per-slide benchmark measures frame time. A
// talk's test file is a few lines:
//
//	func TestSlides(t *testing.T)      { decktest.Slides(t, talk()) }
//	func TestReview(t *testing.T)      { decktest.Review(t, talk()) }
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

	"github.com/austinlparker/decker"
)

// Slides renders every slide at every step, at three sizes and four moments
// (appearing, mid-entrance, entered, settled), failing on panics.
func Slides(t *testing.T, d decker.Deck) {
	t.Helper()
	for i, s := range d.Slides {
		for _, size := range [][2]int{{80, 24}, {120, 36}, {200, 50}} {
			for step := 0; step < d.Steps(i); step++ {
				for _, at := range []float64{0, 0.3, 1.5, decker.Settled} {
					view(t, s, decker.Ctx{W: size[0], H: size[1], T: at, Step: step, StepT: at, Theme: d.Theme,
						Index: i, Count: len(d.Slides), Section: d.Section(i)})
				}
			}
		}
	}
}

// view draws one frame straight from the slide, as the engine does (View,
// then the elements it placed, then the theme's overlay), so a panic in any
// of them fails the test instead of being drawn.
func view(t *testing.T, s decker.Slide, c decker.Ctx) {
	t.Helper()
	if part, r := drawFrame(s, c); r != nil {
		t.Fatalf("slide %q: %s panicked at %dx%d step %d t=%v: %v", s.Title, part, c.W, c.H, c.Step, c.T, r)
	}
}

// drawFrame draws one frame and returns what panicked, if anything: "View",
// "a placed element" or "Theme.Overlay". The scene is off-screen, so a panic
// reaches here rather than being drawn.
func drawFrame(s decker.Slide, c decker.Ctx) (part string, r any) {
	sc := decker.NewScene(c.W, c.H, c.Theme)
	defer func() {
		if r = recover(); r != nil {
			sc.Release()
		}
	}()
	part = "View"
	if s.View != nil {
		s.View(c, sc)
	}
	part = "Theme.Overlay"
	if c.Theme.Overlay != nil {
		c.Theme.Overlay(c, sc.Px)
	}
	part = "a placed element"
	sc.Render()
	return "", nil
}

// Review fails t for every error a review of the deck finds: a build that
// loses content at 240×67, 320×90 or 682×171 (Code lines clipped, Table rows
// dropped, text off the canvas, a slide's own Ctx.Fits failing) or panics.
// Warnings and notes, such as text below the readable size, are logged. A
// slide that means to have an issue lists its code in Slide.Allow.
func Review(t *testing.T, d decker.Deck) {
	t.Helper()
	for _, is := range d.Review(decker.ReviewOptions{}) {
		if is.Severity == decker.SeverityError {
			t.Error(is)
		} else {
			t.Log(is)
		}
	}
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
				c := decker.Ctx{W: size[0], H: size[1], T: 1.3, Step: d.Steps(i) - 1, StepT: 1.3,
					Index: i, Count: len(d.Slides), Section: d.Section(i)}
				for b.Loop() {
					c.T += 1.0 / 60
					c.StepT += 1.0 / 60
					d.Draw(i, c)
				}
			})
		}
	}
}
