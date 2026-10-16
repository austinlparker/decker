package bsky

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/austinlparker/decker"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/zalando/go-keyring"
)

// keyringService names the sessions kept in the system keychain (the macOS
// Keychain, the Secret Service on Linux, the Windows Credential Manager),
// one item per account.
const keyringService = "decker-bsky"

// keptSession is a session as the keychain holds it, with the scopes it
// asked for: a session from before decker asked for more signs in again.
// The access token and DPoP nonces are left out: they last minutes, the
// refresh on resume replaces them, and leaving them out keeps the item
// under what the macOS keychain's command line takes.
type keptSession struct {
	Asked   []string                `json:"asked"`
	Session oauth.ClientSessionData `json:"session"`
}

func keep(d oauth.ClientSessionData) ([]byte, error) {
	d.AccessToken, d.DPoPAuthServerNonce, d.DPoPHostNonce = "", "", ""
	return json.Marshal(keptSession{Asked: scopes, Session: d})
}

// keyringAccount is the item an account's session is kept under: handles
// are case-insensitive. An account that's neither a handle nor a DID, such
// as a host's address, is lowercased as it is.
func keyringAccount(account string) string {
	account = strings.TrimPrefix(account, "@")
	if id, err := syntax.ParseAtIdentifier(account); err == nil {
		return id.Normalize().String()
	}
	return strings.ToLower(account)
}

// keychainStore is the OAuth client's store: memory, with each session
// written through to the keychain. A refresh token is good for one
// refresh, so the one each refresh returns, mid-talk included, must be
// kept for the next run to use. A session calls SaveSession under a lock
// of its own, so kept and err need none here.
type keychainStore struct {
	*oauth.MemStore
	account string
	kept    []byte // what the keychain holds, to skip writing it again on a nonce change
	err     error  // the last write that failed
}

func newKeychainStore(account string) *keychainStore {
	return &keychainStore{MemStore: oauth.NewMemStore(), account: keyringAccount(account)}
}

// SaveSession keeps d in memory and the keychain. A keychain that can't be
// written isn't an error here: the session works for this run, and
// SignIn warns the next run signs in again.
func (s *keychainStore) SaveSession(ctx context.Context, d oauth.ClientSessionData) error {
	if err := s.MemStore.SaveSession(ctx, d); err != nil {
		return err
	}
	b, err := keep(d)
	if err == nil && bytes.Equal(b, s.kept) {
		return nil
	}
	if err == nil {
		err = keyring.Set(keyringService, s.account, string(b))
	}
	if err != nil {
		s.err = err
		return nil
	}
	s.kept, s.err = b, nil
	return nil
}

// SignIn signs in as account: it picks up the session the keychain keeps
// for it, refreshed, or else has the speaker approve a new one at the page
// it gives visit. A session that can't be kept in the keychain still works
// for this run; the log says the next run signs in again.
func (p *Publisher) SignIn(ctx context.Context, account string, visit func(url string)) (decker.Session, error) {
	account = strings.TrimPrefix(strings.TrimSpace(account), "@")
	if account == "" {
		return nil, errors.New("bsky: no account to sign in to")
	}
	sess, store, err := p.resume(ctx, account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		slog.Info("bsky: the kept session can't be used; signing in again", "account", account, "err", err)
	}
	if err != nil {
		if sess, store, err = p.signIn(ctx, account, visit); err != nil {
			return nil, fmt.Errorf("bsky: signing in as %s: %w", account, err)
		}
	}
	if store.err != nil {
		slog.Warn("bsky: couldn't keep the session in the system keychain; the next run signs in again",
			"account", account, "err", store.err)
	}
	return sess, nil
}

// resume picks up the session kept for account and refreshes it, which
// checks it's still good and leaves a fresh access token for the talk.
func (p *Publisher) resume(ctx context.Context, account string) (*session, *keychainStore, error) {
	raw, err := keyring.Get(keyringService, keyringAccount(account))
	if err != nil {
		return nil, nil, err
	}
	var ks keptSession
	if err := json.Unmarshal([]byte(raw), &ks); err != nil {
		return nil, nil, fmt.Errorf("the kept session doesn't read: %w", err)
	}
	if !slices.Equal(ks.Asked, scopes) {
		return nil, nil, errors.New("decker asks for different permissions now")
	}
	app, store := p.newApp(account, clientRedirect)
	store.kept = []byte(raw)
	if err := store.MemStore.SaveSession(ctx, ks.Session); err != nil {
		return nil, nil, err
	}
	sess, err := app.ResumeSession(ctx, ks.Session.AccountDID, ks.Session.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := sess.RefreshTokens(ctx); err != nil {
		return nil, nil, err
	}
	return p.adopt(sess), store, nil
}

// adopt is the session that posts as sess's account.
func (p *Publisher) adopt(sess *oauth.ClientSession) *session {
	return &session{
		pub:    p,
		api:    sess.APIClient(),
		did:    sess.Data.AccountDID,
		pds:    sess.Data.HostURL,
		scopes: sess.Data.Scopes,
		video:  cmp.Or(p.video, videoService),
	}
}
