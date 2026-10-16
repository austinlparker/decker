package bsky

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/rivo/uniseg"
)

// A post's text is plain; facets mark the spans that link somewhere, by
// UTF-8 byte offsets. These find what Bluesky's own apps link: http(s)
// URLs, @handles and #hashtags, each starting a word. A bare domain isn't a
// link here: "main.go" is more often a file than a site.
var (
	linkRe    = regexp.MustCompile(`(?:^|[\s(])(https?://\S+)`)
	mentionRe = regexp.MustCompile(`(?:^|[\s(])(@[a-zA-Z0-9.-]+)`)
	tagRe     = regexp.MustCompile(`(?:^|\s)([#＃]\S+)`)
)

// maxTagGraphemes is the longest hashtag a facet takes.
const maxTagGraphemes = 64

// facets are text's links, mentions and hashtags, in order. A mention links
// only if mentions resolves its handle.
func facets(text string, mentions map[syntax.Handle]syntax.DID) []*appbsky.RichtextFacet {
	var out []*appbsky.RichtextFacet
	add := func(start, end int, feature *appbsky.RichtextFacet_Features_Elem) {
		out = append(out, &appbsky.RichtextFacet{
			Index:    &appbsky.RichtextFacet_ByteSlice{ByteStart: int64(start), ByteEnd: int64(end)},
			Features: []*appbsky.RichtextFacet_Features_Elem{feature},
		})
	}
	for _, m := range linkRe.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[2], m[2]+len(trimLink(text[m[2]:m[3]]))
		add(start, end, &appbsky.RichtextFacet_Features_Elem{
			RichtextFacet_Link: &appbsky.RichtextFacet_Link{Uri: text[start:end]}})
	}
	for _, m := range mentionRe.FindAllStringSubmatchIndex(text, -1) {
		h, end := mentionAt(text, m[2], m[3])
		if did, ok := mentions[h]; ok && h != "" {
			add(m[2], end, &appbsky.RichtextFacet_Features_Elem{
				RichtextFacet_Mention: &appbsky.RichtextFacet_Mention{Did: did.String()}})
		}
	}
	for _, m := range tagRe.FindAllStringSubmatchIndex(text, -1) {
		start := m[2]
		_, hash := utf8.DecodeRuneInString(text[start:])
		tag := strings.TrimRightFunc(text[start+hash:m[3]], unicode.IsPunct)
		if tag == "" || strings.TrimFunc(tag, unicode.IsDigit) == "" || uniseg.GraphemeClusterCount(tag) > maxTagGraphemes {
			continue
		}
		add(start, start+hash+len(tag), &appbsky.RichtextFacet_Features_Elem{
			RichtextFacet_Tag: &appbsky.RichtextFacet_Tag{Tag: tag}})
	}
	slices.SortFunc(out, func(a, b *appbsky.RichtextFacet) int { return cmp.Compare(a.Index.ByteStart, b.Index.ByteStart) })
	return out
}

// trimLink drops the punctuation that ends a sentence around a URL, and a
// closing parenthesis it doesn't open, as in "(see https://x.example)".
func trimLink(u string) string {
	for {
		t := strings.TrimRight(u, `.,;:!?'"`)
		if strings.HasSuffix(t, ")") && strings.Count(t, "(") < strings.Count(t, ")") {
			t = t[:len(t)-1]
		}
		if t == u {
			return u
		}
		u = t
	}
}

// mentionAt is the handle an @mention matched at text[start:end] names,
// and where it ends, without a trailing dot or hyphen ("thanks @me.bsky.social.");
// "" if it isn't a handle, like "@home".
func mentionAt(text string, start, end int) (syntax.Handle, int) {
	raw := strings.TrimRight(text[start+1:end], ".-")
	h, err := syntax.ParseHandle(raw)
	if err != nil {
		return "", 0
	}
	return h.Normalize(), start + 1 + len(raw)
}

// mentionHandles are the handles text mentions, for Check to resolve.
func mentionHandles(text string) []syntax.Handle {
	var hs []syntax.Handle
	for _, m := range mentionRe.FindAllStringSubmatchIndex(text, -1) {
		if h, _ := mentionAt(text, m[2], m[3]); h != "" && !slices.Contains(hs, h) {
			hs = append(hs, h)
		}
	}
	return hs
}
