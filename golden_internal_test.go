package decker

// Reference checks for the engine paths the gallery (decktest.Golden) can't
// reach, because they're unexported: transitions in cells and video, the
// video frame loop, PNG export, the live model's screen, the terminal
// writer, the presenter view. They live in testdata/internal: images of
// what's drawn, compared within internal/visual's tolerance, and text for
// everything else (the character layer, escape sequences, traces through
// Update), compared exactly. Record with
//
//	UPDATE_GOLDEN=1 go test -run TestInternalGolden
//
// and only when a change in output is intended. Everything here is
// deterministic: fixed times, no clock, no rand, no ffmpeg, no sockets.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/austinlparker/decker/internal/visual"
)

const internalDir = "testdata/internal"

func ref(name string) string { return filepath.Join(internalDir, name) }

// canvasImage is a canvas, shrunk for a reference image.
func canvasImage(p *Pixels) *image.RGBA {
	return visual.Shrink(p.W, p.H, func(x, y int) (r, g, b float32) {
		c := p.Pix[y*p.W+x]
		return c.R, c.G, c.B
	})
}

// rgbImage is a w×h video frame, shrunk for a reference image.
func rgbImage(b []byte, w, h int) *image.RGBA {
	return visual.Shrink(w, h, func(x, y int) (r, g, bl float32) {
		i := 3 * (y*w + x)
		return float32(b[i]), float32(b[i+1]), float32(b[i+2])
	})
}

// shots gathers what a group of cell grids looks like: their pixels as a
// sheet of images, where a character blends its colors, and their
// characters as text, exactly.
type shots struct {
	tiles []visual.Tile
	text  visual.Text
}

func (s *shots) add(name string, g *grid) {
	s.tiles = append(s.tiles, visual.Tile{Name: name, Img: canvasImage(g.pixels())})
	s.text.Add(name, gridChars(g))
}

// addScreen adds a w×h view's content, as the terminal would show it.
func (s *shots) addScreen(name, content string, w, h int) {
	g := parseGrid(content, w, h, testTheme)
	s.add(name, g)
}

func (s *shots) check(t *testing.T, name string) {
	t.Helper()
	visual.CheckSheet(t, ref(name+".png"), s.tiles)
	s.text.Check(t, ref(name+".txt"))
}

// gridChars is a grid's character layer: a line per row, blank where a
// cell only shows pixels (a space or a half block).
func gridChars(g *grid) string {
	var b strings.Builder
	for y := range g.H {
		for x := range g.W {
			switch ch := g.Cells[y*g.W+x].ch; ch {
			case "": // the second half of a wide character
			case halfBlock:
				b.WriteByte(' ')
			default:
				b.WriteString(ch)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// frameLog keeps every video frame written to it.
type frameLog struct {
	size int
	all  [][]byte
}

func (f *frameLog) Write(b []byte) (int, error) {
	if len(b) != f.size {
		panic("partial frame")
	}
	f.all = append(f.all, slices.Clone(b))
	return len(b), nil
}

// sample is n frame numbers spread evenly over count, first and last
// included.
func sample(count, n int) []int {
	if count <= n {
		out := make([]int, count)
		for i := range out {
			out[i] = i
		}
		return out
	}
	out := make([]int, n)
	for i := range out {
		out[i] = i * (count - 1) / (n - 1)
	}
	return out
}

// ---- fixtures ----

var goldenBase = time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

// wideSlide has wide characters (CJK, emoji) at odd and even columns, rich
// text attributes, a Lip Gloss box and a gradient: what transitions' seams
// and the cell writer have to get right.
func wideSlide() Slide {
	return Slide{Title: "Wide", Steps: 2, Transition: TransitionWipe,
		View: func(c Ctx, sc *Scene) {
			sc.Px.VGradient(0, sc.Px.H-1, c.Theme.Panel, c.Theme.Accent2.Scale(0.4))
			for i := 0; i < 9 && i < c.H; i++ {
				sc.Text(i%3, 1+i*2, "日本語のテキスト 🎉 wide ✓ 界a界b界", c.Theme.Text.Color())
			}
			box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.Theme.Accent.Color()).
				Padding(0, 1).Render("界 box 世\n🎉 emoji")
			sc.Put(c.W/3, 2, box)
			sc.Put(c.W/2+1, c.H/2, box)
			sc.Text(c.W-5, 3, "界界界界界", c.Theme.Good.Color())
			sc.Text(0, c.H-2, "bold", c.Theme.Warn.Color(), uv.AttrBold)
			sc.Text(6, c.H-2, "italic", c.Theme.Warn.Color(), uv.AttrItalic)
			sc.Text(13, c.H-2, "faint", c.Theme.Warn.Color(), uv.AttrFaint)
			sc.Cell(20, c.H-2, "u", uv.Style{Fg: c.Theme.Text.Color(), Underline: uv.UnderlineSingle})
			sc.Cell(22, c.H-2, "r", uv.Style{Fg: c.Theme.Text.Color(), Attrs: uv.AttrReverse | uv.AttrBold})
			if c.Step > 0 {
				sc.Text(2, c.H-4, fmt.Sprintf("step %d t=%.1f", c.Step, math.Min(c.StepT, 9)), c.Theme.Muted.Color())
			}
		}}
}

// morphSlides are two slides that place elements for TransitionMorph: some
// keys in both (one moving, one staying put, one whose draw changes), one
// in each alone, a repeated key, an unkeyed one, a glow, and characters.
func morphSlides() []Slide {
	slide := func(title string, b bool) Slide {
		return Slide{Title: title, Transition: TransitionMorph, View: func(c Ctx, sc *Scene) {
			th := c.Theme
			sc.Px.VGradient(0, sc.Px.H-1, th.Background, th.Panel)
			sc.Text(1, 1, title, th.Text.Color())
			moved := c.Rect(0.05, 0.1, 0.3, 0.3)
			col := th.Accent
			if b {
				moved, col = c.Rect(0.55, 0.5, 0.4, 0.4), th.Good
			}
			sc.Place("moved", moved, func(p *Pixels, r Rect) {
				p.RoundRect(r.X, r.Y, r.W, r.H, r.H/5, 0, col, 1)
				x, y := r.Center()
				p.Glow(x, y, r.H, col, 0.4)
			})
			sc.Place("still", c.Rect(0.45, 0.05, 0.1, 0.1), func(p *Pixels, r Rect) {
				x, y := r.Center()
				p.Disc(x, y, r.W/2, th.Accent2, 0.7)
			})
			for i := range 2 {
				r := c.Rect(0.1+0.1*float64(i), 0.75, 0.05, 0.1)
				if b {
					r = c.Rect(0.1+0.1*float64(i), 0.6, 0.05, 0.1)
				}
				sc.Place("twin", r, func(p *Pixels, r Rect) { p.Rect(r.X, r.Y, r.W, r.H, th.Muted, 1) })
			}
			only := "a-only"
			if b {
				only = "b-only"
			}
			sc.Place(only, c.Rect(0.7, 0.1, 0.25, 0.15), func(p *Pixels, r Rect) {
				Text{Font: th.Body, Size: c.SmallText(th.Body), Color: th.Text}.Draw(p, only, r.X, r.Y)
			})
			sc.Place("", c.Rect(0.02, 0.9, 0.05, 0.08), func(p *Pixels, r Rect) { p.Rect(r.X, r.Y, r.W, r.H, th.Warn, 1) })
		}}
	}
	return []Slide{slide("Morph A", false), slide("Morph B", true)}
}

// plainSlide is styled text and nothing else: no pixels drawn.
func plainSlide() Slide {
	return Slide{Title: "Plain", View: func(c Ctx, sc *Scene) {
		st := lipgloss.NewStyle().Foreground(c.Theme.Good.Color()).Background(c.Theme.Panel.Color())
		sc.Put(0, 0, st.Render("plain text, no scene")+"\n\x1b[1;4mbold underline\x1b[m\n界 wide\n"+strings.Repeat("x", c.W+10))
	}}
}

func extraSlides() []Slide {
	return []Slide{
		wideSlide(),
		plainSlide(),
		{Title: "Nil view"},
		{Title: "Panics", View: func(Ctx, *Scene) { panic("on purpose") }},
	}
}

// glyphSlide is a frame of text with every block element, quadrant and
// box-drawing character the PNG exporter draws by hand, plus ordinary text.
func glyphSlide() Slide {
	return Slide{Title: "Glyphs", View: func(c Ctx, sc *Scene) {
		var box []rune
		for r := range boxArms {
			box = append(box, r)
		}
		sort.Slice(box, func(i, j int) bool { return box[i] < box[j] })
		st := lipgloss.NewStyle().Foreground(c.Theme.Accent.Color()).Background(c.Theme.Panel.Color())
		lines := []string{
			st.Render("█▀▄▌▐░▒▓■▘▝▖▗▚▞▙▛▜▟"),
			st.Render(string(box)),
			st.Render("plain text ▁▁▁ ✓ é"),
			lipgloss.NewStyle().Foreground(c.Theme.Good.Color()).Render("no background, ▀▄█ █▀▄"),
		}
		sc.Put(0, 0, strings.Join(lines, "\n"))
	}}
}

// slideGrid draws a slide as cells, as the deck does.
func slideGrid(s Slide, w, h int, t float64, step int) *grid {
	return renderSlideGrid(s, Ctx{W: w, H: h, T: t, Step: step, StepT: t, Theme: testTheme})
}

// view is the model's screen as a frame string; a model with no size draws
// nothing.
func view(m model) string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	g := m.frame()
	defer g.release()
	return g.String()
}

func testModel(d *Deck, idx, step, w, h int, age float64, dev *devState) model {
	m := newModel(d, idx, step, 60, dev)
	m.w, m.h = w, h
	m.now = goldenBase
	m.enter = goldenBase.Add(-time.Duration(age * float64(time.Second)))
	m.stepStart = m.enter
	return m
}

var transitionNames = map[Transition]string{
	TransitionDefault: "default", TransitionNone: "none", TransitionPush: "push",
	TransitionDissolve: "dissolve", TransitionWipe: "wipe", TransitionMorph: "morph",
	TransitionFade: "fade", TransitionFadeThrough: "fadeThrough", TransitionCover: "cover",
	TransitionUncover: "uncover", TransitionSplit: "split", TransitionIris: "iris",
	TransitionZoom: "zoom", TransitionPixelate: "pixelate", TransitionGlitch: "glitch",

	TransitionPush.From(DirUp):      "push-top",
	TransitionPush.From(DirLeft):    "push-left",
	TransitionWipe.From(DirUp):      "wipe-top",
	TransitionWipe.From(DirRight):   "wipe-right",
	TransitionCover.From(DirUp):     "cover-top",
	TransitionUncover.From(DirLeft): "uncover-left",
	TransitionSplit.From(DirUp):     "split-top",
}

var transitionKinds = []Transition{
	TransitionDefault, TransitionNone, TransitionPush, TransitionDissolve, TransitionWipe, TransitionMorph,
	TransitionFade, TransitionFadeThrough, TransitionCover, TransitionUncover, TransitionSplit, TransitionIris,
	TransitionZoom, TransitionPixelate, TransitionGlitch,
	TransitionPush.From(DirUp), TransitionPush.From(DirLeft),
	TransitionWipe.From(DirUp), TransitionWipe.From(DirRight),
	TransitionCover.From(DirUp), TransitionUncover.From(DirLeft), TransitionSplit.From(DirUp),
}

// ---- the test ----

func TestInternalGolden(t *testing.T) {
	if testing.Short() {
		t.Skip("internal references take a few seconds; skipped with -short")
	}
	for _, part := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"transitions", goldenTransitions},
		{"video", goldenVideo},
		{"png", goldenPNG},
		{"model", goldenModel},
		{"modelTransitions", goldenModelTransitions},
		{"morph", goldenMorph},
		{"termWriter", goldenTermWriter},
		{"presenter", goldenPresenter},
		{"misc", goldenMisc},
	} {
		t.Run(part.name, part.run)
	}
}

// goldenTransitions draws every transition part-way, from the title slide
// to the wide one: on the canvas as video does, and in cells as the live
// deck does, both ways, with the character layer.
func goldenTransitions(t *testing.T) {
	deck := testDeck().Slides
	wide := wideSlide()
	var video []visual.Tile
	const w, h = 480, 270
	for _, k := range transitionKinds {
		for _, p := range []float64{0.33, 0.77} {
			a := renderSlide(deck[0], Ctx{W: w, H: h / 2, T: Settled, StepT: Settled, Theme: testTheme})
			b := renderSlide(wide, Ctx{W: w, H: h / 2, T: 0.4, Step: 1, StepT: 0.4, Theme: testTheme})
			mixTransition(k, a, b, p, true, testTheme)
			video = append(video, visual.Tile{Name: fmt.Sprintf("%s p=%v", transitionNames[k], p), Img: canvasImage(b.Px)})
			a.Release()
			b.Release()
		}
	}
	visual.CheckSheet(t, ref("transitions-video.png"), video)

	var cells shots
	for _, k := range transitionKinds {
		for _, fwd := range []bool{true, false} {
			g := mixSlides(k, deck[0], wide, 0, 1, 120, 34, 0.4, fwd).flatten()
			cells.add(fmt.Sprintf("%s fwd=%v p=0.4", transitionNames[k], fwd), g)
			g.release()
		}
	}
	cells.check(t, "transitions-cells")
}

// goldenVideo runs the video frame loop: how many frames each run makes, a
// sample of them, the timing of each build, and the sizes it refuses.
func goldenVideo(t *testing.T) {
	var text visual.Text
	var tiles []visual.Tile
	run := func(name string, d *Deck, o videoOptions) {
		f := &frameLog{size: 3 * o.width * o.height}
		if err := writeVideoFrames(d, o, f); err != nil {
			t.Fatal(err)
		}
		text.Add("frames "+name, fmt.Sprint(len(f.all)))
		for _, i := range sample(len(f.all), 8) {
			tiles = append(tiles, visual.Tile{Name: fmt.Sprintf("%s frame %d", name, i), Img: rgbImage(f.all[i], o.width, o.height)})
		}
	}
	d := testDeck()
	run("testdeck-320x180", d, videoOptions{width: 320, height: 180, fps: 10, hold: 0.3, first: 0, last: 2, quiet: true})
	run("testdeck-tail", d, videoOptions{width: 200, height: 112, fps: 8, hold: 0.25, first: 2, last: 4, quiet: true})
	extra := &Deck{Name: "extra", Theme: testTheme, Slides: append(testDeck().Slides[:1], extraSlides()...)}
	extra.Slides[2].Hold = 0.5 // a slide that asks for longer
	run("extras-320x180", extra, videoOptions{width: 320, height: 180, fps: 10, hold: 0.2, first: 0, last: 4, quiet: true})
	visual.CheckSheet(t, ref("video.png"), tiles)

	for i, s := range append(testDeck().Slides, wideSlide()) {
		for step := 0; step < s.steps(); step++ {
			text.Add(fmt.Sprintf("timing %d.%d", i, step),
				fmt.Sprint(videoTiming(s, step, 1), videoTiming(s, step, 0), videoTiming(Slide{Hold: 3}, step, 1)))
		}
	}
	for _, bad := range []videoOptions{{width: 0, height: 10}, {width: 10, height: 11}} {
		text.Add(fmt.Sprintf("refuses %dx%d", bad.width, bad.height), fmt.Sprint(renderVideo(testDeck(), bad)))
	}
	text.Check(t, ref("video.txt"))
}

// goldenPNG checks PNG export: a frame of hand-drawn block and box-drawing
// characters and plain text, the file it writes, and contact sheets.
func goldenPNG(t *testing.T) {
	dir := t.TempDir()
	decode := func(path string) *image.RGBA {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		rgba := image.NewRGBA(img.Bounds())
		for y := range img.Bounds().Dy() {
			for x := range img.Bounds().Dx() {
				rgba.Set(x, y, img.At(x, y))
			}
		}
		return rgba
	}
	var frames []*grid
	var tiles []visual.Tile
	for i, s := range []Slide{glyphSlide(), plainSlide()} {
		fr := slideGrid(s, 80, 24, Settled, 0)
		frames = append(frames, fr)
		img := frameImage(fr)
		tiles = append(tiles, visual.Tile{Name: s.Title, Img: img})
		path := filepath.Join(dir, fmt.Sprintf("s%d.png", i))
		if err := writePNG(fr, path); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(decode(path).Pix, img.Pix) {
			t.Errorf("%s: the PNG file isn't the image it was made from", s.Title)
		}
	}
	for _, shrink := range []int{1, 4} {
		path := filepath.Join(dir, fmt.Sprintf("sheet-%d.png", shrink))
		if err := writeSheet(frames, shrink, path); err != nil {
			t.Fatal(err)
		}
		tiles = append(tiles, visual.Tile{Name: fmt.Sprintf("sheet shrink=%d", shrink), Img: decode(path)})
	}
	// A sheet with fewer frames than columns.
	path := filepath.Join(dir, "sheet2.png")
	if err := writeSheet([]*grid{frames[0], frames[0]}, 2, path); err != nil {
		t.Fatal(err)
	}
	tiles = append(tiles, visual.Tile{Name: "sheet of two", Img: decode(path)})
	visual.CheckSheet(t, ref("png.png"), tiles)
	// Errors from an unwritable path are reported, not panicked.
	if writePNG(frames[0], filepath.Join(dir, "no", "such", "dir.png")) == nil {
		t.Error("writing to a missing directory: no error")
	}
}

func goldenModel(t *testing.T) {
	d := testDeck()
	devs := map[string]func() *devState{
		"plain":    func() *devState { return nil },
		"dev":      func() *devState { return &devState{} },
		"building": func() *devState { return &devState{building: true} },
		"failed": func() *devState {
			return &devState{buildErr: "./main.go:12:3: undefined: nope\n./main.go:13:1: syntax error\n" + strings.Repeat("more output\n", 30)}
		},
	}
	var screens shots
	shot := func(m model, dev, what string) {
		g := m.frame()
		screens.add(fmt.Sprintf("%s %dx%d s%d.%d %s", dev, m.w, m.h, m.idx, m.step, what), g)
		g.release()
	}
	for _, size := range [][2]int{{80, 24}, {200, 56}} {
		for _, dev := range []string{"plain", "dev"} {
			for idx, s := range d.Slides {
				m := testModel(d, idx, s.steps()-1, size[0], size[1], Settled, devs[dev]())
				shot(m, dev, "settled")
			}
		}
		// Footer states in dev mode.
		for _, dev := range []string{"building", "failed"} {
			m := testModel(d, 2, 2, size[0], size[1], Settled, devs[dev]())
			shot(m, dev, "footer")
			shot(press(m, "n"), dev, "footer+notes")
		}
		m := testModel(d, 2, 1, size[0], size[1], Settled, devs["dev"]())
		m.count = "12"
		shot(m, "dev", "count")

		// Notes and help, through the keys that toggle them.
		for _, idx := range []int{0, 2} {
			m := testModel(d, idx, 0, size[0], size[1], Settled, nil)
			m = press(m, "n")
			shot(m, "plain", "notes")
			m = press(m, "?")
			shot(m, "plain", "notes+help")
			m = press(m, "n")
			shot(m, "plain", "help")
		}
		// Long notes get cut off.
		long := *d
		long.Notes = "## Block letters\n" + strings.Repeat("a long line of speaker notes that wraps\n", 30)
		m = testModel(&long, 0, 0, size[0], size[1], Settled, nil)
		m.showNotes = true
		shot(m, "plain", "long-notes")
		// Blanked screens, black and white, over notes and help.
		m = press(press(testModel(d, 2, 1, size[0], size[1], Settled, devs["dev"]()), "n"), "?")
		for _, k := range []string{"b", "w"} {
			m = press(m, k)
			shot(m, "dev", "blank-"+m.blank.String())
		}
	}
	screens.check(t, "model-screens")

	var text visual.Text
	// The key handler walks the deck: record where each key lands.
	m := testModel(d, 0, 0, 120, 36, 0, nil)
	var trail strings.Builder
	for _, k := range strings.Fields("right right right right right right right left left [ ] 3 g 1 2 enter G g home end h l space pgdn pgup j k down up enter backspace r n ? esc q ctrl+l 99 g 0 g") {
		m = press(m, k)
		fmt.Fprintf(&trail, "%s -> s%d.%d count=%q notes=%v help=%v trans=%d fwd=%v from=%v\n",
			k, m.idx, m.step, m.count, m.showNotes, m.showHelp, m.trans.kind, m.transFwd, m.transFrom != nil)
	}
	text.Add("keys", trail.String())
	st, _ := json.Marshal(m.linkState())
	text.Add("linkState", string(st))
	for _, cmd := range []linkCmd{{Key: "right"}, {Slide: 4}, {Key: "["}, {Key: "bogus"}, {Key: "q"}, {Key: "end"}, {Key: "r"}} {
		next, _ := m.Update(cmd)
		m = next.(model)
		b, _ := json.Marshal(cmd)
		text.Add("link "+string(b), fmt.Sprintf("s%d.%d", m.idx, m.step))
	}
	b, _ := json.Marshal(linkOutline{Title: "t", Steps: 2})
	text.Add("link outline", string(b))
	var keys []string
	for k, v := range keyActs {
		keys = append(keys, fmt.Sprint(k, "=", v.act, v.nav))
	}
	sort.Strings(keys)
	text.Add("key table", strings.Join(keys, "\n"))
	var help strings.Builder
	for _, b := range bindings {
		fmt.Fprintf(&help, "%q %q\n", b.help, b.what)
	}
	text.Add("key help", help.String())

	// Messages through Update: sizes, ticks, link commands, builds.
	um := testModel(d, 0, 0, 0, 0, 0, &devState{})
	var upd strings.Builder
	send := func(msg tea.Msg) {
		next, cmd := um.Update(msg)
		um = next.(model)
		fmt.Fprintf(&upd, "%T s%d.%d %dx%d cmd=%v from=%v now=%v build=%+v\n", msg, um.idx, um.step, um.w, um.h, cmd != nil,
			um.transFrom != nil, um.now.Sub(goldenBase), *um.dev)
	}
	send(tea.WindowSizeMsg{Width: 0, Height: 0}) // Bubble Tea's placeholder: ignored
	send(tea.WindowSizeMsg{Width: 90, Height: 30})
	send(tickMsg(goldenBase.Add(time.Second)))
	um.goTo(1, 0, true)
	send(tickMsg(um.transStart.Add(100 * time.Millisecond)))
	send(tickMsg(um.transStart.Add(600 * time.Millisecond))) // the transition is over
	send(tea.KeyPressMsg{Code: 'l', Text: "l"})
	send(tea.KeyPressMsg{Code: tea.KeyRight})
	send(linkCmd{Slide: 1})
	send(linkCmd{Key: "bogus"})
	send(buildDoneMsg{out: "  oops \n", err: fmt.Errorf("exit 1")})
	send(buildDoneMsg{err: fmt.Errorf("exit 2")})
	send(buildDoneMsg{})
	send(struct{}{})
	text.Add("update", upd.String())
	bodyH, _ := um.layout()
	text.Add("cur", um.cur().Title, fmt.Sprint(bodyH, um.showChrome()))

	m = testModel(d, 1, 0, 120, 36, Settled, nil)
	trail.Reset()
	for _, k := range strings.Fields("b b w w . , , b x ctrl+l right w 5 enter w ] b [ w g b G b r w n b ? b esc") {
		m = press(m, k)
		fmt.Fprintf(&trail, "%s -> s%d.%d count=%q blank=%q notes=%v help=%v\n", k, m.idx, m.step, m.count, m.blank, m.showNotes, m.showHelp)
	}
	text.Add("blank keys", trail.String())
	m = press(m, "w")
	st, _ = json.Marshal(m.linkState())
	text.Add("linkState blank", string(st))
	for _, cmd := range []linkCmd{{Key: "b"}, {Key: "right"}, {Key: "."}, {Key: "w"}, {Key: ","}} {
		next, _ := m.Update(cmd)
		m = next.(model)
		b, _ := json.Marshal(cmd)
		text.Add("blank link "+string(b), fmt.Sprintf("s%d.%d %q", m.idx, m.step, m.blank))
	}
	// A zero-size model draws nothing, and the view is always empty: the
	// writer owns the screen.
	text.Add("empty", fmt.Sprintf("%q", view(testModel(d, 0, 0, 0, 0, 0, nil))))
	text.Add("live view", fmt.Sprintf("%q", testModel(d, 0, 0, 80, 24, 0, nil).View().Content))
	text.Check(t, ref("model.txt"))
}

// press sends a key to the model.
func press(m model, k string) model {
	m, _ = m.handleKey(k)
	return m
}

// goldenModelTransitions records which transition the live model runs on
// each move, and whether it's still running part-way; transitions-cells has
// how they look.
func goldenModelTransitions(t *testing.T) {
	d := testDeck()
	// from slide, to slide (and step), direction; the target's own
	// Transition field picks the kind: push (default), wipe, dissolve, none.
	cases := []struct {
		name         string
		from, fstep  int
		to, tstep    int
		forward, dev bool
	}{
		{"push-fwd", 0, 0, 1, 0, true, false},
		{"wipe-fwd", 1, 2, 2, 0, true, false},
		{"dissolve-back", 2, 3, 0, 0, false, false},
		{"push-back", 2, 0, 1, 2, false, false},
		{"none-fwd", 2, 3, 3, 0, true, false},
		{"wipe-fwd-dev", 1, 2, 2, 0, true, true},
	}
	var text visual.Text
	var screens shots
	for _, tc := range cases {
		var b strings.Builder
		for _, p := range []float64{0, 0.2, 0.5, 0.8, 1.2} {
			var dev *devState
			if tc.dev {
				dev = &devState{}
			}
			m := testModel(d, tc.from, tc.fstep, 80, 24, Settled, dev)
			m.goTo(tc.to, tc.tstep, tc.forward)
			m.now = goldenBase.Add(time.Duration(p * TransitionDuration * float64(time.Second)))
			if m.transFrom != nil && m.now.Sub(m.transStart).Seconds() >= m.transDur {
				m.transFrom = nil // what the tick does
			}
			fmt.Fprintf(&b, "p=%v kind=%d fwd=%v running=%v\n", p, m.trans.kind, m.transFwd, m.transFrom != nil)
			if p == 0.5 {
				g := m.frame()
				screens.add(tc.name+" p=0.5", g)
				g.release()
			}
		}
		text.Add(tc.name, b.String())
	}
	// goTo to the same place does nothing; a size change drops the
	// transition's source frame (it no longer fits).
	m := testModel(d, 1, 0, 80, 24, Settled, nil)
	m.goTo(1, 0, true)
	m.goTo(2, 0, true)
	m.w, m.h = 100, 30
	m.now = goldenBase.Add(100 * time.Millisecond)
	g := m.frame()
	screens.add("resized mid-transition", g)
	g.release()
	text.Check(t, ref("model-transitions.txt"))
	screens.check(t, "model-transitions-screens")
}

func goldenMorph(t *testing.T) {
	ms := morphSlides()
	var canvas []visual.Tile
	for _, p := range []float64{0.1, 0.33, 0.5, 0.77} {
		const w, h = 320, 180
		a := drawSlide(ms[0], Ctx{W: w, H: h / 2, T: Settled, StepT: Settled, Theme: testTheme})
		b := drawSlide(ms[1], Ctx{W: w, H: h / 2, T: 0.4, StepT: 0.4, Theme: testTheme})
		mixTransition(TransitionMorph, a, b, p, true, testTheme)
		canvas = append(canvas, visual.Tile{Name: fmt.Sprintf("canvas p=%v", p), Img: canvasImage(b.Px)})
		a.Release()
		b.Release()
	}
	visual.CheckSheet(t, ref("morph-canvas.png"), canvas)

	var screens shots
	for _, p := range []float64{0.33, 0.77} {
		g := mixSlides(TransitionMorph, ms[0], ms[1], 0, 0, 120, 34, p, true).flatten()
		screens.add(fmt.Sprintf("cells p=%v", p), g)
		g.release()
	}
	// The live model: forward, back, and starting over mid-way, when the
	// frame on screen has no elements left to move and cross-fades instead.
	d := &Deck{Name: "morph", Theme: testTheme, Slides: append(ms, testDeck().Slides[0])}
	shot := func(name string, m model) {
		g := m.frame()
		screens.add(name, g)
		g.release()
	}
	at := goldenBase.Add(time.Duration(0.5 * MorphDuration * float64(time.Second)))
	m := testModel(d, 0, 0, 100, 30, Settled, nil)
	m.goTo(1, 0, true)
	m.now = at
	shot("model fwd p=0.5", m)
	m = testModel(d, 1, 0, 100, 30, Settled, nil)
	m.goTo(0, 0, false)
	m.now = at
	shot("model back p=0.5", m)
	m.goTo(1, 0, true)
	shot("model restart p=0.5", m)
	// A slower morph: TransitionTime stretches the same frames over 0.9s.
	slow := &Deck{Name: "slow", Theme: testTheme, Slides: append([]Slide(nil), d.Slides...)}
	slow.Slides[1].Transition = TransitionMorph.Over(0.9)
	m = testModel(slow, 0, 0, 100, 30, Settled, nil)
	m.goTo(1, 0, true)
	m.advance(goldenBase.Add(time.Duration(0.5 * 0.9 * float64(time.Second))))
	shot("model slow p=0.5", m)
	screens.check(t, "morph-screens")

	var text visual.Text
	o := videoOptions{width: 160, height: 90, fps: 10, hold: 0.5, first: 0, last: 2, quiet: true}
	for _, deck := range []*Deck{d, slow} {
		f := &frameLog{size: 3 * o.width * o.height}
		if err := writeVideoFrames(deck, o, f); err != nil {
			t.Fatal(err)
		}
		text.Add("video frames, "+deck.Name, fmt.Sprint(len(f.all)))
	}
	text.Check(t, ref("morph.txt"))
}

// flatSlide is a slide whose every color is a whole number and every edge
// on a pixel: it renders to the same cells on any machine, so the terminal
// writer's output can be checked exactly. A bar moves a pixel a frame.
func flatSlide(title string, bg RGB) Slide {
	return Slide{Title: title, Transition: TransitionNone, View: func(c Ctx, sc *Scene) {
		sc.Px.Fill(bg)
		x := float64(int(c.T*60) % sc.Px.W)
		sc.Px.Rect(x, 4, 6, 6, c.Theme.Accent, 1)
		sc.Text(1, 1, title, c.Theme.Text.Color())
	}}
}

// attrsSlide is text in every attribute the writer sends, on a flat
// background.
func attrsSlide() Slide {
	return Slide{Title: "Attrs", Transition: TransitionNone, View: func(c Ctx, sc *Scene) {
		sc.Px.Fill(c.Theme.Panel)
		sc.Text(0, 0, "日本語 🎉 wide", c.Theme.Text.Color())
		sc.Text(0, 1, "bold", c.Theme.Warn.Color(), uv.AttrBold)
		sc.Text(6, 1, "italic", c.Theme.Warn.Color(), uv.AttrItalic)
		sc.Text(13, 1, "faint", c.Theme.Warn.Color(), uv.AttrFaint)
		sc.Cell(20, 1, "u", uv.Style{Fg: c.Theme.Text.Color(), Underline: uv.UnderlineSingle})
		sc.Cell(22, 1, "r", uv.Style{Fg: c.Theme.Text.Color(), Attrs: uv.AttrReverse | uv.AttrBold})
		box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.Theme.Accent.Color()).Render("界 box")
		sc.Put(2, 3, box)
		if c.Step > 0 {
			sc.Text(2, 7, fmt.Sprintf("step %d", c.Step), c.Theme.Muted.Color())
		}
	}, Steps: 2}
}

// goldenTermWriter checks the escape sequences the terminal writer sends,
// exactly, for frames that render the same everywhere.
func goldenTermWriter(t *testing.T) {
	flat := *testTheme
	flat.Overlay = nil
	d := &Deck{Name: "flat", Theme: &flat, Slides: []Slide{
		flatSlide("One", Hex("#101820")), flatSlide("Two", Hex("#203040")), flatSlide("Three", Hex("#405060")),
	}}
	var text visual.Text
	var out bytes.Buffer
	tw := &termWriter{out: &out}
	keep := func(name string) {
		text.Add(name, fmt.Sprintf("%q", out.String()))
		out.Reset()
	}
	const w, h = 40, 12
	m := testModel(d, 0, 0, w, h, 0, nil)
	for f := range 60 {
		m.now = goldenBase.Add(time.Duration(f) * time.Second / 60)
		switch f {
		case 20:
			m.goTo(1, 0, true)
		case 40:
			m.goTo(2, 0, true)
		}
		fr := m.frame()
		tw.write(fr.clone(), m.cur().Title, false)
		fr.release()
		if f%10 == 0 || f == 59 {
			keep(fmt.Sprintf("frame %02d", f))
		}
		out.Reset()
	}

	// A frame that doesn't change writes nothing; one that changes a
	// little writes only that.
	fr := m.frame()
	tw.write(fr.clone(), m.cur().Title, false)
	keep("same, first")
	tw.write(fr.clone(), m.cur().Title, false)
	keep("same, again")
	small := fr.clone()
	small.at(3, 4).ch, small.at(3, 4).fg = "x", [3]uint8{1, 2, 3}
	small.at(9, 4).bg = [3]uint8{9, 9, 9} // within the rewrite gap of the first
	small.at(30, 10).ch = "▀"             // far away: a cursor move
	small.at(w-1, h-1).bg = [3]uint8{5, 6, 7}
	tw.write(small.clone(), m.cur().Title, false)
	keep("small change")
	// A new title is sent even when the cells are the same.
	tw.write(small.clone(), "A new title", false)
	keep("title only")
	// invalidate (ctrl+l) redraws everything.
	tw.write(small.clone(), "A new title", true)
	keep("forced full")
	fr.release()

	// A resize redraws everything at the new size.
	m.w, m.h = 30, 8
	fr = m.frame()
	tw.write(fr.clone(), m.cur().Title, false)
	fr.release()
	keep("resize")

	// Wide characters survive partial updates.
	vt := &termWriter{out: &out}
	for i, line := range []string{"ab界cd", "ab世cd", "a界界cd", "界b界cd", "abcde", "日本語ab", "ab🎉cd"} {
		vt.write(parseGrid(line, 10, 1, testTheme), "", false)
		keep(fmt.Sprintf("wide %d", i))
	}
	// Rich attributes (bold, italic, underline, reverse) on a changing frame.
	at := &termWriter{out: &out}
	for i, step := range []int{0, 1, 0} {
		at.write(renderSlideGrid(attrsSlide(), Ctx{W: 30, H: 9, T: Settled, Step: step, StepT: 0.2, Theme: &flat}), "Attrs", false)
		keep(fmt.Sprintf("attrs %d", i))
	}

	// The writer's own lifecycle sequences: set-up and restore.
	var life bytes.Buffer
	lw := newTermWriter(&life, testTheme.Background)
	lw.close()
	text.Add("lifecycle", fmt.Sprintf("%q", life.String()))
	text.Add("moveTo", fmt.Sprintf("%q %q", moveTo(nil, 0, 0), moveTo(nil, 681, 170)))
	text.Check(t, ref("termwriter.txt"))
}

func presenterFor(d *Deck, w, h int, linked bool, slide, step int, previews map[previewKey]string) presenter {
	slides := d.Slides
	outline := make([]linkOutline, len(slides))
	for i, s := range slides {
		outline[i] = linkOutline{Title: s.Title, Steps: s.steps()}
	}
	p := newPresenter(d, "/tmp/x.sock", 30*time.Minute)
	p.previews, p.now, p.w, p.h, p.linked = previews, goldenBase, w, h, linked
	if slide >= 0 {
		p.st = linkState{Slide: slide, Step: step, Outline: outline, Notes: d.slideNotes()[slide], W: 682, H: 171}
	}
	return p
}

func goldenPresenter(t *testing.T) {
	d := testDeck()
	d.Slides = append(d.Slides, wideSlide()) // 6 slides; the last has notes and steps
	d.Notes += "\n## Character layer\n\n## Last\n\n## Wide\n\nwide characters\nand a second line\n"
	previews := map[previewKey]string{} // shared: a preview doesn't depend on who asks
	timers := []struct {
		name    string
		running bool
		since   time.Duration // before now
		banked  time.Duration
	}{
		{"idle", false, 0, 0},
		{"running", true, 7 * time.Minute, 0},
		{"paused", false, 0, 12 * time.Minute},
		{"over", true, 31 * time.Minute, 2 * time.Minute},
		{"behind", true, 25 * time.Minute, 0},
		{"ahead", true, 1 * time.Minute, 0},
	}
	var screens shots
	for _, size := range [][2]int{{60, 15}, {80, 24}, {120, 36}, {200, 50}} {
		w, h := size[0], size[1]
		every := w == 120 // every state at one size; the ones that change with the size at all four
		shot := func(name string, p presenter) {
			screens.addScreen(fmt.Sprintf("%dx%d %s", w, h, name), p.View().Content, w, h)
		}
		shot("waiting", presenterFor(d, w, h, false, -1, 0, previews))
		for _, at := range []struct{ slide, step int }{{0, 0}, {1, 1}, {2, 3}, {5, 1}} {
			p := presenterFor(d, w, h, true, at.slide, at.step, previews)
			p.running, p.since = true, goldenBase.Add(-7*time.Minute)
			shot(fmt.Sprintf("s%d.%d running", at.slide, at.step), p)
		}
		p := presenterFor(d, w, h, true, 0, 0, previews)
		p.st.Notes = strings.Repeat("a long line of speaker notes, which wraps over several lines\n", 20)
		shot("long-notes", p)
		p = presenterFor(d, w, h, true, 1, 1, previews)
		p.dialog = dialogKeys
		shot("keys", p)
		p = presenterFor(d, w, h, true, 3, 0, previews)
		p.st.W, p.st.H = 120, 40
		shot("small-deck", p)
		for _, ps := range postingStates(d, w, h, previews) {
			if every || ps.name == "live-dialog" || ps.name == "rehearsing" {
				shot("posting/"+ps.name, ps.p)
			}
		}
		if !every {
			continue
		}
		shot("waiting-linked", presenterFor(d, w, h, true, -1, 0, previews))
		launch := presenterFor(d, w, h, false, -1, 0, previews)
		launch.launch = func(int, int) error { return nil }
		shot("launch/waiting", launch)
		launch.launching = true
		shot("launch/opening", launch)
		launch.launching, launch.launchErr = false, "Ghostty could not open the presentation: Automation permission denied"
		shot("launch/error", launch)
		launch = presenterFor(d, w, h, false, 1, 1, previews)
		launch.launch = func(int, int) error { return nil }
		launch.launchErr = "The presentation has not connected. Press p to retry, or check its window."
		shot("launch/disconnected", launch)
		images := presenterFor(d, w, h, true, 1, 1, previews)
		images.images = newKittyImages()
		if k, ok := images.key(1, 1); ok {
			images.images.add(k)
			images.images.ready[k] = true
			if next, _, ok := images.nextTarget(); ok {
				nk := k
				nk.slide, nk.step = next[0], next[1]
				images.images.add(nk)
				images.images.ready[nk] = true
			}
		}
		shot("image-previews", images)
		for _, tm := range timers {
			p := presenterFor(d, w, h, true, 2, 0, previews)
			p.running, p.since, p.banked = tm.running, goldenBase.Add(-tm.since), tm.banked
			shot("s2.0 "+tm.name, p)
		}
		p = presenterFor(d, w, h, false, 1, 1, previews)
		p.running, p.since = true, goldenBase.Add(-7*time.Minute)
		shot("unlinked", p)
		p.linked, p.count = true, "12"
		shot("count", p)
		// A deck that no longer matches this build's slides: out-of-date previews.
		p = presenterFor(d, w, h, true, 1, 0, previews)
		p.st.Outline = append([]linkOutline(nil), p.st.Outline...)
		p.st.Outline[1].Title = "Renamed"
		p.st.Outline[2].Title = "Also renamed"
		shot("stale", p)
		p = presenterFor(d, w, h, true, 3, 0, previews)
		p.st.W, p.st.H = 0, 0 // unreported: no previews
		shot("no-deck-size", p)
		// A blanked deck: the header says so, the notes and previews stay.
		for _, color := range []string{"black", "white"} {
			p = presenterFor(d, w, h, true, 1, 1, previews)
			p.st.Blank = color
			shot("blank-"+color, p)
		}
	}
	screens.check(t, "presenter-screens")

	var text visual.Text
	// Messages through the presenter's Update: size, tick, report, keys.
	var pm tea.Model = presenterFor(d, 0, 0, false, -1, 0, previews)
	var pu strings.Builder
	psend := func(msg tea.Msg) {
		next, cmd := pm.Update(msg)
		pm = next
		q := pm.(presenter)
		fmt.Fprintf(&pu, "%T %dx%d linked=%v s%d.%d now=%v count=%q running=%v banked=%v cmd=%v\n", msg, q.w, q.h, q.linked,
			q.st.Slide, q.st.Step, q.now.Sub(goldenBase), q.count, q.running, q.banked, cmd != nil)
	}
	psend(tea.WindowSizeMsg{Width: 100, Height: 30})
	psend(presTickMsg(goldenBase.Add(90 * time.Second)))
	psend(linkUpMsg{})
	psend(linkStateMsg(presenterFor(d, 0, 0, true, 0, 1, previews).st))
	for _, k := range []string{"1", "2", "g", "right", "left", "]", "[", "home", "G", "r", "T", "x"} {
		psend(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	psend(tea.KeyPressMsg{Code: tea.KeyRight})
	psend(tea.KeyPressMsg{Code: 'q', Text: "q"})
	psend(linkDownMsg{})
	psend(struct{}{})
	text.Add("update", pu.String())
	// A zero-size presenter draws nothing.
	text.Add("empty", fmt.Sprintf("%q", presenterFor(d, 0, 0, true, 0, 0, previews).View().Content))

	// The pieces.
	outline := []linkOutline{{Title: "a", Steps: 1}, {Title: "b", Steps: 4}, {Title: "c", Steps: 1}}
	for _, c := range []struct {
		slide, step int
		el          time.Duration
	}{{0, 0, 0}, {1, 2, 15 * time.Minute}, {1, 2, 10 * time.Minute}, {0, 0, 5 * time.Minute}, {2, 0, 29 * time.Minute}} {
		text.Add(fmt.Sprintf("pace %d.%d at %v", c.slide, c.step, c.el), fmt.Sprint(pace(outline, c.slide, c.step, c.el, 30*time.Minute)))
	}
	text.Add("pace, degenerate", fmt.Sprint(pace(nil, 0, 0, time.Minute, time.Minute), pace(outline, 0, 0, time.Minute, 0)))
	text.Add("clock", strings.Join([]string{clock(0), clock(59 * time.Second), clock(61 * time.Second), clock(-90 * time.Second),
		clock(3601 * time.Second), clock(1499 * time.Millisecond), clock(1500 * time.Millisecond)}, "\n"))
	text.Add("text helpers", fmt.Sprintf("%q\n%q\n%q\n%q\n%q", spread("left", "right", 20), spread("left", "right", 8),
		indent("a\nb", "  "), truncate("abcdef", 3), truncate("abc", -1)))
	p := presenterFor(d, 100, 30, true, 1, 1, previews)
	text.Add("placeholder", ansi.Strip(p.placeholder(20, 5, "message")))
	text.Add("frame", ansi.Strip(frame("a\nb", testTheme.Accent)))
	var nt strings.Builder
	for _, at := range [][2]int{{0, 0}, {1, 3}, {5, 1}, {4, 0}, {9, 0}} {
		p := presenterFor(d, 100, 30, true, min(at[0], 5), at[1], previews)
		p.st.Slide = at[0] // may be past the end: it is clamped
		n, label, ok := p.nextTarget()
		fmt.Fprintln(&nt, n, label, ok)
	}
	text.Add("nextTarget", nt.String())
	for _, sz := range [][4]int{{100, 30, 682, 171}, {40, 12, 682, 171}, {200, 60, 120, 40}, {30, 9, 100, 100}, {120, 5, 682, 171}} {
		p := presenterFor(d, sz[0], sz[1], true, 1, 1, previews)
		p.st.W, p.st.H = sz[2], sz[3]
		k, ok := p.key(1, 1)
		if !ok {
			k.pw, k.ph = 0, 0
		}
		text.Add(fmt.Sprintf("previewBox %dx%d for %dx%d", sz[0], sz[1], sz[2], sz[3]), fmt.Sprint(k.pw, k.ph))
	}
	text.Check(t, ref("presenter.txt"))

	var pv shots
	for _, i := range []int{0, 2, 5} {
		for _, step := range []int{0, 1} {
			for _, k := range []previewKey{{i, step, 50, 12, 682, 171}, {i, step, 30, 9, 120, 40}} {
				pv.addScreen(fmt.Sprintf("preview %d.%d %dx%d", i, step, k.pw, k.ph), renderPreview(d, k), k.pw, k.ph)
			}
		}
	}
	pv.check(t, "presenter-previews")
}

// goldenMisc covers the small engine pieces nothing above reaches directly:
// the panic screen, grids and the off-screen Render.
func goldenMisc(t *testing.T) {
	var screens shots
	add := func(name string, g *grid) {
		screens.add(name, g)
		g.release()
	}
	add("panic", slideGrid(Slide{Title: "boom", View: func(Ctx, *Scene) { panic("on purpose") }}, 60, 10, 0, 0))
	add("nil view", slideGrid(Slide{Title: "nil"}, 20, 4, 0, 0))
	// Off-screen Render, with the character layer.
	off := NewScene(24, 5, testTheme)
	off.Px.Disc(8, 5, 6, testTheme.Accent, 1)
	off.Px.Glow(30, 5, 10, testTheme.Accent2, 0.8)
	off.Text(2, 1, "off-screen", testTheme.Text.Color(), uv.AttrBold)
	add("off-screen", off.flatten())
	sc := NewScene(40, 10, testTheme)
	sc.Put(2, 2, "put")
	sc.Overlay(4, 3, "over")
	add("scene", sc.flatten())
	// grid helpers: blank, clone, setUV clipping a wide char.
	bg := blankGrid(6, 2, testTheme.Background)
	bg.draw(4, 0, "界界", false, testTheme.Text) // the second one doesn't fit
	bg.draw(-1, 1, "界x", true, testTheme.Text)
	add("grid, cloned", bg.clone())
	add("grid", bg)
	screens.check(t, "misc-screens")

	var text visual.Text
	text.Add("pixelCell", fmt.Sprintf("%+v\n%+v", pixelCell(testTheme.Accent, testTheme.Accent), pixelCell(testTheme.Accent, testTheme.Accent2)))
	text.Add("styles", fmt.Sprintf("%q", testTheme.styles().accent.Render("x")))
	text.Add("helpBox", ansi.Strip(testModel(testDeck(), 0, 0, 100, 30, 0, nil).helpBox()))
	text.Add("chrome", ansi.Strip(testModel(testDeck(), 2, 1, 100, 30, 0, &devState{}).chrome()),
		ansi.Strip(testModel(testDeck(), 1, 2, 60, 30, 0, &devState{}).chrome()))
	text.Add("defaultSocket", filepath.Base(defaultSocket("name")))
	text.Check(t, ref("misc.txt"))
}

type namedPresenter struct {
	name string
	p    presenter
}

// postingStates are the presenter view, for d with posts added to its
// notes, with posting in each of its states, panel open and closed. The
// files would live under a root that doesn't exist, so none is read or
// written, and times are goldenBase's.
func postingStates(d *Deck, w, h int, previews map[previewKey]string) []namedPresenter {
	dp := *d
	dp.Publisher = &fakePublisher{}
	dp.Notes = "```post media=none\nopening\n```\n" +
		strings.Replace(d.Notes, "## Bullets\n", "## Bullets\n\n```post media=builds\nthe builds\n```\n", 1)
	const account = "me.bsky.social"
	var out []namedPresenter
	at := func(name string, slide int, set func(p *presenter, ps *posting)) {
		p := presenterFor(&dp, w, h, slide >= 0, slide, 1, previews)
		p.running, p.since = true, goldenBase.Add(-7*time.Minute)
		p.post = newPosting(&dp, "/nonexistent/decker", &sync.Mutex{})
		set(&p, p.post)
		out = append(out, namedPresenter{name, p})
	}
	open := func(p *presenter, ps *posting) *posting { p.dialog = dialogPosting; return ps }
	live := func(ps *posting, dry bool) *publishing {
		pb := &publishing{pub: dp.Publisher, dry: dry, on: true, thread: &thread{}, titles: dp.titles(),
			posts: map[int][]Post{-1: {{Slide: -1}}, 1: {{Slide: 1}}}}
		ps.pub, ps.account, ps.signedIn = pb, account, account
		return pb
	}
	at("not-set-up", 1, func(*presenter, *posting) {})
	at("dialog", 1, func(p *presenter, ps *posting) { open(p, ps) })
	at("typing", 1, func(p *presenter, ps *posting) { open(p, ps).input, ps.focus = "me.bs", ctlAccount })
	at("carry-on", 1, func(p *presenter, ps *posting) {
		open(p, ps).account, ps.signedIn, ps.input, ps.cont = account, account, account, true
		ps.last = lastThread{dir: "/nonexistent/decker/talks/deck-test/threads/x", posted: 5, at: goldenBase.Add(-24 * time.Hour)}
	})
	at("signing-in", 1, func(p *presenter, ps *posting) {
		open(p, ps).account = account
		ps.start = &start{kind: startLive, job: jobSignIn}
		ps.start.visit = "https://bsky.social/oauth/authorize?client_id=http%3A%2F%2Flocalhost%3Fredirect_uri%3Dhttp%253A%252F%252F127.0.0.1%252Fcallback&request_uri=urn%3Aietf%3Aparams%3Aoauth%3Arequest_uri%3Areq-abc123"
	})
	at("rendering", 1, func(p *presenter, ps *posting) {
		open(p, ps).account, ps.signedIn = account, account
		ps.start = &start{kind: startLive, job: jobRender, slide: 1}
	})
	at("checking", 1, func(p *presenter, ps *posting) {
		open(p, ps).account, ps.signedIn = account, account
		ps.start = &start{kind: startRehearsal, job: jobCheck}
		ps.rendered[1] = []Post{{}}
	})
	at("setup-failed", 1, func(p *presenter, ps *posting) {
		ps.account = account
		ps.fail("Couldn't sign in as "+account+":", errors.New("resolving the handle: no such host"))
	})
	at("setup-failed-dialog", 1, func(p *presenter, ps *posting) {
		open(p, ps).account = account
		ps.fail("Couldn't sign in as "+account+":", errors.New("resolving the handle: no such host"))
	})
	at("live", 1, func(p *presenter, ps *posting) { live(ps, false) })
	at("live-dialog", 1, func(p *presenter, ps *posting) { live(open(p, ps), false) })
	at("sent", 2, func(p *presenter, ps *posting) {
		pb := live(ps, false)
		pb.thread.Posts = []threadPost{{Slide: -1}, {Slide: 1}}
		pb.slide = 2
	})
	retrying := func(p *presenter, ps *posting) {
		pb := live(ps, false)
		pb.queue, pb.fails = []queuedPost{{-1, 0}, {1, 0}}, 3
		pb.err = "bsky: posting: API request failed (HTTP 502): UpstreamFailure"
		pb.retryAt = goldenBase.Add(20 * time.Second)
	}
	at("retrying", 1, retrying)
	at("retrying-dialog", 1, func(p *presenter, ps *posting) { retrying(p, open(p, ps)) })
	at("paused", 1, func(p *presenter, ps *posting) {
		pb := live(ps, false)
		pb.on, pb.queue = false, []queuedPost{{-1, 0}, {1, 0}}
	})
	at("save-error", 1, func(p *presenter, ps *posting) {
		live(ps, false).saveErr = "rename thread.json.tmp thread.json: permission denied"
	})
	at("save-error-dialog", 1, func(p *presenter, ps *posting) {
		live(open(p, ps), false).saveErr = "rename thread.json.tmp thread.json: permission denied"
	})
	at("rehearsing", 1, func(p *presenter, ps *posting) {
		live(open(p, ps), true).file = "/nonexistent/decker/talks/deck-test/rehearsal/dry-run.md"
		ps.account, ps.signedIn = "", ""
		ps.say(msgLine{text: "▲ Posting for real would stop here:", tone: toneWarn},
			msgLine{text: "bsky: slide 2's post \"the builds\": 11 images, but a post takes up to 10", detail: true},
			msgLine{text: "The rehearsal goes on regardless."})
	})
	at("rehearsal-paused", 1, func(p *presenter, ps *posting) { live(ps, true).on = false })
	at("quit-armed", 1, func(p *presenter, ps *posting) {
		live(ps, false).queue = []queuedPost{{1, 0}}
		p.quitArmed = goldenBase.Add(3 * time.Second)
	})
	at("waiting-dialog", -1, func(p *presenter, ps *posting) { open(p, ps) })
	at("keys", 1, func(p *presenter, _ *posting) { p.dialog = dialogKeys })
	return out
}
