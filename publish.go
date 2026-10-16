package decker

import (
	"cmp"
	"context"
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
	// SignIn signs in as account, the one the speaker typed into the
	// posting dialog, picking up a session kept from an earlier run if it
	// can. When the speaker must approve it in a browser, SignIn calls
	// visit with the page's address, which the presenter view shows and
	// opens, and waits until they do or ctx ends.
	SignIn(ctx context.Context, account string, visit func(url string)) error

	// Prepare runs after SignIn, before posting starts, with every post the
	// talk can make: it checks each against the service's limits, without
	// posting anything.
	Prepare(ctx context.Context, posts []Post) error

	// Upload sends p's media ahead of Post, while its slide is up, so the
	// post goes out quickly when the speaker moves on. It runs alongside
	// Post and other Uploads. Post works whatever became of it: it waits
	// for an upload still running, uploads what one didn't, and sends
	// nothing twice.
	Upload(ctx context.Context, p Post) error

	// Post publishes p in reply to parent, in the thread started by root,
	// and returns a reference to it, which the next post gets as its parent
	// (and the first as root). Both are empty for the thread's first post.
	Post(ctx context.Context, p Post, root, parent string) (ref string, err error)
}

// Post is one post of a live thread, from a post block in Deck.Notes.
type Post struct {
	// Slide is the slide the post is about, from 0; -1 for a post before
	// the first slide's section, which opens the thread.
	Slide int

	Text string

	// Media is what the post attaches: images, or one video.
	Media []Media
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
func openerPosts(opener []notePost) []Post {
	var posts []Post
	for _, np := range opener {
		posts = append(posts, Post{Slide: -1, Text: np.text})
	}
	return posts
}

// Posts are every post the deck's notes make, in order, with their media
// rendered into dir as the presenter view's posting dialog renders it.
// Clips need ffmpeg. decktest.Posts checks them before the talk.
func (d *Deck) Posts(dir string) ([]Post, error) {
	dn, err := parseNotes(d.Notes, d.Slides)
	if err != nil {
		return nil, err
	}
	posts := openerPosts(dn.opener)
	lock := new(sync.Mutex)
	for i, sn := range dn.slides {
		ps, err := slidePosts(d, i, sn.posts, dir, lock)
		if err != nil {
			return nil, err
		}
		posts = append(posts, ps...)
	}
	return posts, nil
}

// slidePosts are slide i's posts, from its notes, with their media rendered
// into dir. lock is held while a slide draws: slide Views expect one caller
// at a time, and the presenter view draws its previews alongside.
func slidePosts(d *Deck, i int, notes []notePost, dir string, lock sync.Locker) ([]Post, error) {
	if len(notes) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var posts []Post
	for n, np := range notes {
		p := Post{Slide: i, Text: np.text}
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

// thread is what has been posted of a live thread, in order, kept in a file
// so a presenter view that restarts carries on the same thread.
type thread struct {
	path  string       // "" keeps it in memory, for a dry run
	Posts []threadPost `json:"posts"`
}

type threadPost struct {
	Slide int       `json:"slide"` // from 0; -1 for the opener
	N     int       `json:"n"`     // which of the slide's posts, from 0
	Title string    `json:"title,omitempty"`
	Ref   string    `json:"ref"`
	At    time.Time `json:"at"`
}

// loadThread reads the thread kept at path, or starts one if there is no
// file yet.
func loadThread(path string) (*thread, error) {
	t := &thread{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return t, nil
	}
	if err == nil {
		err = json.Unmarshal(b, t)
	}
	if err != nil {
		return nil, fmt.Errorf("thread %s: %w", path, err)
	}
	return t, nil
}

// saveThread saves the thread at path as it is once tp is added to posts;
// a path of "" saves nothing.
func saveThread(path string, posts []threadPost, tp threadPost) error {
	if path == "" {
		return nil
	}
	return writeJSON(path, thread{Posts: append(posts, tp)})
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

func (t *thread) has(slide, n int) bool {
	return slices.ContainsFunc(t.Posts, func(tp threadPost) bool { return tp.Slide == slide && tp.N == n })
}

// refs are what the next post replies to: the thread's first post and its
// last.
func (t *thread) refs() (root, parent string) {
	if len(t.Posts) == 0 {
		return "", ""
	}
	return t.Posts[0].Ref, t.Posts[len(t.Posts)-1].Ref
}

// publishing is the presenter view's side of a live thread: it watches the
// deck move, decides what to post, and posts it in order, one at a time.
type publishing struct {
	pub    Publisher
	dry    bool
	file   string         // a rehearsal's, the posts are written to
	posts  map[int][]Post // by slide; -1 opens the thread
	titles []string       // the slides' titles, for the thread file
	thread *thread

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

// pending reports whether slide's post n is neither posted nor queued.
func (pb *publishing) pending(slide, n int) bool {
	return !pb.thread.has(slide, n) && !slices.Contains(pb.queue, queuedPost{slide, n})
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
		if len(p.Media) > 0 && !pb.thread.has(slide, n) {
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
		case pb.thread.has(slide, n):
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

// sent is how many posts the thread has, including any from before a
// restart; total, how many the talk makes.
func (pb *publishing) sent() int { return len(pb.thread.Posts) }

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
// flight or waiting to retry.
func (pb *publishing) next() tea.Cmd {
	if !pb.on || pb.busy || !pb.retryAt.IsZero() || len(pb.queue) == 0 {
		return nil
	}
	q := pb.queue[0]
	p := pb.posts[q.slide][q.n]
	root, parent := pb.thread.refs()
	title := ""
	if q.slide >= 0 {
		title = pb.titles[q.slide]
	}
	path, posts := pb.thread.path, slices.Clone(pb.thread.Posts)
	pb.busy = true
	pub := pb.pub
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), postTimeout)
		defer cancel()
		ref, err := pub.Post(ctx, p, root, parent)
		if err != nil {
			return postDoneMsg{pb: pb, q: q, err: err}
		}
		tp := threadPost{Slide: q.slide, N: q.n, Title: title, Ref: ref, At: time.Now()}
		return postDoneMsg{pb: pb, q: q, tp: tp, saveErr: saveThread(path, posts, tp)}
	}
}

// done records a finished post. A failed one stays first in the queue, so
// the thread keeps its order, and is retried after backoff, which doubles
// with each failure in a row up to a minute.
func (pb *publishing) done(msg postDoneMsg, now time.Time) (retry time.Duration) {
	pb.busy = false
	if msg.err == nil {
		pb.thread.Posts = append(pb.thread.Posts, msg.tp)
		pb.queue = pb.queue[1:]
		pb.err, pb.fails, pb.saveErr = "", 0, ""
		if msg.saveErr != nil {
			// Posted, but not saved: a restart would post it again.
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

// dryRun is the Publisher a dry run posts to: nothing leaves the machine.
// Each post is added to a Markdown file instead, with its media, to
// rehearse with.
type dryRun struct {
	path string
	n    int
}

func (*dryRun) SignIn(context.Context, string, func(string)) error { return nil }
func (*dryRun) Prepare(context.Context, []Post) error              { return nil }
func (*dryRun) Upload(context.Context, Post) error                 { return nil }

func (d *dryRun) Post(_ context.Context, p Post, root, parent string) (string, error) {
	d.n++
	var b strings.Builder
	about := "opens the thread"
	if p.Slide >= 0 {
		about = fmt.Sprintf("slide %d", p.Slide+1)
	}
	fmt.Fprintf(&b, "## Post %d, %s, in reply to %s\n\n%s\n", d.n, about, cmp.Or(parent, "nothing"), p.Text)
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
