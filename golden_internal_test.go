package decker

// Golden hashes for the engine paths the deck-level golden (gallery_test.go)
// cannot reach, because they are unexported: cell and video transitions,
// the video frame loop, PNG export, the live model's screen, the terminal
// writer's escape sequences, the presenter view and kitty previews.
//
// Each entry is "key sha256-hex" in testdata/internal.golden. Record with
//
//	UPDATE_GOLDEN=1 go test -run TestInternalGolden
//
// and only when a change in output is intended. Everything here is
// deterministic: fixed times, no clock, no rand, no ffmpeg, no sockets.
// (PNG and kitty hashes depend on the Go standard library's image/png
// encoder, so a toolchain upgrade that changes its output re-records them.)

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const internalGoldenPath = "testdata/internal.golden"

// ---- the hash file ----

type goldenEntries struct {
	keys []string
	sums map[string]string
}

func newGoldenEntries() *goldenEntries { return &goldenEntries{sums: map[string]string{}} }

// add records the hash of data under key. Keys must be unique.
func (g *goldenEntries) add(key string, data ...[]byte) {
	if strings.ContainsAny(key, " \n") {
		panic("golden key has whitespace: " + key)
	}
	if _, dup := g.sums[key]; dup {
		panic("duplicate golden key " + key)
	}
	h := sha256.New()
	for _, d := range data {
		h.Write(d)
		h.Write([]byte{0}) // so ("ab","c") and ("a","bc") differ
	}
	g.keys = append(g.keys, key)
	g.sums[key] = hex.EncodeToString(h.Sum(nil))
}

func (g *goldenEntries) addString(key string, parts ...string) {
	bs := make([][]byte, len(parts))
	for i, p := range parts {
		bs[i] = []byte(p)
	}
	g.add(key, bs...)
}

// addPix hashes a framebuffer exactly, down to the float bits.
func (g *goldenEntries) addPix(key string, p *Pixels) {
	buf := make([]byte, 0, 12*len(p.Pix)+16)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(p.W))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(p.H))
	for _, c := range p.Pix {
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(c.R))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(c.G))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(c.B))
	}
	g.add(key, buf)
}

func (g *goldenEntries) check(t *testing.T, path string) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		var b strings.Builder
		b.WriteString("# Internal golden hashes: golden_internal_test.go. Regenerate with UPDATE_GOLDEN=1 go test -run TestInternalGolden.\n")
		for _, k := range g.keys {
			b.WriteString(k + " " + g.sums[k] + "\n")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d hashes in %s", len(g.keys), path)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v (record them with UPDATE_GOLDEN=1)", err)
	}
	defer f.Close()
	want := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("%s: bad line %q", path, line)
		}
		want[k] = v
	}
	bad := 0
	report := func(format string, args ...any) {
		if bad++; bad <= 40 { // a broken refactor shouldn't print thousands of lines
			t.Errorf(format, args...)
		}
	}
	for _, k := range g.keys {
		switch w, ok := want[k]; {
		case !ok:
			report("%s: new (record it with UPDATE_GOLDEN=1)", k)
		case w != g.sums[k]:
			report("%s: output changed", k)
		}
	}
	for k := range want {
		if _, ok := g.sums[k]; !ok {
			report("%s: gone", k)
		}
	}
	if bad > 40 {
		t.Errorf("... and %d more", bad-40)
	}
}

// ---- fixtures ----

var goldenBase = time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

// wideSlide has wide characters (CJK, emoji) at odd and even columns, rich
// text attributes, a Lip Gloss box and a gradient: what fixWideEdges and
// the cell writer have to get right.
func wideSlide() Slide {
	return Slide{Title: "Wide", Notes: "wide characters\nand a second line", Steps: 2, Transition: TransitionWipe,
		View: func(c Ctx) string {
			sc := c.Scene()
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
			return sc.Render()
		}}
}

// plainSlide draws no Scene: the engine decodes its styled text.
func plainSlide() Slide {
	return Slide{Title: "Plain", View: func(c Ctx) string {
		st := lipgloss.NewStyle().Foreground(c.Theme.Good.Color()).Background(c.Theme.Panel.Color())
		return st.Render("plain text, no scene") + "\n\x1b[1;4mbold underline\x1b[m\n界 wide\n" + strings.Repeat("x", c.W+10)
	}}
}

func extraSlides() []Slide {
	return []Slide{
		wideSlide(),
		plainSlide(),
		{Title: "Nil view"},
		{Title: "Panics", View: func(Ctx) string { panic("on purpose") }},
	}
}

// glyphSlide is a frame of text with every block element, quadrant and
// box-drawing character the PNG exporter draws by hand, plus ordinary text.
func glyphSlide() Slide {
	return Slide{Title: "Glyphs", View: func(c Ctx) string {
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
		return strings.Join(lines, "\n")
	}}
}

// slideFrame draws a slide as a frame string, as the deck does.
func slideFrame(s Slide, w, h int, t float64, step int) string {
	return renderSlide(s, Ctx{W: w, H: h, T: t, Step: step, StepT: t, Theme: testTheme})
}

// rgbFrame draws a slide through the video pipeline (canvas pixels, then
// toRGB24), at w×h video pixels.
func rgbFrame(s Slide, w, h int, t float64, step int) []byte {
	out := make([]byte, 3*w*h)
	captureFrame = func(p *Pixels) {
		if p.W == w && p.H == h {
			toRGB24(p, out)
		}
	}
	defer func() { captureFrame = nil }()
	renderSlide(s, Ctx{W: w, H: h / 2, T: t, Step: step, StepT: t, Theme: testTheme})
	return out
}

type hashWriter struct {
	h      hash.Hash
	frames int
	size   int
}

func (w *hashWriter) Write(b []byte) (int, error) {
	if w.size != 0 && len(b) != w.size {
		panic("partial frame")
	}
	w.frames++
	w.h.Write(b)
	return len(b), nil
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
	TransitionDissolve: "dissolve", TransitionWipe: "wipe",
}

var transitionKinds = []Transition{TransitionDefault, TransitionNone, TransitionPush, TransitionDissolve, TransitionWipe}

// ---- the test ----

func TestInternalGolden(t *testing.T) {
	if testing.Short() {
		t.Skip("internal golden hashes take a few seconds; skipped with -short")
	}
	g := newGoldenEntries()
	for _, part := range []struct {
		name string
		run  func(*testing.T, *goldenEntries)
	}{
		{"cellTransitions", goldenCellTransitions},
		{"videoTransitions", goldenVideoTransitions},
		{"videoFrames", goldenVideoFrames},
		{"png", goldenPNG},
		{"model", goldenModel},
		{"modelTransitions", goldenModelTransitions},
		{"termWriter", goldenTermWriter},
		{"presenter", goldenPresenter},
		{"kitty", goldenKitty},
		{"misc", goldenMisc},
	} {
		t.Run(part.name, func(t *testing.T) { part.run(t, g) })
	}
	if !t.Failed() {
		g.check(t, internalGoldenPath)
	}
}

func goldenCellTransitions(t *testing.T, g *goldenEntries) {
	deck := testDeck().Slides
	wide := wideSlide()
	pairs := []struct {
		name         string
		from, to     Slide
		fstep, tstep int
	}{
		{"title-wide", deck[0], wide, 0, 1},
		{"wide-diagram", wide, deck[2], 0, 3},
	}
	ps := []float64{0, 0.1, 0.33, 0.5, 0.77, 1}
	for _, size := range [][2]int{{80, 24}, {240, 67}} {
		w, h := size[0], size[1]
		for _, pr := range pairs {
			from := slideFrame(pr.from, w, h, Settled, pr.fstep)
			to := slideFrame(pr.to, w, h, 0.4, pr.tstep)
			g.addString(fmt.Sprintf("cells/frame/%dx%d/%s/from", w, h, pr.name), from)
			g.addString(fmt.Sprintf("cells/frame/%dx%d/%s/to", w, h, pr.name), to)
			for _, k := range transitionKinds {
				for _, fwd := range []bool{true, false} {
					for _, p := range ps {
						out := composeTransition(k, from, to, w, h, p, fwd, testTheme)
						g.addString(fmt.Sprintf("cells/%dx%d/%s/%s/fwd=%v/p=%v", w, h, pr.name, transitionNames[k], fwd, p), out)
					}
				}
			}
		}
	}
	// composeGrid on the grids the live deck builds (wide flags set by the
	// scene, not parsed back from a string), without releasing them.
	w, h := 120, 34
	a := renderSlideGrid(deck[0], Ctx{W: w, H: h, T: Settled, Theme: testTheme})
	b := renderSlideGrid(wide, Ctx{W: w, H: h, T: 0.4, Step: 1, StepT: 0.4, Theme: testTheme})
	for _, k := range transitionKinds {
		for _, fwd := range []bool{true, false} {
			for _, p := range ps {
				out := composeGrid(k, a, b, p, fwd, testTheme)
				g.addString(fmt.Sprintf("cells/grid/%s/fwd=%v/p=%v", transitionNames[k], fwd, p), out.String())
				out.release()
			}
		}
	}
	// A degenerate size returns the new frame unchanged.
	g.addString("cells/degenerate", composeTransition(TransitionPush, "a", "b", 0, 0, 0.5, true, testTheme))
}

func goldenVideoTransitions(t *testing.T, g *goldenEntries) {
	deck := testDeck().Slides
	wide := wideSlide()
	ps := []float64{0, 0.1, 0.33, 0.5, 0.77, 1}
	for _, size := range [][2]int{{160, 90}, {800, 450}} {
		w, h := size[0], size[1]
		from := rgbFrame(deck[0], w, h, Settled, 0)
		to := rgbFrame(wide, w, h, 0.4, 1)
		g.add(fmt.Sprintf("video/frame/%dx%d/from", w, h), from)
		g.add(fmt.Sprintf("video/frame/%dx%d/to", w, h), to)
		for _, k := range transitionKinds {
			for _, p := range ps {
				mix := append([]byte(nil), to...)
				blendTransition(k, from, mix, w, h, p, testTheme)
				g.add(fmt.Sprintf("video/%dx%d/%s/p=%v", w, h, transitionNames[k], p), mix)
			}
		}
	}
}

func goldenVideoFrames(t *testing.T, g *goldenEntries) {
	run := func(name string, d *Deck, o videoOptions) {
		hw := &hashWriter{h: sha256.New(), size: 3 * o.width * o.height}
		if err := writeVideoFrames(d, o, hw); err != nil {
			t.Fatal(err)
		}
		if captureFrame != nil {
			t.Fatal("video rendering left the capture hook on")
		}
		g.addString("videoloop/"+name+"/frames", fmt.Sprint(hw.frames))
		g.add("videoloop/"+name+"/stream", hw.h.Sum(nil))
	}
	d := testDeck()
	run("testdeck-160x90", d, videoOptions{width: 160, height: 90, fps: 10, hold: 0.3, first: 0, last: 2})
	run("testdeck-tail", d, videoOptions{width: 200, height: 112, fps: 8, hold: 0.25, first: 2, last: 4})
	extra := &Deck{Name: "extra", Theme: testTheme, Slides: append(testDeck().Slides[:1], extraSlides()...)}
	extra.Slides[2].Hold = 0.5 // a slide that asks for longer
	run("extras-160x90", extra, videoOptions{width: 160, height: 90, fps: 10, hold: 0.2, first: 0, last: 4})

	for i, s := range append(testDeck().Slides, wideSlide()) {
		for step := 0; step < s.steps(); step++ {
			g.addString(fmt.Sprintf("videoTiming/%d/%d", i, step),
				fmt.Sprint(videoTiming(s, step, 1), videoTiming(s, step, 0), videoTiming(Slide{Hold: 3}, step, 1)))
		}
	}
	for _, bad := range []videoOptions{{width: 0, height: 10}, {width: 10, height: 11}} {
		g.addString(fmt.Sprintf("videoErr/%dx%d", bad.width, bad.height), fmt.Sprint(renderVideo(testDeck(), bad)))
	}
}

func goldenPNG(t *testing.T, g *goldenEntries) {
	dir := t.TempDir()
	hashFile := func(key, path string) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		g.add(key, b)
	}
	slides := append(testDeck().Slides, extraSlides()[:2]...)
	slides = append(slides, glyphSlide())
	for _, size := range [][2]int{{80, 24}, {160, 45}} {
		w, h := size[0], size[1]
		var frames []string
		for i, s := range slides {
			steps := s.steps()
			fr := slideFrame(s, w, h, Settled, steps-1)
			frames = append(frames, fr)
			img := frameImage(fr, w, h, testTheme)
			g.add(fmt.Sprintf("png/image/%dx%d/%d", w, h, i), img.Pix, []byte(fmt.Sprint(img.Bounds())))
			path := filepath.Join(dir, fmt.Sprintf("s%d-%dx%d.png", i, w, h))
			if err := writePNG(fr, w, h, path, testTheme); err != nil {
				t.Fatal(err)
			}
			hashFile(fmt.Sprintf("png/file/%dx%d/%d", w, h, i), path)
			g.addPix(fmt.Sprintf("png/framePixels/%dx%d/%d", w, h, i), framePixels(fr, w, h, testTheme))
		}
		for _, shrink := range []int{1, 4} {
			path := filepath.Join(dir, fmt.Sprintf("sheet-%dx%d-%d.png", w, h, shrink))
			if err := writeSheet(frames, w, h, 4, shrink, path, testTheme); err != nil {
				t.Fatal(err)
			}
			hashFile(fmt.Sprintf("png/sheet/%dx%d/shrink=%d", w, h, shrink), path)
		}
	}
	// A sheet with fewer frames than columns.
	path := filepath.Join(dir, "sheet2.png")
	fr := slideFrame(slides[0], 80, 24, Settled, 0)
	if err := writeSheet([]string{fr, fr}, 80, 24, 4, 2, path, testTheme); err != nil {
		t.Fatal(err)
	}
	hashFile("png/sheet/two-frames", path)
	// Errors from an unwritable path are reported, not panicked.
	g.addString("png/err", fmt.Sprint(writePNG(fr, 80, 24, filepath.Join(dir, "no", "such", "dir.png"), testTheme) != nil))
}

func goldenModel(t *testing.T, g *goldenEntries) {
	d := testDeck()
	devs := map[string]func() *devState{
		"plain":    func() *devState { return nil },
		"dev":      func() *devState { return &devState{} },
		"building": func() *devState { return &devState{building: true} },
		"failed": func() *devState {
			return &devState{buildErr: "./main.go:12:3: undefined: nope\n./main.go:13:1: syntax error\n" + strings.Repeat("more output\n", 30)}
		},
	}
	view := func(m model) string { return m.View().Content }
	key := func(m model, dev string, extra string) string {
		return fmt.Sprintf("model/%s/%dx%d/s%d.%d/%s", dev, m.w, m.h, m.idx, m.step, extra)
	}
	for _, size := range [][2]int{{80, 24}, {200, 56}} {
		for _, dev := range []string{"plain", "dev"} {
			for idx, s := range d.Slides {
				stepList := []int{0}
				if s.steps() > 1 {
					stepList = append(stepList, s.steps()-1)
				}
				for _, step := range stepList {
					for _, age := range []float64{0.3, Settled} {
						m := testModel(d, idx, step, size[0], size[1], age, devs[dev]())
						g.addString(key(m, dev, fmt.Sprintf("age=%v", age)), view(m))
					}
				}
			}
		}
		// Footer states in dev mode.
		for _, dev := range []string{"building", "failed"} {
			m := testModel(d, 2, 2, size[0], size[1], Settled, devs[dev]())
			g.addString(key(m, dev, "footer"), view(m))
			m = press(m, "n")
			g.addString(key(m, dev, "footer+notes"), view(m))
		}
		m := testModel(d, 2, 1, size[0], size[1], Settled, devs["dev"]())
		m.count = "12"
		g.addString(key(m, "dev", "count"), view(m))
		m.count = ""

		// Notes and help, through the keys that toggle them.
		for _, dev := range []string{"plain", "dev"} {
			for idx := range d.Slides {
				m := testModel(d, idx, 0, size[0], size[1], Settled, devs[dev]())
				m = press(m, "n")
				g.addString(key(m, dev, "notes"), view(m))
				m = press(m, "?")
				g.addString(key(m, dev, "notes+help"), view(m))
				m = press(m, "n")
				g.addString(key(m, dev, "help"), view(m))
				m = press(m, "esc")
				g.addString(key(m, dev, "after-esc"), view(m))
			}
		}
		// Long notes get cut off.
		long := *d
		long.Slides = append([]Slide(nil), d.Slides...)
		long.Slides[0].Notes = strings.Repeat("a long line of speaker notes that wraps\n", 30)
		m = testModel(&long, 0, 0, size[0], size[1], Settled, nil)
		m.showNotes = true
		g.addString(key(m, "plain", "long-notes"), view(m))
	}

	// The key handler walks the deck: record where each key lands.
	m := testModel(d, 0, 0, 120, 36, 0, nil)
	var trail strings.Builder
	for _, k := range strings.Fields("right right right right right right right left left [ ] 3 g 1 2 enter G g home end h l space pgdn pgup j k down up enter backspace r n ? esc q ctrl+l 99 g 0 g") {
		m = press(m, k)
		fmt.Fprintf(&trail, "%s -> s%d.%d count=%q notes=%v help=%v trans=%d fwd=%v from=%v\n",
			k, m.idx, m.step, m.count, m.showNotes, m.showHelp, m.trans, m.transFwd, m.transFrom != nil)
	}
	g.addString("model/keys", trail.String())
	st, _ := json.Marshal(m.linkState())
	g.add("model/linkState", st)
	for _, cmd := range []linkCmd{{Cmd: "next"}, {Cmd: "goto", Slide: 4}, {Cmd: "prevSlide"}, {Cmd: "bogus"}, {Cmd: "last"}, {Cmd: "replay"}} {
		next, _ := m.Update(cmd)
		m = next.(model)
		b, _ := json.Marshal(cmd)
		g.addString("model/link/"+string(b), fmt.Sprintf("s%d.%d", m.idx, m.step), view(m))
	}
	b, _ := json.Marshal(linkOutline{"t", 2})
	g.add("link/outline", b)
	var keys []string
	for k, v := range linkKeys {
		keys = append(keys, k+"="+v)
	}
	sort.Strings(keys)
	g.addString("link/keys", strings.Join(keys, ","))

	// Messages through Update: sizes, ticks, link commands, builds.
	um := testModel(d, 0, 0, 0, 0, 0, &devState{})
	var upd strings.Builder
	send := func(msg tea.Msg) {
		next, cmd := um.Update(msg)
		um = next.(model)
		fmt.Fprintf(&upd, "%T s%d.%d %dx%d cmd=%v from=%v now=%v build=%+v exec=%q\n", msg, um.idx, um.step, um.w, um.h, cmd != nil,
			um.transFrom != nil, um.now.Sub(goldenBase), *um.dev, um.execOnQuit)
	}
	send(tea.WindowSizeMsg{Width: 0, Height: 0}) // Bubble Tea's placeholder: ignored
	send(tea.WindowSizeMsg{Width: 90, Height: 30})
	send(tickMsg(goldenBase.Add(time.Second)))
	um.goTo(1, 0, true)
	send(tickMsg(um.transStart.Add(100 * time.Millisecond)))
	send(tickMsg(um.transStart.Add(600 * time.Millisecond))) // the transition is over
	send(tea.KeyPressMsg{Code: 'l', Text: "l"})
	send(tea.KeyPressMsg{Code: tea.KeyRight})
	send(linkCmd{Cmd: "goto", Slide: 1})
	send(linkCmd{Cmd: "bogus"})
	send(buildDoneMsg{out: "  oops \n", err: fmt.Errorf("exit 1")})
	send(buildDoneMsg{err: fmt.Errorf("exit 2")})
	send(buildDoneMsg{})
	send(struct{}{})
	g.addString("model/update", upd.String(), view(um))
	g.addString("model/cur", um.cur().Title, fmt.Sprint(um.bodyHeight(), um.showChrome()))

	// A zero-size model draws nothing.
	g.addString("model/empty", view(testModel(d, 0, 0, 0, 0, 0, nil)))
	// A live model returns an empty view (the writer owns the screen).
	live := testModel(d, 0, 0, 80, 24, 0, nil)
	live.live = &termWriter{}
	g.addString("model/live", view(live))
}

// press sends a key to the model.
func press(m model, k string) model {
	next, _ := m.handleKey(k)
	return next.(model)
}

func goldenModelTransitions(t *testing.T, g *goldenEntries) {
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
		{"push-fwd-dev", 0, 0, 1, 0, true, true},
	}
	for _, size := range [][2]int{{80, 24}, {200, 56}} {
		for _, tc := range cases {
			for _, p := range []float64{0, 0.2, 0.5, 0.8, 1.2} {
				var dev *devState
				if tc.dev {
					dev = &devState{}
				}
				m := testModel(d, tc.from, tc.fstep, size[0], size[1], Settled, dev)
				m.goTo(tc.to, tc.tstep, tc.forward)
				m.now = goldenBase.Add(time.Duration(p * TransitionDuration * float64(time.Second)))
				if m.transFrom != nil && m.now.Sub(m.transStart).Seconds() >= TransitionDuration {
					m.transFrom = nil // what the tick does
				}
				g.addString(fmt.Sprintf("modelTrans/%dx%d/%s/p=%v", size[0], size[1], tc.name, p),
					fmt.Sprint(m.trans, m.transFwd, m.transFrom != nil), m.View().Content)
			}
		}
	}
	// goTo to the same place does nothing; a size change drops the
	// transition's source frame (it no longer fits).
	m := testModel(d, 1, 0, 80, 24, Settled, nil)
	m.goTo(1, 0, true)
	m.goTo(2, 0, true)
	m.w, m.h = 100, 30
	m.now = goldenBase.Add(100 * time.Millisecond)
	g.addString("modelTrans/resized", m.View().Content)
}

func goldenTermWriter(t *testing.T, g *goldenEntries) {
	d := testDeck()
	var out bytes.Buffer
	tw := &termWriter{out: &out}
	hashOut := func(key string) {
		g.add(key, out.Bytes())
		out.Reset()
	}
	w, h := 100, 30
	m := testModel(d, 0, 0, w, h, 0, nil)
	for f := 0; f < 140; f++ {
		m.now = goldenBase.Add(time.Duration(f) * time.Second / 60)
		if f == 40 {
			m.goTo(1, 0, true) // a Push transition to slide 2
		}
		if f == 100 {
			m.goTo(2, 0, true) // and a Wipe, to slide 3
		}
		if m.transFrom != nil && m.now.Sub(m.transStart).Seconds() >= TransitionDuration {
			m.transFrom = nil
		}
		fr := m.frame()
		tw.write(fr.clone(), m.cur().Title, false)
		fr.release()
		if f%10 == 0 || f == 139 {
			hashOut(fmt.Sprintf("termwriter/frame/%03d", f))
		}
	}
	out.Reset()

	// A frame that doesn't change writes nothing; one that changes a
	// little writes only that.
	fr := m.frame()
	tw.write(fr.clone(), m.cur().Title, false)
	hashOut("termwriter/same-first")
	tw.write(fr.clone(), m.cur().Title, false)
	hashOut("termwriter/same-again")
	small := fr.clone()
	small.at(3, 4).ch, small.at(3, 4).fg = "x", [3]uint8{1, 2, 3}
	small.at(9, 4).bg = [3]uint8{9, 9, 9} // within the rewrite gap of the first
	small.at(60, 20).ch = "▀"             // far away: a cursor move
	small.at(w-1, h-1).bg = [3]uint8{5, 6, 7}
	tw.write(small.clone(), m.cur().Title, false)
	hashOut("termwriter/small-change")
	// A new title is sent even when the cells are the same.
	tw.write(small.clone(), "A new title", false)
	hashOut("termwriter/title-only")
	// invalidate (ctrl+l) redraws everything.
	tw.write(small.clone(), "A new title", true)
	hashOut("termwriter/forced-full")

	// A resize redraws everything at the new size.
	m.w, m.h = 70, 20
	fr = m.frame()
	tw.write(fr.clone(), m.cur().Title, false)
	hashOut("termwriter/resize")

	// Wide characters survive partial updates.
	vt := &termWriter{out: &out}
	for i, line := range []string{"ab界cd", "ab世cd", "a界界cd", "界b界cd", "abcde", "日本語ab", "ab🎉cd"} {
		vt.write(parseGrid(line, 10, 1, testTheme), "", false)
		hashOut(fmt.Sprintf("termwriter/wide/%d", i))
	}
	// Rich attributes (bold, italic, underline, reverse) on a changing frame.
	at := &termWriter{out: &out}
	for i, step := range []int{0, 1, 0} {
		fr := renderSlideGrid(wideSlide(), Ctx{W: 60, H: 20, T: Settled, Step: step, StepT: 0.2, Theme: testTheme})
		at.write(fr, "Wide", false)
		hashOut(fmt.Sprintf("termwriter/attrs/%d", i))
	}

	// The writer's own lifecycle sequences: set-up and restore.
	var life bytes.Buffer
	lw := newTermWriter(&life, testTheme.Background)
	lw.close()
	g.add("termwriter/lifecycle", life.Bytes())
	g.addString("termwriter/hex2", hex2(0), hex2(15), hex2(16), hex2(255))
	g.addString("termwriter/moveTo", string(moveTo(nil, 0, 0)), string(moveTo(nil, 681, 170)))
}

func presenterFor(d *Deck, w, h int, linked bool, slide, step int, previews map[previewKey]string) presenter {
	slides := d.Slides
	outline := make([]linkOutline, len(slides))
	for i, s := range slides {
		outline[i] = linkOutline{s.Title, s.steps()}
	}
	p := presenter{slides: slides, theme: testTheme, sty: testTheme.styles(), socket: "/tmp/x.sock", length: 30 * time.Minute,
		link: &linkClient{}, previews: previews, now: goldenBase, w: w, h: h, linked: linked}
	if slide >= 0 {
		p.st = linkState{Slide: slide, Step: step, Outline: outline, Notes: slides[slide].Notes, W: 682, H: 171}
	}
	return p
}

func goldenPresenter(t *testing.T, g *goldenEntries) {
	d := testDeck()
	d.Slides = append(d.Slides, wideSlide()) // 6 slides; the last has notes and steps
	previews := map[previewKey]string{}      // shared: a preview doesn't depend on who asks
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
	for _, size := range [][2]int{{60, 15}, {80, 24}, {120, 36}, {200, 50}} {
		w, h := size[0], size[1]
		key := func(s string) string { return fmt.Sprintf("presenter/%dx%d/%s", w, h, s) }
		g.addString(key("waiting"), presenterFor(d, w, h, false, -1, 0, previews).View().Content)
		g.addString(key("waiting-linked"), presenterFor(d, w, h, true, -1, 0, previews).View().Content)
		for _, at := range []struct{ slide, step int }{{0, 0}, {1, 1}, {2, 0}, {2, 3}, {4, 0}, {5, 1}} {
			for _, tm := range timers {
				if tm.name != "running" && at.slide != 2 {
					continue
				}
				p := presenterFor(d, w, h, true, at.slide, at.step, previews)
				p.running, p.since, p.banked = tm.running, goldenBase.Add(-tm.since), tm.banked
				g.addString(key(fmt.Sprintf("s%d.%d/%s", at.slide, at.step, tm.name)), p.View().Content)
			}
		}
		p := presenterFor(d, w, h, false, 1, 1, previews)
		p.running, p.since = true, goldenBase.Add(-7*time.Minute)
		g.addString(key("unlinked"), p.View().Content)
		p.linked, p.count = true, "12"
		g.addString(key("count"), p.View().Content)
		// A deck that no longer matches this build's slides: out-of-date previews.
		p = presenterFor(d, w, h, true, 1, 0, previews)
		p.st.Outline = append([]linkOutline(nil), p.st.Outline...)
		p.st.Outline[1].Title = "Renamed"
		p.st.Outline[2].Title = "Also renamed"
		g.addString(key("stale"), p.View().Content)
		// A different deck shape changes the preview boxes.
		p = presenterFor(d, w, h, true, 3, 0, previews)
		p.st.W, p.st.H = 120, 40
		g.addString(key("small-deck"), p.View().Content)
		p.st.W, p.st.H = 0, 0 // unreported: the default shape
		g.addString(key("no-deck-size"), p.View().Content)
		// Long notes are cut to fit.
		p = presenterFor(d, w, h, true, 0, 0, previews)
		p.st.Notes = strings.Repeat("a long line of speaker notes, which wraps over several lines\n", 20)
		g.addString(key("long-notes"), p.View().Content)
		// Image previews: placeholders for uploaded images, and the same
		// view when only some are uploaded.
		p = presenterFor(d, w, h, true, 1, 1, previews)
		p.images = newKittyImages()
		if pw, ph := p.previewBox(); ph > 0 {
			dw, dh := p.deckSize()
			p.images.add(previewKey{1, 1, pw, ph, dw, dh})
			g.addString(key("image-one"), p.View().Content)
			p.images.add(previewKey{1, 2, pw, ph, dw, dh})
			g.addString(key("image-both"), p.View().Content)
		} else {
			g.addString(key("image-none"), p.View().Content)
		}
	}
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
	psend(kittyUploadMsg{})
	psend(struct{}{})
	g.addString("presenter/update", pu.String(), pm.View().Content)

	// A zero-size presenter draws nothing.
	g.addString("presenter/empty", presenterFor(d, 0, 0, true, 0, 0, previews).View().Content)

	// The pieces.
	outline := []linkOutline{{"a", 1}, {"b", 4}, {"c", 1}}
	for _, c := range []struct {
		slide, step int
		el          time.Duration
	}{{0, 0, 0}, {1, 2, 15 * time.Minute}, {1, 2, 10 * time.Minute}, {0, 0, 5 * time.Minute}, {2, 0, 29 * time.Minute}} {
		g.addString(fmt.Sprintf("presenter/pace/%d.%d/%v", c.slide, c.step, c.el), fmt.Sprint(pace(outline, c.slide, c.step, c.el, 30*time.Minute)))
	}
	g.addString("presenter/pace/degenerate", fmt.Sprint(pace(nil, 0, 0, time.Minute, time.Minute), pace(outline, 0, 0, time.Minute, 0)))
	g.addString("presenter/clock", clock(0), clock(59*time.Second), clock(61*time.Second), clock(-90*time.Second), clock(3601*time.Second), clock(1499*time.Millisecond), clock(1500*time.Millisecond))
	g.addString("presenter/text", spread("left", "right", 20), spread("left", "right", 8), cutAt("abcdefghij", 10, 3, "X"), indent("a\nb", "  "), truncate("abcdef", 3), truncateLeft("abcdef", 3), truncate("abc", -1))
	p := presenterFor(d, 100, 30, true, 1, 1, previews)
	g.addString("presenter/placeholder", p.placeholder(20, 5, "message"))
	g.addString("presenter/frame", frame("a\nb", testTheme.Accent))
	for _, i := range []int{0, 2, 5} {
		for _, step := range []int{0, 1} {
			g.addString(fmt.Sprintf("presenter/renderPreview/%d.%d", i, step),
				renderPreview(d.Slides[i], step, 682, 171, 50, 12, testTheme),
				renderPreview(d.Slides[i], step, 120, 40, 30, 9, testTheme))
		}
	}
	g.addString("presenter/nextTarget", func() string {
		var b strings.Builder
		for _, at := range [][2]int{{0, 0}, {1, 3}, {5, 1}, {4, 0}, {9, 0}} {
			p := presenterFor(d, 100, 30, true, min(at[0], 5), at[1], previews)
			p.st.Slide = at[0] // may be past the end: it is clamped
			n, label, ok := p.nextTarget()
			fmt.Fprintln(&b, n, label, ok)
		}
		return b.String()
	}())
	for _, sz := range [][4]int{{100, 30, 682, 171}, {40, 12, 682, 171}, {200, 60, 120, 40}, {30, 9, 100, 100}, {120, 5, 682, 171}} {
		p := presenterFor(d, sz[0], sz[1], true, 1, 1, previews)
		p.st.W, p.st.H = sz[2], sz[3]
		pw, ph := p.previewBox()
		g.addString(fmt.Sprintf("presenter/previewBox/%dx%d/%dx%d", sz[0], sz[1], sz[2], sz[3]), fmt.Sprint(pw, ph))
	}
	for _, s := range []string{"image", "cells", "auto", "bogus"} {
		for ei, env := range []map[string]string{{}, {"TMUX": "x"}, {"TERM_PROGRAM": "ghostty"}, {"TERM": "xterm-kitty"}, {"ZELLIJ": "0", "KITTY_WINDOW_ID": "3"}, {"KITTY_WINDOW_ID": "3"}} {
			for _, k := range []string{"TMUX", "ZELLIJ", "TERM_PROGRAM", "TERM", "KITTY_WINDOW_ID"} {
				t.Setenv(k, "")
			}
			for k, v := range env {
				t.Setenv(k, v)
			}
			g.addString(fmt.Sprintf("presenter/imagePreviews/%s/env%d", s, ei), fmt.Sprint(imagePreviews(s)))
		}
	}
}

func goldenKitty(t *testing.T, g *goldenEntries) {
	d := testDeck()
	for i, s := range d.Slides {
		img := slideImage(s, s.steps()-1, 120, 36, testTheme)
		g.add(fmt.Sprintf("kitty/slideImage/%d", i), img.Pix, []byte(fmt.Sprint(img.Bounds())))
		seq, err := kittyTransmit(i+1, img, 30, 8)
		if err != nil {
			t.Fatal(err)
		}
		g.addString(fmt.Sprintf("kitty/transmit/%d", i), seq)
	}
	img := slideImage(wideSlide(), 1, 240, 67, testTheme)
	g.add("kitty/slideImage/wide-240x67", img.Pix)
	seq, err := kittyTransmit(255, img, 60, 17)
	if err != nil {
		t.Fatal(err)
	}
	g.addString("kitty/transmit/wide-240x67", seq)
	for _, sz := range [][3]int{{7, 40, 12}, {1, 1, 1}, {255, 5, 3}, {3, 300, 300}} {
		g.addString(fmt.Sprintf("kitty/placeholders/%d/%dx%d", sz[0], sz[1], sz[2]), kittyPlaceholders(sz[0], sz[1], sz[2]))
	}
	g.addString("kitty/delete", kittyDelete(1), kittyDelete(255))

	ki := newKittyImages()
	var trace strings.Builder
	for i := 0; i < 258; i++ {
		id, free := ki.add(previewKey{slide: i})
		if i < 3 || i > 252 {
			fmt.Fprintf(&trace, "add %d -> %d %q\n", i, id, free)
		}
	}
	_, ok := ki.id(previewKey{slide: 0})
	id3, ok3 := ki.id(previewKey{slide: 3})
	fmt.Fprintln(&trace, ok, id3, ok3, ki.finish(kittyUploadMsg{gen: 0, key: previewKey{slide: 3}, seq: "S"}),
		ki.finish(kittyUploadMsg{gen: 1, key: previewKey{slide: 3}, seq: "S"}),
		ki.finish(kittyUploadMsg{gen: 0, key: previewKey{slide: 4}, seq: ""}),
		ki.finish(kittyUploadMsg{gen: 0, key: previewKey{slide: 999}, seq: "S"}))
	_, ok4 := ki.id(previewKey{slide: 4})
	fmt.Fprintln(&trace, ok4)
	clear := ki.clear()
	fmt.Fprintln(&trace, len(clear), ki.gen, len(ki.ids))
	g.addString("kitty/images", trace.String(), clear)

	// Presenter upload: the commands it would run, as targets (not run).
	p := presenterFor(d, 120, 36, true, 1, 0, map[previewKey]string{})
	p.images = newKittyImages()
	g.addString("kitty/upload-cmds", fmt.Sprint(p.upload() == nil, presenterFor(d, 120, 36, true, -1, 0, nil).upload() == nil))
}

// goldenMisc pins the small engine pieces nothing above reaches directly:
// fit/narrowWidth, the panic screen, grids and the off-screen Render.
func goldenMisc(t *testing.T, g *goldenEntries) {
	for i, c := range []struct {
		s    string
		w, h int
	}{
		{"abc\ndef\nghi", 2, 2},
		{"abc", 5, 3},
		{"界界界界\nab", 5, 2},
		{"\x1b[31mred\x1b[m and more text\nx", 6, 1},
		{"", 3, 3},
		{"🎉🎉🎉", 3, 1},
	} {
		g.addString(fmt.Sprintf("misc/fit/%d", i), fit(c.s, c.w, c.h))
	}
	for i, s := range []string{"plain", "\x1b[1mbold\x1b[m", "界", "─│█▀", "\x1b]0;x", "\x1b[31m", "é", "→←"} {
		n, ok := narrowWidth(s)
		g.addString(fmt.Sprintf("misc/narrowWidth/%d", i), fmt.Sprint(n, ok))
	}
	g.addString("misc/panic", renderSlide(Slide{Title: "boom", View: func(Ctx) string { panic("on purpose") }}, Ctx{W: 60, H: 10, Theme: testTheme}))
	g.addString("misc/nilview", renderSlide(Slide{Title: "nil"}, Ctx{W: 20, H: 4, Theme: testTheme}))
	g.addString("misc/opaque", opaque("hi\n界", 10, 3, testTheme))

	// Off-screen Render (outside any capture hook), with the character layer.
	off := NewScene(24, 5, testTheme)
	off.Px.Disc(8, 5, 6, testTheme.Accent, 1)
	off.Px.Glow(30, 5, 10, testTheme.Accent2, 0.8)
	off.Text(2, 1, "off-screen", testTheme.Text.Color(), uv.AttrBold)
	g.addString("misc/offscreen", off.Render())
	sc := NewScene(40, 10, testTheme)
	sc.Put(2, 2, "put")
	sc.Overlay(4, 3, "over")
	g.addString("misc/scene", sc.Render())

	// Grid helpers: blank, clone, pixelCell, setUV clipping a wide char.
	bg := blankGrid(6, 2, testTheme.Background)
	bg.draw(4, 0, "界界", false, testTheme.Text) // the second one doesn't fit
	bg.draw(-1, 1, "界x", true, testTheme.Text)
	g.addString("misc/grid", bg.String(), bg.clone().String())
	c := pixelCell(testTheme.Accent, testTheme.Accent)
	c2 := pixelCell(testTheme.Accent, testTheme.Accent2)
	g.addString("misc/pixelCell", fmt.Sprintf("%+v %+v", c, c2))
	g.addString("misc/styles", fmt.Sprintf("%+v", testTheme.styles().accent.Render("x")))
	g.addString("misc/helpBox", testModel(testDeck(), 0, 0, 100, 30, 0, nil).helpBox())
	g.addString("misc/chrome", testModel(testDeck(), 2, 1, 100, 30, 0, &devState{}).chrome(),
		testModel(testDeck(), 1, 2, 60, 30, 0, &devState{}).chrome())
	g.addString("misc/defaultSocket", filepath.Base(defaultSocket("name")))
}
