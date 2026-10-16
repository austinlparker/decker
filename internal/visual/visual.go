// Package visual checks rendered frames against reference images kept in
// the repository, and text against reference files.
//
// Frames are compared as images, shrunk by Factor, within a tolerance. The
// same source doesn't render the same bits on every machine: Go may fuse a
// multiply and an add into one instruction (arm64 does, amd64 with
// GOAMD64=v3 does), which rounds once instead of twice, and that moves the
// odd pixel, mostly along antialiased edges. Shrinking averages that out; a
// change anyone could see doesn't average away. The tolerance was set by
// rendering every frame of the gallery with and without fused arithmetic,
// and by planting regressions (a letter changed in small text, an element
// moved half a pixel, a bar one unit taller, a 3px dot gone, a color moved
// by 8): the first stays well inside it, every one of the second fails it.
//
// Run with UPDATE_GOLDEN=1 to record: a reference is written when it's
// missing or no longer matches, so re-recording on another machine doesn't
// churn every file.
package visual

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Factor is how much a frame shrinks, each way, before it's compared or kept.
const Factor = 2

const (
	// MaxDelta is the most any channel of a shrunk pixel may move. Fused
	// arithmetic moved one by 24 at most; the smallest planted regression,
	// an element moved half a pixel, moved one by 63.
	MaxDelta = 40
	// SmallDelta and MaxSmallShare bound broad, faint changes: at most
	// MaxSmallShare of the pixels may move by more than SmallDelta. Fused
	// arithmetic moved 0.14% of a frame's pixels that much; a color moved by
	// 3 moves every pixel it covers.
	SmallDelta    = 2
	MaxSmallShare = 0.005
)

// Updating reports whether the test run records references instead of
// checking them.
func Updating() bool { return os.Getenv("UPDATE_GOLDEN") != "" }

// Shrink averages a w×h frame down by Factor each way, rounding once at
// the end; at gives the color of pixel (x, y), 0..255 a channel.
func Shrink(w, h int, at func(x, y int) (r, g, b float32)) *image.RGBA {
	sw, sh := w/Factor, h/Factor
	img := image.NewRGBA(image.Rect(0, 0, sw, sh))
	const n = Factor * Factor
	q := func(v float64) uint8 { return uint8(math.Round(min(max(v/n, 0), 255))) }
	for y := range sh {
		for x := range sw {
			var r, g, b float64
			for dy := range Factor {
				for dx := range Factor {
					pr, pg, pb := at(x*Factor+dx, y*Factor+dy)
					r, g, b = r+float64(pr), g+float64(pg), b+float64(pb)
				}
			}
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = q(r), q(g), q(b), 255
		}
	}
	return img
}

// Compare reports how got differs from want beyond the tolerance, or ""
// if it doesn't. diff marks each pixel that moved by more than SmallDelta,
// brighter the more it moved, over a dimmed copy of want.
func Compare(got, want *image.RGBA) (problem string, diff *image.RGBA) {
	if got.Rect.Size() != want.Rect.Size() {
		return fmt.Sprintf("size %v, want %v", got.Rect.Size(), want.Rect.Size()), nil
	}
	b := want.Rect
	diff = image.NewRGBA(b)
	worst, worstAt, small := 0, image.Point{}, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := want.PixOffset(x, y)
			d := 0
			for c := range 3 {
				d = max(d, absDiff(got.Pix[i+c], want.Pix[i+c]))
			}
			if d > worst {
				worst, worstAt = d, image.Pt(x, y)
			}
			dim := func(v uint8) uint8 { return v / 4 }
			px := color.RGBA{dim(want.Pix[i]), dim(want.Pix[i+1]), dim(want.Pix[i+2]), 255}
			if d > SmallDelta {
				small++
				px = color.RGBA{255, uint8(255 - min(d*4, 255)), 0, 255}
			}
			diff.SetRGBA(x, y, px)
		}
	}
	share := float64(small) / float64(b.Dx()*b.Dy())
	switch {
	case worst > MaxDelta:
		return fmt.Sprintf("a pixel moved by %d (at %d,%d in the shrunk image), more than %d", worst, worstAt.X, worstAt.Y, MaxDelta), diff
	case share > MaxSmallShare:
		return fmt.Sprintf("%.2f%% of pixels moved by more than %d, more than %.1f%%", 100*share, SmallDelta, 100*MaxSmallShare), diff
	}
	return "", diff
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// CheckImage compares img with the PNG at path, or records it there. On a
// mismatch it writes what it got and a diff image to a directory it names,
// for a look.
func CheckImage(t testing.TB, path string, img *image.RGBA) {
	t.Helper()
	want, err := readPNG(path)
	var problem string
	var diff *image.RGBA
	if err == nil {
		problem, diff = Compare(img, want)
	}
	settle(t, path, img, err, problem, diff)
}

// settle records got at path if the run is recording and the reference
// is missing (readErr) or doesn't match (problem), or else reports either.
func settle(t testing.TB, path string, got *image.RGBA, readErr error, problem string, diff *image.RGBA) {
	t.Helper()
	switch {
	case readErr == nil && problem == "":
	case Updating():
		if err := writePNG(path, got); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %s", path)
	case readErr != nil:
		t.Errorf("%v (record it with UPDATE_GOLDEN=1)", readErr)
	default:
		t.Errorf("%s:\n\t%s%s", path, problem, saveFailure(path, got, diff))
	}
}

// saveFailure keeps a failed image and its diff where a person can look at
// them, and says where.
func saveFailure(path string, got, diff *image.RGBA) string {
	dir := filepath.Join(os.TempDir(), "decker-visual")
	name := strings.NewReplacer(string(filepath.Separator), "_", "/", "_").Replace(strings.TrimSuffix(path, ".png"))
	gotPath, diffPath := filepath.Join(dir, name+".got.png"), filepath.Join(dir, name+".diff.png")
	if writePNG(gotPath, got) != nil {
		return ""
	}
	if diff == nil || writePNG(diffPath, diff) != nil {
		return "; got: " + gotPath
	}
	return "; got: " + gotPath + ", diff: " + diffPath
}

func readPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x-b.Min.X, y-b.Min.Y, img.At(x, y))
		}
	}
	return rgba, nil
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Tile is one frame of a sheet, named for the report.
type Tile struct {
	Name string
	Img  *image.RGBA
}

// sheetGap is the strip of background between a sheet's tiles.
const sheetGap = 2

// Sheet lays tiles out in a column, each below the last on a strip of
// background, so a group of small frames is one file to keep and to look
// at. Tiles may differ in size.
func Sheet(tiles []Tile) *image.RGBA {
	w, h := 0, 0
	for _, t := range tiles {
		w = max(w, t.Img.Rect.Dx())
		h += t.Img.Rect.Dy() + sheetGap
	}
	sheet := image.NewRGBA(image.Rect(0, 0, w, h))
	y := 0
	for _, t := range tiles {
		for row := range t.Img.Rect.Dy() {
			copy(sheet.Pix[sheet.PixOffset(0, y+row):], t.Img.Pix[t.Img.PixOffset(0, row):t.Img.PixOffset(t.Img.Rect.Dx(), row)])
		}
		y += t.Img.Rect.Dy() + sheetGap
	}
	for i := 3; i < len(sheet.Pix); i += 4 {
		sheet.Pix[i] = 255
	}
	return sheet
}

// CheckSheet is CheckImage for a sheet of tiles. Each tile is held to the
// tolerance on its own: a faint change over one frame is as much of a
// change beside others that didn't move. A failure names the tiles that
// moved.
func CheckSheet(t testing.TB, path string, tiles []Tile) {
	t.Helper()
	got := Sheet(tiles)
	want, err := readPNG(path)
	var problem string
	var diff *image.RGBA
	if err == nil {
		problem, diff = compareSheet(got, want, tiles)
	}
	settle(t, path, got, err, problem, diff)
}

// compareSheet is Compare for a sheet laid out from tiles, tile by tile.
func compareSheet(got, want *image.RGBA, tiles []Tile) (problem string, diff *image.RGBA) {
	whole, diff := Compare(got, want)
	if diff == nil {
		return whole, nil // a different size: the tiles don't line up
	}
	var bad []string
	y := 0
	for _, tile := range tiles {
		r := image.Rect(0, y, tile.Img.Rect.Dx(), y+tile.Img.Rect.Dy())
		if p, _ := Compare(sub(got, r), sub(want, r)); p != "" {
			bad = append(bad, tile.Name+": "+p)
		}
		y += tile.Img.Rect.Dy() + sheetGap
	}
	return strings.Join(bad, "\n\t"), diff
}

func sub(img *image.RGBA, r image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := range r.Dy() {
		copy(out.Pix[out.PixOffset(0, y):], img.Pix[img.PixOffset(r.Min.X, r.Min.Y+y):img.PixOffset(r.Max.X, r.Min.Y+y)])
	}
	return out
}

// Text gathers named sections of text for one reference file.
type Text struct {
	b strings.Builder
}

// Add appends a section. Trailing spaces are dropped from each line, so
// the file reads cleanly and diffs well.
func (x *Text) Add(name string, parts ...string) {
	fmt.Fprintf(&x.b, "=== %s ===\n", name)
	for _, p := range parts {
		for line := range strings.Lines(p) {
			x.b.WriteString(strings.TrimRight(line, " \n") + "\n")
		}
	}
}

// Empty reports whether no section has been added.
func (x *Text) Empty() bool { return x.b.Len() == 0 }

// Check compares the sections with the file at path, exactly, or records
// them there, and reports the first section that differs, line by line.
func (x *Text) Check(t testing.TB, path string) {
	t.Helper()
	got := x.b.String()
	if Updating() {
		if b, err := os.ReadFile(path); err == nil && string(b) == got {
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %s", path)
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%v (record it with UPDATE_GOLDEN=1)", err)
		return
	}
	if string(b) == got {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(string(b), "\n")
	section := ""
	for i := range max(len(gl), len(wl)) {
		g, w := at(gl, i), at(wl, i)
		if strings.HasPrefix(w, "=== ") {
			section = w
		}
		if g != w {
			t.Errorf("%s line %d, in %s:\n got  %q\n want %q", path, i+1, section, g, w)
			return
		}
	}
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(end of file)"
}
