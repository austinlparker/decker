package decker

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The presenter view's screen, top to bottom: the header (where the talk
// is, and how it's doing for time), the status line (whatever needs the
// speaker, and posting), the middle (the previews and notes, or a
// dialog), and the time bar. The regions never move; when lines run
// short, the notes give way.
const (
	presMargin    = 2   // left and right edges
	presGutter    = 3   // between the two previews
	presHeader    = 2   // the header and the status line
	footerLines   = 1   // the time bar
	previewChrome = 4   // the previews' label line, their frames' top and bottom, and a blank line
	notesMin      = 6   // lines of notes the previews leave, at least
	notesMeasure  = 100 // the widest the notes wrap, so a line reads at a glance
	maxStepDots   = 12  // more steps than this show as a count
)

func (p presenter) inner() int { return max(p.w-2*presMargin, 10) }

func (p presenter) curOutline() linkOutline {
	return p.st.Outline[min(p.st.Slide, len(p.st.Outline)-1)]
}

func (p presenter) View() tea.View {
	if p.w == 0 || p.h == 0 {
		return tea.NewView("")
	}
	v := tea.NewView(p.screen())
	v.AltScreen = true
	v.BackgroundColor = p.deck.Theme.Background.Color()
	v.WindowTitle = "presenter"
	return v
}

// screen is the whole view, exactly p.h lines.
func (p presenter) screen() string {
	inner := p.inner()
	rest := max(p.h-presHeader-footerLines, 0)
	var middle []string
	if p.st.Outline == nil {
		middle = p.waitingLines(inner, rest)
	} else {
		middle = p.middleLines(inner, rest)
	}
	lines := append([]string{p.headerLine(inner), p.statusLine(inner)}, fitLines(middle, rest)...)
	lines = append(lines, p.bar(inner))
	if len(lines) > p.h {
		lines = lines[:p.h]
	}
	return indent(strings.Join(lines, "\n"), strings.Repeat(" ", presMargin))
}

// fitLines is lines cut or padded to n.
func fitLines(lines []string, n int) []string {
	if len(lines) > n {
		return lines[:n]
	}
	return append(lines, make([]string, n-len(lines))...)
}

// headerLine is where the talk is, on the left, and its time, on the
// right. Short of room, it drops the section, then shortens the title,
// then the time.
func (p presenter) headerLine(inner int) string {
	sty := p.sty
	times := []string{p.timeCluster(false), p.timeCluster(true)}
	if p.st.Outline == nil {
		for _, t := range times {
			if lipgloss.Width(sty.muted.Render(p.deck.Name))+1+lipgloss.Width(t) <= inner {
				return spread(sty.muted.Render(p.deck.Name), t, inner)
			}
		}
		return truncate(spread("", times[1], inner), inner)
	}
	cur := p.curOutline()
	left := func(title string, section bool) string {
		s := sty.accent.Render(fmt.Sprintf("%d/%d", p.st.Slide+1, len(p.st.Outline))) + "  " + sty.text.Bold(true).Render(title)
		switch {
		case cur.Steps > maxStepDots:
			s += "  " + sty.muted.Render(fmt.Sprintf("step %d/%d", p.st.Step+1, cur.Steps))
		case cur.Steps > 1:
			s += "  " + sty.meter("●", "○", p.st.Step+1, cur.Steps)
		}
		if section && cur.Section != "" {
			s += "   " + sty.muted.Render(cur.Section)
		}
		return s
	}
	fits := func(l, r string) bool { return lipgloss.Width(l)+1+lipgloss.Width(r) <= inner }
	for _, t := range times {
		if l := left(cur.Title, true); fits(l, t) {
			return spread(l, t, inner)
		}
		if l := left(cur.Title, false); fits(l, t) {
			return spread(l, t, inner)
		}
		room := inner - 1 - lipgloss.Width(t) - lipgloss.Width(left("", false))
		if room >= 8 {
			return spread(left(ansi.Truncate(cur.Title, max(room, 0), "…"), false), t, inner)
		}
	}
	return truncate(spread(left(cur.Title, false), times[1], inner), inner)
}

// timeCluster is the header's time: how long is left, or that the talk is
// over or paused, and the pace.
func (p presenter) timeCluster(compact bool) string {
	sty, el := p.sty, p.elapsed()
	pace := p.paceText()
	join := func(parts ...string) string {
		var keep []string
		for _, s := range parts {
			if s != "" {
				keep = append(keep, s)
			}
		}
		return strings.Join(keep, "   ")
	}
	left := p.length - el
	switch {
	case !p.running && el == 0:
		if compact {
			return sty.muted.Render(clock(p.length))
		}
		return sty.muted.Render(clock(p.length)) + sty.faint.Render(" · starts at slide 2")
	case !p.running:
		rest := clock(left) + " left"
		if left < 0 {
			rest = clock(-left) + " over"
		}
		if compact {
			return sty.warn.Render("PAUSED") + " " + sty.muted.Render(clock(left))
		}
		return join(sty.warn.Render("PAUSED")+"  "+sty.muted.Render(rest), pace)
	case left < 0:
		if compact {
			return join(sty.warn.Render("+"+clock(left)), pace)
		}
		return join(sty.warn.Render(clock(left)+" OVER"), pace)
	}
	if compact {
		return join(sty.text.Bold(true).Render(clock(left)), pace)
	}
	return join(sty.text.Bold(true).Render(clock(left))+sty.muted.Render(" left"), pace)
}

// paceText is how far ahead of or behind an even pace the talk is, once
// it has started.
func (p presenter) paceText() string {
	el := p.elapsed()
	if p.st.Outline == nil || el == 0 {
		return ""
	}
	switch d := pace(p.st.Outline, p.st.Slide, p.st.Step, el, p.length); {
	case d > 30*time.Second:
		return p.sty.accent2.Render(clock(d) + " ahead")
	case d >= -30*time.Second:
		return p.sty.good.Render("on pace")
	case d >= -2*time.Minute:
		return p.sty.accent.Render(clock(d) + " behind")
	default:
		return p.sty.warn.Render(clock(d) + " behind")
	}
}

// chip is an alert on the status line, with a short form that drops its
// hint, for when the line is short of room.
type chip struct{ full, short string }

// statusLine is whatever needs the speaker, most urgent first, on the
// left, and posting on the right. Short of room, posting goes compact,
// then alerts lose their hints, the least urgent first, then the least
// urgent alerts drop.
func (p presenter) statusLine(inner int) string {
	chips := p.alerts()
	badges := []string{p.postingBadge(false), p.postingBadge(true), ""}
	fits := func(l, r string) bool {
		w := lipgloss.Width(l) + lipgloss.Width(r)
		if l != "" && r != "" {
			w++
		}
		return w <= inner
	}
	for n := len(chips); n >= 1; n-- {
		// Shorten from the least urgent: short counts how many, from the end.
		for short := 0; short <= n; short++ {
			parts := make([]string, n)
			for i, c := range chips[:n] {
				parts[i] = c.full
				if i >= n-short {
					parts[i] = c.short
				}
			}
			left := strings.Join(parts, "   ")
			for _, b := range badges {
				if b == "" && short < n && p.post != nil {
					continue // posting stays while an alert can still shorten
				}
				if fits(left, b) {
					return spread(left, b, inner)
				}
			}
		}
	}
	if len(chips) > 0 {
		return truncate(chips[0].short, inner)
	}
	hint := p.sty.faint.Render("? keys")
	for _, b := range badges {
		if fits(hint, b) {
			return spread(hint, b, inner)
		}
		if fits("", b) {
			return spread("", b, inner)
		}
	}
	return ""
}

// alerts are the status line's chips, most urgent first.
func (p presenter) alerts() []chip {
	sty := p.sty
	var chips []chip
	if p.quitArmed.After(p.now) {
		lead := sty.warn.Render(fmt.Sprintf("▲ %s not sent yet", plural(p.unsent(), "post")))
		chips = append(chips, chip{lead + sty.muted.Render(" · q again to quit"), lead + sty.muted.Render(" · q quits")})
	}
	if p.count != "" {
		lead := sty.accent2.Render("GO TO " + p.count + "▏")
		chips = append(chips, chip{lead + sty.faint.Render(" g/⏎"), lead})
	}
	if p.st.Outline != nil && !p.linked {
		lead := sty.warn.Render("○ DISCONNECTED")
		s := lead + sty.muted.Render(" · reconnecting…")
		if p.launch != nil && !p.launching {
			s += sty.muted.Render(" · p reopens it")
		}
		chips = append(chips, chip{s, lead})
	}
	switch p.st.Blank {
	case "":
	case "white":
		lead := sty.warn.Render("□ BLANK")
		chips = append(chips, chip{lead + sty.muted.Render(" · white · w resumes"), lead})
	default:
		lead := sty.warn.Render("■ BLANK")
		chips = append(chips, chip{lead + sty.muted.Render(" · black · b resumes"), lead})
	}
	switch {
	case p.launching:
		s := sty.accent.Render("◌ Opening the deck window…")
		chips = append(chips, chip{s, s})
	case p.launchErr != "":
		lead := sty.warn.Render("▲ DECK WINDOW")
		chips = append(chips, chip{lead + sty.muted.Render(" "+strings.ReplaceAll(p.launchErr, "\n", " ")+" · p retries"), lead})
	}
	return chips
}

// middleLines are the previews of now and next, if they fit and leave
// room for notes, and the notes; or a dialog over what's next and the
// notes, without the previews, which as images can't dim.
func (p presenter) middleLines(inner, rest int) []string {
	if p.dialog != dialogNone {
		return p.drawDialog(append([]string{p.nextLine()}, p.notesLines(inner, rest-1)...), inner, rest)
	}
	if k, ok := p.key(p.st.Slide, p.st.Step); ok {
		block := p.previewBlock(k)
		return append(block, p.notesLines(inner, rest-len(block))...)
	}
	return append([]string{p.nextLine()}, p.notesLines(inner, rest-1)...)
}

// nextLine says what's next, for when the previews don't fit.
func (p presenter) nextLine() string {
	if _, label, ok := p.nextTarget(); ok {
		return p.sty.muted.Render(label)
	}
	return p.sty.muted.Render("NEXT · end of deck")
}

// previewBlock is the previews of now and next, labelled, framed and
// followed by a blank line: previewChrome lines more than k is tall.
func (p presenter) previewBlock(k previewKey) []string {
	nextLabel, nextBox := "NEXT · end of deck", p.placeholder(k.pw, k.ph, "that's the last slide")
	if nk, label, ok := p.nextKey(k); ok {
		nextLabel, nextBox = label, p.preview(nk)
	}
	now := lipgloss.JoinVertical(lipgloss.Left, p.sty.accent.Render("NOW"), frame(p.preview(k), p.deck.Theme.Accent))
	next := lipgloss.JoinVertical(lipgloss.Left, p.sty.muted.Render(truncate(nextLabel, k.pw+2)), frame(nextBox, p.deck.Theme.Faint))
	block := lipgloss.JoinHorizontal(lipgloss.Top, now, strings.Repeat(" ", presGutter), next)
	return append(strings.Split(block, "\n"), "")
}

// notesLines are the NOTES label, with the slide's posts' status, and the
// notes, in n lines at most.
func (p presenter) notesLines(inner, n int) []string {
	if n <= 0 {
		return nil
	}
	sty := p.sty
	label := sty.accent2.Render("NOTES")
	if pb := p.pub(); pb != nil && p.matches(p.st.Slide) {
		if s, t := pb.status(p.st.Slide); s != "" {
			count := "post"
			if k := len(pb.posts[p.st.Slide]); k > 1 {
				count = plural(k, "post")
			}
			label = spread(label, sty.faint.Render(count)+"  "+sty.tone(t).Render(s), inner)
		}
	}
	notes := p.st.Notes
	if notes == "" {
		notes = sty.faint.Render(noNotes)
	}
	w := min(inner, notesMeasure)
	lines := strings.Split(lipgloss.NewStyle().Width(w).Foreground(p.deck.Theme.Text.Color()).Render(notes), "\n")
	switch avail := n - 1; {
	case avail <= 0:
		lines = nil
	case len(lines) > avail:
		lines = append(lines[:avail-1], sty.faint.Render("…"))
	}
	return append([]string{label}, lines...)
}

// waitingLines are the middle of the screen before the deck reports: what
// to do to start it, centred, under a dialog if one is open, which doesn't
// need the deck.
func (p presenter) waitingLines(inner, rest int) []string {
	sty := p.sty
	centre := func(st lipgloss.Style, s string) []string {
		return strings.Split(st.Width(inner).Align(lipgloss.Center).Render(s), "\n")
	}
	lines := centre(sty.accent, "Waiting for the deck…")
	lines = append(lines, "")
	switch {
	case p.launching:
		lines = append(lines, centre(sty.accent, "Opening the deck window…")...)
	case p.launchErr != "":
		lines = append(lines, centre(sty.warn, p.launchErr)...)
		lines = append(lines, centre(sty.muted, "Press p to try again, or start the deck in another window.")...)
	case p.launch != nil:
		lines = append(lines, centre(sty.text, "Press p to open it in a new Ghostty window,")...)
		lines = append(lines, centre(sty.muted, "or start the deck in another window.")...)
	default:
		lines = append(lines, centre(sty.text, "Start the deck in another window.")...)
	}
	lines = append(lines, centre(sty.faint, p.socket)...)
	if pad := (rest - len(lines)) / 2; pad > 0 {
		lines = append(make([]string, pad), lines...)
	}
	if p.dialog != dialogNone {
		return p.drawDialog(lines, inner, rest)
	}
	return lines
}

// bar is time used against the talk's length, with the clock.
func (p presenter) bar(inner int) string {
	sty, el := p.sty, p.elapsed()
	labelStyle, fill := sty.text.Bold(true), sty.accent
	switch {
	case !p.running && el == 0:
		labelStyle = sty.muted
	case el > p.length:
		labelStyle, fill = sty.warn, sty.warn
	}
	label := labelStyle.Render(clock(el))
	for _, tail := range []string{
		sty.muted.Render(" "+clock(p.length)) + "   " + sty.muted.Render(p.now.Format("15:04")),
		sty.muted.Render(" " + clock(p.length)),
		"",
	} {
		barW := inner - lipgloss.Width(label) - 1 - lipgloss.Width(tail)
		if barW < 10 && tail != "" {
			continue
		}
		barW = max(barW, 1)
		used := min(int(float64(barW)*float64(el)/float64(max(p.length, 1))), barW)
		return label + " " + fill.Render(strings.Repeat("━", used)) + sty.faint.Render(strings.Repeat("─", barW-used)) + tail
	}
	return label
}
