package decker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Run runs the presenter view until the user quits: notes, previews and a
// timer in a window of its own, linked to the deck over the socket. Its keys
// drive the deck, so a clicker aimed here runs the show.
func (c presentCmd) Run(d *Deck) error {
	images, err := imagePreviews(c.Previews)
	if err != nil {
		return err
	}
	window, err := newPresentationWindow(c.Socket, c.PresentationFontSize, c.args())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := newPresenter(d, c.Socket, c.Length)
	p.st.Slide, p.st.Step = max(c.Slide-1, 0), max(c.Step-1, 0)
	if p.post = newPosting(d, postingRoot(), p.drawMu); p.post != nil {
		restore, err := logTo(filepath.Join(p.post.home, "posting.log"))
		if err != nil {
			return err
		}
		defer restore()
		defer p.post.end()
		// Media is rendered afresh in each run; what an earlier one left is
		// stale.
		if err := os.RemoveAll(filepath.Join(p.post.home, "media")); err != nil {
			return err
		}
	}
	window.ctx = ctx
	p.launch = window.open
	if images {
		p.images = newKittyImages()
		defer func() { fmt.Fprint(os.Stdout, p.images.clear()) }()
	}
	_, err = tea.NewProgram(p, tea.WithFilter(presenterFilter)).Run()
	p.link.close()
	return err
}

// A resize may arrive after an upload command was queued but before RawMsg
// executes. Check again before Bubble Tea writes it to the terminal.
func presenterFilter(m tea.Model, msg tea.Msg) tea.Msg {
	if raw, ok := msg.(tea.RawMsg); ok {
		if upload, ok := raw.Msg.(kittyUploadMsg); ok {
			p := m.(presenter)
			if p.images == nil || !p.images.valid(upload) {
				return nil
			}
		}
	}
	return msg
}

func newPresenter(d *Deck, socket string, length time.Duration) presenter {
	return presenter{
		deck: d, sty: d.Theme.styles(), socket: socket, length: length,
		link: &linkClient{}, previews: map[previewKey]string{}, now: time.Now(),
		drawMu: &sync.Mutex{},
	}
}

type presenter struct {
	deck     *Deck // this build's, for its theme and to draw previews
	sty      styles
	socket   string
	length   time.Duration
	link     *linkClient
	previews map[previewKey]string // drawn with half blocks
	images   *kittyImages
	drawMu   *sync.Mutex // slide Views expect one caller at a time

	st            linkState // the deck's last report; Outline is nil until the first
	linked        bool
	w, h          int
	now           time.Time
	count         string // numeric prefix for jumps, e.g. "12g"
	launch        func(slide, step int) error
	launching     bool
	launchBusy    bool
	launchErr     string
	launchAttempt int

	post      *posting   // nil if the notes have no posts
	dialog    dialogKind // the dialog open over the middle of the screen
	quitArmed time.Time  // until when a second q quits, with posts still to send

	// The talk timer starts when the deck first leaves slide 1, or with t.
	running bool
	since   time.Time     // when it last started
	banked  time.Duration // time counted before the last pause
}

// dialogKind is which of the presenter view's dialogs is open.
type dialogKind uint8

const (
	dialogNone dialogKind = iota
	dialogKeys
	dialogPosting
)

type (
	presTickMsg           time.Time
	linkUpMsg             struct{}
	linkDownMsg           struct{}
	linkStateMsg          linkState
	presentationOpenedMsg struct {
		attempt int
		err     error
	}
	presentationWaitMsg int
)

const reconnectEvery = 500 * time.Millisecond

func (p presenter) Init() tea.Cmd { return tea.Batch(p.tick(), p.connect(0)) }

// pub is the open thread, live or a rehearsal; nil if there's none.
func (p presenter) pub() *publishing {
	if p.post == nil {
		return nil
	}
	return p.post.pub
}

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
	if p, cmd, ok := p.postingMsg(msg); ok {
		return p, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		changed := p.w != msg.Width || p.h != msg.Height
		p.w, p.h = msg.Width, msg.Height
		if changed {
			clear(p.previews)
			if p.images != nil {
				return p, tea.Sequence(tea.Raw(p.images.clear()), p.upload())
			}
		}
	case presTickMsg:
		p.now = time.Time(msg)
		return p, p.tick()
	case linkUpMsg:
		p.linked = true
		p.launching, p.launchErr = false, ""
		return p, p.listen()
	case linkDownMsg:
		p.linked = false
		if pb := p.pub(); pb != nil {
			pb.known = false
		}
		p.link.close()
		return p, p.connect(reconnectEvery)
	case linkStateMsg:
		resized := p.st.W != msg.W || p.st.H != msg.H
		p.st = linkState(msg)
		if !p.running && p.banked == 0 && p.st.Slide > 0 {
			p.running, p.since = true, time.Now()
		}
		post := p.track()
		if resized {
			clear(p.previews)
			if p.images != nil {
				return p, tea.Batch(p.listen(), post, tea.Sequence(tea.Raw(p.images.clear()), p.upload()))
			}
		}
		return p, tea.Batch(p.listen(), post, p.upload())
	case kittyUploadMsg:
		if p.images != nil {
			if p.images.finish(msg) != "" {
				return p, tea.Raw(msg)
			}
		}
	case tea.RawMsg:
		if upload, ok := msg.Msg.(kittyUploadMsg); ok && p.images != nil && p.images.valid(upload) {
			p.images.ready[upload.key] = true
		}
	case presentationOpenedMsg:
		if msg.attempt != p.launchAttempt {
			return p, nil
		}
		p.launchBusy = false
		if p.linked {
			return p, nil
		}
		if msg.err != nil {
			p.launching, p.launchErr = false, msg.err.Error()
			return p, nil
		}
		return p, tea.Tick(10*time.Second, func(time.Time) tea.Msg { return presentationWaitMsg(msg.attempt) })
	case presentationWaitMsg:
		if int(msg) == p.launchAttempt && p.launching && !p.linked {
			p.launching = false
			p.launchErr = "it hasn't connected; check its window"
		}
	case tea.KeyPressMsg:
		if p.dialog == dialogPosting {
			if p, cmd, ok := p.postingKey(msg); ok {
				return p, cmd
			}
		}
		return p.handleKey(msg.String())
	}
	return p, nil
}

func (p presenter) handleKey(k string) (tea.Model, tea.Cmd) {
	if k != "q" {
		p.quitArmed = time.Time{}
	}
	if n, done := countKey(&p.count, k); done {
		if n > 0 {
			p.link.send(linkCmd{Slide: n})
		}
		return p, nil
	}
	switch {
	case keyActs[k].act == "quit":
		// q asks again while a live thread has posts to send: they'd be
		// lost. ctrl+c always quits.
		if now := time.Now(); k == "q" && p.unsent() > 0 && !p.quitArmed.After(now) {
			p.quitArmed, p.now = now.Add(3*time.Second), now
			return p, nil
		}
		if p.images != nil {
			return p, tea.Sequence(tea.Raw(p.images.clear()), tea.Quit)
		}
		return p, tea.Quit
	case k == "p":
		if p.linked || p.launching || p.launchBusy || p.launch == nil {
			return p, nil
		}
		p.launching, p.launchBusy, p.launchErr = true, true, ""
		p.launchAttempt++
		slide, step := max(p.st.Slide+1, 1), max(p.st.Step+1, 1)
		attempt := p.launchAttempt
		return p, func() tea.Msg { return presentationOpenedMsg{attempt, p.launch(slide, step)} }
	case k == "t":
		if p.running {
			p.banked += time.Since(p.since)
			p.running = false
		} else {
			p.running, p.since = true, time.Now()
		}
	case k == "T":
		p.running, p.banked = false, 0
	case keyActs[k].act == "help":
		if p.dialog == dialogKeys {
			p.dialog = dialogNone
		} else {
			p.dialog = dialogKeys
		}
	case keyActs[k].act == "closeHelp":
		p.dialog = dialogNone
	case k == "P" && p.post != nil:
		p.post.show()
		p.dialog = dialogPosting
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

// nextTarget is the slide and step the "next" preview shows, with its
// label. ok is false on the last step of the last slide.
func (p presenter) nextTarget() (next [2]int, label string, ok bool) {
	switch cur := p.curOutline(); {
	case p.st.Step < cur.Steps-1:
		return [2]int{p.st.Slide, p.st.Step + 1}, fmt.Sprintf("NEXT · step %d", p.st.Step+2), true
	case p.st.Slide < len(p.st.Outline)-1:
		n := p.st.Slide + 1
		return [2]int{n, 0}, fmt.Sprintf("NEXT · %d/%d %s", n+1, len(p.st.Outline), p.st.Outline[n].Title), true
	}
	return next, "", false
}

// nextKey is k moved to the slide and step the "next" preview shows
// (nextTarget), with its label.
func (p presenter) nextKey(k previewKey) (next previewKey, label string, ok bool) {
	at, label, ok := p.nextTarget()
	k.slide, k.step = at[0], at[1]
	return k, label, ok
}

// matches reports whether this build's slide i is the deck's slide i; in dev
// mode the deck rebuilds and the presenter view doesn't, so they can drift.
func (p presenter) matches(i int) bool {
	return i < len(p.deck.Slides) && i < len(p.st.Outline) && p.deck.Slides[i].Title == p.st.Outline[i].Title
}

// preview draws the slide k names, settled. A slide whose title no longer
// matches the deck's (dev mode) is left out.
func (p presenter) preview(k previewKey) string {
	if !p.matches(k.slide) {
		return p.placeholder(k.pw, k.ph, "preview out of date: restart the presenter view")
	}
	if p.images != nil && p.images.ready[k] {
		return kittyPlaceholders(p.images.ids[k], k.pw, k.ph)
	}
	if s, ok := p.previews[k]; ok {
		return s
	}
	p.drawMu.Lock()
	s := renderPreview(p.deck, k)
	p.drawMu.Unlock()
	p.previews[k] = s
	return s
}

// upload draws and encodes missing previews off the event loop. Placeholders
// replace the cell fallback only after the terminal has received the image.
func (p presenter) upload() tea.Cmd {
	if p.images == nil || p.st.Outline == nil {
		return nil
	}
	k, ok := p.key(p.st.Slide, p.st.Step)
	if !ok {
		return nil
	}
	targets := []previewKey{k}
	if nk, _, ok := p.nextKey(k); ok {
		targets = append(targets, nk)
	}
	var cmds []tea.Cmd
	for _, k := range targets {
		if _, ok := p.images.id(k); ok || !p.matches(k.slide) {
			continue
		}
		id, free := p.images.add(k)
		gen := p.images.gen
		cmd := func() tea.Msg {
			p.drawMu.Lock()
			img := slideImage(p.deck, k)
			p.drawMu.Unlock()
			return kittyUploadMsg{gen, k, id, kittyTransmit(id, img, k.pw, k.ph)}
		}
		if free != "" {
			cmds = append(cmds, tea.Sequence(tea.Raw(free), cmd))
		} else {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// noNotes stands in for the speaker notes of a slide that has none.
const noNotes = "(no notes for this slide)"

// meter is a gauge total cells long: lit of them drawn as on in the accent
// color, the rest as off, faint. Build steps are dots, progress a bar.
func (s styles) meter(on, off string, lit, total int) string {
	return s.accent.Render(strings.Repeat(on, lit)) + s.faint.Render(strings.Repeat(off, total-lit))
}

// jumping is the prompt shown while the slide number of a jump is typed.
func (s styles) jumping(count string) string { return s.accent2.Render("go to " + count + "…") }

func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return truncate(left+" "+right, w)
	}
	return left + strings.Repeat(" ", gap) + right
}

func truncate(s string, w int) string { return ansi.Truncate(s, max(w, 0), "") }

func indent(s, pad string) string {
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func frame(s string, c RGB) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.Color()).Render(s)
}

func (p presenter) placeholder(w, h int, msg string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, p.sty.faint.Render(msg))
}

// track tells the open thread where the deck is, as the link last reported
// it, and starts its next post if one is due. The slide the deck moved on
// from posts, and the last slide does once its last build is up; the
// slide up uploads its media meanwhile. A slide of this build that isn't
// the deck's (dev mode) does none of it.
func (p presenter) track() tea.Cmd {
	pb := p.pub()
	if pb == nil {
		return nil
	}
	var upload tea.Cmd
	if p.linked && p.st.Outline != nil {
		if left, ok := pb.at(p.st.Slide, p.st.Step); ok && p.matches(left) {
			pb.enqueue(left)
		}
		if p.matches(pb.slide) {
			if pb.lastShown(p.st.Outline) {
				pb.enqueue(pb.slide)
			}
			upload = pb.upload(pb.slide)
		}
	}
	return tea.Batch(upload, pb.next())
}
