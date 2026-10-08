package decker

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestImagePreviewSelection(t *testing.T) {
	for _, name := range []string{"TERM_PROGRAM", "TERM", "KITTY_WINDOW_ID", "TMUX", "ZELLIJ"} {
		t.Setenv(name, "")
	}
	check := func(mode string, want bool) {
		t.Helper()
		got, err := imagePreviews(mode)
		if err != nil || got != want {
			t.Fatalf("imagePreviews(%q) = %v, %v, want %v", mode, got, err, want)
		}
	}
	check("auto", false)
	t.Setenv("TERM_PROGRAM", "ghostty")
	check("auto", true)
	check("cells", false)
	for _, name := range []string{"TMUX", "ZELLIJ"} {
		t.Setenv(name, "active")
		check("auto", false)
		check("image", true)
		t.Setenv(name, "")
	}
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "xterm-kitty")
	check("auto", true)
	if _, err := imagePreviews("typo"); err == nil {
		t.Fatal("invalid preview mode was accepted")
	}
}

func TestKittyPlaceholdersSurviveCellRendering(t *testing.T) {
	const id, cols, rows = 0x123456, 40, 12
	s := kittyPlaceholders(id, cols, rows)
	lines := strings.Split(s, "\n")
	if len(lines) != rows {
		t.Fatalf("%d rows, want %d", len(lines), rows)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != cols {
			t.Fatalf("row %d is %d cells wide, want %d", i, w, cols)
		}
	}
	cv := lipgloss.NewCanvas(cols, rows)
	uv.NewStyledString(s).Draw(cv, cv.Bounds())
	cell := cv.CellAt(5, 3)
	want := string([]rune{kitty.Placeholder, kitty.Diacritic(3), kitty.Diacritic(5)})
	if cell == nil || cell.Content != want || colorQ(cell.Style.Fg, [3]uint8{}) != [3]uint8{0x12, 0x34, 0x56} {
		t.Fatalf("placeholder lost its position or image ID: %+v", cell)
	}
}

func TestKittyTransmitRoundTrip(t *testing.T) {
	slides := testDeck().Slides
	img := slideImage(slides, previewKey{slide: 0, dw: 682, dh: 171}, testTheme)
	seq := kittyTransmit(9, img, 60, 15)
	parts := strings.Split(strings.TrimSuffix(seq, "\x1b\\"), "\x1b\\")
	if len(parts) < 2 {
		t.Fatal("expected a chunked upload")
	}
	if !strings.HasPrefix(parts[0], "\x1b_Ga=T,U=1,f=100,i=9,c=60,r=15,q=2,m=1;") {
		t.Fatal("upload does not create a silent virtual placement")
	}
	var encoded strings.Builder
	for i, part := range parts {
		control, data, ok := strings.Cut(part, ";")
		if !ok || len(data) > kitty.MaxChunkSize {
			t.Fatalf("invalid upload chunk %d", i)
		}
		more := "m=1"
		if i == len(parts)-1 {
			more = "m=0"
		}
		if !strings.HasSuffix(control, more) {
			t.Fatalf("invalid continuation marker in chunk %d", i)
		}
		encoded.WriteString(data)
	}
	data, err := base64.StdEncoding.DecodeString(encoded.String())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != image.Rect(0, 0, 682, 342) {
		t.Fatalf("image size %v is not the full deck canvas", decoded.Bounds())
	}
	for y := range 342 {
		for x := range 682 {
			if decoded.At(x, y) != img.At(x, y) {
				t.Fatalf("image pixel (%d,%d) changed in transmission", x, y)
			}
		}
	}
}

func TestSlideImagePreservesContextAndCharacters(t *testing.T) {
	var got Ctx
	slides := []Slide{{Section: "section"}, {Title: "native", View: func(c Ctx, sc *Scene) {
		got = c
		sc.Put(1, 1, "hello")
	}}}
	k := previewKey{slide: 1, step: 2, dw: 60, dh: 20}
	img := slideImage(slides, k, testTheme)
	if got.W != 60 || got.H != 20 || got.Step != 2 || got.T != Settled || got.StepT != Settled || got.Index != 1 || got.Count != 2 || got.Section != "section" {
		t.Fatalf("preview context = %+v", got)
	}
	if img.Bounds() != image.Rect(0, 0, 60*cellW, 20*cellH) {
		t.Fatal("native text did not use the character rasterizer")
	}
	bg := img.RGBAAt(0, 0)
	ink := false
	for y := cellH; y < 2*cellH; y++ {
		for x := cellW; x < 6*cellW; x++ {
			ink = ink || img.RGBAAt(x, y) != bg
		}
	}
	if !ink {
		t.Fatal("native text disappeared from the image")
	}
}

func TestKittyCacheEvictionAndResize(t *testing.T) {
	ki := newKittyImages()
	for i := range 256 {
		id, free := ki.add(previewKey{slide: i})
		if id != i+1 || (i < 255 && free != "") || (i == 255 && free != kittyDelete(1)) {
			t.Fatalf("image %d: id %d, cleanup %q", i, id, free)
		}
	}
	if len(ki.ids) != 255 || len(ki.byID) != 255 {
		t.Fatal("image cache grew beyond its limit")
	}
	if _, ok := ki.id(previewKey{slide: 0}); ok {
		t.Fatal("evicted image remained cached")
	}
	old := kittyUploadMsg{ki.gen, previewKey{slide: 255}, 256, "upload"}
	if count := strings.Count(ki.clear(), "a=d"); count != 255 {
		t.Fatalf("resize freed %d images, want 255", count)
	}
	id, _ := ki.add(old.key)
	if id == old.id || ki.finish(old) != "" {
		t.Fatal("resize reused an ID or accepted a stale upload")
	}
	failed := kittyUploadMsg{ki.gen, old.key, id, ""}
	ki.finish(failed)
	if _, ok := ki.id(old.key); ok {
		t.Fatal("failed image upload prevented a retry")
	}
}

func TestPresenterImageUploadOrdering(t *testing.T) {
	d := testDeck()
	p := presenterFor(d, 120, 36, true, 0, 0, map[previewKey]string{})
	p.images = newKittyImages()
	k, ok := p.key(0, 0)
	if !ok {
		t.Fatal("preview does not fit")
	}
	id, _ := p.images.add(k)
	upload := kittyUploadMsg{p.images.gen, k, id, "image bytes"}
	next, cmd := p.Update(upload)
	p = next.(presenter)
	if p.images.ready[k] || strings.ContainsRune(p.preview(k), kitty.Placeholder) {
		t.Fatal("placeholders appeared before the terminal received the upload")
	}
	raw := cmd()
	if presenterFilter(p, raw) == nil {
		t.Fatal("valid upload was filtered out")
	}
	next, _ = p.Update(raw)
	p = next.(presenter)
	if !p.images.ready[k] || !strings.ContainsRune(p.preview(k), kitty.Placeholder) {
		t.Fatal("uploaded image did not replace the cell fallback")
	}
	// An upload can be queued, then invalidated by a resize before RawMsg
	// executes. Filtering must happen before the actual terminal write.
	next, _ = p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	p = next.(presenter)
	if presenterFilter(p, raw) != nil || p.images.ready[k] {
		t.Fatal("resize allowed a queued old upload to overwrite the new layout")
	}
	if next, _ := p.Update(tea.RawMsg{Msg: "unrelated"}); next.(presenter).images.ready[k] {
		t.Fatal("unrelated terminal output marked an image ready")
	}
}

func TestPresenterImageViewFits(t *testing.T) {
	d := testDeck()
	for _, size := range [][2]int{{60, 15}, {120, 36}, {200, 50}, {800, 200}} {
		p := presenterFor(d, size[0], size[1], true, 0, 0, map[previewKey]string{})
		p.images = newKittyImages()
		k, ok := p.key(0, 0)
		if ok {
			if k.pw > kittyMaxCells || k.ph > kittyMaxCells {
				t.Fatal("preview exceeds the placeholder protocol's size limit")
			}
			for _, target := range []previewKey{k, {1, 0, k.pw, k.ph, k.dw, k.dh}} {
				p.images.add(target)
				p.images.ready[target] = true
			}
		}
		view := p.View().Content
		if lipgloss.Height(view) != size[1] || lipgloss.Width(view) > size[0] {
			t.Fatalf("image view does not fit %dx%d", size[0], size[1])
		}
	}
}

func TestPresenterUploadCache(t *testing.T) {
	d := testDeck()
	p := presenterFor(d, 120, 36, true, 0, 0, map[previewKey]string{})
	p.images = newKittyImages()
	cmd := p.upload()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("current and next images were not scheduled together")
	}
	for _, cmd := range batch {
		msg := cmd().(kittyUploadMsg)
		if msg.seq == "" || !p.images.valid(msg) {
			t.Fatal("preview upload was not encoded or registered")
		}
	}
	if p.upload() != nil {
		t.Fatal("cached images were scheduled again")
	}
	next, _ := p.Update(presTickMsg(time.Now()))
	if next.(presenter).upload() != nil {
		t.Fatal("timer tick caused a preview upload")
	}
}

func TestPresenterSerializesSlideDrawing(t *testing.T) {
	var active atomic.Int32
	var concurrent atomic.Bool
	view := func(c Ctx, sc *Scene) {
		if active.Add(1) != 1 {
			concurrent.Store(true)
		}
		for range 50 {
			runtime.Gosched()
		}
		active.Add(-1)
	}
	d := testDeck()
	d.Slides = []Slide{{Title: "a", View: view}, {Title: "b", View: view}}
	p := presenterFor(d, 120, 36, true, 0, 0, map[previewKey]string{})
	p.images = newKittyImages()
	batch := p.upload()().(tea.BatchMsg)
	var wg sync.WaitGroup
	for _, cmd := range batch {
		wg.Go(func() { cmd() })
	}
	k, _ := p.key(0, 0)
	p.preview(k)
	wg.Wait()
	if concurrent.Load() {
		t.Fatal("cell fallback and image uploads called a slide View concurrently")
	}
}
