package decker

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type tickMsg time.Time

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
	transFrom  *Grid
	transStart time.Time
	transFwd   bool

	// live writes frames to the terminal; nil when frames are only
	// returned from View (snapshots, tests).
	live *termWriter

	showNotes, showHelp bool
	count               string // numeric prefix for jumps, e.g. "12g"

	dev        *devState // nil unless -dev
	execOnQuit []string  // set when dev mode wants to restart into a new binary

	link      *linkServer // nil unless the presenter link is on
	published [4]int      // slide, step, w, h last sent over the link
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

// tick waits for the next frame. Frames fall on multiples of the frame time
// on the clock, so drawing time doesn't stretch the interval; a frame that
// runs long skips to the next slot instead of piling up.
func (m model) tick() tea.Cmd {
	return tea.Every(time.Second/time.Duration(m.fps), func(t time.Time) tea.Msg { return tickMsg(t) })
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
	next, cmd := m.update(msg)
	nm := next.(model)
	if nm.link != nil {
		if key := [4]int{nm.idx, nm.step, nm.w, nm.h}; key != nm.published {
			nm.published = key
			nm.link.Publish(nm.linkState())
		}
	}
	return nm, cmd
}

// linkState is the position report for the presenter view.
func (m model) linkState() linkState {
	st := linkState{Slide: m.idx, Step: m.step, Notes: m.cur().Notes, W: m.w, H: m.h}
	for _, s := range m.slides {
		st.Outline = append(st.Outline, linkOutline{Title: s.Title, Steps: s.steps()})
	}
	return st
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case linkCmd:
		// The presenter view drives the deck with the same keys.
		m.count = ""
		if msg.Cmd == "goto" {
			return m.jump(strconv.Itoa(msg.Slide)), nil
		}
		if k, ok := linkKeys[msg.Cmd]; ok {
			return m.handleKey(k)
		}
		return m, nil

	case tea.WindowSizeMsg:
		if msg.Width <= 0 || msg.Height <= 0 {
			return m, nil // Bubble Tea's placeholder size when it isn't rendering
		}
		m.w, m.h = msg.Width, msg.Height

	case tickMsg:
		m.now = time.Time(msg)
		if m.transFrom != nil && m.now.Sub(m.transStart).Seconds() >= TransitionDuration {
			m.transFrom = nil
		}
		if m.live != nil && m.w > 0 && m.h > 0 {
			m.live.submit(m.frame(), m.cur().Title)
		}
		cmds := []tea.Cmd{m.tick()}
		if d := m.dev; d != nil && d.pending && !d.building && m.now.Sub(d.lastChange) > debounce {
			d.pending, d.building = false, true
			cmds = append(cmds, d.build())
		}
		return m, tea.Batch(cmds...)

	case fileChangedMsg:
		m.dev.pending, m.dev.lastChange = true, time.Now()
		return m, m.dev.wait()

	case buildDoneMsg:
		m.dev.building = false
		if msg.err != nil {
			m.dev.buildErr = strings.TrimSpace(msg.out)
			if m.dev.buildErr == "" {
				m.dev.buildErr = msg.err.Error()
			}
			return m, nil
		}
		m.dev.buildErr = ""
		m.execOnQuit = []string{m.dev.bin, "-dev",
			"-slide", strconv.Itoa(m.idx + 1), "-step", strconv.Itoa(m.step + 1),
			"-fps", strconv.Itoa(m.fps)}
		return m, tea.Quit

	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m model) handleKey(k string) (tea.Model, tea.Cmd) {
	if len(k) == 1 && k[0] >= '0' && k[0] <= '9' {
		m.count += k
		return m, nil
	}
	count := m.count
	m.count = ""

	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.showHelp = false
	case "?":
		m.showHelp = !m.showHelp
	case "n":
		m.showNotes = !m.showNotes
	case "ctrl+l": // redraw the whole screen
		if m.live != nil {
			m.live.invalidate()
		}
	case "r": // replay the current slide from the top
		m.step = 0
		m.enter, m.stepStart = m.now, m.now

	case "right", "l", "space", "pgdown", "j", "down", "enter":
		if count != "" && k == "enter" {
			return m.jump(count), nil
		}
		if m.step < m.cur().steps()-1 {
			m.step++
			m.stepStart = m.now
		} else if m.idx < len(m.slides)-1 {
			m.goTo(m.idx+1, 0, true)
		}
	case "left", "h", "pgup", "k", "up", "backspace":
		if m.step > 0 {
			m.step--
			m.stepStart = m.now.Add(-Settled * time.Second)
		} else if m.idx > 0 {
			m.goTo(m.idx-1, m.slides[m.idx-1].steps()-1, false)
		}
	case "]":
		if m.idx < len(m.slides)-1 {
			m.goTo(m.idx+1, 0, true)
		}
	case "[":
		if m.idx > 0 {
			m.goTo(m.idx-1, 0, false)
		}
	case "g", "home":
		if count != "" {
			return m.jump(count), nil
		}
		m.goTo(0, 0, false)
	case "G", "end":
		m.goTo(len(m.slides)-1, 0, true)
	}
	return m, nil
}

func (m model) jump(count string) model {
	n, _ := strconv.Atoi(count)
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
		// Start from what's on screen, even mid-transition.
		// The grid is never released to the pool, so it stays valid.
		m.transFrom = m.body(m.bodyHeight())
	}
	m.idx, m.step = idx, step
	m.enter, m.stepStart = m.now, m.now
	if !forward {
		past := m.now.Add(-Settled * time.Second)
		m.enter, m.stepStart = past, past
	}
	m.trans = m.cur().Transition
	if m.trans == TransitionDefault {
		m.trans = DefaultTransition
	}
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
	}
}

// body draws the slide area at exactly m.w × h: the current slide, mixed
// with the previous one during a transition.
func (m model) body(h int) *Grid {
	g := renderSlideGrid(m.cur(), m.ctx(h))
	if from := m.transFrom; from != nil && from.W == g.W && from.H == g.H {
		p := m.now.Sub(m.transStart).Seconds() / TransitionDuration
		mixed := composeGrid(m.trans, from, g, p, m.transFwd, m.theme)
		g.release()
		g = mixed
	}
	return g
}

// renderSlideGrid draws one frame of s as cells, at exactly c.W × c.H.
func renderSlideGrid(s Slide, c Ctx) *Grid {
	var got *Grid
	captureGrid = func(g *Grid) {
		if got == nil && g.W == c.W && g.H == c.H {
			got = g
		} else {
			g.release()
		}
	}
	out := renderSlide(s, c)
	captureGrid = nil
	if got != nil {
		return got
	}
	// The slide drew without a scene: decode its text.
	return parseGrid(out, max(c.W, 1), max(c.H, 1), c.Theme)
}

func renderSlide(s Slide, c Ctx) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = opaque(c.Theme.styles().warn.Render(fmt.Sprintf("slide %q panicked:\n\n%v", s.Title, r)), c.W, c.H, c.Theme)
		}
	}()
	if s.View == nil {
		return opaque("", c.W, c.H, c.Theme)
	}
	return fit(s.View(c), c.W, c.H)
}

// fit clips or pads s to exactly w x h cells.
func fit(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if n, ok := narrowWidth(l); !ok || n > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// narrowWidth is a fast width for lines made of escape sequences and
// single-width characters, which is what Scene.Render produces. It reports
// ok=false if the line might hold wide characters (CJK, emoji), so the
// caller can fall back to a full measurement. Checking every frame with
// ansi.StringWidth costs several milliseconds at big terminal sizes.
func narrowWidth(s string) (n int, ok bool) {
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b:
			// Skip a CSI sequence: ESC [ params final-byte.
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
					i++
				}
				i++
				continue
			}
			return 0, false
		case c < 0x80:
			n++
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			// Box drawing, blocks, arrows, punctuation and the like are
			// narrow; anything past U+1100 outside that range might not be.
			if r >= 0x1100 && (r < 0x2000 || r > 0x2bff) {
				return 0, false
			}
			n++
			i += size
		}
	}
	return n, true
}

const chromeHeight = 1

// showChrome reports whether to draw the footer. The audience never sees
// it: it's a dev-mode aid (build status, slide counter). When presenting,
// progress lives in the presenter view.
func (m model) showChrome() bool { return m.dev != nil && !m.cur().HideChrome }

func (m model) bodyHeight() int {
	h := m.h
	if m.showChrome() {
		h -= chromeHeight
	}
	if p := m.panels(); p != "" {
		h -= lipgloss.Height(p)
	}
	return max(h, 1)
}

// frame draws the whole screen: the slide area, then any panels and the
// footer below it.
func (m model) frame() *Grid {
	bodyH := m.bodyHeight()
	body := m.body(bodyH)
	if m.showHelp {
		hb := m.helpBox()
		w, h := lipgloss.Size(hb)
		body.draw((m.w-w)/2, (bodyH-h)/2, hb, false, m.theme.Text)
	}
	if bodyH == m.h {
		return body
	}
	g := blankGrid(m.w, m.h, m.theme.Background)
	copy(g.Cells, body.Cells[:min(len(body.Cells), len(g.Cells))])
	body.release()
	y := bodyH
	if p := m.panels(); p != "" {
		g.draw(0, y, p, false, m.theme.Text)
		y += lipgloss.Height(p)
	}
	if m.showChrome() {
		g.draw(0, y, m.chrome(), false, m.theme.Text)
	}
	return g
}

// View is the frame as a string, for snapshots and tests. The live deck
// writes frames itself (see termout.go), so there it's empty.
func (m model) View() tea.View {
	if m.live != nil || m.w == 0 || m.h == 0 {
		return tea.NewView("")
	}
	g := m.frame()
	v := tea.NewView(g.String())
	g.release()
	v.AltScreen = true
	v.BackgroundColor = m.theme.Background.Color()
	v.WindowTitle = m.cur().Title
	return v
}

// panels renders the optional bottom panels: build errors and speaker notes.
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
		out = append(out, panel("notes", notes, m.theme.Accent2, max(m.h/4, 3)))
	}
	return strings.Join(out, "\n")
}

// chrome is the one-line footer: a thin progress bar, with the slide
// counter and any status indicators at the right.
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
	rows := [][2]string{
		{"→ l space pgdn", "next step / slide"},
		{"← h pgup", "previous"},
		{"] [", "next / previous slide (skip steps)"},
		{"g G", "first / last slide"},
		{"12g  12⏎", "jump to slide 12"},
		{"r", "replay this slide"},
		{"n", "speaker notes"},
		{"?", "this help"},
		{"q", "quit"},
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.st.accent.Render(fmt.Sprintf("%-16s", r[0])) + m.st.text.Render(r[1]))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent.Color()).
		Background(m.theme.Panel.Color()).
		Padding(1, 2).
		Render(b.String())
}
