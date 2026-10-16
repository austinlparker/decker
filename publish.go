package decker

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Publisher posts a talk as a thread while it is given: when the speaker
// moves on from a slide, the post blocks in its Notes section go out as
// replies, in order. The presenter view's posting dialog (P) signs in and
// starts posting. Deck.Publisher sets one; the bsky package has one for
// Bluesky.
type Publisher interface {
	// Check holds every post the talk can make to the service's limits,
	// without signing in or posting anything: decktest.Posts runs it, and
	// so does a rehearsal, to show what posting for real would stop on. It
	// may look things up, as bsky resolves @mentions, but keeps nothing for
	// later calls.
	Check(ctx context.Context, posts []Post) error

	// SignIn signs in as account, the one the speaker typed into the
	// posting dialog, picking up a session kept from an earlier run if it
	// can, and returns the Session that posts as it. When the speaker must
	// approve it in a browser, SignIn calls visit with the page's address,
	// which the presenter view shows and opens, and waits until they do or
	// ctx ends.
	SignIn(ctx context.Context, account string, visit func(url string)) (Session, error)
}

// Session is a Publisher signed in to an account.
type Session interface {
	// Account names the account in a way that doesn't change, as a handle
	// can: the thread file records it, and a thread posted as one account
	// isn't carried on as another.
	Account() string

	// Open readies posts to go out, as starting to post does: it checks
	// them as Publisher.Check does, and what only the account can answer,
	// such as whether it may upload that many clips today. The Thread it
	// returns posts them, and holds whatever Open worked out for them.
	Open(ctx context.Context, posts []Post) (Thread, error)
}

// Thread posts the posts a Session opened it with.
type Thread interface {
	// Upload sends p's media ahead of Post, while its slide is up, so the
	// post goes out quickly when the speaker moves on. It runs alongside
	// Post and other Uploads. Post works whatever became of it: it waits
	// for an upload still running, uploads what one didn't, and sends
	// nothing twice.
	Upload(ctx context.Context, p Post) error

	// Post publishes p where d says, and returns a reference to it, which
	// later posts get as their Parent (and Root, if it's the first). A try
	// d marks as a Retry may have gone out already: Post finds it if it
	// did, rather than posting it twice.
	Post(ctx context.Context, p Post, d Delivery) (ref string, err error)
}

// Delivery is where a post goes in its thread, and which post it is.
type Delivery struct {
	// Root and Parent are the refs of the thread's first post and of the
	// post this replies to; both empty for the first.
	Root, Parent string

	// Key names the post: the same on every try of it, after a restart
	// too, and no other post's, in this thread or another. First is when
	// it was first tried. The thread file keeps both before that try goes
	// out, so a Thread can derive what it sends from them, as bsky derives
	// a post's record key, and find the post again on a retry.
	Key   string
	First time.Time

	// Retry is set when an earlier try of the post may have gone out: its
	// answer never came, or the presenter view stopped before recording it.
	Retry bool
}

// Post is one post of a live thread, from a post block in Deck.Notes.
type Post struct {
	// Slide is the slide the post is about, from 0; -1 for a post before
	// the first slide's section, which opens the thread.
	Slide int

	Text string

	// Media is what the post attaches: images, or one video.
	Media []Media

	key string // from postKeys
}

// Media is an image or a video of a slide that a Post attaches, rendered
// from the slide's pixel canvas as the video command draws it.
type Media struct {
	Path  string // a PNG, or an MP4 if Video
	Video bool   // a clip of the slide building, to play muted and looping, like a GIF
	Alt   string
	W, H  int // in pixels
}

// The size of a post's media: images the video command's default, clips
// smaller, since every slide's clip is uploaded as the talk goes.
const (
	postImageW, postImageH = 1920, 1080
	postClipW, postClipH   = 1280, 720
	postClipFPS            = 30
	postClipHold           = 2.0 // seconds per build, unless the slide sets Hold
)

// openerPosts are the posts before the first slide's section, which open
// the thread. They have no media.
func openerPosts(opener []notePost, keys []string) []Post {
	var posts []Post
	for n, np := range opener {
		posts = append(posts, Post{Slide: -1, Text: np.text, key: keys[n]})
	}
	return posts
}

// postKeys name each post in the notes, by slide (-1 opens the thread),
// for the thread file: by what it says and shows and its slide's title,
// not by where it is, so a thread carried on after slides or posts have
// moved finds the posts it has, and a post edited since is a new one.
// Posts alike in all of that are told apart by how many came before.
func postKeys(dn deckNotes, slides []Slide) map[int][]string {
	keys := map[int][]string{}
	seen := map[string]int{}
	add := func(slide int, title string, np notePost) {
		sum := sha256.Sum256(fmt.Appendf(nil, "%q %q %d %v %q", title, np.text, np.media, np.builds, np.alt))
		k := hex.EncodeToString(sum[:8])
		if seen[k]++; seen[k] > 1 {
			k += fmt.Sprintf("-%d", seen[k])
		}
		keys[slide] = append(keys[slide], k)
	}
	for _, np := range dn.opener {
		add(-1, "", np)
	}
	for i, sn := range dn.slides {
		for _, np := range sn.posts {
			add(i, slides[i].Title, np)
		}
	}
	return keys
}

// Posts are every post the deck's notes make, in order, with their media
// rendered into dir as the presenter view's posting dialog renders it.
// Clips need ffmpeg. decktest.Posts checks them before the talk.
func (d *Deck) Posts(dir string) ([]Post, error) {
	dn, err := parseNotes(d.Notes, d.Slides)
	if err != nil {
		return nil, err
	}
	keys := postKeys(dn, d.Slides)
	posts := openerPosts(dn.opener, keys[-1])
	lock := new(sync.Mutex)
	for i, sn := range dn.slides {
		ps, err := slidePosts(d, i, sn.posts, keys[i], dir, lock)
		if err != nil {
			return nil, err
		}
		posts = append(posts, ps...)
	}
	return posts, nil
}

// slidePosts are slide i's posts, from its notes and named by keys, with
// their media rendered into dir. lock is held while a slide draws: slide
// Views expect one caller at a time, and the presenter view draws its
// previews alongside.
func slidePosts(d *Deck, i int, notes []notePost, keys []string, dir string, lock sync.Locker) ([]Post, error) {
	if len(notes) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var posts []Post
	for n, np := range notes {
		p := Post{Slide: i, Text: np.text, key: keys[n]}
		name := slideNumber(i+1, len(d.Slides)) + "-" + fmt.Sprint(n+1)
		alt := np.alt
		if alt == "" {
			alt = fmt.Sprintf("Slide %d: %s", i+1, d.Slides[i].Title)
		}
		switch np.media {
		case postFinal, postBuilds:
			for _, b := range np.builds {
				m := Media{Path: filepath.Join(dir, fmt.Sprintf("%s-%d.png", name, b+1)), Alt: alt,
					W: postImageW, H: postImageH}
				if len(np.builds) > 1 {
					m.Alt = fmt.Sprintf("%s (build %d of %d)", alt, b+1, d.Steps(i))
				}
				lock.Lock()
				sc := d.scene(i, stillCtx(postImageW, postImageH/2, b, Settled))
				lock.Unlock()
				img := pixelsImage(sc.Px)
				sc.Release()
				if err := savePNG(img, m.Path); err != nil {
					return nil, err
				}
				p.Media = append(p.Media, m)
			}
		case postClip:
			m := Media{Path: filepath.Join(dir, name+".mp4"), Video: true, Alt: alt, W: postClipW, H: postClipH}
			err := renderVideo(d, videoOptions{path: m.Path, width: postClipW, height: postClipH,
				fps: postClipFPS, hold: postClipHold, first: i, last: i, quiet: true, lock: lock})
			if err != nil {
				return nil, fmt.Errorf("slide %d's clip: %w", i+1, err)
			}
			p.Media = append(p.Media, m)
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// threadLog is what has been posted of a live thread, kept in a file so a
// presenter view that restarts carries on the same thread.
type threadLog struct {
	path string // "" keeps it in memory, for a dry run

	// ID tells this thread's posts from another's, in Delivery.Key.
	ID string `json:"id"`
	// Account is who posts it, as Session.Account names them, and As as
	// the speaker typed it; both empty until the first post.
	Account string `json:"account,omitempty"`
	As      string `json:"as,omitempty"`

	Posts []threadPost `json:"posts"`
	// Pending is the post being tried: it's saved before each try, so a
	// restart knows it may have gone out.
	Pending *pendingPost `json:"pending,omitempty"`
}

type threadPost struct {
	Key   string    `json:"key"`   // from postKeys
	Slide int       `json:"slide"` // the slide it was about, from 0; -1 for the opener
	Title string    `json:"title,omitempty"`
	Ref   string    `json:"ref"`
	At    time.Time `json:"at"`
}

type pendingPost struct {
	Key   string    `json:"key"`
	First time.Time `json:"first"` // Delivery.First
}

// otherAccountError is carrying on a thread as an account other than the
// one that posted it, as typed.
type otherAccountError struct{ as string }

func (e otherAccountError) Error() string {
	return fmt.Sprintf("this thread was posted as @%s; sign in as that account to carry it on, or start a new thread", e.as)
}

// loadThread reads the thread kept at path, or starts one if there is no
// file yet.
func loadThread(path string) (*threadLog, error) {
	t := &threadLog{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.ID = newThreadID()
		return t, nil
	}
	if err == nil {
		err = json.Unmarshal(b, t)
	}
	if err == nil && t.ID == "" {
		err = errors.New("no id")
	}
	if err != nil {
		return nil, fmt.Errorf("thread %s: %w", path, err)
	}
	return t, nil
}

func newThreadID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// postAs claims the thread for the account a Session names, as typed: one
// with posts may only go on as the account that posted them.
func (t *threadLog) postAs(account, as string) error {
	if len(t.Posts) > 0 && t.Account != account {
		return otherAccountError{t.As}
	}
	t.Account, t.As = account, as
	return nil
}

// snapshot is a copy of t to save from a command, while t goes on
// changing.
func (t *threadLog) snapshot() threadLog {
	c := *t
	c.Posts = slices.Clone(t.Posts)
	return c
}

// save writes t to its file; one kept in memory saves nothing.
func (t threadLog) save() error {
	if t.path == "" {
		return nil
	}
	return writeJSON(t.path, t)
}

// writeJSON saves v at path, making its directory if need be. The file is
// replaced whole, so a crash mid-write leaves the last good one.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (t *threadLog) has(key string) bool {
	return slices.ContainsFunc(t.Posts, func(tp threadPost) bool { return tp.Key == key })
}

// refs are what the next post replies to: the thread's first post and its
// last.
func (t *threadLog) refs() (root, parent string) {
	if len(t.Posts) == 0 {
		return "", ""
	}
	return t.Posts[0].Ref, t.Posts[len(t.Posts)-1].Ref
}

// publishing is the presenter view's side of a live thread: it watches the
// deck move, decides what to post, and posts it in order, one at a time.
type publishing struct {
	pub    Thread
	dry    bool
	file   string         // a rehearsal's, the posts are written to
	posts  map[int][]Post // by slide; -1 opens the thread
	titles []string       // the slides' titles, for the thread file
	thread *threadLog

	on      bool
	queue   []queuedPost
	busy    bool         // a post is in flight
	err     string       // why the last post failed, until one succeeds
	fails   int          // failures in a row, for the backoff
	retryAt time.Time    // when the failed post first in the queue is tried again; zero if none waits
	saveErr string       // why the thread file wasn't saved after a post that went out
	uploads map[int]bool // slides whose media has started uploading

	// Where the deck is, as the link last reported it.
	known       bool
	slide, step int
}

type queuedPost struct{ slide, n int }

// at records that the deck is at slide and step. It returns the slide the
// deck just moved on from, forward, however briefly it was up; slides
// jumped over were skipped on purpose, and moving back never posts, so
// neither is returned. The first report after the link (re)connects only
// sets the position: what happened while it was down isn't known.
func (pb *publishing) at(slide, step int) (left int, ok bool) {
	if pb.known && slide != pb.slide {
		left, ok = pb.slide, slide > pb.slide
	}
	pb.known, pb.slide, pb.step = true, slide, step
	return left, ok
}

// lastShown reports whether the deck is on its last slide's last build:
// the last slide has nothing to move on to, so it posts then.
func (pb *publishing) lastShown(outline []linkOutline) bool {
	n := len(outline)
	return pb.known && n > 0 && pb.slide == n-1 && pb.step == outline[n-1].Steps-1
}

// enqueue queues slide's posts that aren't posted or queued yet, after the
// thread's opener if nothing has been posted. Nothing queues while posting
// is off.
func (pb *publishing) enqueue(slide int) {
	if !pb.on || len(pb.posts[slide]) == 0 {
		return
	}
	add := func(slide int) {
		for n := range pb.posts[slide] {
			if pb.pending(slide, n) {
				pb.queue = append(pb.queue, queuedPost{slide, n})
			}
		}
	}
	if len(pb.thread.Posts) == 0 {
		add(-1)
	}
	add(slide)
}

// resume queues the post the thread was trying when posting last stopped,
// first: the deck had moved on from its slide, and it may have gone out.
// It reports false if the notes no longer have that post, which may then
// be in the thread unrecorded.
func (pb *publishing) resume() bool {
	pp := pb.thread.Pending
	if pp == nil {
		return true
	}
	for slide, posts := range pb.posts {
		for n, p := range posts {
			if p.key == pp.Key {
				pb.queue = append(pb.queue, queuedPost{slide, n})
				return true
			}
		}
	}
	return false
}

// posted reports whether slide's post n is in the thread.
func (pb *publishing) posted(slide, n int) bool { return pb.thread.has(pb.posts[slide][n].key) }

// pending reports whether slide's post n is neither posted nor queued.
func (pb *publishing) pending(slide, n int) bool {
	return !pb.posted(slide, n) && !slices.Contains(pb.queue, queuedPost{slide, n})
}

// verb is what a post that went out was: written, in a rehearsal, or sent.
func (pb *publishing) verb() string {
	if pb.dry {
		return "written"
	}
	return "sent"
}

// upload starts uploading the media of slide's posts still to go, while
// the slide is up, so posting it when the deck moves on is quick: a clip
// can take a minute to upload and process, and the posts after it would
// wait. Each slide uploads once, unless that fails.
func (pb *publishing) upload(slide int) tea.Cmd {
	if !pb.on || pb.uploads[slide] {
		return nil
	}
	var posts []Post
	for n, p := range pb.posts[slide] {
		if len(p.Media) > 0 && !pb.posted(slide, n) {
			posts = append(posts, p)
		}
	}
	if len(posts) == 0 {
		return nil
	}
	if pb.uploads == nil {
		pb.uploads = map[int]bool{}
	}
	pb.uploads[slide] = true
	pub := pb.pub
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), postTimeout)
		defer cancel()
		for _, p := range posts {
			if err := pub.Upload(ctx, p); err != nil {
				return uploadedMsg{pb, slide, err}
			}
		}
		return uploadedMsg{pb, slide, nil}
	}
}

// threadState is how an open thread stands, the most pressing first.
type threadState uint8

const (
	threadGoing    threadState = iota
	threadRetrying             // a post failed and waits to try again
	threadUnsaved              // a post went out, but the thread file didn't save
	threadPaused
)

func (pb *publishing) state() threadState {
	switch {
	case !pb.retryAt.IsZero():
		return threadRetrying
	case pb.saveErr != "":
		return threadUnsaved
	case !pb.on:
		return threadPaused
	}
	return threadGoing
}

// tone is how a word of status reads: as information, a good sign,
// something happening, or a problem.
type tone uint8

const (
	toneMuted tone = iota
	toneGood
	toneAccent
	toneWarn
)

func (s styles) tone(t tone) lipgloss.Style {
	switch t {
	case toneGood:
		return s.good
	case toneAccent:
		return s.accent
	case toneWarn:
		return s.warn
	}
	return s.muted
}

// status is how a slide's posts stand, for the presenter view's NOTES
// line: "" if it has none.
func (pb *publishing) status(slide int) (string, tone) {
	posts := pb.posts[slide]
	if len(posts) == 0 {
		return "", toneMuted
	}
	done, queued := 0, 0
	for n := range posts {
		switch {
		case pb.posted(slide, n):
			done++
		case !pb.pending(slide, n):
			queued++
		}
	}
	switch {
	case done == len(posts):
		return "✓ " + pb.verb(), toneGood
	case queued > 0:
		return "queued", toneAccent
	case !pb.on && pb.dry:
		return "won't write: paused", toneMuted
	case !pb.on:
		return "won't send: paused", toneWarn
	case slide == len(pb.titles)-1:
		return "sends after the last build", toneMuted
	}
	return "sends when you move on", toneMuted
}

// sent is how many of the talk's posts the thread has, including any from
// before a restart; total, how many the talk makes.
func (pb *publishing) sent() int {
	n := 0
	for slide, posts := range pb.posts {
		for i := range posts {
			if pb.posted(slide, i) {
				n++
			}
		}
	}
	return n
}

func (pb *publishing) total() int {
	n := 0
	for _, ps := range pb.posts {
		n += len(ps)
	}
	return n
}

// skipped is how many posts the deck has moved past without sending or
// queuing them: on slides jumped over, or passed while paused or before
// posting started. Going back to one makes its posts due
// again. The opener isn't counted: it goes out with the next slide's.
func (pb *publishing) skipped() int {
	n := 0
	for slide, posts := range pb.posts {
		if !pb.known || slide < 0 || slide >= pb.slide {
			continue
		}
		for i := range posts {
			if pb.pending(slide, i) {
				n++
			}
		}
	}
	return n
}

// skippedText is ", N skipped", or "" if none were.
func (pb *publishing) skippedText() string {
	if n := pb.skipped(); n > 0 {
		return fmt.Sprintf(", %d skipped", n)
	}
	return ""
}

// postTimeout is how long a post, or uploading a slide's media, may take.
const postTimeout = 3 * time.Minute

type (
	// These carry their thread: one replaced, as a rehearsal is by going
	// live, may still have a post in flight.
	postDoneMsg struct {
		pb      *publishing
		q       queuedPost
		tp      threadPost // the post as the thread file records it, if it went out
		err     error
		saveErr error // why the thread file wasn't saved after it went out
	}
	postRetryMsg struct{ pb *publishing }
	uploadedMsg  struct {
		pb    *publishing
		slide int
		err   error
	}
)

// next starts the queue's first post, if posting is on and nothing is in
// flight or waiting to retry. The thread file is saved with the post as
// pending before it goes out, and with it recorded after.
func (pb *publishing) next() tea.Cmd {
	if !pb.on || pb.busy || !pb.retryAt.IsZero() || len(pb.queue) == 0 {
		return nil
	}
	q := pb.queue[0]
	p, th := pb.posts[q.slide][q.n], pb.thread
	d := Delivery{Key: th.ID + "/" + p.key, First: time.Now()}
	d.Root, d.Parent = th.refs()
	if pp := th.Pending; pp != nil && pp.Key == p.key {
		d.First, d.Retry = pp.First, true
	} else {
		th.Pending = &pendingPost{Key: p.key, First: d.First}
	}
	title := ""
	if q.slide >= 0 {
		title = pb.titles[q.slide]
	}
	snap := th.snapshot()
	pb.busy = true
	pub := pb.pub
	return func() tea.Msg {
		if err := snap.save(); err != nil {
			return postDoneMsg{pb: pb, q: q, err: fmt.Errorf("not sent: the thread file wasn't saved: %w", err)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), postTimeout)
		defer cancel()
		ref, err := pub.Post(ctx, p, d)
		if err != nil {
			return postDoneMsg{pb: pb, q: q, err: err}
		}
		tp := threadPost{Key: p.key, Slide: q.slide, Title: title, Ref: ref, At: time.Now()}
		snap.Posts, snap.Pending = append(snap.Posts, tp), nil
		return postDoneMsg{pb: pb, q: q, tp: tp, saveErr: snap.save()}
	}
}

// done records a finished post. A failed one stays first in the queue, and
// pending, so the thread keeps its order and the next try looks for it;
// it's retried after backoff, which doubles with each failure in a row up
// to a minute.
func (pb *publishing) done(msg postDoneMsg, now time.Time) (retry time.Duration) {
	pb.busy = false
	if msg.err == nil {
		pb.thread.Posts = append(pb.thread.Posts, msg.tp)
		pb.thread.Pending = nil
		pb.queue = pb.queue[1:]
		pb.err, pb.fails, pb.saveErr = "", 0, ""
		if msg.saveErr != nil {
			pb.saveErr = msg.saveErr.Error()
		}
		return 0
	}
	pb.err = msg.err.Error()
	pb.fails++
	retry = min(5*time.Second<<min(pb.fails-1, 4), time.Minute)
	pb.retryAt = now.Add(retry)
	return retry
}

// dryRun is the Thread a dry run posts to: nothing leaves the machine.
// Each post is added to a Markdown file instead, with its media, to
// rehearse with.
type dryRun struct {
	path string
	n    int
}

func (*dryRun) Upload(context.Context, Post) error { return nil }

func (d *dryRun) Post(_ context.Context, p Post, to Delivery) (string, error) {
	d.n++
	var b strings.Builder
	about := "opens the thread"
	if p.Slide >= 0 {
		about = fmt.Sprintf("slide %d", p.Slide+1)
	}
	fmt.Fprintf(&b, "## Post %d, %s, in reply to %s\n\n%s\n", d.n, about, cmp.Or(to.Parent, "nothing"), p.Text)
	for _, m := range p.Media {
		link, err := filepath.Rel(filepath.Dir(d.path), m.Path)
		if err != nil {
			link = m.Path
		}
		fmt.Fprintf(&b, "\n![%s](%s)\n", mdText(m.Alt), filepath.ToSlash(link))
	}
	f, err := os.OpenFile(d.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(b.String() + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return fmt.Sprintf("dry:%d", d.n), err
}

// logTo sends the default slog logger, and the log package through it, to
// the file at path until restore: a Publisher's libraries log as they post,
// and a line on stderr would land in the middle of the presenter view.
// restore puts both back as they were; slog.SetDefault alone doesn't
// restore the log package's writer.
func logTo(path string) (restore func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	prev, w, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
	return func() {
		slog.SetDefault(prev)
		log.SetOutput(w)
		log.SetFlags(flags)
		f.Close()
	}, nil
}
