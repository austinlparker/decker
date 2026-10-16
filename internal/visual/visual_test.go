package visual

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func flat(w, h int, v uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = v
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

func TestCompare(t *testing.T) {
	base := flat(100, 100, 100)
	for _, tc := range []struct {
		name  string
		edit  func(*image.RGBA)
		fails string
	}{
		{"identical", func(*image.RGBA) {}, ""},
		{"a few pixels a little off, as fused arithmetic leaves them", func(img *image.RGBA) {
			for i := range 40 { // 0.4% of the pixels, each by up to MaxDelta
				img.Pix[img.PixOffset(i, i)] += MaxDelta
			}
		}, ""},
		{"one pixel moved past MaxDelta", func(img *image.RGBA) { img.Pix[img.PixOffset(5, 5)] += MaxDelta + 1 }, "a pixel moved by 41"},
		{"a faint change over a broad area", func(img *image.RGBA) {
			for y := range 10 {
				for x := range 10 {
					img.Pix[img.PixOffset(x, y)+2] += SmallDelta + 1
				}
			}
		}, "1.00% of pixels (100) moved by more than 2"},
	} {
		got := flat(100, 100, 100)
		tc.edit(got)
		problem, _ := Compare(got, base)
		if tc.fails == "" && problem != "" || tc.fails != "" && !strings.Contains(problem, tc.fails) {
			t.Errorf("%s: %q, want %q", tc.name, problem, tc.fails)
		}
	}
	// In a frame as small as an 80×24 terminal's, halved, a few pixels
	// moved are noise; a few more than MinSmall are a change.
	for n, fails := range map[int]bool{5: false, MinSmall + 1: true} {
		got := flat(40, 24, 100)
		for i := range n {
			got.Pix[got.PixOffset(i, 0)] += SmallDelta + 1
		}
		if problem, _ := Compare(got, flat(40, 24, 100)); (problem != "") != fails {
			t.Errorf("%d of 960 pixels moved: %q", n, problem)
		}
	}
	if problem, _ := Compare(flat(10, 10, 0), flat(10, 11, 0)); !strings.Contains(problem, "size") {
		t.Errorf("different sizes: %q", problem)
	}
}

func TestShrinkAverages(t *testing.T) {
	// A checkerboard of 0 and 255 shrinks to mid grey, rounded once.
	img := Shrink(4, 2, func(x, y int) (r, g, b float32) {
		v := float32(255 * ((x + y) % 2))
		return v, v, v
	})
	if img.Rect.Dx() != 2 || img.Rect.Dy() != 1 || img.Pix[0] != 128 || img.Pix[4] != 128 || img.Pix[3] != 255 {
		t.Errorf("%v %v", img.Rect, img.Pix)
	}
}

// TestRecordAndCheck records an image and some text, checks them, then
// checks what changed after re-recording: only what moved is rewritten.
func TestRecordAndCheck(t *testing.T) {
	dir := t.TempDir()
	path, textPath := filepath.Join(dir, "a.png"), filepath.Join(dir, "a.txt")
	var text Text
	text.Add("section", "line one  \nline two")
	t.Setenv("UPDATE_GOLDEN", "1")
	CheckSheet(t, path, []Tile{{"one", flat(10, 10, 50)}, {"two", flat(6, 4, 200)}})
	text.Check(t, textPath)
	if b, _ := os.ReadFile(textPath); string(b) != "=== section ===\nline one\nline two\n" {
		t.Errorf("text file: %q", b)
	}
	st, _ := os.Stat(path)
	// Within the tolerance: not rewritten.
	nudged := flat(10, 10, 50)
	nudged.Pix[0]++
	CheckSheet(t, path, []Tile{{"one", nudged}, {"two", flat(6, 4, 200)}})
	if st2, _ := os.Stat(path); !st2.ModTime().Equal(st.ModTime()) {
		t.Error("a sheet within the tolerance was rewritten")
	}
	t.Setenv("UPDATE_GOLDEN", "")
	CheckSheet(t, path, []Tile{{"one", nudged}, {"two", flat(6, 4, 200)}})
	text.Check(t, textPath)
}

// errors is a testing.TB that keeps what's reported as an error.
type errors struct {
	testing.TB
	got []string
}

func (e *errors) Helper() {}
func (e *errors) Errorf(format string, args ...any) {
	e.got = append(e.got, fmt.Sprintf(format, args...))
}

// TestSheetHoldsEachTile changes one tile of three faintly over 1% of its
// pixels: twice the share a tile may have, but a third of that across the
// sheet. The check fails it, names it, and recording rewrites it.
func TestSheetHoldsEachTile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sheet.png")
	tiles := func(faint bool) []Tile {
		one := flat(100, 100, 100)
		if faint {
			for i := range 100 {
				one.Pix[one.PixOffset(i%10, i/10)] += SmallDelta + 1
			}
		}
		return []Tile{{"one", one}, {"two", flat(100, 100, 100)}, {"three", flat(100, 100, 100)}}
	}
	t.Setenv("UPDATE_GOLDEN", "1")
	CheckSheet(t, path, tiles(false))
	t.Setenv("UPDATE_GOLDEN", "")
	if problem, _ := Compare(Sheet(tiles(true)), Sheet(tiles(false))); problem != "" {
		t.Fatalf("the sheet as a whole should pass, to test anything: %s", problem)
	}
	e := &errors{TB: t}
	CheckSheet(e, path, tiles(true))
	if len(e.got) != 1 || !strings.Contains(e.got[0], "one: 1.00% of pixels") || strings.Contains(e.got[0], "two") {
		t.Errorf("reported %q", e.got)
	}
	t.Setenv("UPDATE_GOLDEN", "1")
	CheckSheet(t, path, tiles(true))
	t.Setenv("UPDATE_GOLDEN", "")
	CheckSheet(t, path, tiles(true))
}
