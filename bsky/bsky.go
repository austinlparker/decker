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
// from it. A slide's images or clip upload while it's up, so its posts go
// out without waiting on them. The session is kept in the system keychain,
// so later runs pick it up without the browser, until the account's host
// ends it: within two weeks of signing in, for a client like this one.
package bsky

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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
// signs it in to. Its zero value is ready to use, and its methods may be
// called at once: Upload runs alongside Post and other Uploads.
type Publisher struct {
	// Langs are the posts' languages, as BCP 47 tags such as "en", up to
	// three. None leaves them for readers' apps to guess.
	Langs []string

	client *http.Client // for OAuth; nil keeps indigo's, which refuses private addresses
	video  string       // the video service; "" is Bluesky's

	// mu guards the rest. It's never held across a call to the network:
	// a clip can take minutes to upload, and a post shouldn't wait on it
	// for anything but the clip.
	mu       sync.Mutex
	sess     *session // nil until SignIn
	dir      identity.Directory
	mentions map[syntax.Handle]syntax.DID // the posts' @mentions, resolved
	images   map[string]string            // a rendered image too big to post, to the smaller copy that goes instead
	blobs    map[string]*upload           // each file uploaded or uploading, so no file goes up twice
	rkeys    map[postID]syntax.TID        // each post's record key, from its first try
}

// session is a signed-in account, as each call takes it when it starts:
// SignIn may replace it while the call goes on.
type session struct {
	api    *atclient.APIClient
	did    syntax.DID
	pds    string   // the account's host, the audience of a video upload's token
	scopes []string // what the account granted
	video  string   // the video service
}

// upload is a file's upload, once started: done closes when it ends, with
// blob or err set. One that failed is gone from blobs by then, so a caller
// waiting on it uploads the file itself.
type upload struct {
	done chan struct{}
	blob *lexutil.LexBlob
	err  error
}

var _ decker.Publisher = (*Publisher)(nil)

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

// Prepare checks every post against Bluesky's limits and resolves its
// @mentions, and, signed in, checks the account may post its clips: that
// it granted what they need and may upload that much video today. A
// rehearsal prepares before signing in, and gets every check but those. An
// image too big to post is re-encoded smaller beside the rendered one.
func (p *Publisher) Prepare(ctx context.Context, posts []decker.Post) error {
	if len(p.Langs) > maxLangs {
		return fmt.Errorf("bsky: %d Langs, but a post takes up to %d", len(p.Langs), maxLangs)
	}
	images := map[string]string{}
	var videos int
	var videoBytes int64
	for _, post := range posts {
		n, size, err := check(post, images)
		if err != nil {
			return fmt.Errorf("bsky: %s: %w", about(post), err)
		}
		videos, videoBytes = videos+n, videoBytes+size
	}
	p.mu.Lock()
	p.images = images
	// A thread's first post has no root or parent to tell it from the last
	// thread's, so a new start mustn't take that one for its retry.
	for id := range p.rkeys {
		if id.root == "" {
			delete(p.rkeys, id)
		}
	}
	if p.dir == nil {
		p.dir = identity.DefaultDirectory()
	}
	dir, s := p.dir, p.sess
	p.mu.Unlock()
	mentions, err := resolveMentions(ctx, dir, posts)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.mentions = mentions
	p.mu.Unlock()
	if videos > 0 && s != nil {
		for _, scope := range []string{scopeVideoUpload, scopeVideoLimits} {
			if !slices.Contains(s.scopes, scope) {
				return fmt.Errorf("bsky: the account didn't grant %q, which posting clips needs; "+
					"its host may not support it, so use media=final or builds instead", scope)
			}
		}
		if err := s.checkVideoLimits(ctx, videos, videoBytes); err != nil {
			return fmt.Errorf("bsky: %w", err)
		}
	}
	return nil
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

// signedIn is the session calls post with.
func (p *Publisher) signedIn() (*session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sess == nil {
		return nil, errors.New("bsky: not signed in")
	}
	return p.sess, nil
}

// Upload uploads post's media ahead of Post, which then finds it uploaded,
// or waits for it if it's still going. A file that fails to upload is
// tried again by the next call that needs it.
func (p *Publisher) Upload(ctx context.Context, post decker.Post) error {
	s, err := p.signedIn()
	if err != nil {
		return err
	}
	_, err = p.upload(ctx, s, post.Media)
	return err
}

// Post uploads post's media, unless an Upload has, and posts it in reply
// to parent, in the thread root starts. A post is created with the same
// record key every time it's tried, so a retry after an answer that never
// arrived finds the post already there instead of posting it twice.
func (p *Publisher) Post(ctx context.Context, post decker.Post, root, parent string) (string, error) {
	s, err := p.signedIn()
	if err != nil {
		return "", err
	}
	key := newPostID(post, root, parent)
	p.mu.Lock()
	if p.rkeys == nil {
		p.rkeys = map[postID]syntax.TID{}
	}
	rkey, retry := p.rkeys[key]
	if !retry {
		rkey = syntax.NewTIDNow(0)
		p.rkeys[key] = rkey
	}
	mentions := p.mentions
	p.mu.Unlock()
	if retry {
		if ref, err := s.existing(ctx, rkey); err != nil || ref != "" {
			return ref, err
		}
	}
	rec := &appbsky.FeedPost{
		Text:      post.Text,
		CreatedAt: syntax.DatetimeNow().String(),
		Langs:     p.Langs,
		Facets:    facets(post.Text, mentions),
	}
	if root != "" {
		r, err := strongRef(root)
		if err != nil {
			return "", err
		}
		pr, err := strongRef(parent)
		if err != nil {
			return "", err
		}
		rec.Reply = &appbsky.FeedPost_ReplyRef{Root: r, Parent: pr}
	}
	if rec.Embed, err = p.embed(ctx, s, post.Media); err != nil {
		return "", err
	}
	out, err := comatproto.RepoCreateRecord(ctx, s.api, &comatproto.RepoCreateRecord_Input{
		Repo:       s.did.String(),
		Collection: "app.bsky.feed.post",
		Rkey:       new(rkey.String()),
		Record:     &lexutil.LexiconTypeDecoder{Val: rec},
	})
	if blobMissing(err) {
		p.forget(post.Media)
	}
	if err != nil {
		return "", fmt.Errorf("bsky: posting: %w", err)
	}
	return out.Uri + " " + out.Cid, nil
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
func (p *Publisher) forget(media []decker.Media) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range media {
		delete(p.blobs, m.Path)
	}
}

// postID tells posts apart across retries. A retry replies to the same
// parent, and a post that repeats an earlier one's text replies to a
// different one.
type postID struct {
	root, parent string
	slide        int
	text, media  string
}

func newPostID(post decker.Post, root, parent string) postID {
	id := postID{root: root, parent: parent, slide: post.Slide, text: post.Text}
	for _, m := range post.Media {
		id.media += "\x00" + m.Path
	}
	return id
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
func (p *Publisher) embed(ctx context.Context, s *session, media []decker.Media) (*appbsky.FeedPost_Embed, error) {
	if len(media) == 0 {
		return nil, nil
	}
	blobs, err := p.upload(ctx, s, media)
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
func (p *Publisher) upload(ctx context.Context, s *session, media []decker.Media) ([]*lexutil.LexBlob, error) {
	blobs := make([]*lexutil.LexBlob, len(media))
	var g errgroup.Group
	g.SetLimit(parallel)
	for i, m := range media {
		g.Go(func() (err error) {
			blobs[i], err = p.blob(ctx, s, m)
			return err
		})
	}
	return blobs, g.Wait()
}

// blob is m's blob: one uploaded before, or by a call uploading it now,
// which blob waits for, or else one blob uploads.
func (p *Publisher) blob(ctx context.Context, s *session, m decker.Media) (*lexutil.LexBlob, error) {
	for {
		p.mu.Lock()
		u, started := p.blobs[m.Path]
		if !started {
			u = &upload{done: make(chan struct{})}
			if p.blobs == nil {
				p.blobs = map[string]*upload{}
			}
			p.blobs[m.Path] = u
		}
		p.mu.Unlock()
		if !started {
			return p.send(ctx, s, m, u)
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
func (p *Publisher) send(ctx context.Context, s *session, m decker.Media, u *upload) (*lexutil.LexBlob, error) {
	if m.Video {
		u.blob, u.err = s.uploadVideo(ctx, m.Path)
	} else {
		u.blob, u.err = s.uploadImage(ctx, p.fitted(m.Path))
	}
	if u.err != nil {
		u.err = fmt.Errorf("bsky: uploading %s: %w", m.Path, u.err)
		p.mu.Lock()
		if p.blobs[m.Path] == u {
			delete(p.blobs, m.Path)
		}
		p.mu.Unlock()
	}
	close(u.done)
	return u.blob, u.err
}

// fitted is the file that goes up for the image at path: the smaller copy
// Prepare made of it, if it needed one.
func (p *Publisher) fitted(path string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return cmp.Or(p.images[path], path)
}
