package decker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// posting is the presenter view's posting dialog (P) and what it runs:
// the account to post as and signing in to it, which thread to post to,
// and posting or rehearsing. Files live under root: the account in
// posting.json, and each talk's threads, rehearsal, media and log under
// talks/NAME. Whatever touches them or the network runs in a command, so
// the presenter view never waits on it.
type posting struct {
	deck    *Deck
	notes   deckNotes
	lock    sync.Locker // held while a slide draws for a post's media
	root    string
	home    string // this talk's directory
	openURL func(string) error

	account  string  // as the speaker typed it, kept between runs
	signedIn string  // the account session is signed in as, as typed
	session  Session // nil until signed in
	cont     bool    // carry on the last thread rather than start one
	last     lastThread

	keys map[int][]string // postKeys

	// rendered are the posts with their media drawn, by slide (-1 opens
	// the thread). They're kept for every start after the first: the
	// deck's frames don't change while the presenter view runs.
	rendered map[int][]Post
	renders  int // starts that rendered, each into a directory of its own

	start  *start      // the start in progress; nil if none is
	pub    *publishing // the open thread, live or a rehearsal; nil until one starts
	msg    []msgLine
	failed bool // the last start failed, so nothing is posting

	input string // the account field
	focus ctl    // the focused control; 0 for the one each state starts on
}

// start is a start in progress, each step's message starting the next:
// signing in if posting needs it, rendering the posts' media, then having
// the Publisher check them. Its messages carry it: one from a start that
// has since ended is stale.
type start struct {
	kind   startKind
	job    jobKind
	visit  string // the sign-in page, while it waits on the speaker
	dir    string // the thread's, or the rehearsal's
	media  string // where this start renders, if it has to
	slide  int    // the next slide to render
	ctx    context.Context
	cancel context.CancelFunc
}

type startKind uint8

const (
	startLive startKind = iota
	startRehearsal
)

type jobKind uint8

const (
	jobSignIn jobKind = iota
	jobRender
	jobCheck
)

// msgLine is a line of the dialog's message: a detail line is indented
// under the one before.
type msgLine struct {
	text   string
	tone   tone
	detail bool
}

// lastThread is the most recent live thread on disk with anything posted.
type lastThread struct {
	dir    string
	posted int
	at     time.Time // its last post
}

type (
	signedInMsg struct {
		st      *start
		account string
		session Session
		err     error
	}
	signInVisitMsg struct {
		st  *start
		url string
	}
	postsRenderedMsg struct {
		st    *start
		slide int
		posts []Post
		err   error
	}
	// postsPreparedMsg ends a start: the thread it opens and what posts
	// to it, or why it can't, and what the Publisher's check found.
	postsPreparedMsg struct {
		st       *start
		thread   *threadLog
		pub      Thread
		err      error
		checkErr error
	}
	accountSavedMsg struct{ err error }
)

// postingRoot is where posting keeps its files: the user's config
// directory, or the temp directory if there is none.
func postingRoot() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "decker")
}

// newPosting is d's posting dialog, keeping its files under root and
// holding lock while a slide draws; nil if d's notes have no posts.
func newPosting(d *Deck, root string, lock sync.Locker) *posting {
	dn := d.notes()
	if opener, onSlides := dn.postCounts(); opener+onSlides == 0 {
		return nil
	}
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == os.PathSeparator || unicode.IsControl(r) {
			return '-'
		}
		return r
	}, d.Name)
	if name == "." || name == ".." {
		name = strings.Repeat("-", len(name)) // either would name a directory above the talk's
	}
	keys := postKeys(dn, d.Slides)
	ps := &posting{deck: d, notes: dn, keys: keys, lock: lock, root: root, home: filepath.Join(root, "talks", name), openURL: openURL,
		rendered: map[int][]Post{-1: openerPosts(dn.opener, keys[-1])}}
	ps.account = loadAccount(root)
	ps.last = findLastThread(filepath.Join(ps.home, "threads"))
	return ps
}

// postCounts are how many posts the notes make: opening the thread, and on
// slides.
func (dn deckNotes) postCounts() (opener, onSlides int) {
	for _, sn := range dn.slides {
		onSlides += len(sn.posts)
	}
	return len(dn.opener), onSlides
}

type accountFile struct {
	Account string `json:"account"`
}

// loadAccount is the account saved under root, or "" if none is.
func loadAccount(root string) string {
	var f accountFile
	if b, err := os.ReadFile(filepath.Join(root, "posting.json")); err == nil {
		_ = json.Unmarshal(b, &f) // a file that doesn't read is no account
	}
	return f.Account
}

// saveAccount saves account under root, to type in again next time.
func saveAccount(root, account string) error {
	return writeJSON(filepath.Join(root, "posting.json"), accountFile{account})
}

// findLastThread looks through the thread directories, named by when each
// started, for the newest with a post.
func findLastThread(dir string) lastThread {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return lastThread{}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		path := filepath.Join(dir, entries[i].Name())
		t, err := loadThread(filepath.Join(path, "thread.json"))
		if err != nil || len(t.Posts) == 0 {
			continue
		}
		return lastThread{dir: path, posted: len(t.Posts), at: t.Posts[len(t.Posts)-1].At}
	}
	return lastThread{}
}

func (ps *posting) live() bool       { return ps.pub != nil && !ps.pub.dry }
func (ps *posting) rehearsing() bool { return ps.pub != nil && ps.pub.dry }

func (ps *posting) say(lines ...msgLine) { ps.msg = lines }

// refuse says why a key did nothing.
func (ps *posting) refuse(why string) { ps.say(msgLine{text: why, tone: toneWarn}) }

// typed is the account in the dialog's field.
func (ps *posting) typed() string { return strings.TrimPrefix(strings.TrimSpace(ps.input), "@") }

// needsSignIn reports whether starting would sign in first: the account
// typed isn't the one signed in.
func (ps *posting) needsSignIn() bool {
	a := ps.typed()
	return a == "" || a != ps.signedIn
}

// end ends the start in progress, if there is one.
func (ps *posting) end() {
	if ps.start != nil {
		ps.start.cancel()
		ps.start = nil
	}
	ps.focus = 0
}

// fail ends the start in progress with what went wrong.
func (ps *posting) fail(headline string, err error) {
	ps.end()
	ps.failed = true
	ps.say(msgLine{text: "▲ " + headline, tone: toneWarn}, msgLine{text: err.Error(), tone: toneMuted, detail: true})
}

// stop abandons the start in progress.
func (ps *posting) stop() {
	ps.end()
	ps.say(msgLine{text: "Cancelled. Nothing was posted."})
}

// begin starts posting or rehearsing.
func (ps *posting) begin(kind startKind) tea.Cmd {
	ps.end()
	ctx, cancel := context.WithCancel(context.Background())
	st := &start{kind: kind, ctx: ctx, cancel: cancel}
	switch {
	case kind == startRehearsal:
		st.dir = filepath.Join(ps.home, "rehearsal")
	case ps.cont && ps.last.dir != "":
		st.dir = ps.last.dir
	default:
		st.dir = filepath.Join(ps.home, "threads", time.Now().Format("2006-01-02-150405"))
	}
	ps.start, ps.msg, ps.failed = st, nil, false
	if kind == startLive && (ps.session == nil || ps.signedIn != ps.account) {
		return ps.signIn()
	}
	return ps.renderNext()
}

// signIn signs the Publisher in to the account. A browser page the speaker
// must approve comes back as a signInVisitMsg while it waits.
func (ps *posting) signIn() tea.Cmd {
	st, account, pub, open := ps.start, ps.account, ps.deck.Publisher, ps.openURL
	st.job, st.visit = jobSignIn, ""
	ctx := st.ctx
	urls := make(chan string, 1)
	signIn := func() tea.Msg {
		defer close(urls)
		sess, err := pub.SignIn(ctx, account, func(u string) {
			select {
			case urls <- u:
			default:
			}
			_ = open(u) // the dialog shows the address for when no browser opens
		})
		return signedInMsg{st, account, sess, err}
	}
	wait := func() tea.Msg {
		if u, ok := <-urls; ok {
			return signInVisitMsg{st, u}
		}
		return nil
	}
	return tea.Batch(signIn, wait)
}

// renderNext renders the next slide with posts not rendered yet, one slide
// a message, so the dialog can count them off.
func (ps *posting) renderNext() tea.Cmd {
	st, d, slides := ps.start, ps.deck, ps.notes.slides
	for st.slide < len(d.Slides) {
		if _, done := ps.rendered[st.slide]; !done && len(slides[st.slide].posts) > 0 {
			break
		}
		st.slide++
	}
	if st.slide == len(d.Slides) {
		return ps.prepare()
	}
	if st.media == "" {
		// A start abandoned mid-render may still be writing its files.
		ps.renders++
		st.media = filepath.Join(ps.home, "media", strconv.Itoa(ps.renders))
	}
	i, dir, lock, notes, keys := st.slide, st.media, ps.lock, slides[st.slide].posts, ps.keys[st.slide]
	st.job = jobRender
	return func() tea.Msg {
		posts, err := slidePosts(d, i, notes, keys, dir, lock)
		return postsRenderedMsg{st, i, posts, err}
	}
}

func (ps *posting) allPosts() []Post {
	var all []Post
	for i := -1; i < len(ps.deck.Slides); i++ {
		all = append(all, ps.rendered[i]...)
	}
	return all
}

// prepare readies the thread's file, claimed for the account signed in,
// and opens the posts on the Session, which checks them. A rehearsal has
// the Publisher check them too, if the talk has one, signed in or not, so
// it shows what posting for real would stop on.
func (ps *posting) prepare() tea.Cmd {
	st, pub, sess, as, all := ps.start, ps.deck.Publisher, ps.session, ps.signedIn, ps.allPosts()
	st.job = jobCheck
	ctx, dir := st.ctx, st.dir
	if st.kind == startRehearsal {
		return func() tea.Msg {
			msg := postsPreparedMsg{st: st, err: emptyFile(filepath.Join(dir, "dry-run.md"))}
			if msg.err == nil && pub != nil {
				msg.checkErr = pub.Check(ctx, all)
			}
			return msg
		}
	}
	return func() tea.Msg {
		msg := postsPreparedMsg{st: st}
		msg.thread, msg.err = loadThread(filepath.Join(dir, "thread.json"))
		if msg.err == nil {
			msg.err = msg.thread.postAs(sess.Account(), as)
		}
		if msg.err == nil {
			msg.pub, msg.checkErr = sess.Open(ctx, all)
		}
		return msg
	}
}

// emptyFile makes the file at path, and its directory, empty.
func emptyFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, nil, 0o644)
}

// opened opens the thread the start led to, with posting on.
func (ps *posting) opened(msg postsPreparedMsg) {
	st := msg.st
	pb := &publishing{pub: msg.pub, posts: ps.rendered, titles: ps.deck.titles(), thread: msg.thread, on: true}
	ps.msg = nil
	if st.kind == startRehearsal {
		pb.dry, pb.thread, pb.file = true, &threadLog{}, filepath.Join(st.dir, "dry-run.md")
		pb.pub = &dryRun{path: pb.file}
		if msg.checkErr != nil {
			ps.say(msgLine{text: "▲ Posting for real would stop here:", tone: toneWarn},
				msgLine{text: msg.checkErr.Error(), detail: true},
				msgLine{text: "The rehearsal goes on regardless."})
		}
	}
	if !pb.resume() {
		ps.say(msgLine{text: "▲ A post was going out when posting last stopped, and the notes no longer have it:", tone: toneWarn},
			msgLine{text: "it may be in the thread without the thread file knowing; look before you go on.", detail: true})
	}
	ps.pub = pb
	ps.end()
}

// postingMsg handles posting's messages; ok is false for another message.
func (p presenter) postingMsg(msg tea.Msg) (_ presenter, cmd tea.Cmd, ok bool) {
	ps := p.post
	if ps == nil {
		return p, nil, false
	}
	switch msg := msg.(type) {
	case signInVisitMsg:
		if msg.st == ps.start {
			msg.st.visit = msg.url
		}
	case signedInMsg:
		if msg.st != ps.start {
			break
		}
		msg.st.visit = ""
		if msg.err != nil {
			ps.fail(fmt.Sprintf("Couldn't sign in as %s:", msg.account), msg.err)
			break
		}
		ps.signedIn, ps.session = msg.account, msg.session
		return p, ps.renderNext(), true
	case postsRenderedMsg:
		if msg.st != ps.start {
			break
		}
		if msg.err != nil {
			ps.fail(fmt.Sprintf("Couldn't render slide %d's media:", msg.slide+1), msg.err)
			break
		}
		ps.rendered[msg.slide] = msg.posts
		msg.st.slide++
		return p, ps.renderNext(), true
	case postsPreparedMsg:
		switch {
		case msg.st != ps.start:
		case errors.As(msg.err, new(otherAccountError)):
			ps.fail("Can't carry on this thread:", msg.err)
		case msg.err != nil:
			ps.fail("Couldn't set up the thread's file:", msg.err)
		case msg.checkErr != nil && msg.st.kind == startLive:
			ps.fail("The posts didn't pass the check, so nothing will post:", msg.checkErr)
		default:
			ps.opened(msg)
			if len(ps.msg) == 0 && p.dialog == dialogPosting {
				p.dialog = dialogNone
			}
			// The slide up now posts when the deck moves on from it.
			return p, p.track(), true
		}
	case accountSavedMsg:
		if msg.err != nil {
			ps.msg = append(ps.msg, msgLine{text: "▲ The account won't be remembered:", tone: toneWarn}, msgLine{text: msg.err.Error(), detail: true})
		}
	case postDoneMsg:
		if msg.pb != ps.pub {
			break
		}
		if retry := msg.pb.done(msg, time.Now()); retry > 0 {
			return p, tea.Tick(retry, func(time.Time) tea.Msg { return postRetryMsg{msg.pb} }), true
		}
		return p, msg.pb.next(), true
	case postRetryMsg:
		if msg.pb != ps.pub {
			break
		}
		msg.pb.retryAt = time.Time{}
		return p, msg.pb.next(), true
	case uploadedMsg:
		if msg.pb == ps.pub && msg.err != nil {
			// Coming back to the slide tries again; posting it uploads
			// what's missing regardless.
			delete(msg.pb.uploads, msg.slide)
		}
	default:
		return p, nil, false
	}
	return p, nil, true
}

// postingBadge is posting's word on the status line, full or compact.
func (p presenter) postingBadge(compact bool) string {
	ps, sty := p.post, p.sty
	if ps == nil {
		return ""
	}
	pick := func(full, short string) string {
		if compact {
			return short
		}
		return full
	}
	if st := ps.start; st != nil {
		var phase, short string
		switch st.job {
		case jobSignIn:
			phase, short = "signing in", "SIGNING IN"
		case jobRender:
			n := fmt.Sprintf("%d/%d", st.slide+1, len(ps.deck.Slides))
			phase, short = "rendering "+n, "RENDERING "+n
		default:
			phase, short = "checking posts", "CHECKING"
		}
		what := "STARTING"
		if st.kind == startRehearsal {
			what = "STARTING REHEARSAL"
		}
		return sty.accent.Render(pick("◌ "+what+" · "+phase, "◌ "+short))
	}
	pb := ps.pub
	switch {
	case pb == nil && ps.failed:
		return sty.warn.Render(pick("▲ POSTING SETUP FAILED · P", "▲ SETUP FAILED"))
	case pb == nil:
		lead := sty.muted.Render("○ not posting")
		return pick(lead+sty.faint.Render(" · P to set up"), lead)
	}
	switch pb.state() {
	case threadRetrying:
		return sty.warn.Render(pick(fmt.Sprintf("▲ POST FAILED · retry in %s · %d waiting",
			clock(max(pb.retryAt.Sub(p.now), 0)), len(pb.queue)), "▲ POST FAILED"))
	case threadUnsaved:
		return sty.warn.Render(pick("▲ THREAD FILE NOT SAVED · P", "▲ NOT SAVED"))
	case threadPaused:
		if pb.dry {
			return sty.muted.Bold(true).Render(pick("‖ REHEARSAL PAUSED", "‖ PAUSED"))
		}
		return sty.warn.Render(pick(fmt.Sprintf("‖ POSTING PAUSED · %d held", len(pb.queue)), "‖ PAUSED"))
	}
	count := fmt.Sprintf(" · %d/%d %s%s", pb.sent(), pb.total(), pb.verb(), pb.skippedText())
	if pb.dry {
		lead := sty.accent2.Render("● REHEARSING")
		return pick(lead+sty.muted.Render(count), lead)
	}
	lead, as := sty.good.Render("● POSTING"), " as @"+ps.signedIn
	if len(pb.queue) > 0 {
		return pick(lead+sty.muted.Render(as)+sty.accent.Render(fmt.Sprintf(" · sending %d", len(pb.queue))), lead)
	}
	return pick(lead+sty.muted.Render(as+count), lead)
}

// unsent is how many posts a live thread still has to send.
func (p presenter) unsent() int {
	if pb := p.pub(); pb != nil && !pb.dry {
		return len(pb.queue)
	}
	return 0
}

// wrapHard cuts s into lines w cells wide, wherever it falls: an address
// stays whole to copy.
func wrapHard(s string, w int) []string {
	return strings.Split(ansi.Hardwrap(s, max(w, 1), true), "\n")
}

// shortPath is path with the home directory as ~, to fit the dialog.
func shortPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rest, ok := strings.CutPrefix(path, home+string(os.PathSeparator)); ok {
			return filepath.Join("~", rest)
		}
	}
	return path
}

// openURL opens u in the browser.
func openURL(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
