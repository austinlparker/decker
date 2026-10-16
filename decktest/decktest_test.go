package decktest

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/austinlparker/decker"
)

var theme = &decker.Theme{
	Display: decker.StockFont("SpaceGrotesk-Bold"),
	Body:    decker.StockFont("SpaceGrotesk-Medium"),
	Mono:    decker.StockFont("JetBrainsMono-ExtraBold"),
}

// boomDeck is a one-slide deck that panics in part ("View", "a placed
// element" or "Theme.Overlay"), or nowhere for "".
func boomDeck(part string) decker.Deck {
	boom := func() { panic(errBoom) }
	th := *theme
	s := decker.Slide{Title: "Boom", View: func(decker.Ctx, *decker.Scene) {}}
	switch part {
	case "View":
		s.View = func(decker.Ctx, *decker.Scene) { boom() }
	case "a placed element":
		s.View = func(c decker.Ctx, sc *decker.Scene) {
			sc.Place("k", c.Frame(), func(*decker.Pixels, decker.Rect) { boom() })
		}
	case "Theme.Overlay":
		th.Overlay = func(decker.Ctx, *decker.Pixels) { boom() }
	}
	return decker.Deck{Name: "boom", Theme: &th, Slides: []decker.Slide{s}}
}

var errBoom = errors.New("boom")

var parts = []string{"View", "a placed element", "Theme.Overlay"}

// TestDrawReportsEveryPart checks Deck.Draw, which Slides fails on, returns
// a panic wherever the engine catches one: in View, in a placed element, or
// in the overlay.
func TestDrawReportsEveryPart(t *testing.T) {
	d := boomDeck("")
	if err := d.Draw(0, decker.Ctx{W: 40, H: 12}); err != nil {
		t.Errorf("no panic: got %v", err)
	}
	Slides(t, d)
	for _, part := range parts {
		d := boomDeck(part)
		err := d.Draw(0, decker.Ctx{W: 40, H: 12})
		if want := `slide 1 "Boom": ` + part + " panicked: boom"; err == nil || err.Error() != want {
			t.Errorf("%s: got %v, want %s", part, err, want)
		}
		if !errors.Is(err, errBoom) {
			t.Errorf("%s: %v does not wrap the panic's error", part, err)
		}
	}
}

// TestSlidesFailsOnEveryPart runs Slides on a deck that panics in each part,
// in a child test process, and checks it fails naming the part.
func TestSlidesFailsOnEveryPart(t *testing.T) {
	if part := os.Getenv("DECKTEST_BOOM"); part != "" {
		Slides(t, boomDeck(part))
		return
	}
	for _, part := range parts {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSlidesFailsOnEveryPart$")
		cmd.Env = append(os.Environ(), "DECKTEST_BOOM="+part)
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), part+" panicked: boom (at 80x24, build 1, t=0)") {
			t.Errorf("%s: Slides passed or said something else: %v\n%s", part, err, out)
		}
	}
}

// TestSlidesChecksTheDeck runs Slides, in a child test process, on a deck
// whose notes have fallen out of step with its slides.
func TestSlidesChecksTheDeck(t *testing.T) {
	if os.Getenv("DECKTEST_NOTES") != "" {
		d := boomDeck("")
		d.Notes = "## Renamed\n"
		Slides(t, d)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSlidesChecksTheDeck$")
	cmd.Env = append(os.Environ(), "DECKTEST_NOTES=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), `deck: Notes line 1: section 1 is "Renamed"`) {
		t.Errorf("Slides passed or said something else: %v\n%s", err, out)
	}
}

func TestReviewPassesACleanDeck(t *testing.T) {
	d := decker.Deck{Name: "clean", Theme: theme, Slides: []decker.Slide{{
		Title: "Hello",
		View: func(c decker.Ctx, sc *decker.Scene) {
			decker.Text{Font: c.Theme.Display, Size: c.Size(0.15)}.Draw(sc.Px, "Hello", c.X(0.1), c.Y(0.2))
		},
	}}}
	Review(t, d)
	if is := d.Review(); len(is) != 0 {
		t.Errorf("issues in a clean deck: %v", is)
	}
	d.Slides[0].View = func(c decker.Ctx, sc *decker.Scene) {
		decker.Text{Font: c.Theme.Display, Size: c.Size(0.15)}.Draw(sc.Px, "Hello, off the edge", c.X(0.8), c.Y(0.2))
	}
	is := d.Review()
	if len(is) != 3 || is[0].Severity != decker.SeverityError || !strings.Contains(is[0].Msg, "past the right edge") {
		t.Errorf("off-edge deck: %v", is)
	}
}

// prepared is a Publisher that keeps what Prepare is given and turns away a
// post that says "too long".
type prepared struct{ posts []decker.Post }

func (*prepared) SignIn(context.Context, string, func(string)) error { return nil }
func (*prepared) Upload(context.Context, decker.Post) error          { return nil }

func (p *prepared) Prepare(_ context.Context, posts []decker.Post) error {
	p.posts = posts
	for _, post := range posts {
		if post.Text == "too long" {
			return errors.New("too long for the service")
		}
	}
	return nil
}

func (*prepared) Post(context.Context, decker.Post, string, string) (string, error) {
	return "", errors.New("Posts posted")
}

// TestPosts checks Posts renders every post and hands them all to the
// Publisher, and, in a child test process, fails when it turns one away.
func TestPosts(t *testing.T) {
	pub := &prepared{}
	d := boomDeck("")
	d.Publisher = pub
	d.Notes = "```post\nopening\n```\n## Boom\n```post\nthe slide\n```\n"
	if os.Getenv("DECKTEST_POSTS") != "" {
		d.Notes = "## Boom\n```post\ntoo long\n```\n"
		Posts(t, d)
		return
	}
	posts := Posts(t, d)
	if len(posts) != 2 || len(pub.posts) != 2 || posts[0].Text != "opening" || len(posts[0].Media) != 0 ||
		posts[1].Slide != 0 || len(posts[1].Media) != 1 {
		t.Fatalf("posts %+v, prepared %+v", posts, pub.posts)
	}
	if _, err := os.Stat(posts[1].Media[0].Path); err != nil {
		t.Errorf("the slide's image: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPosts$")
	cmd.Env = append(os.Environ(), "DECKTEST_POSTS=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "too long for the service") {
		t.Errorf("Posts passed or said something else: %v\n%s", err, out)
	}
}

// TestSlidesFailsAnImpureView runs Slides, in a child test process, on a
// deck whose View counts the frames it has drawn.
func TestSlidesFailsAnImpureView(t *testing.T) {
	if os.Getenv("DECKTEST_IMPURE") != "" {
		d := boomDeck("")
		frames := 0
		d.Slides[0].View = func(c decker.Ctx, sc *decker.Scene) {
			frames++
			sc.Px.Rect(float64(frames%10), 0, 4, 4, decker.Hex("#FF8800"), 1)
		}
		Slides(t, d)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSlidesFailsAnImpureView$")
	cmd.Env = append(os.Environ(), "DECKTEST_IMPURE=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "drawn again, it changed") {
		t.Errorf("Slides passed or said something else: %v\n%s", err, out)
	}
}

// goldenDeck is a slide whose box sits at x, with words in the character
// layer.
func goldenDeck(x float64) decker.Deck {
	d := boomDeck("")
	d.Slides[0].Steps = 2
	d.Slides[0].View = func(c decker.Ctx, sc *decker.Scene) {
		sc.Px.Rect(c.X(x), c.Y(0.2), c.X(0.3), c.Y(0.3), decker.Hex("#FF8800"), 1)
		if c.Reached(1) {
			sc.Text(2, 1, "words", decker.Hex("#FFFFFF").Color())
		}
	}
	return d
}

// TestGolden records a deck's references, checks them, and, in a child
// test process, fails a deck whose box moved and a reference no slide
// draws.
func TestGolden(t *testing.T) {
	if dir := os.Getenv("DECKTEST_GOLDEN"); dir != "" {
		Golden(t, goldenDeck(0.2), dir)
		return
	}
	dir := t.TempDir()
	t.Setenv("UPDATE_GOLDEN", "1")
	Golden(t, goldenDeck(0.1), dir)
	t.Setenv("UPDATE_GOLDEN", "")
	Golden(t, goldenDeck(0.1), dir)
	chars, _ := os.ReadFile(filepath.Join(dir, "characters.txt"))
	if want := "=== 1.2 Boom, 80×24 ===\n\n  words\n"; !strings.HasSuffix(string(chars), want) {
		t.Errorf("characters.txt:\n%s", chars)
	}
	if err := os.WriteFile(filepath.Join(dir, "02-Gone.png"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestGolden$")
	cmd.Env = append(os.Environ(), "DECKTEST_GOLDEN="+dir)
	out, err := cmd.CombinedOutput()
	for _, want := range []string{"01-Boom.png:", "build 1 at 0.25: a pixel moved by", "build 2 at 80×24: a pixel moved by", "02-Gone.png: no slide draws it"} {
		if err == nil || !strings.Contains(string(out), want) {
			t.Errorf("Golden passed or didn't say %q: %v\n%s", want, err, out)
		}
	}
}
