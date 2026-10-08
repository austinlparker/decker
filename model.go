package decker

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type tickMsg time.Time

// blankMode is what covers the screen: nothing, black or white, like
// PowerPoint's B and W.
type blankMode uint8

const (
	blankNone blankMode = iota
	blankBlack
	blankWhite
)

// String names the mode as the link reports it; "" for blankNone.
func (b blankMode) String() string {
	switch b {
	case blankBlack:
		return "black"
	case blankWhite:
		return "white"
	}
	return ""
}

// color is the pure color the mode paints.
func (b blankMode) color() RGB {
	if b == blankWhite {
		return RGB{255, 255, 255}
	}
	return RGB{}
}

// toggleBlank blanks the screen with mode, or restores the slide if that mode
// is already up. Overlays close, so they don't pop back at the audience.
func (m *model) toggleBlank(mode blankMode) {
	if m.blank == mode {
		m.blank = blankNone
		return
	}
	m.blank = mode
	m.showHelp, m.showNotes = false, false
}

// model is the Bubble Tea model of a running deck. With live set, frames go to
// the terminal writer; tests call frame directly.
type model struct {
	slides    []Slide
	theme     *Theme
	st        styles
	idx, step int
	w, h      int
	fps       int

	now, enter, stepStart time.Time

	// Active transition, if transFrom != nil.
	trans      Transition
	transDur   float64 // seconds
	transFrom  *Scene
	transStart time.Time
	transFwd   bool

	live *termWriter

	showNotes, showHelp bool
	count               string // numeric prefix for jumps, e.g. "12g"

	// blank covers the whole screen with one color. The slide's clock (Ctx.T)
	// keeps running underneath, so animations finish while it is up and the
	// slide is settled when it comes back; frame doesn't draw the slide at
	// all while blanked.
	blank blankMode

	dev *devState // nil unless -dev

	link      *linkServer // nil unless the presenter link is on
	published [5]int      // slide, step, w, h, blank last sent over the link
}

func newModel(d *Deck, idx, step, fps int, dev *devState) model {
	now := time.Now()
	m := model{slides: d.Slides, theme: d.Theme, st: d.Theme.styles(),
		fps: fps, dev: dev, now: now, enter: now, stepStart: now}
	m.idx = min(max(idx, 0), len(m.slides)-1)
	m.step = min(max(step, 0), m.cur().steps()-1)
	return m
}

func (m model) cur() Slide { return m.slides[m.idx] }

// tick waits for the next frame. Frames land on multiples of the frame time, so
// drawing time doesn't stretch the interval; a long frame skips to the next
// slot instead of piling up.
func (m model) tick() tea.Cmd {
	return tea.Every(time.Second/time.Duration(m.fps), func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) advance(now time.Time) {
	m.now = now
	if m.transFrom != nil && now.Sub(m.transStart).Seconds() >= m.transDur {
		m.transFrom = nil
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.tick()}
	if m.dev != nil {
		cmds = append(cmds, m.dev.wait())
	}
	return tea.Batch(cmds...)
}

// Update handles msg, then tells the presenter view if the position moved.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.handle(msg)
	if m.link != nil {
		if key := [5]int{m.idx, m.step, m.w, m.h, int(m.blank)}; key != m.published {
			m.published = key
			m.link.publish(m.linkState())
		}
	}
	return m, cmd
}

func (m model) linkState() linkState {
	st := linkState{Slide: m.idx, Step: m.step, Notes: m.cur().Notes, Sources: m.cur().Sources, W: m.w, H: m.h, Blank: m.blank.String()}
	for i, s := range m.slides {
		st.Outline = append(st.Outline, linkOutline{Title: s.Title, Steps: s.steps(), Section: sectionAt(m.slides, i)})
	}
	return st
}

func (m model) handle(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case linkCmd:
		m.count = ""
		if msg.Slide > 0 {
			return m.jump(msg.Slide), nil
		}
		if keyActs[msg.Key].nav { // the presenter may not quit the deck
			return m.handleKey(msg.Key)
		}

	case tea.WindowSizeMsg:
		if msg.Width <= 0 || msg.Height <= 0 {
			return m, nil // Bubble Tea's placeholder size when it isn't rendering
		}
		m.w, m.h = msg.Width, msg.Height

	case tickMsg:
		m.advance(time.Time(msg))
		if m.live != nil && m.w > 0 && m.h > 0 {
			m.live.submit(m.frame(), m.cur().Title)
		}
		cmds := []tea.Cmd{m.tick()}
		if m.dev != nil {
			cmds = append(cmds, m.dev.tick(m.now))
		}
		return m, tea.Batch(cmds...)

	case fileChangedMsg:
		return m, m.dev.changed()

	case buildDoneMsg:
		if !m.dev.built(msg) {
			return m, nil
		}
		m.dev.restart = m.dev.restartArgs(m.idx+1, m.step+1, m.fps)
		return m, tea.Quit

	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}
	return m, nil
}

// handleKey applies one key; digits build a count that g, Home or Enter turn
// into a jump.
func (m model) handleKey(k string) (model, tea.Cmd) {
	if n, done := countKey(&m.count, k); done {
		if n > 0 {
			m = m.jump(n)
		}
		return m, nil
	}
	act := keyActs[k].act
	// Anything that moves, or asks to see something, brings the slide back
	// first and then does its job. ctrl+l and unbound keys leave the blank up,
	// so a stray key on a clicker doesn't flash the slide at the audience.
	switch act {
	case "next", "nextSlide", "prev", "prevSlide", "first", "last", "replay", "notes", "help", "closeHelp":
		m.blank = blankNone
	}
	switch act {
	case "blank":
		m.toggleBlank(blankBlack)
	case "blankWhite":
		m.toggleBlank(blankWhite)
	case "quit":
		return m, tea.Quit
	case "closeHelp":
		m.showHelp = false
	case "help":
		m.showHelp = !m.showHelp
	case "notes":
		m.showNotes = !m.showNotes
	case "redraw":
		if m.live != nil {
			m.live.invalidate()
		}
	case "replay":
		m.step = 0
		m.enter, m.stepStart = m.now, m.now
	case "next":
		if m.step < m.cur().steps()-1 {
			m.step++
			m.stepStart = m.now
			break
		}
		fallthrough
	case "nextSlide":
		if m.idx < len(m.slides)-1 {
			m.goTo(m.idx+1, 0, true)
		}
	case "prev":
		if m.step > 0 {
			m.step--
			m.stepStart = m.now.Add(-Settled * time.Second)
		} else if m.idx > 0 {
			m.goTo(m.idx-1, m.slides[m.idx-1].steps()-1, false)
		}
	case "prevSlide":
		if m.idx > 0 {
			m.goTo(m.idx-1, 0, false)
		}
	case "first":
		m.goTo(0, 0, false)
	case "last":
		m.goTo(len(m.slides)-1, 0, true)
	}
	return m, nil
}

// jump goes to the 1-based slide n, clamped to the deck.
func (m model) jump(n int) model {
	m.blank = blankNone
	n = min(max(n, 1), len(m.slides))
	m.goTo(n-1, 0, n-1 >= m.idx)
	return m
}

// goTo switches slides, starting a transition. Moving forward plays the new
// slide's entrance animations; moving backward shows it already settled.
func (m *model) goTo(idx, step int, forward bool) {
	if idx == m.idx && step == m.step {
		return
	}
	if m.w > 0 && m.h > 0 {
		// Start from what's on screen, even mid-transition. The scene is never
		// released to the pool, so it stays valid.
		bodyH, _ := m.layout()
		m.transFrom = m.body(bodyH)
	}
	m.idx, m.step = idx, step
	m.enter, m.stepStart = m.now, m.now
	if !forward {
		past := m.now.Add(-Settled * time.Second)
		m.enter, m.stepStart = past, past
	}
	m.trans = m.cur().Transition.resolve()
	m.transDur = m.trans.Duration()
	if m.trans == TransitionNone {
		m.transFrom = nil
	}
	m.transStart, m.transFwd = m.now, forward
}

func (m model) ctx(h int) Ctx {
	return Ctx{
		W: m.w, H: h,
		T:     m.now.Sub(m.enter).Seconds(),
		Step:  m.step,
		StepT: m.now.Sub(m.stepStart).Seconds(),
		Theme: m.theme,
	}.at(m.slides, m.idx)
}

// body draws the slide area at exactly m.w × h, mixed with the previous slide
// during a transition. Outside one it isn't finished yet, so a transition
// that starts from it can still move its placed elements. The caller
// releases it.
func (m model) body(h int) *Scene {
	sc := drawSlide(m.cur(), m.ctx(h))
	if m.transFrom != nil {
		p := m.now.Sub(m.transStart).Seconds() / m.transDur
		mixTransition(m.trans, m.transFrom, sc, p, m.transFwd, m.theme)
	}
	return sc
}

func (m model) frame() *grid {
	if m.blank != blankNone {
		// All of the screen, footer included, and none of the slide's drawing:
		// the writer diffs a static frame down to nothing.
		return blankGrid(m.w, m.h, m.blank.color())
	}
	bodyH, panels := m.layout()
	sc := m.body(bodyH)
	sc.finish()
	body := sc.toGrid()
	sc.Release()
	if m.showHelp {
		hb := m.helpBox()
		w, h := lipgloss.Size(hb)
		body.draw((m.w-w)/2, (bodyH-h)/2, hb, false, m.theme.Text)
	}
	if bodyH == m.h {
		return body
	}
	g := blankGrid(m.w, m.h, m.theme.Background)
	copy(g.Cells, body.Cells)
	body.release()
	y := bodyH
	if panels != "" {
		g.draw(0, y, panels, false, m.theme.Text)
		y += lipgloss.Height(panels)
	}
	if m.showChrome() {
		g.draw(0, y, m.chrome(), false, m.theme.Text)
	}
	return g
}

// View is empty: the live deck writes its frames itself (termout.go), so
// Bubble Tea runs without a renderer and only handles keys.
func (m model) View() tea.View { return tea.NewView("") }

const chromeHeight = 1

// showChrome reports whether to draw the footer: a dev-mode aid (build status,
// slide counter) the audience never sees; presenting shows progress in the
// presenter view.
func (m model) showChrome() bool { return m.dev != nil && !m.cur().HideChrome }

// layout returns the height left for the slide and the panels below it.
func (m model) layout() (bodyH int, panels string) {
	panels = m.panels()
	h := m.h
	if m.showChrome() {
		h -= chromeHeight
	}
	if panels != "" {
		h -= lipgloss.Height(panels)
	}
	return max(h, 1), panels
}

func (m model) panels() string {
	var out []string
	panel := func(title, body string, accent RGB, maxLines int) string {
		lines := strings.Split(body, "\n")
		if len(lines) > maxLines {
			lines = append(lines[:maxLines-1], "…")
		}
		return lipgloss.NewStyle().
			Width(m.w).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(accent.Color()).
			Foreground(m.theme.Muted.Color()).
			Padding(0, 1).
			Render(lipgloss.NewStyle().Foreground(accent.Color()).Bold(true).Render(title) + "\n" + strings.Join(lines, "\n"))
	}
	if m.dev != nil && m.dev.buildErr != "" {
		out = append(out, panel("build failed — fix and save to retry", m.dev.buildErr, m.theme.Warn, max(m.h/3, 3)))
	}
	if m.showNotes {
		notes := m.cur().Notes
		if notes == "" {
			notes = "(no notes for this slide)"
		}
		if src := m.cur().Sources; len(src) > 0 {
			notes = strings.TrimRight(notes, "\n") + "\n\nsources:"
			for _, s := range src {
				notes += "\n" + s.line()
			}
		}
		out = append(out, panel("notes", notes, m.theme.Accent2, max(m.h/4, 3)))
	}
	return strings.Join(out, "\n")
}

func (m model) chrome() string {
	n := len(m.slides)
	var right []string
	if m.count != "" {
		right = append(right, m.st.accent2.Render("go to "+m.count+"…"))
	}
	if m.dev != nil {
		right = append(right, m.dev.status(m.st))
	}
	if s := m.cur().steps(); s > 1 {
		right = append(right, m.st.accent.Render(strings.Repeat("●", m.step+1))+m.st.faint.Render(strings.Repeat("○", s-m.step-1)))
	}
	if m.cur().Notes != "" {
		right = append(right, m.st.faint.Render("✎"))
	}
	right = append(right, m.st.faint.Render(fmt.Sprintf("%d/%d ", m.idx+1, n)))
	r := " " + strings.Join(right, "  ")
	barW := max(m.w-lipgloss.Width(r), 0)
	filled := barW * (m.idx + 1) / n
	return m.st.accent.Render(strings.Repeat("▁", filled)) + m.st.faint.Render(strings.Repeat("▁", barW-filled)) + r
}

func (m model) helpBox() string {
	var rows []string
	for _, b := range bindings {
		if b.help != "" {
			rows = append(rows, m.st.accent.Render(fmt.Sprintf("%-16s", b.help))+m.st.text.Render(b.what))
		}
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent.Color()).
		Background(m.theme.Panel.Color()).
		Padding(1, 2).
		Render(strings.Join(rows, "\n"))
}
