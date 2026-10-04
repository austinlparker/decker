package decker

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// runPresenter runs the presenter view (-presenter) until the user quits: speaker
// notes, position, next-slide preview and a talk timer in a window of its own,
// linked to the deck over socket. Its keys drive the deck, so a clicker aimed
// at this window runs the show. images selects image previews over cells.
func runPresenter(d *Deck, socket string, length time.Duration, images bool) error {
	p := newPresenter(d, socket, length)
	if images {
		p.images = newKittyImages()
	}
	_, err := tea.NewProgram(p).Run()
	p.link.close()
	return err
}

func newPresenter(d *Deck, socket string, length time.Duration) presenter {
	return presenter{
		slides: d.Slides, theme: d.Theme, sty: d.Theme.styles(), socket: socket, length: length,
		link: &linkClient{}, previews: map[previewKey]string{}, now: time.Now(),
	}
}

type presenter struct {
	slides   []Slide // this build's slides, used only to draw previews
	theme    *Theme
	sty      styles
	socket   string
	length   time.Duration
	link     *linkClient
	previews map[previewKey]string // cell previews, drawn with half blocks
	images   *kittyImages          // nil unless previews are images (see kitty.go)

	st     linkState // the deck's last report; Outline is nil until the first
	linked bool
	w, h   int
	now    time.Time
	count  string // numeric prefix for jumps, e.g. "12g"

	// The talk timer starts itself when the deck first leaves slide 1, or
	// with t.
	running bool
	since   time.Time     // when it last started
	banked  time.Duration // time counted before the last pause
}

type (
	presTickMsg  time.Time
	linkUpMsg    struct{}
	linkDownMsg  struct{}
	linkStateMsg linkState
)

const reconnectEvery = 500 * time.Millisecond

func (p presenter) Init() tea.Cmd { return tea.Batch(p.tick(), p.connect(0)) }

func (p presenter) tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return presTickMsg(t) })
}

// connect dials the deck after wait. On failure it reports linkDownMsg,
// which schedules the next try.
func (p presenter) connect(wait time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(wait)
		if p.link.dial(p.socket) != nil {
			return linkDownMsg{}
		}
		return linkUpMsg{}
	}
}

func (p presenter) listen() tea.Cmd {
	return func() tea.Msg {
		st, err := p.link.next()
		if err != nil {
			return linkDownMsg{}
		}
		return linkStateMsg(st)
	}
}

func (p presenter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.w, p.h = msg.Width, msg.Height
		if p.images != nil {
			// Images are uploaded at their preview size; start over.
			return p, tea.Sequence(tea.Raw(p.images.clear()), p.upload())
		}
	case presTickMsg:
		p.now = time.Time(msg)
		return p, p.tick()
	case linkUpMsg:
		p.linked = true
		return p, p.listen()
	case linkDownMsg:
		p.linked = false
		p.link.close()
		return p, p.connect(reconnectEvery)
	case linkStateMsg:
		p.st = linkState(msg)
		if !p.running && p.banked == 0 && p.st.Slide > 0 {
			p.running, p.since = true, time.Now()
		}
		return p, tea.Batch(p.listen(), p.upload())
	case kittyUploadMsg:
		if p.images != nil {
			if seq := p.images.finish(msg); seq != "" {
				return p, tea.Raw(seq)
			}
		}
	case tea.KeyPressMsg:
		return p.handleKey(msg.String())
	}
	return p, nil
}

func (p presenter) handleKey(k string) (tea.Model, tea.Cmd) {
	if n, done := countKey(&p.count, k); done {
		if n > 0 {
			p.link.send(linkCmd{Slide: n})
		}
		return p, nil
	}
	switch {
	case k == "q" || k == "ctrl+c":
		if p.images != nil {
			return p, tea.Sequence(tea.Raw(p.images.clear()), tea.Quit)
		}
		return p, tea.Quit
	case k == "t":
		if p.running {
			p.banked += time.Since(p.since)
			p.running = false
		} else {
			p.running, p.since = true, time.Now()
		}
	case k == "T":
		p.running, p.banked = false, 0
	case keyActs[k].nav:
		p.link.send(linkCmd{Key: k})
	}
	return p, nil
}

func (p presenter) elapsed() time.Duration {
	if p.running {
		return p.banked + p.now.Sub(p.since)
	}
	return p.banked
}

// pace is how far ahead (positive) or behind (negative) of an even pace the
// talk is: the share of build steps shown against the share of time used.
// Steps, because a slide with four builds takes longer than a section opener.
func pace(outline []linkOutline, slide, step int, elapsed, length time.Duration) time.Duration {
	total, done := 0, step
	for i, o := range outline {
		total += o.Steps
		if i < slide {
			done += o.Steps
		}
	}
	if total == 0 || length <= 0 {
		return 0
	}
	shown := float64(done) / float64(total)
	used := float64(elapsed) / float64(length)
	return time.Duration((shown - used) * float64(length))
}

// clock formats d as m:ss.
func clock(d time.Duration) string {
	d = d.Round(time.Second)
	if d < 0 {
		d = -d
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

func (p presenter) View() tea.View {
	if p.w == 0 || p.h == 0 {
		return tea.NewView("")
	}
	view := p.mainView
	if p.st.Outline == nil {
		view = p.waitingView
	}
	v := tea.NewView(view())
	v.AltScreen = true
	v.BackgroundColor = p.theme.Background.Color()
	v.WindowTitle = "presenter"
	return v
}

func (p presenter) waitingView() string {
	msg := p.sty.accent.Render("Waiting for the deck…") + "\n\n" +
		p.sty.text.Render("Run the deck in another window.") + "\n" +
		p.sty.faint.Render(p.socket)
	mid := lipgloss.Place(p.w, max(p.h-footerLines, 1), lipgloss.Center, lipgloss.Center, msg)
	return mid + "\n" + p.footer()
}

// Layout constants for mainView, in cells.
const (
	presMargin = 2 // left and right edges
	presGutter = 3 // between the two previews
	presHeader = 2 // the header and the blank line under it
)

// inner is the width between the margins.
func (p presenter) inner() int { return max(p.w-2*presMargin, 10) }

func (p presenter) curOutline() linkOutline {
	return p.st.Outline[min(p.st.Slide, len(p.st.Outline)-1)]
}

func (p presenter) mainView() string {
	cur, inner := p.curOutline(), p.inner()
	left := p.sty.accent.Render(fmt.Sprintf("%d/%d", p.st.Slide+1, len(p.st.Outline))) + "  " + p.sty.text.Bold(true).Render(cur.Title)
	var right []string
	if !p.linked {
		right = append(right, p.sty.warn.Render("○ reconnecting to the deck…"))
	}
	if p.count != "" {
		right = append(right, p.sty.accent2.Render("go to "+p.count+"…"))
	}
	if cur.Steps > 1 {
		right = append(right, p.sty.muted.Render(fmt.Sprintf("step %d/%d ", p.st.Step+1, cur.Steps))+
			p.sty.accent.Render(strings.Repeat("●", p.st.Step+1))+p.sty.faint.Render(strings.Repeat("○", cur.Steps-p.st.Step-1)))
	}
	header := spread(left, strings.Join(right, "   "), inner)
	rest := p.h - presHeader - footerLines

	// Previews of what the audience sees now and what comes next, if
	// there's room for them and still some notes.
	var previews string
	if pw, ph := p.previewBox(); ph > 0 {
		nextLabel, nextBox := "END OF DECK", p.placeholder(pw, ph, "that's the last slide")
		if next, label, ok := p.nextTarget(); ok {
			nextLabel, nextBox = label, p.preview(next[0], next[1], pw, ph)
		}
		now := lipgloss.JoinVertical(lipgloss.Left, p.sty.accent.Render("NOW"), frame(p.preview(p.st.Slide, p.st.Step, pw, ph), p.theme.Accent))
		next := lipgloss.JoinVertical(lipgloss.Left, p.sty.muted.Render(truncate(nextLabel, pw+2)), frame(nextBox, p.theme.Faint))
		previews = lipgloss.JoinHorizontal(lipgloss.Top, now, strings.Repeat(" ", presGutter), next)
		rest -= lipgloss.Height(previews) + 1
	}

	notes := p.st.Notes
	if notes == "" {
		notes = p.sty.faint.Render("(no notes for this slide)")
	}
	lines := strings.Split(lipgloss.NewStyle().Width(inner).Foreground(p.theme.Text.Color()).Render(notes), "\n")
	if avail := rest - 1; len(lines) > avail { // the NOTES label takes a line
		lines = append(lines[:max(avail-1, 0)], p.sty.faint.Render("…"))
	}
	noteBlock := p.sty.accent2.Render("NOTES") + "\n" + strings.Join(lines, "\n")

	parts := []string{header, ""}
	if previews != "" {
		parts = append(parts, previews, "")
	}
	parts = append(parts, noteBlock)
	top := indent(strings.Join(parts, "\n"), strings.Repeat(" ", presMargin))
	gap := max(p.h-lipgloss.Height(top)-footerLines, 0)
	return top + strings.Repeat("\n", gap) + "\n" + p.footer()
}

// footerLines is the footer's height: a rule, the timer line, and keys.
const footerLines = 3

// previewBox is the inside size of each preview frame on this screen, or
// 0, 0 when previews don't fit.
func (p presenter) previewBox() (pw, ph int) {
	return p.previewSize(max(p.w-2*presMargin, 10), p.h-2-footerLines) // 2: header and blank line
}

// nextTarget is the slide and step the "next" preview shows, with its
// label. ok is false on the last step of the last slide.
func (p presenter) nextTarget() (next [2]int, label string, ok bool) {
	switch cur := p.curOutline(); {
	case p.st.Step < cur.Steps-1:
		return [2]int{p.st.Slide, p.st.Step + 1}, fmt.Sprintf("NEXT · step %d", p.st.Step+2), true
	case p.st.Slide < len(p.st.Outline)-1:
		return [2]int{p.st.Slide + 1, 0}, "NEXT · " + p.st.Outline[p.st.Slide+1].Title, true
	}
	return next, "", false
}

func (p presenter) deckSize() (int, int) {
	if p.st.W <= 0 || p.st.H <= 0 {
		return 682, 171 // a typical presenting terminal: 4pt font, full screen
	}
	return p.st.W, p.st.H
}

// matches reports whether this build's slide i is the deck's slide i. In
// dev mode the deck rebuilds and the presenter view doesn't, so they can
// drift apart.
func (p presenter) matches(i int) bool {
	return i < len(p.slides) && i < len(p.st.Outline) && p.slides[i].Title == p.st.Outline[i].Title
}

// upload sends the terminal any image previews the screen needs that it
// doesn't have yet. Drawing and encoding happen off the event loop.
func (p presenter) upload() tea.Cmd {
	if p.images == nil || p.st.Outline == nil {
		return nil
	}
	pw, ph := p.previewBox()
	if ph == 0 {
		return nil
	}
	targets := [][2]int{{p.st.Slide, p.st.Step}}
	if next, _, ok := p.nextTarget(); ok {
		targets = append(targets, next)
	}
	dw, dh := p.deckSize()
	var cmds []tea.Cmd
	for _, t := range targets {
		key := previewKey{t[0], t[1], pw, ph, dw, dh}
		if _, ok := p.images.id(key); ok || !p.matches(t[0]) {
			continue
		}
		id, free := p.images.add(key)
		s, step, gen := p.slides[t[0]], t[1], p.images.gen
		if free != "" {
			cmds = append(cmds, tea.Raw(free))
		}
		cmds = append(cmds, func() tea.Msg {
			previewMu.Lock()
			img := slideImage(s, step, dw, dh, p.theme)
			previewMu.Unlock()
			seq, err := kittyTransmit(id, img, pw, ph)
			if err != nil {
				seq = ""
			}
			return kittyUploadMsg{gen: gen, key: key, seq: seq}
		})
	}
	return tea.Batch(cmds...)
}

// previewMu keeps preview drawing on one goroutine at a time: slides were
// written to be drawn by one caller, and image previews draw in the
// background.
var previewMu sync.Mutex

// previewSize picks the inside size of each preview frame, keeping the
// deck's shape and leaving at least a few lines for notes. It returns 0, 0
// when previews don't fit.
func (p presenter) previewSize(inner, rest int) (pw, ph int) {
	dw, dh := p.deckSize()
	pw = (inner - presGutter - 4) / 2 // each frame adds 2 columns
	// A cell shows one pixel across and two down, like the deck's, so the
	// deck's shape in cells carries over directly.
	ph = pw * dh / dw
	if most := rest - 3 - 6; ph > most { // label + frame, and 6 lines of notes
		ph = most
		pw = ph * dw / dh
	}
	if ph < 5 || pw < 20 {
		return 0, 0
	}
	return pw, ph
}

type previewKey struct{ slide, step, pw, ph, dw, dh int }

// preview draws slide i at the given step, settled, into a pw×ph box. It
// uses this build's slides, so after the deck changes shape (in dev mode) a
// preview whose title no longer matches is left out.
func (p presenter) preview(i, step, pw, ph int) string {
	if !p.matches(i) {
		return p.placeholder(pw, ph, "preview out of date: restart the presenter view")
	}
	dw, dh := p.deckSize()
	key := previewKey{i, step, pw, ph, dw, dh}
	if p.images != nil {
		if id, ok := p.images.id(key); ok {
			return kittyPlaceholders(id, pw, ph)
		}
	}
	if s, ok := p.previews[key]; ok {
		return s
	}
	previewMu.Lock()
	s := renderPreview(p.slides[i], step, dw, dh, pw, ph, p.theme)
	previewMu.Unlock()
	p.previews[key] = s
	return s
}

// renderPreview draws a slide at a size it's designed for (240 cells wide,
// in the deck's shape), then shrinks the picture into pw×ph cells.
// Drawing straight at preview size would lay the slide out for a tiny
// screen instead of shrinking the real thing.
func renderPreview(s Slide, step, dw, dh, pw, ph int, t *Theme) string {
	rw := 240
	rh := max(rw*dh/dw, 20)
	frame := renderSlide(s, Ctx{W: rw, H: rh, T: Settled, Step: step, StepT: Settled, Theme: t})
	src := framePixels(frame, rw, rh, t)
	sc := NewScene(pw, ph, t)
	shrinkInto(src, sc.Px)
	return sc.Render()
}

// framePixels turns a rendered frame back into pixels: a "▀" cell is its
// foreground color over its background. Other characters (rare on slides)
// are approximated by blending the two.
func framePixels(frame string, w, h int, t *Theme) *Pixels {
	cv := lipgloss.NewCanvas(w, h)
	uv.NewStyledString(frame).Draw(cv, cv.Bounds())
	bgDefault, fgDefault := t.Background, t.Text
	px := NewPixels(w, 2*h, bgDefault)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := cv.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			fg, bg := fgDefault, bgDefault
			if cell.Style.Fg != nil {
				fg = toRGB(cell.Style.Fg)
			}
			if cell.Style.Bg != nil {
				bg = toRGB(cell.Style.Bg)
			}
			top, bot := bg, bg
			switch cell.Content {
			case "", " ":
			case "▀":
				top = fg
			case "▄":
				bot = fg
			case "█":
				top, bot = fg, fg
			default:
				top = Mix(bg, fg, 0.5)
				bot = top
			}
			px.Set(x, 2*y, top)
			px.Set(x, 2*y+1, bot)
		}
	}
	return px
}

// shrinkInto scales src down into dst, averaging each block of pixels.
func shrinkInto(src, dst *Pixels) {
	for y := 0; y < dst.H; y++ {
		y0, y1 := y*src.H/dst.H, max((y+1)*src.H/dst.H, y*src.H/dst.H+1)
		for x := 0; x < dst.W; x++ {
			x0, x1 := x*src.W/dst.W, max((x+1)*src.W/dst.W, x*src.W/dst.W+1)
			var r, g, b float32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.At(xx, yy)
					r, g, b = r+c.R, g+c.G, b+c.B
				}
			}
			n := float32((y1 - y0) * (x1 - x0))
			dst.Set(x, y, RGB{r / n, g / n, b / n})
		}
	}
}

func (p presenter) footer() string {
	inner := p.inner()
	el := p.elapsed()

	timer := p.sty.text.Bold(true).Render(clock(el)) + p.sty.muted.Render(" / "+clock(p.length))
	switch {
	case !p.running && el == 0:
		timer += p.sty.faint.Render("   starts on slide 2, or press t")
	case !p.running:
		timer += p.sty.warn.Render("   paused")
	case el > p.length:
		timer += p.sty.warn.Render("   " + clock(el-p.length) + " over")
	default:
		timer += p.sty.muted.Render("   " + clock(p.length-el) + " left")
	}

	status := ""
	if p.st.Outline != nil && el > 0 {
		d := pace(p.st.Outline, p.st.Slide, p.st.Step, el, p.length)
		switch {
		case d > 30*time.Second:
			status = p.sty.accent2.Render(clock(d) + " ahead")
		case d >= -30*time.Second:
			status = p.sty.good.Render("on pace")
		case d >= -2*time.Minute:
			status = p.sty.accent.Render(clock(d) + " behind")
		default:
			status = p.sty.warn.Render(clock(d) + " behind")
		}
	}

	// A bar of time used, with a marker for how far through the deck you are.
	barW := max(inner/4, 10)
	cells := func(d time.Duration) int { return int(float64(barW) * float64(d) / float64(max(p.length, 1))) }
	used := min(cells(el), barW)
	bar := p.sty.accent.Render(strings.Repeat("━", used)) + p.sty.faint.Render(strings.Repeat("━", barW-used))
	if p.st.Outline != nil {
		at := pace(p.st.Outline, p.st.Slide, p.st.Step, 0, p.length) // share of the deck shown, as time
		pos := min(cells(at), barW-1)
		bar = cutAt(bar, pos, p.sty.text.Bold(true).Render("┃"))
	}

	line1 := spread(timer+"   "+bar+"   "+status, p.sty.muted.Render(p.now.Format("15:04")), inner)
	keys := p.sty.faint.Render("→ ← step   ] [ slide   12g jump   r replay   t timer   T reset   q quit")
	rule := p.sty.faint.Render(strings.Repeat("─", inner))
	return indent(rule+"\n"+line1+"\n"+truncate(keys, inner), strings.Repeat(" ", presMargin))
}

// spread puts left and right at either end of a line w cells wide.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return truncate(left+" "+right, w)
	}
	return left + strings.Repeat(" ", gap) + right
}

// cutAt replaces the cell at pos in a styled line with mark.
func cutAt(line string, pos int, mark string) string {
	return truncate(line, pos) + mark + truncateLeft(line, pos+1)
}

// truncate keeps the first w cells of a styled line; truncateLeft drops
// the first n.
func truncate(s string, w int) string     { return ansi.Truncate(s, max(w, 0), "") }
func truncateLeft(s string, n int) string { return ansi.TruncateLeft(s, max(n, 0), "") }

// indent prefixes every line of s with pad.
func indent(s, pad string) string {
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func frame(s string, c RGB) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.Color()).Render(s)
}

func (p presenter) placeholder(w, h int, msg string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, p.sty.faint.Render(msg))
}
