package decker

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// runPresenter runs the presenter view (-presenter) until the user quits:
// notes, previews and a timer in a window of its own, linked to the deck over
// socket. Its keys drive the deck, so a clicker aimed here runs the show.
func runPresenter(d *Deck, socket string, length time.Duration) error {
	p := newPresenter(d, socket, length)
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
	previews map[previewKey]string // drawn with half blocks

	st     linkState // the deck's last report; Outline is nil until the first
	linked bool
	w, h   int
	now    time.Time
	count  string // numeric prefix for jumps, e.g. "12g"

	// The talk timer starts when the deck first leaves slide 1, or with t.
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

// connect dials the deck after wait; failure yields linkDownMsg, which retries.
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
		return p, p.listen()
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
// talk is: share of build steps shown against share of time used. Steps, not
// slides, because a slide with four builds takes longer than a section opener.
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

const (
	presMargin = 2 // left and right edges
	presGutter = 3 // between the two previews
	presHeader = 2 // the header and the blank line under it
)

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
	if p.st.Blank != "" {
		right = append(right, p.sty.warn.Bold(true).Render("BLANK")+p.sty.muted.Render(" ("+p.st.Blank+")"))
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

	// Previews of now and next, if they fit and leave room for notes.
	var previews string
	if k, ok := p.key(p.st.Slide, p.st.Step); ok {
		nextLabel, nextBox := "END OF DECK", p.placeholder(k.pw, k.ph, "that's the last slide")
		if next, label, ok := p.nextTarget(); ok {
			nk := k
			nk.slide, nk.step = next[0], next[1]
			nextLabel, nextBox = label, p.preview(nk)
		}
		now := lipgloss.JoinVertical(lipgloss.Left, p.sty.accent.Render("NOW"), frame(p.preview(k), p.theme.Accent))
		next := lipgloss.JoinVertical(lipgloss.Left, p.sty.muted.Render(truncate(nextLabel, k.pw+2)), frame(nextBox, p.theme.Faint))
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

// matches reports whether this build's slide i is the deck's slide i; in dev
// mode the deck rebuilds and the presenter view doesn't, so they can drift.
func (p presenter) matches(i int) bool {
	return i < len(p.slides) && i < len(p.st.Outline) && p.slides[i].Title == p.st.Outline[i].Title
}

// preview draws the slide k names, settled. A slide whose title no longer
// matches the deck's (dev mode) is left out.
func (p presenter) preview(k previewKey) string {
	if !p.matches(k.slide) {
		return p.placeholder(k.pw, k.ph, "preview out of date: restart the presenter view")
	}
	if s, ok := p.previews[k]; ok {
		return s
	}
	s := renderPreview(p.slides[k.slide], k, p.theme)
	p.previews[k] = s
	return s
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

func truncate(s string, w int) string     { return ansi.Truncate(s, max(w, 0), "") }
func truncateLeft(s string, n int) string { return ansi.TruncateLeft(s, max(n, 0), "") }

func indent(s, pad string) string {
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func frame(s string, c RGB) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.Color()).Render(s)
}

func (p presenter) placeholder(w, h int, msg string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, p.sty.faint.Render(msg))
}
