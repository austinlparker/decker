// Package bsky posts a decker talk to Bluesky as a thread while it's given:
// a [decker.Publisher] for any atproto account whose host runs the
// app.bsky lexicons.
//
//	d := &decker.Deck{
//		Notes:     notes,
//		Publisher: &bsky.Publisher{Langs: []string{"en"}},
//		...
//	}
//
// Then in the presenter view, P opens the posting dialog: type the
// account and start posting, which signs in through the browser, checks
// every post in the deck's Notes and posts each slide's as you move on
// from it. A post is created under a record key made from what the
// thread file keeps of it, so one tried again, after a restart too, is
// found if it went out rather than posted twice. A slide's images or clip upload while it's up, so its posts go
// out without waiting on them. The session is kept in the system keychain,
// so later runs pick it up without the browser, until the account's host
// ends it: within two weeks of signing in, for a client like this one.
package bsky

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/austinlparker/decker"
	comatproto "github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/rivo/uniseg"
	"golang.org/x/sync/errgroup"
)

// Publisher posts a talk to Bluesky, from the account the presenter view
// signs it in to. Its zero value is ready to use.
type Publisher struct {
	// Langs are the posts' languages, as BCP 47 tags such as "en", up to
	// three. None leaves them for readers' apps to guess.
	Langs []string

	client *http.Client       // for OAuth; nil keeps indigo's, which refuses private addresses
	video  string             // the video service; "" is Bluesky's
	dir    identity.Directory // for @mentions; nil is the network's
}

// session is a signed-in account, from SignIn.
type session struct {
	pub    *Publisher
	api    *atclient.APIClient
	did    syntax.DID
	pds    string   // the account's host, the audience of a video upload's token
	scopes []string // what the account granted
	video  string   // the video service
}

// thread posts the posts a session opened it with, and holds what opening
// worked out for them. Its methods may be called at once: Upload runs
// alongside Post and other Uploads.
type thread struct {
	s        *session
	langs    []string
	mentions map[syntax.Handle]syntax.DID // the posts' @mentions, resolved
	images   map[string]string            // a rendered image too big to post, to the smaller copy that goes instead

	// mu guards blobs. It's never held across a call to the network: a
	// clip can take minutes to upload, and a post shouldn't wait on it for
	// anything but the clip.
	mu    sync.Mutex
	blobs map[string]*upload // each file uploaded or uploading, so no file goes up twice
}

// upload is a file's upload, once started: done closes when it ends, with
// blob or err set. One that failed is gone from blobs by then, so a caller
// waiting on it uploads the file itself.
type upload struct {
	done chan struct{}
	blob *lexutil.LexBlob
	err  error
}

var (
	_ decker.Publisher = (*Publisher)(nil)
	_ decker.Session   = (*session)(nil)
	_ decker.Thread    = (*thread)(nil)
)

// Bluesky's limits, from the app.bsky lexicons. A gallery's schema takes 20
// items, but asks authoring clients to hold to 10 for now.
const (
	maxGraphemes  = 300
	maxTextBytes  = 3000
	maxImages     = 4  // in app.bsky.embed.images
	maxGallery    = 10 // in app.bsky.embed.gallery
	maxImageBytes = 2_000_000
	maxVideoBytes = 300_000_000
	maxLangs      = 3
)

const (
	videoService    = "https://video.bsky.app"
	videoServiceDID = "did:web:video.bsky.app"
)

// parallel is how many images, or handles, a call sends or looks up at
// once.
const parallel = 4

// checked is what checking a talk's posts finds: what Open needs to post
// them.
type checked struct {
	images     map[string]string
	mentions   map[syntax.Handle]syntax.DID
	videos     int
	videoBytes int64
}

// Check holds every post to Bluesky's limits and resolves its @mentions.
// An image too big to post is re-encoded smaller beside the rendered one.
func (p *Publisher) Check(ctx context.Context, posts []decker.Post) error {
	_, err := p.check(ctx, posts)
	return err
}

func (p *Publisher) check(ctx context.Context, posts []decker.Post) (checked, error) {
	c := checked{images: map[string]string{}}
	if len(p.Langs) > maxLangs {
		return c, fmt.Errorf("bsky: %d Langs, but a post takes up to %d", len(p.Langs), maxLangs)
	}
	for _, post := range posts {
		n, size, err := check(post, c.images)
		if err != nil {
			return c, fmt.Errorf("bsky: %s: %w", about(post), err)
		}
		c.videos, c.videoBytes = c.videos+n, c.videoBytes+size
	}
	dir := p.dir
	if dir == nil {
		dir = identity.DefaultDirectory()
	}
	var err error
	c.mentions, err = resolveMentions(ctx, dir, posts)
	return c, err
}

// Account is the account's DID: its handle can change.
func (s *session) Account() string { return s.did.String() }

// Open checks posts as Check does, and that the account may post their
// clips: that it granted what they need and may upload that much video
// today.
func (s *session) Open(ctx context.Context, posts []decker.Post) (decker.Thread, error) {
	c, err := s.pub.check(ctx, posts)
	if err != nil {
		return nil, err
	}
	if c.videos > 0 {
		for _, scope := range []string{scopeVideoUpload, scopeVideoLimits} {
			if !slices.Contains(s.scopes, scope) {
				return nil, fmt.Errorf("bsky: the account didn't grant %q, which posting clips needs; "+
					"its host may not support it, so use media=final or builds instead", scope)
			}
		}
		if err := s.checkVideoLimits(ctx, c.videos, c.videoBytes); err != nil {
			return nil, fmt.Errorf("bsky: %w", err)
		}
	}
	return &thread{s: s, langs: slices.Clone(s.pub.Langs), mentions: c.mentions, images: c.images}, nil
}

// check holds post to Bluesky's limits, and returns the video it uploads.
// An image too big to post gets a smaller copy, which images maps it to.
func check(post decker.Post, images map[string]string) (videos int, videoBytes int64, err error) {
	if n := uniseg.GraphemeClusterCount(post.Text); n > maxGraphemes {
		return 0, 0, fmt.Errorf("%d characters, but a post takes up to %d", n, maxGraphemes)
	}
	if len(post.Text) > maxTextBytes {
		return 0, 0, fmt.Errorf("%d bytes of text, but a post takes up to %d", len(post.Text), maxTextBytes)
	}
	var stills int
	for _, m := range post.Media {
		if m.Video {
			size, err := fileSize(m.Path)
			if err != nil {
				return 0, 0, err
			}
			if size > maxVideoBytes {
				return 0, 0, fmt.Errorf("%s is %d bytes, but a video can be up to %d", m.Path, size, maxVideoBytes)
			}
			videos, videoBytes = videos+1, videoBytes+size
			continue
		}
		stills++
		small, err := fitImage(m.Path, maxImageBytes)
		if err != nil {
			return 0, 0, err
		}
		if small != m.Path {
			images[m.Path] = small
		}
	}
	switch {
	case videos > 1:
		return 0, 0, errors.New("a post takes one clip; split the clips into posts of their own")
	case videos > 0 && stills > 0:
		return 0, 0, errors.New("a post takes a clip or images, not both; split them into posts of their own")
	case stills > maxGallery:
		return 0, 0, fmt.Errorf("%d images, but a post takes up to %d; pick %d with builds=", stills, maxGallery, maxGallery)
	}
	return videos, videoBytes, nil
}

// about names a post for errors, as the speaker would find it in Notes.
func about(post decker.Post) string {
	text, _, _ := strings.Cut(post.Text, "\n")
	if g := uniseg.GraphemeClusterCount(text); g > 40 {
		gr := uniseg.NewGraphemes(text)
		var b strings.Builder
		for i := 0; i < 39 && gr.Next(); i++ {
			b.WriteString(gr.Str())
		}
		text = b.String() + "…"
	}
	if post.Slide < 0 {
		return fmt.Sprintf("the opening post (%q)", text)
	}
	return fmt.Sprintf("slide %d's post %q", post.Slide+1, text)
}

// resolveMentions looks up each @handle the posts mention, so a typo shows
// before the talk and each post links the account it names. A handle that
// doesn't resolve is reported for the first post to mention it.
func resolveMentions(ctx context.Context, dir identity.Directory, posts []decker.Post) (map[syntax.Handle]syntax.DID, error) {
	var handles []syntax.Handle
	first := map[syntax.Handle]decker.Post{}
	for _, post := range posts {
		for _, h := range mentionHandles(post.Text) {
			if _, ok := first[h]; !ok {
				first[h] = post
				handles = append(handles, h)
			}
		}
	}
	dids := make([]syntax.DID, len(handles))
	errs := make([]error, len(handles))
	var g errgroup.Group
	g.SetLimit(parallel)
	for i, h := range handles {
		g.Go(func() error {
			id, err := dir.LookupHandle(ctx, h)
			if err == nil {
				dids[i] = id.DID
			}
			errs[i] = err
			return nil
		})
	}
	g.Wait()
	mentions := make(map[syntax.Handle]syntax.DID, len(handles))
	for i, h := range handles {
		if errs[i] != nil {
			return nil, fmt.Errorf("bsky: %s mentions @%s, which doesn't resolve: %w", about(first[h]), h, errs[i])
		}
		mentions[h] = dids[i]
	}
	return mentions, nil
}

// Upload uploads post's media ahead of Post, which then finds it uploaded,
// or waits for it if it's still going. A file that fails to upload is
// tried again by the next call that needs it.
func (t *thread) Upload(ctx context.Context, post decker.Post) error {
	_, err := t.upload(ctx, post.Media)
	return err
}

// Post uploads post's media, unless an Upload has, and posts it where d
// says. The post's record key is made from d's Key and First, the same on
// every try, so a retry finds the post if an earlier try made it.
func (t *thread) Post(ctx context.Context, post decker.Post, d decker.Delivery) (string, error) {
	rkey := recordKey(d)
	if d.Retry {
		if ref, err := t.s.existing(ctx, rkey); err != nil || ref != "" {
			return ref, err
		}
	}
	rec := &appbsky.FeedPost{
		Text:      post.Text,
		CreatedAt: syntax.DatetimeNow().String(),
		Langs:     t.langs,
		Facets:    facets(post.Text, t.mentions),
	}
	if d.Root != "" {
		r, err := strongRef(d.Root)
		if err != nil {
			return "", err
		}
		pr, err := strongRef(d.Parent)
		if err != nil {
			return "", err
		}
		rec.Reply = &appbsky.FeedPost_ReplyRef{Root: r, Parent: pr}
	}
	var err error
	if rec.Embed, err = t.embed(ctx, post.Media); err != nil {
		return "", err
	}
	out, err := comatproto.RepoCreateRecord(ctx, t.s.api, &comatproto.RepoCreateRecord_Input{
		Repo:       t.s.did.String(),
		Collection: "app.bsky.feed.post",
		Rkey:       new(rkey.String()),
		Record:     &lexutil.LexiconTypeDecoder{Val: rec},
	})
	if blobMissing(err) {
		t.forget(post.Media)
	}
	if err != nil {
		return "", fmt.Errorf("bsky: posting: %w", err)
	}
	return out.Uri + " " + out.Cid, nil
}

// recordKey is the record key of the post d delivers: a TID, as a post's
// key must be, of when it was first tried, and its clock ID from d.Key,
// which tells it from a post first tried the same microsecond.
func recordKey(d decker.Delivery) syntax.TID {
	h := fnv.New32a()
	h.Write([]byte(d.Key))
	return syntax.NewTIDFromTime(d.First, uint(h.Sum32()))
}

// blobMissing reports whether err is the account's host refusing a post
// for a blob it doesn't have: a host deletes a blob no record uses after a
// while, and the post's may have gone up long before it. The reference PDS
// names the error BlobNotFound.
func blobMissing(err error) bool {
	var apiErr *atclient.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	msg := strings.ToLower(apiErr.Message)
	return apiErr.Name == "BlobNotFound" || apiErr.Name == "InvalidRequest" &&
		(strings.Contains(msg, "could not find blob") || strings.Contains(msg, "blob not found"))
}

// forget drops media's blobs, for the next try to upload them again. Only
// a host that lost them calls for it: uploading a clip again spends the
// account's daily video allowance.
func (t *thread) forget(media []decker.Media) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range media {
		delete(t.blobs, m.Path)
	}
}

// existing is the ref of the post at rkey, or "" if there is none. It
// reads the ref alone, not the record: one this version can't decode would
// otherwise fail every retry, and the post would never go out.
func (s *session) existing(ctx context.Context, rkey syntax.TID) (string, error) {
	var out struct {
		Uri string  `json:"uri"`
		Cid *string `json:"cid"`
	}
	params := map[string]any{"repo": s.did.String(), "collection": "app.bsky.feed.post", "rkey": rkey.String()}
	err := s.api.LexDo(ctx, lexutil.Query, "", "com.atproto.repo.getRecord", params, nil, &out)
	var apiErr *atclient.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.Name == "RecordNotFound") {
		return "", nil
	}
	if err == nil && out.Cid == nil {
		err = fmt.Errorf("no CID for %s", out.Uri)
	}
	if err != nil {
		return "", fmt.Errorf("bsky: checking whether the last try posted: %w", err)
	}
	return out.Uri + " " + *out.Cid, nil
}

// A ref, as Post returns it and the thread file keeps it, is a post's
// strong ref, its at:// URI and its CID, joined by a space: neither can hold
// one.
func strongRef(ref string) (*comatproto.RepoStrongRef, error) {
	uri, cid, ok := strings.Cut(ref, " ")
	if _, err := syntax.ParseATURI(uri); !ok || err != nil {
		return nil, fmt.Errorf("bsky: %q isn't a post this publisher made; is the thread file from a dry run?", ref)
	}
	return &comatproto.RepoStrongRef{Uri: uri, Cid: cid}, nil
}

// embed uploads media and returns the post's embed for it: images, or one
// clip that plays like a GIF. Up to four images go as an images embed,
// which every client shows; more, as a gallery, which clients from before
// it was added in mid-2026 don't.
func (t *thread) embed(ctx context.Context, media []decker.Media) (*appbsky.FeedPost_Embed, error) {
	if len(media) == 0 {
		return nil, nil
	}
	blobs, err := t.upload(ctx, media)
	if err != nil {
		return nil, err
	}
	if m := media[0]; m.Video {
		return &appbsky.FeedPost_Embed{EmbedVideo: &appbsky.EmbedVideo{
			Video:        blobs[0],
			Alt:          new(m.Alt),
			AspectRatio:  aspect(m),
			Presentation: new("gif"),
		}}, nil
	}
	if len(media) <= maxImages {
		images := make([]*appbsky.EmbedImages_Image, len(media))
		for i, m := range media {
			images[i] = &appbsky.EmbedImages_Image{Image: blobs[i], Alt: m.Alt, AspectRatio: aspect(m)}
		}
		return &appbsky.FeedPost_Embed{EmbedImages: &appbsky.EmbedImages{Images: images}}, nil
	}
	items := make([]*appbsky.EmbedGallery_Items_Elem, len(media))
	for i, m := range media {
		items[i] = &appbsky.EmbedGallery_Items_Elem{EmbedGallery_Image: &appbsky.EmbedGallery_Image{
			Image: blobs[i], Alt: m.Alt, AspectRatio: aspect(m)}}
	}
	return &appbsky.FeedPost_Embed{EmbedGallery: &appbsky.EmbedGallery{Items: items}}, nil
}

func aspect(m decker.Media) *appbsky.EmbedDefs_AspectRatio {
	return &appbsky.EmbedDefs_AspectRatio{Width: int64(m.W), Height: int64(m.H)}
}

// upload uploads media, parallel files at a time, and returns their blobs
// in order. A file that fails doesn't stop the rest, so a retry finds
// them uploaded.
func (t *thread) upload(ctx context.Context, media []decker.Media) ([]*lexutil.LexBlob, error) {
	blobs := make([]*lexutil.LexBlob, len(media))
	var g errgroup.Group
	g.SetLimit(parallel)
	for i, m := range media {
		g.Go(func() (err error) {
			blobs[i], err = t.blob(ctx, m)
			return err
		})
	}
	return blobs, g.Wait()
}

// blob is m's blob: one uploaded before, or by a call uploading it now,
// which blob waits for, or else one blob uploads.
func (t *thread) blob(ctx context.Context, m decker.Media) (*lexutil.LexBlob, error) {
	for {
		t.mu.Lock()
		u, started := t.blobs[m.Path]
		if !started {
			u = &upload{done: make(chan struct{})}
			if t.blobs == nil {
				t.blobs = map[string]*upload{}
			}
			t.blobs[m.Path] = u
		}
		t.mu.Unlock()
		if !started {
			return t.send(ctx, m, u)
		}
		select {
		case <-u.done:
			if u.err == nil {
				return u.blob, nil
			}
		case <-ctx.Done():
			return nil, fmt.Errorf("bsky: uploading %s: %w", m.Path, ctx.Err())
		}
	}
}

// send uploads m for u, and ends it.
func (t *thread) send(ctx context.Context, m decker.Media, u *upload) (*lexutil.LexBlob, error) {
	if m.Video {
		u.blob, u.err = t.s.uploadVideo(ctx, m.Path)
	} else {
		u.blob, u.err = t.s.uploadImage(ctx, cmp.Or(t.images[m.Path], m.Path))
	}
	if u.err != nil {
		u.err = fmt.Errorf("bsky: uploading %s: %w", m.Path, u.err)
		t.mu.Lock()
		if t.blobs[m.Path] == u {
			delete(t.blobs, m.Path)
		}
		t.mu.Unlock()
	}
	close(u.done)
	return u.blob, u.err
}
