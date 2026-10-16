package bsky

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // the rendered images fitImage decodes
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/atclient"
	lexutil "github.com/bluesky-social/indigo/lex/util"
)

// pollEvery is how often a clip's processing is checked on.
var pollEvery = time.Second

func fileSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// fitImage returns path if it is at most limit bytes, or else a JPEG of it
// that is, written beside it. A slide's PNG is usually well under the limit;
// one full of gradients or photographs may not be.
func fitImage(path string, limit int64) (string, error) {
	size, err := fileSize(path)
	if err != nil || size <= limit {
		return path, err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	var buf bytes.Buffer
	for q := 90; q >= 50; q -= 10 {
		buf.Reset()
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return "", err
		}
		if int64(buf.Len()) <= limit {
			small := strings.TrimSuffix(path, filepath.Ext(path)) + ".jpg"
			return small, os.WriteFile(small, buf.Bytes(), 0o644)
		}
	}
	return "", fmt.Errorf("%s is %d bytes, and even as a JPEG over the %d an image can be", path, size, limit)
}

// uploadImage uploads an image to the account's host.
func (s *session) uploadImage(ctx context.Context, path string) (*lexutil.LexBlob, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mime := "image/png"
	if strings.EqualFold(filepath.Ext(path), ".jpg") {
		mime = "image/jpeg"
	}
	// Not comatproto.RepoUploadBlob: it sends every blob as */*.
	var out comatproto.RepoUploadBlob_Output
	err = s.api.LexDo(ctx, lexutil.Procedure, mime, "com.atproto.repo.uploadBlob", nil, bytes.NewReader(b), &out)
	if err != nil {
		return nil, err
	}
	if out.Blob == nil {
		return nil, errors.New("the account's host answered without a blob")
	}
	return out.Blob, nil
}

// uploadVideo sends a clip to the video service, which processes it and
// stores it on the account's host, and waits for the blob, so the post plays
// as soon as it appears. The service acts for the account with a token from
// its host, scoped to uploading a blob there.
//
// Neither call goes through appbsky's: the service has answered an upload
// with its job bare, not wrapped as the lexicon says, and a failure can
// carry the job, which readJob looks for.
func (s *session) uploadVideo(ctx context.Context, path string) (*lexutil.LexBlob, error) {
	u, err := url.Parse(s.pds)
	if err != nil {
		return nil, err
	}
	// did:web escapes a port's colon.
	token, err := s.serviceAuth(ctx, "did:web:"+strings.ReplaceAll(u.Host, ":", "%3A"),
		"com.atproto.repo.uploadBlob", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	q := url.Values{"did": {s.did.String()}, "name": {filepath.Base(path)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.video+"/xrpc/app.bsky.video.uploadVideo?"+q.Encode(), f)
	if err != nil {
		return nil, err
	}
	req.ContentLength = st.Size()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "video/mp4")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	job, err := readJob(resp)
	if err != nil {
		return nil, err
	}
	for {
		// A clip the service has processed before comes back as an error
		// carrying the earlier blob, so the blob is looked for first.
		switch {
		case job.Blob != nil:
			return job.Blob, nil
		case job.State == "JOB_STATE_FAILED" || job.JobId == "":
			return nil, fmt.Errorf("the video service couldn't process it: %s", cmp.Or(cmpJoin(deref(job.Error), deref(job.Message)), job.State))
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the video service is still processing it (%s): %w", job.State, ctx.Err())
		case <-time.After(pollEvery):
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			s.video+"/xrpc/app.bsky.video.getJobStatus?"+url.Values{"jobId": {job.JobId}}.Encode(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.httpClient().Do(req)
		if err != nil {
			return nil, err
		}
		if job, err = readJob(resp); err != nil {
			return nil, err
		}
	}
}

// readJob reads a job status from the video service, whether it comes
// wrapped as the lexicon says ({"jobStatus": ...}) or bare, as the service
// has answered an upload, and whether the call failed or not: a failure can
// carry the job too.
func readJob(resp *http.Response) (appbsky.VideoDefs_JobStatus, error) {
	defer resp.Body.Close()
	var body struct {
		appbsky.VideoDefs_JobStatus
		Wrapped *appbsky.VideoDefs_JobStatus `json:"jobStatus"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return appbsky.VideoDefs_JobStatus{}, fmt.Errorf("the video service (HTTP %d): %w", resp.StatusCode, err)
	}
	job := body.VideoDefs_JobStatus
	if body.Wrapped != nil {
		job = *body.Wrapped
	}
	if resp.StatusCode >= 300 && job.JobId == "" && job.Blob == nil {
		return job, fmt.Errorf("the video service (HTTP %d): %s", resp.StatusCode, cmpJoin(deref(job.Error), deref(job.Message)))
	}
	return job, nil
}

func cmpJoin(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + ": " + b
}

// deref is an optional string of a lexicon type, "" if it's absent.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// checkVideoLimits asks the video service whether the account may upload
// videos clips of bytes in all today: an account needs a verified email,
// and has a daily allowance.
func (s *session) checkVideoLimits(ctx context.Context, videos int, bytes int64) error {
	token, err := s.serviceAuth(ctx, videoServiceDID, "app.bsky.video.getUploadLimits", 5*time.Minute)
	if err != nil {
		return err
	}
	svc := &atclient.APIClient{Client: s.httpClient(), Host: s.video,
		Headers: http.Header{"Authorization": {"Bearer " + token}}}
	lim, err := appbsky.VideoGetUploadLimits(ctx, svc)
	if err != nil {
		return fmt.Errorf("checking video upload limits: %w", err)
	}
	switch {
	case !lim.CanUpload:
		return fmt.Errorf("the account can't upload video now: %s", cmpJoin(deref(lim.Error), deref(lim.Message)))
	case lim.RemainingDailyVideos != nil && int64(videos) > *lim.RemainingDailyVideos:
		return fmt.Errorf("the talk has %d clips, but the account can upload %d more videos today", videos, *lim.RemainingDailyVideos)
	case lim.RemainingDailyBytes != nil && bytes > *lim.RemainingDailyBytes:
		return fmt.Errorf("the talk's clips are %d bytes, but the account can upload %d more today", bytes, *lim.RemainingDailyBytes)
	}
	return nil
}

// serviceAuth gets a token from the account's host for another service to
// act for it: on aud, for the one method lxm, until ttl from now.
func (s *session) serviceAuth(ctx context.Context, aud, lxm string, ttl time.Duration) (string, error) {
	out, err := comatproto.ServerGetServiceAuth(ctx, s.api, aud, time.Now().Add(ttl).Unix(), lxm)
	if err != nil {
		return "", fmt.Errorf("getting a token for %s: %w", lxm, err)
	}
	return out.Token, nil
}

func (s *session) httpClient() *http.Client {
	if s.api.Client != nil {
		return s.api.Client
	}
	return http.DefaultClient
}
