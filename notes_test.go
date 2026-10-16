package decker

import (
	"slices"
	"strings"
	"testing"
)

func TestParseNotes(t *testing.T) {
	slides := []Slide{{Title: "Intro"}, {Title: "C#"}, {Title: ""}, {Title: "Logs *and* traces"}}
	md := strings.Join([]string{
		"# The talk",
		"",
		"About the whole talk.",
		"",
		"## Intro",
		"",
		"Say hello.",
		"### A subheading",
		"    ## indented code, not a heading",
		"",
		"```yaml",
		"## a YAML comment, not a heading",
		"```",
		"",
		"## C#",
		"",
		"~~~~",
		"## still code",
		"~~~",
		"## still code: the closing fence is too short",
		"~~~~",
		"Last line.",
		"",
		"##",
		"## Logs *and* traces ##\r",
		"Both.\r",
	}, "\n")
	dn, err := parseNotes(md, slides)
	if err != nil {
		t.Fatal(err)
	}
	got := noteTexts(dn)
	want := []string{
		"Say hello.\n### A subheading\n    ## indented code, not a heading\n\n```yaml\n## a YAML comment, not a heading\n```",
		"~~~~\n## still code\n~~~\n## still code: the closing fence is too short\n~~~~\nLast line.",
		"",
		"Both.",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestParseNotesStopsEarly(t *testing.T) {
	slides := []Slide{{Title: "a"}, {Title: "b"}, {Title: "c"}}
	for _, md := range []string{"", "no sections, only a preamble", "## a\n\nfirst"} {
		dn, err := parseNotes(md, slides)
		if err != nil {
			t.Fatalf("%q: %v", md, err)
		}
		if got := noteTexts(dn); len(got) != len(slides) || got[1] != "" || got[2] != "" {
			t.Errorf("%q: got %q", md, noteTexts(dn))
		}
	}
}

func TestParseNotesRejects(t *testing.T) {
	slides := []Slide{{Title: "a"}, {Title: "b"}}
	for md, want := range map[string]string{
		"## a\n## c\n":         `line 2: section 2 is "c", but slide 2 is "b"`,
		"## b\n":               `line 1: section 1 is "b", but slide 1 is "a"`,
		"## a\n## b\n\n## c\n": `line 4: section 3, "c", is past the last slide (2)`,
		"## A\n":               `section 1 is "A", but slide 1 is "a"`,
	} {
		_, err := parseNotes(md, slides)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", md, err, want)
		}
	}
}

func TestDeckCheckNotes(t *testing.T) {
	d := testDeck()
	d.Notes = "## Bullets\n"
	if err := d.Check(); err == nil || !strings.Contains(err.Error(), "deck: Notes line 1") {
		t.Errorf("got %v", err)
	}
	if got := d.slideNotes(); len(got) != len(d.Slides) {
		t.Errorf("unparsable notes: got %d entries for %d slides", len(got), len(d.Slides))
	}
}

func noteTexts(dn deckNotes) []string {
	var texts []string
	for _, sn := range dn.slides {
		texts = append(texts, sn.text)
	}
	return texts
}

func TestParseNotesPosts(t *testing.T) {
	slides := []Slide{{Title: "a"}, {Title: "b", Steps: 6}, {Title: "c", Steps: 2}}
	md := "Before.\n\n```post\nWatch along 🧵\n```\n\n" +
		"## a\n\nSay it.\n\n```post\nThe point,\nin two lines.\n```\n```alt\nA headline.\n```\nAfter.\n\n" +
		"## b\n\n```post media=builds builds=2-4,6\nBuilds.\n```\n```post media=clip\nMoving.\n```\n" +
		"```go\n// a post block in code is code\n```\n\n" +
		"## c\n\n```post media=builds\nAll of them.\n```\n"
	dn, err := parseNotes(md, slides)
	if err != nil {
		t.Fatal(err)
	}
	if len(dn.opener) != 1 || dn.opener[0].text != "Watch along 🧵" || dn.opener[0].media != postNone {
		t.Errorf("opener: %+v", dn.opener)
	}
	a := dn.slides[0]
	if a.text != "Say it.\n\nAfter." || len(a.posts) != 1 {
		t.Fatalf("a: %+v", a)
	}
	if p := a.posts[0]; p.text != "The point,\nin two lines." || p.alt != "A headline." || p.media != postFinal || !slices.Equal(p.builds, []int{0}) {
		t.Errorf("a's post: %+v", p)
	}
	b := dn.slides[1]
	if len(b.posts) != 2 || !slices.Equal(b.posts[0].builds, []int{1, 2, 3, 5}) || b.posts[1].media != postClip {
		t.Errorf("b's posts: %+v", b.posts)
	}
	if !strings.Contains(b.text, "a post block in code") {
		t.Errorf("b's notes lost the code block: %q", b.text)
	}
	if c := dn.slides[2].posts[0]; c.media != postBuilds || !slices.Equal(c.builds, []int{0, 1}) {
		t.Errorf("c's post: %+v", c)
	}
}

func TestParseNotesRejectsPosts(t *testing.T) {
	slides := []Slide{{Title: "a", Steps: 2}}
	for md, want := range map[string]string{
		"## a\n```post media=gif\nx\n```\n":                        "media=gif: want final, none, builds or clip",
		"## a\n```post size=big\nx\n```\n":                         `post option "size=big"`,
		"## a\n```post builds=1\nx\n```\n":                         "builds= goes with media=builds",
		"## a\n```post media=builds builds=3\nx\n```\n":            "line 2: build 3, but slide 1 has 2 builds",
		"## a\n```post media=builds builds=2-1\nx\n```\n":          "builds=2-1: want 1-based builds",
		"## a\n```alt\nx\n```\n":                                   "an alt block goes right after the post",
		"## a\n```post media=none\nx\n```\n```alt\ny\n```\n":       "an alt block goes right after the post",
		"## a\n```post\nx\n```\n```alt\ny\n```\n```alt\nz\n```\n":  "an alt block goes right after the post",
		"```post media=final\nx\n```\n## a\n":                      "line 1: a post before the first slide's section",
		"## a\n```post\nnever closed\n":                            "line 2: a code block is never closed",
		"## a\n```go\nfunc f() {\n## a\n":                          "line 2: a code block is never closed",
		"## a\n```post media=builds builds=1-2000000000\nx\n```\n": "build 2000000000, but slide 1 has 2 builds",
	} {
		_, err := parseNotes(md, slides)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", md, err, want)
		}
	}
}
