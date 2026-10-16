package decker

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestPostTrigger walks the deck through moves and checks which slides
// count as moved on from: forward, however briefly, and never a slide
// jumped over or moved back from.
func TestPostTrigger(t *testing.T) {
	pb := &publishing{}
	var left []int
	for _, mv := range [][2]int{
		{0, 0}, // the first report only sets the position
		{0, 1}, // a build isn't moving on
		{1, 0}, // posts slide 0
		{2, 0}, // posts slide 1, however quickly
		{9, 0}, // posts slide 2; 3 to 8 were jumped over
		{8, 0}, // back: nothing
		{9, 0}, // posts slide 8
		{9, 1},
	} {
		if s, ok := pb.at(mv[0], mv[1]); ok {
			left = append(left, s)
		}
	}
	if want := []int{0, 1, 2, 8}; !slices.Equal(left, want) {
		t.Errorf("moved on from %v, want %v", left, want)
	}

	// After the link drops, the first report only sets the position.
	pb.known = false
	if _, ok := pb.at(10, 0); ok {
		t.Error("a reconnect counted as moving on")
	}
}

func TestPostTriggerLastSlide(t *testing.T) {
	outline := []linkOutline{{Steps: 1}, {Steps: 2}}
	pb := &publishing{}
	pb.at(1, 0)
	if pb.lastShown(outline) {
		t.Error("the last slide posted before its last build")
	}
	pb.at(1, 1)
	if !pb.lastShown(outline) {
		t.Error("the last build didn't post")
	}
}

func TestPostQueue(t *testing.T) {
	pb := &publishing{on: true, thread: &thread{}, titles: []string{"a", "b", "c"},
		posts: map[int][]Post{-1: {{Slide: -1}}, 0: {{Slide: 0}, {Slide: 0}}, 2: {{Slide: 2}}}}
	pb.enqueue(1) // no posts
	if len(pb.queue) != 0 {
		t.Fatalf("a slide without posts queued %v", pb.queue)
	}
	pb.enqueue(0)
	pb.enqueue(0)
	want := []queuedPost{{-1, 0}, {0, 0}, {0, 1}}
	if !slices.Equal(pb.queue, want) {
		t.Fatalf("queue %v, want %v", pb.queue, want)
	}
	pb.on = false
	pb.enqueue(2)
	if pb.next() != nil || len(pb.queue) != 3 {
		t.Fatal("posting off still queued or posted")
	}
	if got, _ := pb.status(2); got != "won't send: paused" {
		t.Errorf("status %q", got)
	}
}

// fakePublisher records posts and fails the ones it's told to.
type fakePublisher struct {
	posts      []string // "text root parent"
	fail       int      // fail this many calls first
	prepared   int      // posts Prepare was given
	prepareErr error
	signedIn   []string // accounts, in order
	visit      string   // a page SignIn sends the speaker to, if set
	uploaded   []string // the texts of posts whose media Upload was given
}

func (f *fakePublisher) SignIn(_ context.Context, account string, visit func(string)) error {
	if f.visit != "" {
		visit(f.visit)
	}
	f.signedIn = append(f.signedIn, account)
	return nil
}

func (f *fakePublisher) Prepare(_ context.Context, posts []Post) error {
	f.prepared += len(posts)
	return f.prepareErr
}
func (f *fakePublisher) Upload(_ context.Context, p Post) error {
	f.uploaded = append(f.uploaded, p.Text)
	return nil
}

func (f *fakePublisher) Post(_ context.Context, p Post, root, parent string) (string, error) {
	if f.fail > 0 {
		f.fail--
		return "", errors.New("network down")
	}
	ref := fmt.Sprintf("ref%d", len(f.posts))
	f.posts = append(f.posts, fmt.Sprintf("%s root=%s parent=%s", p.Text, root, parent))
	return ref, nil
}

// TestPostThreading posts a queue through failures and checks the thread
// keeps its order and replies, saves after each post, and carries on from
// the file.
func TestPostThreading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread.json")
	th, err := loadThread(path)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakePublisher{fail: 2}
	pb := &publishing{pub: fake, on: true, thread: th, titles: []string{"a", "b"},
		posts: map[int][]Post{-1: {{Slide: -1, Text: "open"}}, 0: {{Slide: 0, Text: "a1"}, {Slide: 0, Text: "a2"}}, 1: {{Slide: 1, Text: "b"}}}}
	pb.enqueue(0)
	var backoff []time.Duration
	for range 10 {
		f := pb.next()
		if f == nil {
			if pb.retryAt.IsZero() {
				break
			}
			pb.retryAt = time.Time{}
			continue
		}
		if retry := pb.done(f().(postDoneMsg), time.Unix(0, 0)); retry > 0 {
			backoff = append(backoff, retry)
		}
	}
	if want := []time.Duration{5 * time.Second, 10 * time.Second}; !slices.Equal(backoff, want) {
		t.Errorf("backoff %v, want %v", backoff, want)
	}
	want := []string{"open root= parent=", "a1 root=ref0 parent=ref0", "a2 root=ref0 parent=ref1"}
	if !slices.Equal(fake.posts, want) {
		t.Errorf("posted\n%q\nwant\n%q", fake.posts, want)
	}
	if got, _ := pb.status(0); pb.err != "" || got != "✓ sent" {
		t.Errorf("err %q, status %q", pb.err, got)
	}

	// A restart carries on: slide 0 doesn't post again, and b replies to a2.
	th, err = loadThread(path)
	if err != nil || len(th.Posts) != 3 || th.Posts[1].Title != "a" {
		t.Fatalf("reloaded %+v, %v", th, err)
	}
	pb.thread, pb.queue = th, nil
	pb.enqueue(0)
	pb.enqueue(1)
	if want := []queuedPost{{1, 0}}; !slices.Equal(pb.queue, want) {
		t.Fatalf("after restart, queue %v, want %v", pb.queue, want)
	}
	pb.done(pb.next()().(postDoneMsg), time.Unix(0, 0))
	if got := fake.posts[3]; got != "b root=ref0 parent=ref2" {
		t.Errorf("after restart posted %q", got)
	}
}

// postingDeck is testDeck with an opening post and one on its first
// slide, text only so nothing renders.
func postingDeck(pub Publisher) *Deck {
	d := testDeck()
	d.Notes = "```post media=none\nopening\n```\n## Block letters\n```post media=none\nhello\n```\n"
	d.Publisher = pub
	return d
}

// driver runs a presenter view through messages and the posting commands
// they start, as Bubble Tea would. Commands that wait on the link or a
// clock are left out, and only posting's messages are fed back.
type driver struct {
	m       tea.Model
	outline []linkOutline
	opened  []string      // pages the posting dialog opened
	wait    time.Duration // how long a command may take before it counts as waiting on a clock
}

func newDriver(t *testing.T, d *Deck, root string) *driver {
	p := newPresenter(d, "/tmp/x.sock", 30*time.Minute)
	p.w, p.h, p.linked = 120, 40, true
	p.post = newPosting(d, root, &sync.Mutex{})
	if p.post == nil {
		t.Fatal("no posting dialog for a deck with posts")
	}
	dr := &driver{m: p, wait: 200 * time.Millisecond}
	p.post.openURL = func(u string) error { dr.opened = append(dr.opened, u); return nil }
	for _, s := range d.Slides {
		dr.outline = append(dr.outline, linkOutline{Title: s.Title, Steps: s.steps()})
	}
	dr.at(0)
	return dr
}

func (dr *driver) send(msg tea.Msg) {
	var cmd tea.Cmd
	dr.m, cmd = dr.m.Update(msg)
	for _, msg := range runCmd(cmd, dr.wait) {
		switch msg.(type) {
		case signedInMsg, signInVisitMsg, postsRenderedMsg, postsPreparedMsg, accountSavedMsg, postDoneMsg, uploadedMsg:
			dr.send(msg)
		}
	}
}

// namedKeys are the keys tests press by name.
var namedKeys = map[string]tea.KeyPressMsg{
	"enter": {Code: tea.KeyEnter}, "esc": {Code: tea.KeyEscape}, "backspace": {Code: tea.KeyBackspace},
	"tab": {Code: tea.KeyTab}, "shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift}, "space": {Code: tea.KeySpace, Text: " "},
	"right": {Code: tea.KeyRight}, "left": {Code: tea.KeyLeft}, "up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown},
	"pgdown": {Code: tea.KeyPgDown}, "pgup": {Code: tea.KeyPgUp}, "home": {Code: tea.KeyHome}, "end": {Code: tea.KeyEnd},
	"f5": {Code: tea.KeyF5},
}

// keyPress is the key named k, or the character k.
func keyPress(k string) tea.KeyPressMsg {
	if msg, ok := namedKeys[k]; ok {
		return msg
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

// keys presses each key: a name such as "enter", or a character.
func (dr *driver) keys(keys ...string) {
	for _, k := range keys {
		dr.send(keyPress(k))
	}
}

func (dr *driver) at(slide int) {
	dr.send(linkStateMsg{Slide: slide, Outline: dr.outline, W: 120, H: 36})
}

// moveOn moves the deck on from the slide it's on.
func (dr *driver) moveOn() { dr.at(dr.p().st.Slide + 1) }

func (dr *driver) p() presenter { return dr.m.(presenter) }

// open reports whether the posting dialog is open.
func (dr *driver) open() bool { return dr.p().dialog == dialogPosting }

// close closes the dialog, as if it never opened.
func (dr *driver) close() {
	p := dr.p()
	p.dialog = dialogNone
	dr.m = p
}
func (dr *driver) view() string { return ansi.Strip(dr.m.View().Content) }

// dialogSays is the posting dialog's message, as plain text.
func (dr *driver) dialogSays() string {
	var b strings.Builder
	for _, m := range dr.p().dialogMessage() {
		b.WriteString(ansi.Strip(m.text) + "\n")
	}
	return b.String()
}

// TestPostingDialog starts posting from the presenter view: the dialog
// opens on the account field, signs in through the browser, renders and
// checks the posts, and closes once posting starts; the account is
// remembered, and moving on posts as a thread.
func TestPostingDialog(t *testing.T) {
	fake := &fakePublisher{visit: "https://pds.example/authorize"}
	d, root := postingDeck(fake), t.TempDir()
	dr := newDriver(t, d, root)
	if v := dr.view(); !strings.Contains(v, "○ not posting · P to set up") || strings.Contains(v, "Post this talk") {
		t.Errorf("before the dialog:\n%s", v)
	}
	dr.keys("P")
	ps := dr.p().post
	if v := dr.view(); ps.focused() != ctlAccount || !strings.Contains(v, "Post this talk") || !strings.Contains(v, "[ Sign in & start posting ]") {
		t.Errorf("the dialog, focused on %v:\n%s", ps.focused(), v)
	}
	dr.keys("enter")
	if ps.start != nil || ps.focused() != ctlAccount || !strings.Contains(dr.dialogSays(), "Type the account") {
		t.Fatalf("starting with no account: start %+v, says %q", ps.start, dr.dialogSays())
	}
	dr.keys("@", "m", "e", ".", "x", "backspace", "e", "x", "q", "backspace", "enter")
	if !slices.Equal(fake.signedIn, []string{"me.ex"}) || ps.signedIn != "me.ex" || !slices.Equal(dr.opened, []string{fake.visit}) {
		t.Fatalf("signed in %q, dialog %q, opened %q", fake.signedIn, ps.signedIn, dr.opened)
	}
	if !ps.live() || !ps.pub.on || fake.prepared != 2 || dr.open() {
		t.Fatalf("after typing the account: live %v, prepared %d, open %v, says %q", ps.live(), fake.prepared, dr.open(), dr.dialogSays())
	}
	if v := dr.view(); !strings.Contains(v, "● POSTING as @me.ex · 0/2 sent") || !strings.Contains(v, "post  sends when you move on") {
		t.Errorf("posting:\n%s", v)
	}
	dr.keys("P")
	if v := dr.view(); ps.focused() != ctlClose || !strings.Contains(v, "Posting as @me.ex") || !strings.Contains(v, "0 of 2 sent · a new thread") {
		t.Errorf("the dialog while posting, focused on %v:\n%s", ps.focused(), v)
	}
	dr.keys("enter")
	if dr.open() || !ps.pub.on {
		t.Errorf("enter on Close: open %v, on %v", dr.open(), ps.pub.on)
	}
	dr.moveOn()
	if want := []string{"opening root= parent=", "hello root=ref0 parent=ref0"}; !slices.Equal(fake.posts, want) {
		t.Errorf("posted %q, want %q", fake.posts, want)
	}
	if v := dr.view(); !strings.Contains(v, "● POSTING as @me.ex · 2/2 sent") {
		t.Errorf("after posting:\n%s", v)
	}
	dir := filepath.Dir(ps.pub.thread.path)
	th, err := loadThread(ps.pub.thread.path)
	if err != nil || len(th.Posts) != 2 || !strings.HasPrefix(dir, filepath.Join(root, "talks", d.Name, "threads")) {
		t.Errorf("thread in %s: %+v, %v", dir, th, err)
	}

	// The next run remembers the account and finds the thread.
	next := newPosting(d, root, &sync.Mutex{})
	if next.account != "me.ex" || next.last.dir != dir || next.last.posted != 2 {
		t.Errorf("next run: account %q, last %+v", next.account, next.last)
	}
}

// TestPostingDialogKeys checks letters type into the account field and act
// nowhere else, focus goes round, and the dialog keeps every key from the
// deck but ctrl+c.
func TestPostingDialogKeys(t *testing.T) {
	root := t.TempDir()
	if err := saveAccount(root, "me.ex"); err != nil {
		t.Fatal(err)
	}
	dr := newDriver(t, postingDeck(&fakePublisher{}), root)
	dr.keys("P")
	ps := dr.p().post
	if ps.focused() != ctlStart || ps.input != "me.ex" {
		t.Fatalf("opened on %v with %q", ps.focused(), ps.input)
	}
	var seen []ctl
	for range 4 {
		seen = append(seen, ps.focused())
		dr.keys("tab")
	}
	if want := []ctl{ctlStart, ctlDismiss, ctlAccount, ctlRehearse}; !slices.Equal(seen, want) {
		t.Errorf("tab went %v, want %v", seen, want)
	}
	dr.keys("shift+tab", "shift+tab", "P", "q")
	if !dr.open() || ps.input != "me.exPq" {
		t.Errorf("typing P and q in the field: open %v, input %q", dr.open(), ps.input)
	}
	if _, _, ok := dr.p().postingKey(keyPress("right")); !ok {
		t.Error("the dialog let a deck key through")
	}
	if _, _, ok := dr.p().postingKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); ok {
		t.Error("the dialog kept ctrl+c")
	}
	dr.keys("tab", "P")
	if dr.open() {
		t.Error("P outside the field didn't close the dialog")
	}
}

// TestPostingCountsSkipped checks a slide jumped over counts as skipped,
// not as waiting to send, until the deck goes back to it.
func TestPostingCountsSkipped(t *testing.T) {
	root := t.TempDir()
	if err := saveAccount(root, "me.ex"); err != nil {
		t.Fatal(err)
	}
	d := postingDeck(&fakePublisher{})
	d.Notes = "```post media=none\nopening\n```\n## Block letters\n## Bullets\n```post media=none\nhello\n```\n"
	dr := newDriver(t, d, root)
	dr.keys("P", "enter")
	dr.at(2)
	if v := dr.view(); !strings.Contains(v, "● POSTING as @me.ex · 0/2 sent, 1 skipped") {
		t.Errorf("after jumping over the slide:\n%s", v)
	}
	dr.keys("P")
	if v := dr.view(); !strings.Contains(v, "0 of 2 sent, 1 skipped") || !strings.Contains(v, "Skipped posts are on slides") {
		t.Errorf("the dialog:\n%s", v)
	}
	dr.keys("esc")
	dr.at(1)
	if v := dr.view(); !strings.Contains(v, "● POSTING as @me.ex · 0/2 sent") || strings.Contains(v, "skipped") {
		t.Errorf("back on the slide:\n%s", v)
	}
}

// blockingPublisher's sign-in waits until it's cancelled, as one waiting
// on the browser does.
type blockingPublisher struct{ fakePublisher }

func (b *blockingPublisher) SignIn(ctx context.Context, _ string, _ func(string)) error {
	<-ctx.Done()
	return ctx.Err()
}

// TestPostingCancel checks closing the dialog leaves a sign-in running,
// and its Cancel stops it.
func TestPostingCancel(t *testing.T) {
	root := t.TempDir()
	if err := saveAccount(root, "me.ex"); err != nil {
		t.Fatal(err)
	}
	dr := newDriver(t, postingDeck(&blockingPublisher{}), root)
	dr.keys("P", "enter")
	ps := dr.p().post
	if ps.start == nil || ps.start.job != jobSignIn || !strings.Contains(dr.view(), "◌ STARTING · signing in") || !strings.Contains(dr.view(), "Signing in as me.ex") {
		t.Fatalf("start %+v:\n%s", ps.start, dr.view())
	}
	dr.keys("esc")
	if dr.open() || ps.start == nil || ps.start.job != jobSignIn {
		t.Fatalf("esc: open %v, start %+v", dr.open(), ps.start)
	}
	dr.keys("P")
	if ps.focused() != ctlClose {
		t.Errorf("reopened on %v", ps.focused())
	}
	dr.keys("left", "enter")
	if ps.start != nil || !dr.open() || !strings.Contains(dr.dialogSays(), "Cancelled.") {
		t.Errorf("Cancel: start %+v, open %v, says %q", ps.start, dr.open(), dr.dialogSays())
	}
}

// TestPostingCarriesOn picks the last thread back up: its posts aren't
// posted again, and the next replies to the last.
func TestPostingCarriesOn(t *testing.T) {
	fake := &fakePublisher{}
	d, root := postingDeck(fake), t.TempDir()
	if err := saveAccount(root, "me.ex"); err != nil {
		t.Fatal(err)
	}
	old := &thread{path: filepath.Join(root, "talks", d.Name, "threads", "2026-01-01-000000", "thread.json")}
	if err := saveThread(old.path, nil, threadPost{Slide: -1, Ref: "ref-old", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	dr := newDriver(t, d, root)
	dr.keys("P", "up", "up", "space")
	if v := dr.view(); !dr.p().post.cont || !strings.Contains(v, "(•) Carry on ") {
		t.Errorf("choosing to carry on:\n%s", v)
	}
	dr.keys("tab", "tab", "enter")
	ps := dr.p().post
	if !ps.live() || ps.pub.thread.path != old.path {
		t.Fatalf("live %v; says %q", ps.live(), dr.dialogSays())
	}
	dr.keys("P")
	if v := dr.view(); !strings.Contains(v, "1 of 2 sent · carrying on the thread from") {
		t.Errorf("the dialog:\n%s", v)
	}
	dr.keys("esc")
	dr.moveOn()
	if want := []string{"hello root=ref-old parent=ref-old"}; !slices.Equal(fake.posts, want) {
		t.Errorf("posted %q, want %q", fake.posts, want)
	}
}

// TestPostingRehearsal rehearses: posts go to a file, not the Publisher,
// whose checks still run, a failure only a warning, which keeps the
// dialog open. Stopping goes back to setting up.
func TestPostingRehearsal(t *testing.T) {
	fake := &fakePublisher{prepareErr: errors.New("offline")}
	d, root := postingDeck(fake), t.TempDir()
	dr := newDriver(t, d, root)
	dr.keys("P", "tab", "enter")
	ps := dr.p().post
	if says := dr.dialogSays(); !ps.rehearsing() || fake.prepared != 2 || !dr.open() || !strings.Contains(says, "Posting for real would stop here:\noffline\n") {
		t.Fatalf("rehearsing %v, prepared %d, open %v, says %q", ps.rehearsing(), fake.prepared, dr.open(), says)
	}
	if v := dr.view(); !strings.Contains(v, "● REHEARSING · 0/2 written") || !strings.Contains(v, "Rehearsing posts") || !strings.Contains(v, "[ Stop rehearsing ]") {
		t.Errorf("rehearsing:\n%s", v)
	}
	dr.keys("esc")
	dr.moveOn()
	b, err := os.ReadFile(filepath.Join(root, "talks", d.Name, "rehearsal", "dry-run.md"))
	if err != nil || !strings.Contains(string(b), "## Post 2, slide 1, in reply to dry:1\n\nhello\n") || len(fake.posts) != 0 {
		t.Errorf("dry-run.md %q, %v; posted %q", b, err, fake.posts)
	}
	// A rehearsal's post still in flight when its thread is replaced
	// doesn't count toward the new one.
	stale := postDoneMsg{pb: &publishing{}, q: queuedPost{0, 0}, tp: threadPost{Ref: "x"}}
	before := len(ps.pub.thread.Posts)
	dr.send(stale)
	if len(ps.pub.thread.Posts) != before {
		t.Error("a stale post's result was recorded")
	}
	dr.keys("P", "left", "enter")
	if ps.pub != nil || ps.focused() == ctlClose {
		t.Errorf("Stop rehearsing: pub %v, focused on %v", ps.pub, ps.focused())
	}
}

// TestPostingUploadsAhead checks the slide up uploads its media while it's
// up, once, and not after it has posted; and that posting reuses the media
// a rehearsal rendered.
func TestPostingUploadsAhead(t *testing.T) {
	fake := &fakePublisher{}
	d := testDeck()
	d.Notes = "## Block letters\n```post\nhello\n```\n## Bullets\n```post media=none\ntext\n```\n"
	d.Publisher = fake
	root := t.TempDir()
	if err := saveAccount(root, "me.ex"); err != nil {
		t.Fatal(err)
	}
	dr := newDriver(t, d, root)
	dr.wait = time.Minute // rendering takes a while, and nothing here waits on a clock
	dr.keys("P", "shift+tab", "enter")
	ps := dr.p().post
	if !ps.rehearsing() || len(fake.uploaded) != 0 {
		t.Fatalf("rehearsing %v, uploaded %q", ps.rehearsing(), fake.uploaded)
	}
	dr.keys("P", "left", "enter", "enter") // Stop rehearsing, then Start posting
	if !ps.live() || ps.renders != 1 {
		t.Fatalf("live %v after %d renders; says %q", ps.live(), ps.renders, dr.dialogSays())
	}
	if !slices.Equal(fake.uploaded, []string{"hello"}) {
		t.Errorf("on slide 1, uploaded %q", fake.uploaded)
	}
	dr.at(0)
	dr.moveOn()
	dr.at(0)
	if !slices.Equal(fake.uploaded, []string{"hello"}) || len(fake.posts) != 1 {
		t.Errorf("uploaded %q, posted %q", fake.uploaded, fake.posts)
	}
}

func TestPostingWithoutAPublisher(t *testing.T) {
	if newPosting(testDeck(), t.TempDir(), &sync.Mutex{}) != nil {
		t.Error("a deck with no posts has a posting dialog")
	}
	dr := newDriver(t, postingDeck(nil), t.TempDir())
	dr.keys("P")
	if v := dr.view(); strings.Contains(v, "start posting") || !strings.Contains(v, "no Deck.Publisher") || dr.p().post.focused() != ctlRehearse {
		t.Errorf("the dialog with no Publisher:\n%s", v)
	}
	dr.keys("enter")
	if !dr.p().post.rehearsing() {
		t.Error("a talk with no Publisher can't rehearse")
	}
}

// TestDialogIsClickerSafe presses each key a clicker or the deck's table
// sends, once, on the dialog just opened while posting or setting up: none
// starts, stops or pauses anything, and esc closes it.
func TestDialogIsClickerSafe(t *testing.T) {
	setups := map[string]func(dr *driver){
		"busy":       func(dr *driver) { dr.p().post.start = &start{job: jobRender, cancel: func() {}} },
		"rehearsing": func(dr *driver) { dr.keys("P", "shift+tab", "enter", "esc") },
		"live":       func(dr *driver) { dr.keys("P", "enter") },
	}
	var clicker []string
	for k, b := range keyActs {
		if b.nav {
			clicker = append(clicker, k)
		}
	}
	clicker = append(clicker, "f5", "esc", "enter", "space")
	for name, setup := range setups {
		root := t.TempDir()
		if err := saveAccount(root, "me.ex"); err != nil {
			t.Fatal(err)
		}
		dr := newDriver(t, postingDeck(&fakePublisher{}), root)
		setup(dr)
		ps := dr.p().post
		pub, st := ps.pub, ps.start
		on := pub != nil && pub.on
		for _, k := range clicker {
			dr.close()
			dr.keys("P")
			dr.keys(k)
			if ps.pub != pub || ps.start != st || (pub != nil && pub.on != on) {
				t.Errorf("%s: clicker key %q changed posting", name, k)
			}
			if k == "esc" && dr.open() {
				t.Errorf("%s: esc didn't close the dialog", name)
			}
		}
	}
}

// TestQuitWithPostsWaiting checks q asks again while a live thread has
// posts to send, and ctrl+c doesn't.
func TestQuitWithPostsWaiting(t *testing.T) {
	root := t.TempDir()
	if err := saveAccount(root, "me.ex"); err != nil {
		t.Fatal(err)
	}
	dr := newDriver(t, postingDeck(&fakePublisher{}), root)
	dr.keys("P", "enter")
	dr.p().pub().queue = []queuedPost{{0, 0}}
	quits := func(k string) bool {
		_, cmd := dr.p().handleKey(k)
		if cmd == nil {
			return false
		}
		_, ok := cmd().(tea.QuitMsg)
		return ok
	}
	m, _ := dr.p().handleKey("q")
	dr.m = m
	if !dr.p().quitArmed.After(time.Now()) || !strings.Contains(dr.view(), "▲ 1 post not sent yet · q again to quit") {
		t.Fatalf("first q:\n%s", dr.view())
	}
	if !quits("q") {
		t.Error("a second q didn't quit")
	}
	m, _ = dr.p().handleKey("right")
	dr.m = m
	if !dr.p().quitArmed.IsZero() {
		t.Error("another key didn't disarm the quit")
	}
	if !quits("ctrl+c") {
		t.Error("ctrl+c didn't quit")
	}
}

// runCmd runs cmd and any batch it holds, except the ones that wait on the
// link or a clock, and returns their messages. A command that takes longer
// than wait counts as waiting.
func runCmd(cmd tea.Cmd, wait time.Duration) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var msgs []tea.Msg
			for _, c := range batch {
				msgs = append(msgs, runCmd(c, wait)...)
			}
			return msgs
		}
		return []tea.Msg{msg}
	case <-time.After(wait):
		return nil
	}
}

// TestSlidePosts renders every kind of post's media, checks it, and posts
// it through a dry run.
func TestSlidePosts(t *testing.T) {
	if testing.Short() {
		t.Skip("renders full-size media")
	}
	d := testDeck()
	d.Notes = "```post media=none\nopening\n```\n" +
		"## Block letters\n```post\nfinal\n```\n```alt\nHello deck, in block letters.\n```\n" +
		"## Bullets\n```post media=builds builds=1,3\nbuilds\n```\n" +
		"## Diagram\n```post media=none\ntext only\n```\n"
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		d.Notes += "## Character layer\n```post media=clip\nclip\n```\n"
	}
	if err := d.Check(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dn := d.notes()
	posts := map[int][]Post{-1: openerPosts(dn.opener)}
	for i := range d.Slides {
		ps, err := slidePosts(d, i, dn.slides[i].posts, dir, &sync.Mutex{})
		if err != nil {
			t.Fatal(err)
		}
		posts[i] = ps
	}
	b := posts[1][0]
	if len(b.Media) != 2 || b.Media[1].Alt != "Slide 2: Bullets (build 3 of 3)" {
		t.Fatalf("builds post: %+v", b)
	}
	f, err := os.Open(b.Media[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != postImageW || cfg.Height != postImageH {
		t.Errorf("image %dx%d, %v", cfg.Width, cfg.Height, err)
	}
	if clip := posts[3]; clip != nil {
		if fi, err := os.Stat(clip[0].Media[0].Path); err != nil || fi.Size() == 0 || !clip[0].Media[0].Video {
			t.Errorf("clip: %+v, %v", clip, err)
		}
	}

	pb := &publishing{pub: &dryRun{path: filepath.Join(dir, "dry-run.md")}, dry: true, on: true, posts: posts, titles: d.titles(), thread: &thread{}}
	for _, s := range []int{0, 1, 2, 3} {
		pb.enqueue(s)
	}
	for f := pb.next(); f != nil; f = pb.next() {
		pb.done(f().(postDoneMsg), time.Unix(0, 0))
	}
	out, err := os.ReadFile(filepath.Join(dir, "dry-run.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Post 1, opens the thread, in reply to nothing\n\nopening\n",
		"## Post 2, slide 1, in reply to dry:1\n\nfinal\n\n![Hello deck, in block letters.](01-1-1.png)\n",
		"![Slide 2: Bullets (build 1 of 3)](02-1-1.png)\n\n![Slide 2: Bullets (build 3 of 3)](02-1-3.png)",
		"## Post 4, slide 3, in reply to dry:3\n\ntext only\n",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("dry-run.md lacks %q:\n%s", want, out)
		}
	}
}

// TestLogTo checks that slog and the log package both go to the file while
// publishing, and both come back as they were.
func TestLogTo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pub", "publish.log")
	prev, w, flags := slog.Default(), log.Writer(), log.Flags()
	restore, err := logTo(path)
	if err != nil {
		t.Fatal(err)
	}
	slog.Warn("from slog")
	log.Print("from log")
	restore()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "from slog") || !strings.Contains(string(b), "from log") {
		t.Errorf("publish.log: %q", b)
	}
	if slog.Default() != prev || log.Writer() != w || log.Flags() != flags {
		t.Error("the loggers weren't restored")
	}
}
