package bsky

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

// The scopes a session asks for: posting, with images and clips, and no
// more. Clips need the last two, for the video service to act for the
// account; their audience is a wildcard because the one uploadBlob goes to
// is the account's own host, which differs from account to account.
const (
	scopeVideoUpload = "rpc:com.atproto.repo.uploadBlob?aud=*"
	scopeVideoLimits = "rpc:app.bsky.video.getUploadLimits?aud=*"
)

var scopes = []string{
	"atproto",
	"repo:app.bsky.feed.post?action=create",
	"blob:image/*",
	"blob:video/*",
	scopeVideoUpload,
	scopeVideoLimits,
}

// signInTimeout is how long signing in waits for the browser.
const signInTimeout = 10 * time.Minute

// clientRedirect is the redirect a loopback client declares in its
// client_id. The server ignores a loopback redirect's port, so the client_id
// stays the same from run to run while the callback takes whatever port is
// free, and a kept session refreshes under the client_id it was issued to.
const clientRedirect = "http://127.0.0.1/callback"

// newApp is an OAuth client whose store keeps account's sessions in the
// keychain, sending the browser back to callback.
func (p *Publisher) newApp(account, callback string) (*oauth.ClientApp, *keychainStore) {
	cfg := oauth.NewLocalhostConfig(clientRedirect, scopes)
	cfg.CallbackURL = callback
	cfg.UserAgent = "decker"
	store := newKeychainStore(account)
	app := oauth.NewClientApp(&cfg, store)
	if p.client != nil {
		app.Client = p.client
	}
	return app, store
}

// signIn signs in to the account with atproto OAuth, as a loopback client:
// the account's host sends the browser back to a server on 127.0.0.1 that
// lives only until it does. A loopback client needs no client metadata
// hosted anywhere. visit gets the page the speaker approves it on.
func (p *Publisher) signIn(ctx context.Context, account string, visit func(string)) (*session, *keychainStore, error) {
	ctx, cancel := context.WithTimeout(ctx, signInTimeout)
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer ln.Close()
	app, store := p.newApp(account, fmt.Sprintf("http://%s/callback", ln.Addr()))
	authURL, err := app.StartAuthFlow(ctx, account)
	if err != nil {
		return nil, nil, err
	}
	visit(authURL)
	var s *session
	err = awaitCallback(ctx, ln, func(q url.Values) error {
		data, err := app.ProcessCallback(ctx, q)
		if err != nil {
			return err
		}
		sess, err := app.ResumeSession(ctx, data.AccountDID, data.SessionID)
		if err != nil {
			return err
		}
		s = p.adopt(sess)
		return nil
	})
	return s, store, err
}

// awaitCallback serves the OAuth redirect on ln: the first request to
// /callback is handed to finish, and the browser told how it went. It
// returns finish's answer, or ctx's error if no redirect comes.
func awaitCallback(ctx context.Context, ln net.Listener, finish func(url.Values) error) error {
	done := make(chan error, 1)
	var once sync.Once
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		ran := false
		var err error
		once.Do(func() { ran, err = true, finish(r.URL.Query()) })
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch {
		case !ran:
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, page("Already signed in", "This sign-in was already used. Go back to the terminal."))
			return
		case err != nil:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, page("Sign-in failed", err.Error()))
		default:
			fmt.Fprint(w, page("Signed in", "decker can post the talk. Go back to the terminal; you can close this tab."))
		}
		done <- err
	})}
	go srv.Serve(ln)
	defer srv.Close()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("no answer from the browser: %w", ctx.Err())
	}
}

func page(title, msg string) string {
	return fmt.Sprintf("<!doctype html><meta charset=utf-8><title>decker: %[1]s</title>"+
		"<body style=\"font:16px system-ui;margin:4em auto;max-width:32em\"><h1>%[1]s</h1><p>%[2]s</p>",
		html.EscapeString(title), html.EscapeString(msg))
}
