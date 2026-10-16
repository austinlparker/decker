package bsky

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/austinlparker/decker"
	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/zalando/go-keyring"
)

// keptFor is a session h issued, as the keychain would hold it.
func keptFor(t *testing.T, h *fakeHost) oauth.ClientSessionData {
	key, err := atcrypto.GeneratePrivateKeyP256()
	if err != nil {
		t.Fatal(err)
	}
	return oauth.ClientSessionData{
		AccountDID:              "did:plc:me",
		SessionID:               "s1",
		HostURL:                 h.srv.URL,
		AuthServerURL:           h.srv.URL,
		AuthServerTokenEndpoint: h.srv.URL + "/oauth/token",
		Scopes:                  scopes,
		AccessToken:             "at1",
		RefreshToken:            "rt1",
		DPoPAuthServerNonce:     "old",
		DPoPHostNonce:           "old",
		DPoPPrivateKeyMultibase: key.Multibase(),
	}
}

func readKept(t *testing.T, account string) keptSession {
	raw, err := keyring.Get(keyringService, account)
	if err != nil {
		t.Fatal(err)
	}
	var ks keptSession
	if err := json.Unmarshal([]byte(raw), &ks); err != nil {
		t.Fatal(err)
	}
	return ks
}

func TestKeychainStore(t *testing.T) {
	keyring.MockInit()
	h := newFakeHost(t)
	s := newKeychainStore("@Me.bsky.social")
	d := keptFor(t, h)
	if err := s.SaveSession(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	ks := readKept(t, "me.bsky.social")
	if got := ks.Session; got.RefreshToken != "rt1" || got.AccessToken != "" || got.DPoPHostNonce != "" ||
		got.DPoPPrivateKeyMultibase != d.DPoPPrivateKeyMultibase || len(ks.Asked) != len(scopes) {
		t.Errorf("kept: %+v", ks)
	}
	if m, err := s.GetSession(context.Background(), d.AccountDID, d.SessionID); err != nil || m.AccessToken != "at1" {
		t.Errorf("in memory: %+v, %v", m, err)
	}

	keyring.MockInitWithError(errors.New("locked"))
	d.RefreshToken = "rt2"
	if err := s.SaveSession(context.Background(), d); err != nil || s.err == nil {
		t.Errorf("a keychain that can't be written: %v, store err %v", err, s.err)
	}
}

// TestResume signs in with a kept session: it refreshes, under the same client_id
// whatever port signing in used, keeps the new single-use refresh token, and
// posts with it.
func TestResume(t *testing.T) {
	keyring.MockInit()
	h := newFakeHost(t)
	s := newKeychainStore("me.bsky.social")
	if err := s.SaveSession(context.Background(), keptFor(t, h)); err != nil {
		t.Fatal(err)
	}
	p := &Publisher{video: h.srv.URL, client: h.srv.Client()}
	ctx := context.Background()
	if err := p.SignIn(ctx, "@Me.bsky.social", func(u string) { t.Errorf("a kept session sent the speaker to %s", u) }); err != nil {
		t.Fatal(err)
	}
	if len(h.refreshes) != 2 {
		t.Fatalf("%d token requests, want a nonce and a refresh", len(h.refreshes))
	}
	form := h.refreshes[1]
	clientID := "http://localhost?" + url.Values{"redirect_uri": {clientRedirect}, "scope": {strings.Join(scopes, " ")}}.Encode()
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "rt1" || form.Get("client_id") != clientID {
		t.Errorf("refresh: %v", form)
	}
	if ks := readKept(t, "me.bsky.social"); ks.Session.RefreshToken != "rt2" {
		t.Errorf("kept refresh token %q, want the new one", ks.Session.RefreshToken)
	}
	if p.sess.api.Host != h.srv.URL || p.sess.did != "did:plc:me" {
		t.Errorf("api %s, did %s", p.sess.api.Host, p.sess.did)
	}
	if _, err := p.Post(ctx, decker.Post{Text: "resumed"}, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 1 {
		t.Errorf("%d records", len(h.records))
	}
}

func TestResumeRejects(t *testing.T) {
	keyring.MockInit()
	h := newFakeHost(t)
	p := &Publisher{}
	if _, _, err := p.resume(context.Background(), "me.bsky.social"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("nothing kept: %v", err)
	}
	b, _ := json.Marshal(keptSession{Asked: []string{"atproto"}, Session: keptFor(t, h)})
	keyring.Set(keyringService, "me.bsky.social", string(b))
	if _, _, err := p.resume(context.Background(), "me.bsky.social"); err == nil || !strings.Contains(err.Error(), "different permissions") {
		t.Errorf("kept with other scopes: %v", err)
	}
}

func TestKeyringAccount(t *testing.T) {
	for in, want := range map[string]string{
		"me.bsky.social":       "me.bsky.social",
		"@Me.Bsky.Social":      "me.bsky.social",
		"did:plc:AbC123":       "did:plc:AbC123",
		"https://Bsky.Social/": "https://bsky.social/",
	} {
		if got := keyringAccount(in); got != want {
			t.Errorf("keyringAccount(%q) = %q, want %q", in, got, want)
		}
	}
}
