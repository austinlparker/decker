package bsky

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/austinlparker/decker"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

func TestFacets(t *testing.T) {
	mentions := map[syntax.Handle]syntax.DID{"me.bsky.social": "did:plc:me"}
	text := "Slides (https://example.com/talk). Thanks @Me.bsky.social. #OTel #2024 " +
		"mail me@example.com, @home, @nobody.example #ünïcode! https://x.example/a_(b)"
	type got struct{ span, feature string }
	var gots []got
	for _, f := range facets(text, mentions) {
		var feature string
		switch ft := f.Features[0]; {
		case ft.RichtextFacet_Link != nil:
			feature = "link " + ft.RichtextFacet_Link.Uri
		case ft.RichtextFacet_Mention != nil:
			feature = "mention " + ft.RichtextFacet_Mention.Did
		case ft.RichtextFacet_Tag != nil:
			feature = "tag " + ft.RichtextFacet_Tag.Tag
		}
		gots = append(gots, got{text[f.Index.ByteStart:f.Index.ByteEnd], feature})
	}
	want := []got{
		{"https://example.com/talk", "link https://example.com/talk"},
		{"@Me.bsky.social", "mention did:plc:me"},
		{"#OTel", "tag OTel"},
		{"#ünïcode", "tag ünïcode"},
		{"https://x.example/a_(b)", "link https://x.example/a_(b)"},
	}
	if !slices.Equal(gots, want) {
		t.Errorf("got  %q\nwant %q", gots, want)
	}
	if hs := mentionHandles(text); !slices.Equal(hs, []syntax.Handle{"me.bsky.social", "nobody.example"}) {
		t.Errorf("mentionHandles: %q", hs)
	}
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	small := writePNG(t, dir, "small.png", 16, false)
	clip := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(clip, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	img := decker.Media{Path: small, W: 16, H: 9}
	vid := decker.Media{Path: clip, Video: true, W: 16, H: 9}
	for name, tc := range map[string]struct {
		post decker.Post
		want string
	}{
		"fine":          {decker.Post{Text: "hi", Media: []decker.Media{img, img, img, img}}, ""},
		"one clip":      {decker.Post{Text: "hi", Media: []decker.Media{vid}}, ""},
		"graphemes":     {decker.Post{Text: strings.Repeat("e\u0301", 300)}, ""},
		"bytes":         {decker.Post{Text: strings.Repeat("👩‍👩‍👧", 200)}, "3600 bytes"},
		"too long":      {decker.Post{Text: strings.Repeat("a", 301)}, "301 characters"},
		"ten images":    {decker.Post{Media: slices.Repeat([]decker.Media{img}, 10)}, ""},
		"eleven images": {decker.Post{Media: slices.Repeat([]decker.Media{img}, 11)}, "11 images"},
		"two clips":     {decker.Post{Media: []decker.Media{vid, vid}}, "one clip"},
		"mixed":         {decker.Post{Media: []decker.Media{vid, img}}, "a clip or images"},
		"missing":       {decker.Post{Media: []decker.Media{{Path: filepath.Join(dir, "gone.png")}}}, "gone.png"},
	} {
		_, _, err := check(tc.post, map[string]string{})
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
}

func TestFitImage(t *testing.T) {
	dir := t.TempDir()
	noisy := writePNG(t, dir, "noisy.png", 512, true)
	size, _ := fileSize(noisy)
	small, err := fitImage(noisy, size/4)
	if err != nil {
		t.Fatal(err)
	}
	if small != filepath.Join(dir, "noisy.jpg") {
		t.Fatalf("got %s", small)
	}
	if got, _ := fileSize(small); got > size/4 {
		t.Errorf("the JPEG is %d bytes, over %d", got, size/4)
	}
	if same, err := fitImage(noisy, size); err != nil || same != noisy {
		t.Errorf("an image under the limit: got %s, %v", same, err)
	}
	if _, err := fitImage(noisy, 100); err == nil {
		t.Error("an image no JPEG fits: accepted")
	}
}

// writePNG writes an n×n PNG: flat, or noise that compresses badly.
func writePNG(t *testing.T, dir, name string, n int, noisy bool) string {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	x := uint32(1)
	for i := range img.Pix {
		if noisy {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			img.Pix[i] = uint8(x)
		} else if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	img.Set(0, 0, color.White)
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeHost is an account's PDS and the video service in one server: it
// takes uploads and records, and answers the calls a Publisher makes.
type fakeHost struct {
	mu           sync.Mutex
	records      map[string]json.RawMessage // by rkey
	uploads      []string                   // each upload's Content-Type, failed ones included
	failUploads  int                        // uploadBlob calls that fail
	gate         chan struct{}              // if set, uploadBlob calls wait for it to close...
	arrived      chan string                // ...after sending their Content-Type here
	failPosts    int                        // createRecord calls that fail after storing the record, as if the answer was lost
	missingBlobs int                        // createRecord calls that fail for a blob the host deleted
	limits       string
	refreshes    []url.Values // token requests, in order
	srv          *httptest.Server
}

// cidOf is the CID a host gives a blob of b.
func cidOf(b []byte) string {
	sum := sha256.Sum256(b)
	raw := append([]byte{0x01, 0x55, 0x12, 0x20}, sum[:]...) // CIDv1, raw, sha2-256
	return "b" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))
}

func blobJSON(cid, mime string) string {
	return `{"$type":"blob","ref":{"$link":"` + cid + `"},"mimeType":"` + mime + `","size":1}`
}

func newFakeHost(t *testing.T) *fakeHost {
	h := &fakeHost{records: map[string]json.RawMessage{}, limits: `{"canUpload":true,"remainingDailyVideos":5}`}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /xrpc/com.atproto.repo.uploadBlob", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mime := r.Header.Get("Content-Type")
		h.mu.Lock()
		h.uploads = append(h.uploads, mime)
		gate, arrived := h.gate, h.arrived
		fail := h.failUploads > 0
		if fail {
			h.failUploads--
		}
		h.mu.Unlock()
		if gate != nil {
			arrived <- mime
			<-gate
		}
		if fail {
			http.Error(w, `{"error":"InternalServerError"}`, http.StatusInternalServerError)
			return
		}
		io.WriteString(w, `{"blob":`+blobJSON(cidOf(body), mime)+`}`)
	})
	mux.HandleFunc("GET /xrpc/com.atproto.server.getServiceAuth", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"token":"svc:`+r.URL.Query().Get("aud")+`:`+r.URL.Query().Get("lxm")+`"}`)
	})
	mux.HandleFunc("POST /xrpc/app.bsky.video.uploadVideo", func(w http.ResponseWriter, r *http.Request) {
		host := strings.TrimPrefix(h.srv.URL, "http://")
		if r.Header.Get("Authorization") != "Bearer svc:did:web:"+strings.ReplaceAll(host, ":", "%3A")+":com.atproto.repo.uploadBlob" {
			http.Error(w, `{"error":"AuthRequired"}`, http.StatusUnauthorized)
			return
		}
		h.mu.Lock()
		h.uploads = append(h.uploads, r.Header.Get("Content-Type"))
		h.mu.Unlock()
		// Bare, as the service has answered uploads, not wrapped as the
		// lexicon says.
		io.WriteString(w, `{"jobId":"job1","state":"JOB_STATE_CREATED"}`)
	})
	mux.HandleFunc("GET /xrpc/app.bsky.video.getJobStatus", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"jobStatus":{"jobId":"job1","state":"JOB_STATE_COMPLETED","blob":`+blobJSON(cidOf([]byte("mp4")), "video/mp4")+`}}`)
	})
	mux.HandleFunc("GET /xrpc/app.bsky.video.getUploadLimits", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer svc:"+videoServiceDID+":app.bsky.video.getUploadLimits" {
			http.Error(w, `{"error":"AuthRequired"}`, http.StatusUnauthorized)
			return
		}
		io.WriteString(w, h.limits)
	})
	mux.HandleFunc("POST /xrpc/com.atproto.repo.createRecord", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Repo, Collection, Rkey string
			Record                 json.RawMessage
		}
		json.NewDecoder(r.Body).Decode(&in)
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.missingBlobs > 0 {
			h.missingBlobs--
			http.Error(w, `{"error":"BlobNotFound","message":"Could not find blob: bafy"}`, http.StatusBadRequest)
			return
		}
		if _, ok := h.records[in.Rkey]; ok {
			http.Error(w, `{"error":"InvalidRequest","message":"record already exists"}`, http.StatusBadRequest)
			return
		}
		h.records[in.Rkey] = in.Record
		if h.failPosts > 0 {
			h.failPosts--
			http.Error(w, `{"error":"InternalServerError"}`, http.StatusInternalServerError)
			return
		}
		io.WriteString(w, `{"uri":"at://`+in.Repo+`/`+in.Collection+`/`+in.Rkey+`","cid":"cid-`+in.Rkey+`"}`)
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		h.mu.Lock()
		h.refreshes = append(h.refreshes, r.PostForm)
		n := len(h.refreshes)
		h.mu.Unlock()
		// The first request has no nonce: ask for one, as servers do.
		w.Header().Set("DPoP-Nonce", "nonce1")
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"use_dpop_nonce"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"sub":"did:plc:me","scope":"atproto","token_type":"DPoP","access_token":"at2","refresh_token":"rt2"}`)
	})
	mux.HandleFunc("GET /xrpc/com.atproto.repo.getRecord", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		h.mu.Lock()
		_, ok := h.records[q.Get("rkey")]
		h.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"RecordNotFound"}`, http.StatusBadRequest)
			return
		}
		io.WriteString(w, `{"uri":"at://`+q.Get("repo")+`/`+q.Get("collection")+`/`+q.Get("rkey")+`","cid":"cid-`+q.Get("rkey")+`"}`)
	})
	h.srv = httptest.NewServer(mux)
	t.Cleanup(h.srv.Close)
	return h
}

// session is a session signed in to h, as signIn would leave it.
func (h *fakeHost) session() *session {
	did := syntax.DID("did:plc:me")
	return &session{
		pub:    &Publisher{Langs: []string{"en"}, dir: identity.NewMockDirectory()},
		api:    &atclient.APIClient{Client: h.srv.Client(), Host: h.srv.URL, AccountDID: &did},
		did:    did,
		pds:    h.srv.URL,
		scopes: scopes,
		video:  h.srv.URL,
	}
}

// open opens posts on s, as starting to post does.
func open(t *testing.T, s *session, posts ...decker.Post) *thread {
	t.Helper()
	th, err := s.Open(context.Background(), posts)
	if err != nil {
		t.Fatal(err)
	}
	return th.(*thread)
}

// first is a Delivery for a thread's first post, first tried now.
func first(key string) decker.Delivery {
	return decker.Delivery{Key: key, First: time.Now()}
}

func TestPostThread(t *testing.T) {
	pollEvery = 0
	h := newFakeHost(t)
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(clip, []byte("mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	posts := []decker.Post{
		{Slide: -1, Text: "Live from the talk 🧵 #OTel"},
		{Slide: 0, Text: "Slide one", Media: []decker.Media{
			{Path: writePNG(t, dir, "b1.png", 16, false), Alt: "build 1", W: 1920, H: 1080},
			{Path: writePNG(t, dir, "b2.png", 16, false), Alt: "build 2", W: 1920, H: 1080},
		}},
		{Slide: 1, Text: "Moving", Media: []decker.Media{{Path: clip, Video: true, Alt: "a clip", W: 1280, H: 720}}},
	}
	ctx := context.Background()
	th := open(t, h.session(), posts...)
	var refs []string
	for i, post := range posts {
		d := first(fmt.Sprint(i))
		if len(refs) > 0 {
			d.Root, d.Parent = refs[0], refs[len(refs)-1]
		}
		ref, err := th.Post(ctx, post, d)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	if !slices.Equal(h.uploads, []string{"image/png", "image/png", "video/mp4"}) {
		t.Errorf("uploads: %q", h.uploads)
	}
	type strongRef struct{ URI, CID string }
	type record struct {
		Type   string `json:"$type"`
		Text   string
		Langs  []string
		Facets []json.RawMessage
		Reply  *struct{ Root, Parent strongRef }
		Embed  *struct {
			Type   string `json:"$type"`
			Images []struct {
				Alt         string
				AspectRatio struct{ Width, Height int }
				Image       json.RawMessage
			}
			Video        json.RawMessage
			Alt          string
			Presentation string
			AspectRatio  struct{ Width, Height int }
		}
	}
	read := func(ref string) record {
		uri, _, _ := strings.Cut(ref, " ")
		var r record
		if err := json.Unmarshal(h.records[uri[strings.LastIndex(uri, "/")+1:]], &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	opener := read(refs[0])
	if opener.Type != "app.bsky.feed.post" || opener.Reply != nil || opener.Embed != nil ||
		!slices.Equal(opener.Langs, []string{"en"}) || len(opener.Facets) != 1 {
		t.Errorf("opener: %+v", opener)
	}
	uri0, cid0, _ := strings.Cut(refs[0], " ")
	uri1, cid1, _ := strings.Cut(refs[1], " ")
	one := read(refs[1])
	if one.Reply == nil || one.Reply.Root != (strongRef{uri0, cid0}) || one.Reply.Parent != (strongRef{uri0, cid0}) {
		t.Errorf("slide one's reply: %+v", one.Reply)
	}
	if e := one.Embed; e == nil || e.Type != "app.bsky.embed.images" || len(e.Images) != 2 ||
		e.Images[1].Alt != "build 2" || e.Images[0].AspectRatio.Width != 1920 || len(e.Images[0].Image) == 0 {
		t.Errorf("slide one's embed: %+v", one.Embed)
	}
	two := read(refs[2])
	if two.Reply == nil || two.Reply.Root != (strongRef{uri0, cid0}) || two.Reply.Parent != (strongRef{uri1, cid1}) {
		t.Errorf("slide two's reply: %+v", two.Reply)
	}
	if e := two.Embed; e == nil || e.Type != "app.bsky.embed.video" || e.Presentation != "gif" ||
		e.Alt != "a clip" || e.AspectRatio.Height != 720 || !strings.Contains(string(e.Video), "video/mp4") {
		t.Errorf("slide two's embed: %+v", two.Embed)
	}
}

// TestPostRetry loses the answer to a post that went through: the retry
// finds it rather than posting it again, and doesn't upload its media
// again. So does a retry from a session and thread of their own, as after
// a restart, which never saw the first try.
func TestPostRetry(t *testing.T) {
	h := newFakeHost(t)
	h.failPosts = 1
	post := decker.Post{Slide: 0, Text: "once", Media: []decker.Media{{Path: writePNG(t, t.TempDir(), "a.png", 16, false), W: 16, H: 16}}}
	ctx := context.Background()
	th := open(t, h.session(), post)
	d := first("t1/a")
	if _, err := th.Post(ctx, post, d); err == nil {
		t.Fatal("a failed post: no error")
	}
	d.Retry = true
	ref, err := th.Post(ctx, post, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 1 || len(h.uploads) != 1 || ref != "at://did:plc:me/app.bsky.feed.post/"+recordKey(d).String()+" cid-"+recordKey(d).String() {
		t.Errorf("%d records, %d uploads, ref %q", len(h.records), len(h.uploads), ref)
	}
	again, err := open(t, h.session(), post).Post(ctx, post, d)
	if err != nil || again != ref || len(h.records) != 1 || len(h.uploads) != 1 {
		t.Errorf("after a restart: ref %q, %v; %d records, %d uploads", again, err, len(h.records), len(h.uploads))
	}

	// The same text, first tried at the same moment, is another post under
	// another key.
	other := d
	other.Key, other.Retry = "t1/b", false
	if _, err := th.Post(ctx, post, other); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 2 || recordKey(other) == recordKey(d) {
		t.Errorf("another key: %d records, record keys %s and %s", len(h.records), recordKey(other), recordKey(d))
	}
}

func TestRecordKey(t *testing.T) {
	at := time.Date(2026, 10, 16, 9, 30, 0, 123456789, time.UTC)
	d := decker.Delivery{Key: "t1/a", First: at}
	k := recordKey(d)
	if _, err := syntax.ParseTID(k.String()); err != nil {
		t.Fatal(err)
	}
	if !k.Time().Equal(at.Truncate(time.Microsecond)) {
		t.Errorf("record key time %v, want %v", k.Time(), at)
	}
	reloaded := d
	reloaded.First = at.Local()
	if recordKey(reloaded) != k {
		t.Error("the same Delivery in another zone made another record key")
	}
}

// TestCheckRejects checks what Check turns away, signed in or not, and
// TestOpenRejects what only a signed-in account can say.
func TestCheckRejects(t *testing.T) {
	for name, tc := range map[string]struct {
		langs []string
		posts []decker.Post
		want  string
	}{
		"too many langs": {[]string{"en", "de", "fr", "ja"}, nil, "4 Langs"},
		"long post": {nil, []decker.Post{{Slide: 4, Text: strings.Repeat("x", 301)}},
			`slide 5's post "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx…": 301 characters`},
		"unknown mention": {nil, []decker.Post{{Slide: -1, Text: "with @nobody.example"}},
			"the opening post (\"with @nobody.example\") mentions @nobody.example, which doesn't resolve"},
	} {
		p := &Publisher{Langs: tc.langs, dir: identity.NewMockDirectory()}
		err := p.Check(context.Background(), tc.posts)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
}

func TestOpenRejects(t *testing.T) {
	h := newFakeHost(t)
	clip := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(clip, []byte("mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	withClip := []decker.Post{{Slide: 2, Text: "a clip", Media: []decker.Media{{Path: clip, Video: true}}}}
	for name, tc := range map[string]struct {
		setup func(*session)
		posts []decker.Post
		want  string
	}{
		"a Check failure": {nil, []decker.Post{{Slide: 4, Text: strings.Repeat("x", 301)}}, "301 characters"},
		"no video scope":  {func(s *session) { s.scopes = scopes[:4] }, withClip, "didn't grant \"rpc:com.atproto.repo.uploadBlob?aud=*\""},
		"unverified": {func(*session) { h.limits = `{"canUpload":false,"message":"verify your email"}` }, withClip,
			"can't upload video now: verify your email"},
		"over the limit": {func(*session) { h.limits = `{"canUpload":true,"remainingDailyVideos":0}` }, withClip,
			"1 clips, but the account can upload 0 more videos today"},
	} {
		s := h.session()
		if tc.setup != nil {
			tc.setup(s)
		}
		_, err := s.Open(context.Background(), tc.posts)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
	// Check alone doesn't ask the account, so the clip passes it.
	if err := h.session().pub.Check(context.Background(), withClip); err != nil {
		t.Errorf("Check: %v", err)
	}
}

func TestPostRejectsAForeignRef(t *testing.T) {
	th := open(t, newFakeHost(t).session())
	_, err := th.Post(context.Background(), decker.Post{Text: "x"}, decker.Delivery{Root: "dry:1", Parent: "dry:1", Key: "k", First: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "dry run") {
		t.Errorf("got %v", err)
	}
}

func TestAwaitCallback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var got url.Values
	errc := make(chan error, 1)
	go func() {
		errc <- awaitCallback(context.Background(), ln, func(q url.Values) error { got = q; return nil })
	}()
	base := "http://" + ln.Addr().String()
	if resp, err := http.Get(base + "/favicon.ico"); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("another path: %v %v", resp, err)
	}
	resp, err := http.Get(base + "/callback?state=s&code=c&iss=x")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Signed in") {
		t.Errorf("%d %s", resp.StatusCode, body)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if got.Get("code") != "c" {
		t.Errorf("params: %v", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := awaitCallback(ctx, ln2, func(url.Values) error { return nil }); err == nil {
		t.Error("no redirect: no error")
	}
}

func TestGallery(t *testing.T) {
	h := newFakeHost(t)
	img := decker.Media{Path: writePNG(t, t.TempDir(), "a.png", 16, false), Alt: "a", W: 16, H: 9}
	six := decker.Post{Slide: 0, Text: "six", Media: slices.Repeat([]decker.Media{img}, 6)}
	ctx := context.Background()
	ref, err := open(t, h.session(), six).Post(ctx, six, first("six"))
	if err != nil {
		t.Fatal(err)
	}
	uri, _, _ := strings.Cut(ref, " ")
	var rec struct {
		Embed struct {
			Type  string `json:"$type"`
			Items []struct {
				Type        string `json:"$type"`
				Alt         string
				Image       json.RawMessage
				AspectRatio struct{ Width, Height int }
			}
		}
	}
	if err := json.Unmarshal(h.records[uri[strings.LastIndex(uri, "/")+1:]], &rec); err != nil {
		t.Fatal(err)
	}
	e := rec.Embed
	if e.Type != "app.bsky.embed.gallery" || len(e.Items) != 6 || e.Items[5].Type != "app.bsky.embed.gallery#image" ||
		e.Items[0].Alt != "a" || e.Items[0].AspectRatio.Height != 9 || len(e.Items[0].Image) == 0 {
		t.Errorf("embed: %+v", e)
	}
	// The blob is uploaded once, though six items use it.
	if len(h.uploads) != 1 {
		t.Errorf("%d uploads", len(h.uploads))
	}
	eleven := decker.Post{Slide: 0, Text: "eleven", Media: slices.Repeat([]decker.Media{img}, 11)}
	if _, err := h.session().Open(ctx, []decker.Post{eleven}); err == nil || !strings.Contains(err.Error(), "11 images, but a post takes up to 10") {
		t.Errorf("eleven images: %v", err)
	}
}
