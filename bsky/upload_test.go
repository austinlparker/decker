package bsky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/austinlparker/decker"
	"github.com/bluesky-social/indigo/atproto/atclient"
	lexutil "github.com/bluesky-social/indigo/lex/util"
)

// recordOf is the record h keeps for the post ref names.
func recordOf(t *testing.T, h *fakeHost, ref string, v any) {
	t.Helper()
	uri, _, _ := strings.Cut(ref, " ")
	h.mu.Lock()
	raw := h.records[uri[strings.LastIndex(uri, "/")+1:]]
	h.mu.Unlock()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}

// imageLinks are the CIDs of the images a post's record embeds, in order.
func imageLinks(t *testing.T, h *fakeHost, ref string) []string {
	type image struct {
		Alt   string
		Image struct {
			Ref struct {
				Link string `json:"$link"`
			}
		}
	}
	var rec struct {
		Embed struct{ Images, Items []image }
	}
	recordOf(t, h, ref, &rec)
	var links []string
	for _, img := range append(rec.Embed.Images, rec.Embed.Items...) {
		links = append(links, img.Image.Ref.Link)
	}
	return links
}

func fileCID(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return cidOf(b)
}

// TestUploadThenPost uploads each post's media ahead, as the presenter view
// does while a slide is up: posting then uploads nothing.
func TestUploadThenPost(t *testing.T) {
	pollEvery = 0
	h := newFakeHost(t)
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(clip, []byte("mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	posts := []decker.Post{
		{Slide: 0, Text: "two", Media: []decker.Media{
			{Path: writePNG(t, dir, "a.png", 16, false), W: 16, H: 9},
			{Path: writePNG(t, dir, "b.png", 17, false), W: 16, H: 9},
		}},
		{Slide: 1, Text: "a clip", Media: []decker.Media{{Path: clip, Video: true, W: 16, H: 9}}},
	}
	ctx := context.Background()
	th := open(t, h.session(), posts...)
	for _, post := range posts {
		for range 2 {
			if err := th.Upload(ctx, post); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(h.uploads) != 3 {
		t.Fatalf("uploads ahead: %q", h.uploads)
	}
	for i, post := range posts {
		if _, err := th.Post(ctx, post, first(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.uploads) != 3 || len(h.records) != 2 {
		t.Errorf("after posting: uploads %q, %d records", h.uploads, len(h.records))
	}
}

// TestPostDuringUpload posts while an Upload of the same file is held up:
// the file goes up once.
func TestPostDuringUpload(t *testing.T) {
	h := newFakeHost(t)
	h.gate, h.arrived = make(chan struct{}), make(chan string, 4)
	post := decker.Post{Slide: 0, Text: "once", Media: []decker.Media{
		{Path: writePNG(t, t.TempDir(), "a.png", 16, false), W: 16, H: 9}}}
	ctx := context.Background()
	th := open(t, h.session(), post)
	uploaded := make(chan error, 1)
	go func() { uploaded <- th.Upload(ctx, post) }()
	<-h.arrived
	posted := make(chan error, 1)
	go func() {
		_, err := th.Post(ctx, post, first("once"))
		posted <- err
	}()
	close(h.gate)
	if err := <-uploaded; err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if len(h.uploads) != 1 || len(h.records) != 1 {
		t.Errorf("uploads %q, %d records", h.uploads, len(h.records))
	}
}

// TestPostWaitsForUpload puts an upload under way, as Upload leaves it while
// it sends a file, and posts: the post waits for it, whatever became of it.
func TestPostWaitsForUpload(t *testing.T) {
	h := newFakeHost(t)
	path := writePNG(t, t.TempDir(), "a.png", 16, false)
	post := decker.Post{Slide: 0, Text: "waits", Media: []decker.Media{{Path: path, W: 16, H: 9}}}
	var theirs lexutil.LexBlob
	if err := json.Unmarshal([]byte(blobJSON(cidOf([]byte("theirs")), "image/png")), &theirs); err != nil {
		t.Fatal(err)
	}
	start := func(ctx context.Context, th *thread) (*upload, chan string, chan error) {
		u := &upload{done: make(chan struct{})}
		th.blobs = map[string]*upload{path: u}
		refs, errs := make(chan string, 1), make(chan error, 1)
		go func() {
			ref, err := th.Post(ctx, post, first("waits"))
			refs <- ref
			errs <- err
		}()
		return u, refs, errs
	}

	th := open(t, h.session(), post)
	u, refs, errs := start(context.Background(), th)
	u.blob = &theirs
	close(u.done)
	ref := <-refs
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if links := imageLinks(t, h, ref); len(h.uploads) != 0 || len(links) != 1 || links[0] != theirs.Ref.String() {
		t.Errorf("finished: uploads %q, embeds %q", h.uploads, links)
	}

	// One that fails is gone from blobs as it ends: the post uploads the
	// file itself.
	th = open(t, h.session(), post)
	u, refs, errs = start(context.Background(), th)
	th.mu.Lock()
	delete(th.blobs, path)
	th.mu.Unlock()
	u.err = errors.New("lost")
	close(u.done)
	ref = <-refs
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if links := imageLinks(t, h, ref); len(h.uploads) != 1 || len(links) != 1 || links[0] != fileCID(t, path) {
		t.Errorf("failed: uploads %q, embeds %q", h.uploads, links)
	}

	// One that never ends holds the post only until its context does.
	ctx, cancel := context.WithCancel(context.Background())
	_, _, errs = start(ctx, open(t, h.session(), post))
	cancel()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

// TestUploadRetry fails an Upload: the post uploads the file again.
func TestUploadRetry(t *testing.T) {
	h := newFakeHost(t)
	h.failUploads = 1
	post := decker.Post{Slide: 0, Text: "retried", Media: []decker.Media{
		{Path: writePNG(t, t.TempDir(), "a.png", 16, false), W: 16, H: 9}}}
	ctx := context.Background()
	th := open(t, h.session(), post)
	if err := th.Upload(ctx, post); err == nil || !strings.Contains(err.Error(), "a.png") {
		t.Fatalf("a failed upload: %v", err)
	}
	if _, err := th.Post(ctx, post, first("retried")); err != nil {
		t.Fatal(err)
	}
	if len(h.uploads) != 2 || len(h.records) != 1 {
		t.Errorf("uploads %q, %d records", h.uploads, len(h.records))
	}
}

// TestGalleryUpload holds a gallery's uploads until four are under at once,
// then lets them finish in any order: the embed keeps the media's.
func TestGalleryUpload(t *testing.T) {
	h := newFakeHost(t)
	h.gate, h.arrived = make(chan struct{}), make(chan string, maxGallery)
	dir := t.TempDir()
	var media []decker.Media
	var want []string
	for i := range 6 {
		path := writePNG(t, dir, fmt.Sprintf("%d.png", i), 16+i, false)
		media = append(media, decker.Media{Path: path, Alt: fmt.Sprint(i), W: 16, H: 9})
		want = append(want, fileCID(t, path))
	}
	post := decker.Post{Slide: 0, Text: "six", Media: media}
	ctx := context.Background()
	th := open(t, h.session(), post)
	refs, errs := make(chan string, 1), make(chan error, 1)
	go func() {
		ref, err := th.Post(ctx, post, first("six"))
		refs <- ref
		errs <- err
	}()
	for range parallel {
		<-h.arrived
	}
	close(h.gate)
	ref := <-refs
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if got := imageLinks(t, h, ref); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestBlobMissing has the host refuse a post for a blob it deleted: the
// retry uploads the media again. A post refused for anything else keeps
// them, which TestPostRetry checks.
func TestBlobMissing(t *testing.T) {
	h := newFakeHost(t)
	h.missingBlobs = 1
	post := decker.Post{Slide: 0, Text: "lost", Media: []decker.Media{
		{Path: writePNG(t, t.TempDir(), "a.png", 16, false), W: 16, H: 9}}}
	ctx := context.Background()
	th := open(t, h.session(), post)
	d := first("lost")
	if _, err := th.Post(ctx, post, d); err == nil || !strings.Contains(err.Error(), "BlobNotFound") {
		t.Fatalf("a missing blob: %v", err)
	}
	d.Retry = true
	if _, err := th.Post(ctx, post, d); err != nil {
		t.Fatal(err)
	}
	if len(h.uploads) != 2 || len(h.records) != 1 {
		t.Errorf("uploads %q, %d records", h.uploads, len(h.records))
	}

	for err, want := range map[error]bool{
		&atclient.APIError{StatusCode: 400, Name: "BlobNotFound"}:                                      true,
		&atclient.APIError{StatusCode: 400, Name: "InvalidRequest", Message: "Could not find blob: x"}: true,
		&atclient.APIError{StatusCode: 400, Name: "InvalidRequest", Message: "record already exists"}:  false,
		&atclient.APIError{StatusCode: 500, Name: "InternalServerError"}:                               false,
		fmt.Errorf("posting: %w", errors.New("connection reset")):                                      false,
	} {
		if got := blobMissing(err); got != want {
			t.Errorf("blobMissing(%v) = %v", err, got)
		}
	}
}
