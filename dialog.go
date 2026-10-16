package decker

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The presenter view's dialogs, posting (P) and the keys sheet (?), are
// boxes centred on the middle of the screen, which dims behind them.

// boxLine is a line of a dialog, with how readily it's dropped when the
// screen is short of lines: the highest drop goes first, and dropNever
// never does.
type boxLine struct {
	text string
	drop int
}

const (
	dropNever  = iota
	dropDetail // a message's lines after its first, and the keys sheet's rows
	dropHelp   // what the focused control does
	dropPosts  // how many posts the talk makes
	dropBlank
)

const (
	dialogMaxW = 68 // the widest a dialog gets, its border included
	dialogPad  = 2  // cells between a dialog's border and its lines
)

// dialogWidth is the width of a dialog's lines, inside its border and
// padding.
func (p presenter) dialogWidth() int { return min(p.inner(), dialogMaxW) - 2 - 2*dialogPad }

// box draws a dialog: its title in the top border, the first of hints that
// fits in the bottom one, and the body then the buttons between. Short of
// lines, the body drops lines, the most droppable first; the buttons stay.
func (p presenter) box(title string, hints []string, border lipgloss.Style, body []boxLine, buttons string, rest int) []string {
	sty, w, cw := p.sty, min(p.inner(), dialogMaxW), p.dialogWidth()
	n := rest - 2
	if buttons != "" {
		n--
	}
	lines := fitBox(body, max(n, 0))
	if buttons != "" {
		lines = append(lines, buttons)
	}
	edge := func(left, right, label string) string {
		if label == "" || lipgloss.Width(label) > w-6 {
			return border.Render(left + strings.Repeat("─", w-2) + right)
		}
		return border.Render(left+"─ ") + label + border.Render(" "+strings.Repeat("─", w-5-lipgloss.Width(label))+right)
	}
	out := []string{edge("╭", "╮", sty.text.Bold(true).Render(ansi.Truncate(title, max(w-6, 0), "…")))}
	pad, side := strings.Repeat(" ", dialogPad), border.Render("│")
	for _, l := range lines {
		l = truncate(l, cw)
		out = append(out, side+pad+l+strings.Repeat(" ", cw-lipgloss.Width(l))+pad+side)
	}
	hint := ""
	for _, h := range hints {
		if lipgloss.Width(h) <= w-6 {
			hint = sty.muted.Render(h)
			break
		}
	}
	return append(out, edge("╰", "╯", hint))
}

// fitBox drops lines, the most droppable first and from the bottom, until
// n are left; it cuts the rest if those that never drop don't fit.
func fitBox(lines []boxLine, n int) []string {
	for len(lines) > n {
		worst := dropNever
		for _, l := range lines {
			worst = max(worst, l.drop)
		}
		if worst == dropNever {
			lines = lines[:n]
			break
		}
		for i := len(lines) - 1; i >= 0; i-- {
			if lines[i].drop == worst {
				lines = append(lines[:i], lines[i+1:]...)
				break
			}
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.text
	}
	return out
}

// overDimmed is box centred on bg, rest lines of the middle of the screen,
// which shows round it as plain text in the faint color.
func (p presenter) overDimmed(bg, box []string, inner, rest int) []string {
	bg = fitLines(bg, rest)
	if len(box) > rest {
		box = box[:rest]
	}
	if len(box) == 0 {
		return bg
	}
	bw := lipgloss.Width(box[0])
	x, y := max((inner-bw)/2, 0), (rest-len(box))/2
	out := make([]string, rest)
	for i, l := range bg {
		plain := ansi.Strip(l)
		if i < y || i >= y+len(box) {
			out[i] = p.sty.faint.Render(plain)
			continue
		}
		left := ansi.Truncate(plain, x, "")
		left += strings.Repeat(" ", x-lipgloss.Width(left))
		out[i] = p.sty.faint.Render(left) + box[i-y] + p.sty.faint.Render(ansi.TruncateLeft(plain, x+bw, ""))
	}
	return out
}

// drawDialog is the open dialog, over bg.
func (p presenter) drawDialog(bg []string, inner, rest int) []string {
	var box []string
	if p.dialog == dialogKeys {
		box = p.keysBox(rest)
	} else {
		box = p.postingDialog(rest)
	}
	return p.overDimmed(bg, box, inner, rest)
}

// ctl is one of the posting dialog's controls.
type ctl uint8

const (
	ctlAccount ctl = iota + 1
	ctlNewThread
	ctlCarryOn
	ctlRehearse
	ctlStart
	ctlDismiss // Cancel before anything starts, which closes the dialog
	ctlAbort   // Cancel while setting up, which stops it
	ctlClose
	ctlPause // pause or resume
	ctlEndRehearsal
)

// controls are the dialog's controls, in focus order, and the one focused
// at first. While anything runs or posts, that's Close, so a clicker's
// enter or space does nothing.
func (ps *posting) controls() (cs []ctl, first ctl) {
	switch {
	case ps.start != nil:
		return []ctl{ctlAbort, ctlClose}, ctlClose
	case ps.rehearsing():
		return []ctl{ctlPause, ctlEndRehearsal, ctlClose}, ctlClose
	case ps.live():
		return []ctl{ctlPause, ctlClose}, ctlClose
	case ps.deck.Publisher == nil:
		return []ctl{ctlRehearse, ctlDismiss}, ctlRehearse
	}
	cs, first = []ctl{ctlAccount}, ctlStart
	if ps.typed() == "" {
		first = ctlAccount
	}
	if ps.last.dir != "" {
		cs = append(cs, ctlNewThread, ctlCarryOn)
	}
	return append(cs, ctlRehearse, ctlStart, ctlDismiss), first
}

// focused is the control with the focus.
func (ps *posting) focused() ctl {
	cs, first := ps.controls()
	if slices.Contains(cs, ps.focus) {
		return ps.focus
	}
	return first
}

// move moves the focus by d controls, round from the last to the first.
func (ps *posting) move(d int) {
	cs, _ := ps.controls()
	i := slices.Index(cs, ps.focused())
	ps.focus = cs[(i+d+len(cs))%len(cs)]
}

// show readies the dialog to open, focused where it starts, with the
// account as it was saved, and without a message from a setup that has
// since been cancelled or has worked.
func (ps *posting) show() {
	ps.focus = 0
	if ps.start == nil && ps.pub == nil {
		ps.input = ps.account
		if !ps.failed {
			ps.msg = nil
		}
	}
}

// postingKey handles a key while the posting dialog is open: all of them
// but ctrl+c, so none reaches the deck. esc only ever closes the dialog.
func (p presenter) postingKey(msg tea.KeyPressMsg) (_ presenter, cmd tea.Cmd, ok bool) {
	ps := p.post
	f := ps.focused()
	switch k := msg.String(); k {
	case "ctrl+c":
		return p, nil, false
	case "esc":
		p.dialog = dialogNone
	case "tab", "down":
		ps.move(1)
	case "shift+tab", "up":
		ps.move(-1)
	case "right":
		if f != ctlAccount {
			ps.move(1)
		}
	case "left":
		if f != ctlAccount {
			ps.move(-1)
		}
	case "enter":
		if f == ctlAccount {
			f = ctlStart
		}
		p, cmd = p.activate(f)
		return p, cmd, true
	case "space":
		if f != ctlAccount {
			p, cmd = p.activate(f)
			return p, cmd, true
		}
	case "backspace":
		if r := []rune(ps.input); f == ctlAccount && len(r) > 0 {
			ps.input = string(r[:len(r)-1])
		}
	default:
		t := msg.Text
		switch {
		case f == ctlAccount && t != "" && !strings.ContainsFunc(t, unicode.IsSpace) && !strings.ContainsFunc(t, unicode.IsControl):
			ps.input += t
		case k == "P" && f != ctlAccount:
			p.dialog = dialogNone
		}
	}
	return p, nil, true
}

// activate does what control c does.
func (p presenter) activate(c ctl) (presenter, tea.Cmd) {
	ps := p.post
	switch c {
	case ctlNewThread, ctlCarryOn:
		ps.cont = c == ctlCarryOn
	case ctlRehearse:
		return p, ps.begin(startRehearsal)
	case ctlStart:
		account := ps.typed()
		if account == "" {
			ps.focus = ctlAccount
			ps.refuse("Type the account to post as first.")
			return p, nil
		}
		if account == ps.account {
			return p, ps.begin(startLive)
		}
		ps.account, ps.signedIn, ps.session = account, "", nil
		root := ps.root
		save := func() tea.Msg { return accountSavedMsg{saveAccount(root, account)} }
		return p, tea.Batch(ps.begin(startLive), save)
	case ctlDismiss, ctlClose:
		p.dialog = dialogNone
	case ctlAbort:
		ps.stop()
	case ctlPause:
		ps.pub.on = !ps.pub.on
		return p, p.track()
	case ctlEndRehearsal:
		ps.pub, ps.msg = nil, nil
	}
	return p, nil
}

// dialogMessage is what the posting dialog says about how things stand: a
// post failing, the thread file not saving, a pause, or the last thing
// that happened.
func (p presenter) dialogMessage() []msgLine {
	ps, pb := p.post, p.post.pub
	if pb == nil {
		return ps.msg
	}
	switch pb.state() {
	case threadRetrying:
		tries := fmt.Sprintf("%d tries", pb.fails)
		if pb.fails == 1 {
			tries = "1 try"
		}
		return []msgLine{
			{text: fmt.Sprintf("▲ Post %d failed (%s); retrying in %s. Posting carries on by itself:",
				pb.sent()+1, tries, clock(max(pb.retryAt.Sub(p.now), 0))), tone: toneWarn},
			{text: pb.err, detail: true},
		}
	case threadUnsaved:
		return []msgLine{
			{text: "▲ Posted, but the thread file wasn't saved; the next post saves it, or won't go out:", tone: toneWarn},
			{text: pb.saveErr, detail: true},
		}
	case threadPaused:
		if pb.dry {
			return []msgLine{{text: "‖ Paused: slides you move on from won't be written until you resume.", tone: toneWarn}}
		}
		return []msgLine{{text: "‖ Paused: slides you move on from won't post until you resume.", tone: toneWarn}}
	}
	return ps.msg
}

// postingDialog is the posting dialog: setting posting up, its progress
// while it starts, or how posting is going, with the buttons for each.
func (p presenter) postingDialog(rest int) []string {
	ps, sty, cw := p.post, p.sty, p.dialogWidth()
	f := ps.focused()
	var body []boxLine
	add := func(drop int, s string) { body = append(body, boxLine{s, drop}) }
	// wrapped adds s wrapped to the dialog, its first line dropping as
	// first does and the rest as later.
	wrapped := func(first, later int, st lipgloss.Style, s, indent string) {
		for i, l := range strings.Split(lipgloss.Wrap(s, max(cw-len(indent), 10), ""), "\n") {
			drop := later
			if i == 0 {
				drop = first
			}
			add(drop, indent+st.Render(l))
		}
	}
	say := func(msg []msgLine) {
		for i, m := range msg {
			first, indent := dropDetail, ""
			if i == 0 {
				first = dropNever
			}
			if m.detail {
				indent = "  "
			}
			wrapped(first, dropDetail, sty.tone(m.tone), m.text, indent)
		}
	}
	const labelW = 10
	label := func(s string) string { return sty.muted.Render(fmt.Sprintf("%-*s", labelW, s)) }

	title := "Post this talk"
	switch st := ps.start; {
	case st != nil:
		add(dropBlank, "")
		switch st.job {
		case jobSignIn:
			if st.visit == "" {
				wrapped(dropNever, dropDetail, sty.accent, "◌ Signing in as "+ps.account+"…", "")
				break
			}
			wrapped(dropNever, dropDetail, sty.accent, "◌ Finish signing in in your browser…", "")
			add(dropNever, sty.muted.Render("No browser opened? Visit this address:"))
			for _, l := range wrapHard(st.visit, cw) {
				add(dropNever, sty.accent2.Render(l))
			}
		case jobRender:
			n := len(ps.deck.Slides)
			meter := sty.meter("▰", "▱", (st.slide*10+n-1)/max(n, 1), 10)
			add(dropNever, sty.accent.Render(fmt.Sprintf("◌ Preparing posts: slide %d of %d  ", st.slide+1, n))+meter)
			wrapped(dropDetail, dropDetail, sty.muted, "Drawing each post's images and clips. Nothing posts until this is done.", "")
		case jobCheck:
			add(dropNever, sty.accent.Render(fmt.Sprintf("◌ Checking %s…", plural(len(ps.allPosts()), "post"))))
		}
		say(ps.msg)
	case ps.pub != nil:
		pb := ps.pub
		title = "Posting as @" + ps.signedIn
		if pb.dry {
			title = "Rehearsing posts"
		}
		add(dropBlank, "")
		summary := fmt.Sprintf("%d of %d %s%s", pb.sent(), pb.total(), pb.verb(), pb.skippedText())
		if !pb.dry {
			if n := len(pb.queue); n > 0 {
				summary += fmt.Sprintf(", %d to send", n)
			}
			if ps.cont {
				summary += " · carrying on the thread from " + ps.last.at.Format("Jan 2 15:04")
			} else {
				summary += " · a new thread"
			}
		}
		wrapped(dropNever, dropDetail, sty.text, summary, "")
		if pb.skipped() > 0 {
			wrapped(dropDetail, dropDetail, sty.muted, "Skipped posts are on slides you jumped over, or passed while paused. Going back to one and moving on posts it, at the end of the thread.", "")
		}
		if pb.dry {
			for _, l := range wrapHard("in "+shortPath(pb.file), cw) {
				add(dropDetail, sty.muted.Render(l))
			}
		}
		say(p.dialogMessage())
	default:
		opener, onSlides := ps.notes.postCounts()
		posts := fmt.Sprintf("This talk makes %s, on slides.", plural(onSlides, "post"))
		if opener > 0 {
			posts = fmt.Sprintf("This talk makes %s: %d to open the thread, %d on slides.", plural(opener+onSlides, "post"), opener, onSlides)
		}
		add(dropBlank, "")
		wrapped(dropPosts, dropPosts, sty.muted, posts, "")
		add(dropBlank, "")
		if ps.deck.Publisher == nil {
			add(dropNever, label("Account")+sty.faint.Render("none: the talk has no Deck.Publisher"))
			break
		}
		status := ""
		if !ps.needsSignIn() {
			status = "  " + sty.good.Render("✓ signed in")
		}
		add(dropNever, label("Account")+p.field(f == ctlAccount, min(cw-labelW-13, 34))+status)
		add(dropBlank, "")
		if ps.last.dir == "" {
			add(dropPosts, label("Thread")+sty.text.Render("a new thread"))
		} else {
			add(dropNever, label("Thread")+p.radio(!ps.cont, f == ctlNewThread, "Start a new thread"))
			add(dropNever, label("")+p.radio(ps.cont, f == ctlCarryOn,
				fmt.Sprintf("Carry on %s · %s", ps.last.at.Format("Jan 2 15:04"), plural(ps.last.posted, "post"))))
		}
		if len(ps.msg) > 0 {
			add(dropBlank, "")
		}
		say(ps.msg)
	}
	add(dropBlank, "")
	wrapped(dropHelp, dropHelp, sty.muted, p.helpFor(f), "")
	add(dropBlank, "")

	cs, _ := ps.controls()
	hints := []string{"tab moves · enter selects · esc closes · deck keys paused", "tab moves · enter selects · esc closes", "esc closes"}
	return p.box(title, hints, sty.accent, body, p.buttons(cs, f, cw), rest)
}

// field is the account field, w cells wide, with the cursor if focused.
func (p presenter) field(focused bool, w int) string {
	sty, tw := p.sty, w-4
	brackets, cursor, room := sty.muted, "", tw
	if focused {
		brackets, cursor, room = sty.accent, sty.accent.Render("▏"), tw-1
	}
	var shown string
	switch s := p.post.input; {
	case s == "":
		shown = cursor + sty.faint.Render(ansi.Truncate("me.bsky.social", max(room, 0), "…"))
	case lipgloss.Width(s) > room:
		shown = sty.text.Render("…"+ansi.TruncateLeft(s, lipgloss.Width(s)-room+1, "")) + cursor
	default:
		shown = sty.text.Render(s) + cursor
	}
	return brackets.Render("[ ") + shown + strings.Repeat(" ", max(tw-lipgloss.Width(shown), 0)) + brackets.Render(" ]")
}

// radio is one of a choice's options: chosen or not, and focused or not.
func (p presenter) radio(chosen, focused bool, label string) string {
	mark := "( )"
	if chosen {
		mark = "(•)"
	}
	st := p.sty.text
	if focused {
		st = p.sty.accent.Reverse(true)
	}
	return p.sty.accent.Render(mark) + st.Render(" "+label+" ")
}

// buttons are the dialog's buttons, right-aligned, f focused, in short
// forms if the long ones don't fit cw.
func (p presenter) buttons(cs []ctl, f ctl, cw int) string {
	sty := p.sty
	row := func(short bool) string {
		var parts []string
		for _, c := range cs {
			long, brief := p.buttonLabel(c)
			if long == "" {
				continue
			}
			l := long
			if short {
				l = brief
			}
			st := sty.text
			switch {
			case c == f:
				st = sty.accent.Reverse(true)
			case c == ctlStart:
				st = sty.accent
			}
			parts = append(parts, st.Render("[ "+l+" ]"))
		}
		return strings.Join(parts, "  ")
	}
	line := row(false)
	if lipgloss.Width(line) > cw {
		line = row(true)
	}
	return strings.Repeat(" ", max(cw-lipgloss.Width(line), 0)) + line
}

// buttonLabel is c's label, long and short, or "" if it isn't a button.
func (p presenter) buttonLabel(c ctl) (long, short string) {
	ps := p.post
	switch c {
	case ctlRehearse:
		return "Rehearse", "Rehearse"
	case ctlStart:
		if ps.needsSignIn() {
			return "Sign in & start posting", "Start"
		}
		return "Start posting", "Start"
	case ctlDismiss, ctlAbort:
		return "Cancel", "Cancel"
	case ctlClose:
		return "Close", "Close"
	case ctlPause:
		switch {
		case ps.pub.on && ps.pub.dry:
			return "Pause", "Pause"
		case ps.pub.on:
			return "Pause posting", "Pause"
		case ps.pub.dry:
			return "Resume", "Resume"
		}
		return "Resume posting", "Resume"
	case ctlEndRehearsal:
		return "Stop rehearsing", "Stop"
	}
	return "", ""
}

// helpFor is what control c does, for the line over the buttons.
func (p presenter) helpFor(c ctl) string {
	ps := p.post
	switch c {
	case ctlAccount:
		return "The account to post as, like me.bsky.social. Enter starts posting."
	case ctlNewThread:
		return "The opening post starts a new thread, and each slide replies to it."
	case ctlCarryOn:
		return "Slides reply to the thread from last time. What it has isn't posted again."
	case ctlRehearse:
		return "Practise: each post is written to a file as you move on, and nothing is sent."
	case ctlStart:
		if ps.needsSignIn() {
			return "Signs in through your browser. Then each slide posts as you move on from it; slides you skip don't."
		}
		return "Each slide posts as you move on from it; slides you skip don't."
	case ctlDismiss:
		return "Closes this without posting anything."
	case ctlAbort:
		return "Stops setting up. Nothing has been posted."
	case ctlClose:
		switch {
		case ps.start != nil:
			return "Closing doesn't stop it: the status line shows how it goes, and P opens this again."
		case ps.rehearsing():
			return "The rehearsal carries on: the status line shows how it goes, and P opens this again."
		}
		return "Posting carries on: the status line shows how it goes, and P opens this again."
	case ctlPause:
		if ps.pub.on {
			return "While paused, slides you move on from don't post, and queued posts wait."
		}
		return "Slides you move on from post again. Ones passed while paused stay unposted."
	case ctlEndRehearsal:
		return "Stops writing posts to the file, to set up posting again."
	}
	return ""
}

// keysBox is the keys sheet (?): the deck's keys the presenter view
// passes on, from the one key table, and its own. Unlike the posting
// dialog it takes no keys, so the deck's keys still work.
func (p presenter) keysBox(rest int) []string {
	sty := p.sty
	body := []boxLine{{"", dropBlank}}
	row := func(keys, what string) {
		body = append(body, boxLine{sty.text.Bold(true).Render(fmt.Sprintf("%-20s", keys)) + sty.muted.Render(what), dropDetail})
	}
	for _, b := range bindings {
		if b.help != "" && (b.nav || b.act == "") {
			row(b.help, b.what)
		}
	}
	row("t  T", "start or pause the timer / reset it")
	if p.launch != nil {
		row("p", "open the deck in a new Ghostty window")
	}
	if p.post != nil {
		row("P", "post the talk as you give it")
		row("q", "quit (twice while posts wait to send)")
	} else {
		row("q", "quit")
	}
	return p.box("Keys", []string{"? or esc closes · deck keys still work", "esc closes"}, sty.muted, body, "", rest)
}
