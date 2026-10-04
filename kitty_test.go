package decker

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// Bubble Tea lays the screen out by width, so each placeholder (U+10EEEE
// plus two combining marks) must count as exactly one cell, and survive
// being parsed into cells with its marks and color intact.
func TestKittyPlaceholdersAreOneCellEach(t *testing.T) {
	const id, cols, rows = 7, 40, 12
	s := kittyPlaceholders(id, cols, rows)
	lines := strings.Split(s, "\n")
	if len(lines) != rows {
		t.Fatalf("%d rows, want %d", len(lines), rows)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != cols {
			t.Fatalf("row %d is %d cells wide, want %d", i, w, cols)
		}
	}
	cv := lipgloss.NewCanvas(cols, rows)
	uv.NewStyledString(s).Draw(cv, cv.Bounds())
	cell := cv.CellAt(5, 3)
	want := string([]rune{kittyPlaceholder, kittyDiacritics[3], kittyDiacritics[5]})
	if cell == nil || cell.Content != want {
		t.Fatalf("cell (5,3) = %+v, want row 3 column 5 placeholder", cell)
	}
	if got := toRGB(cell.Style.Fg); got != toRGB(lipgloss.ANSIColor(id)) {
		t.Fatalf("cell color %v doesn't carry image id %d", cell.Style.Fg, id)
	}
}

func TestKittyTransmitChunks(t *testing.T) {
	img := slideImage(testDeck().Slides[0], 0, 120, 36, testTheme)
	seq, err := kittyTransmit(9, img, 30, 8)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSuffix(seq, "\x1b\\"), "\x1b\\")
	if len(parts) < 2 {
		t.Fatalf("expected several chunks, got %d", len(parts))
	}
	if !strings.HasPrefix(parts[0], "\x1b_Ga=T,U=1,f=100,i=9,c=30,r=8,q=2,m=1;") {
		t.Fatalf("first chunk header: %q", parts[0][:60])
	}
	for i, p := range parts[1:] {
		want := "\x1b_Gm=1;"
		if i == len(parts)-2 {
			want = "\x1b_Gm=0;"
		}
		if !strings.HasPrefix(p, want) {
			t.Fatalf("chunk %d header: %q", i+1, p[:10])
		}
		if n := len(p) - len(want); n > 4096 {
			t.Fatalf("chunk %d carries %d bytes", i+1, n)
		}
	}
}

func TestKittyImageIDsWrapAndFree(t *testing.T) {
	ki := newKittyImages()
	var first int
	for i := 0; i < 255; i++ {
		id, free := ki.add(previewKey{slide: i})
		if i == 0 {
			first = id
		}
		if id < 1 || id > 255 || free != "" {
			t.Fatalf("add %d: id %d, free %q", i, id, free)
		}
	}
	// The 256th preview reuses the oldest id and frees its image.
	id, free := ki.add(previewKey{slide: 999})
	if id != first || free != kittyDelete(first) {
		t.Fatalf("reuse: id %d free %q", id, free)
	}
	if _, ok := ki.id(previewKey{slide: 0}); ok {
		t.Fatal("the replaced preview still has an id")
	}
	if got := strings.Count(ki.clear(), "a=d"); got != 255 {
		t.Fatalf("clear freed %d images, want 255", got)
	}
}

// A preview image is drawn in the background when the slide changes; it
// should be ready well before you've finished glancing down.
func TestSlideImageIsQuick(t *testing.T) {
	slides := testDeck().Slides
	slideImage(slides[0], 0, 682, 171, testTheme) // warm the font caches
	var worst time.Duration
	for i := range slides {
		start := time.Now()
		img := slideImage(slides[i], slides[i].steps()-1, 682, 171, testTheme)
		if _, err := kittyTransmit(1, img, 60, 15); err != nil {
			t.Fatal(err)
		}
		worst = max(worst, time.Since(start))
	}
	t.Logf("slowest preview image: %v", worst)
	if worst > 500*time.Millisecond {
		t.Fatalf("slowest preview image took %v", worst)
	}
}
