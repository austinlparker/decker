// Package decktest is the test suite every deck wants: every slide renders at
// every step, size and moment without panicking, and draws the same frame
// every time; every build shows all it means to (nothing clipped, dropped or
// off the canvas); the posts in its notes pass its publisher's checks;
// reference images catch unintended changes to how it looks; and a
// per-slide benchmark measures frame time. A talk's test file is a few
// lines:
//
//	func TestSlides(t *testing.T)      { decktest.Slides(t, talk()) }
//	func TestReview(t *testing.T)      { decktest.Review(t, talk()) }
//	func TestPosts(t *testing.T)       { decktest.Posts(t, talk()) }
//	func TestGolden(t *testing.T)      { decktest.Golden(t, talk(), "testdata/golden") }
//	func BenchmarkFrames(b *testing.B) { decktest.Frames(b, talk()) }
package decktest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/austinlparker/decker"
	"github.com/austinlparker/decker/internal/visual"
)

// Slides checks the deck as Main does (Deck.Check), then renders every
// slide at every step, at three sizes and four moments (appearing,
// mid-entrance, entered, settled), as the deck draws them (Deck.Draw), and
// fails on the first panic: in a View, a placed element or the theme's
// overlay. Then it draws each build twice, with another slide between, and
// fails if the two differ: a frame must depend only on its Ctx.
func Slides(t *testing.T, d decker.Deck) {
	t.Helper()
	if err := d.Check(); err != nil {
		t.Fatal(err)
	}
	for i := range d.Slides {
		for _, size := range [][2]int{{80, 24}, {120, 36}, {200, 50}} {
			for step := 0; step < d.Steps(i); step++ {
				for _, at := range []float64{0, 0.3, 1.5, decker.Settled} {
					if err := d.Draw(i, decker.Ctx{W: size[0], H: size[1], T: at, Step: step, StepT: at}); err != nil {
						t.Fatalf("%v (at %dx%d, build %d, t=%v)", err, size[0], size[1], step+1, at)
					}
				}
			}
		}
	}
	// A frame depends only on its Ctx, so drawing it again, after another
	// slide, must give the same frame.
	for i := range d.Slides {
		other := (i + 1) % len(d.Slides)
		for step := range d.Steps(i) {
			c := decker.Ctx{W: 120, H: 36, T: 0.3, Step: step, StepT: 0.3}
			first := d.Render(i, c)
			d.Render(other, decker.Ctx{W: 120, H: 36, T: 1.5, Step: d.Steps(other) - 1, StepT: 1.5})
			if d.Render(i, c) != first {
				t.Errorf("slide %d %q, build %d: drawn again, it changed; a View must depend only on its Ctx "+
					"(no clock, no math/rand, no state kept between frames)", i+1, d.Slides[i].Title, step+1)
			}
		}
	}
}

// Review fails t for every error a review of the deck finds: a build that
// loses content at 240×67, 320×90 or 682×171 (Code lines clipped, Table rows
// dropped, text off the canvas, a slide's own Ctx.Fits failing) or panics.
// Warnings and notes, such as text below the readable size, are logged. A
// slide that means to have an issue lists its code in Slide.Allow.
func Review(t *testing.T, d decker.Deck) {
	t.Helper()
	for _, is := range d.Review() {
		if is.Severity == decker.SeverityError {
			t.Error(is)
		} else {
			t.Log(is)
		}
	}
}

// Posts renders every post the deck's notes make (Deck.Posts) and has the
// deck's Publisher check them (Publisher.Check), as a rehearsal does, so
// a post its service would turn away fails the test instead of the talk.
// Nothing signs in or posts, so a check that needs the account, like how
// many clips it may still upload today, waits for the talk. It
// returns the posts, their media in a directory the test removes, for a
// talk's own checks. Posts skips if a post is a clip and ffmpeg isn't
// installed.
func Posts(t *testing.T, d decker.Deck) []decker.Post {
	t.Helper()
	posts, err := d.Posts(t.TempDir())
	if errors.Is(err, exec.ErrNotFound) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if d.Publisher != nil {
		if err := d.Publisher.Check(t.Context(), posts); err != nil {
			t.Fatal(err)
		}
	}
	return posts
}

// Golden compares how each slide looks with the reference images in dir:
// one PNG per slide, every build at three moments (appearing, entered and
// settled) drawn at 682×171, and settled at 240×67 and 80×24, where layout
// is tightest, each halved; and a text file of any characters slides place
// in the character layer, settled at each size. Images match within a tolerance,
// since the same source renders slightly different bits on different
// machines (see the guide); text matches exactly. Run with UPDATE_GOLDEN=1
// to record after an intentional change: only the images that changed are
// rewritten. On a mismatch it writes what it drew and a diff image to a
// directory it names. Skipped with -short.
func Golden(t *testing.T, d decker.Deck, dir string) {
	t.Helper()
	if testing.Short() {
		t.Skip("golden frames take a few seconds; skipped with -short")
	}
	moments := []float64{0.25, 0.7, decker.Settled}
	type size struct{ w, h int }
	small := []size{{240, 67}, {80, 24}}
	var text visual.Text
	kept := map[string]bool{}
	for i, s := range d.Slides {
		var tiles []visual.Tile
		add := func(name string, c decker.Ctx) {
			px := d.Canvas(i, c)
			img := visual.Shrink(px.W, px.H, func(x, y int) (r, g, b float32) {
				p := px.Pix[y*px.W+x]
				return p.R, p.G, p.B
			})
			tiles = append(tiles, visual.Tile{Name: name, Img: img})
		}
		for step := range d.Steps(i) {
			for _, at := range moments {
				c := decker.Ctx{W: 682, H: 171, T: at, Step: step, StepT: at}
				if step > 0 { // a build appearing on a slide that has settled
					c.T = decker.Settled
				}
				add(fmt.Sprintf("build %d at %v", step+1, at), c)
			}
			for _, sz := range small {
				add(fmt.Sprintf("build %d at %d×%d", step+1, sz.w, sz.h),
					decker.Ctx{W: sz.w, H: sz.h, T: decker.Settled, Step: step, StepT: decker.Settled})
			}
			for _, sz := range append([]size{{682, 171}}, small...) {
				frame := d.Render(i, decker.Ctx{W: sz.w, H: sz.h, T: decker.Settled, Step: step, StepT: decker.Settled})
				if chars := characters(frame); chars != "" {
					text.Add(fmt.Sprintf("%d.%d %s, %d×%d", i+1, step+1, s.Title, sz.w, sz.h), chars)
				}
			}
		}
		name := fmt.Sprintf("%02d-%s.png", i+1, fileName(s.Title))
		kept[name] = true
		visual.CheckSheet(t, filepath.Join(dir, name), tiles)
	}
	textPath := filepath.Join(dir, "characters.txt")
	if _, err := os.Stat(textPath); err == nil || !text.Empty() {
		kept["characters.txt"] = true
		text.Check(t, textPath)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if kept[e.Name()] || e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if visual.Updating() {
			if err := os.Remove(path); err != nil {
				t.Error(err)
			}
			continue
		}
		t.Errorf("%s: no slide draws it any more (remove it, or record with UPDATE_GOLDEN=1)", path)
	}
}

// characters is a rendered frame's character layer: what's left once the
// cells that only show pixels (spaces and half blocks) are blank, without
// the blank lines after the last that has any, or "".
func characters(frame string) string {
	var lines []string
	for line := range strings.Lines(ansi.Strip(frame)) {
		lines = append(lines, strings.Map(func(r rune) rune {
			if r == '▀' {
				return ' '
			}
			return r
		}, strings.TrimSuffix(line, "\n")))
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// fileName is s with what a file name can't hold, or shouldn't, replaced.
func fileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '_'
	}, s)
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
					if err := d.Draw(i, c); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
