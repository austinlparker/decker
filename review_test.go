package decker

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Ctx stays comparable, though review adds a field: apidiff counts losing ==
// as a break.
var _ = Ctx{} == Ctx{}

const longSource = `package main

import "fmt"

func main() {
	for i := range 10 {
		fmt.Println("line", i)
	}
	fmt.Println("done")
}`

// problemDeck has one slide per thing a review should find, and a few it
// should not.
func problemDeck() *Deck {
	offRight := func(c Ctx, sc *Scene) {
		Text{Font: c.Theme.Display, Size: c.Size(0.2), Color: c.Theme.Text}.Draw(sc.Px, "Far too wide", c.X(0.7), c.Y(0.3))
	}
	return &Deck{Name: "problems", Theme: testTheme, Slides: []Slide{
		{Title: "Clean", View: func(c Ctx, sc *Scene) {
			Text{Font: c.Theme.Display, Size: c.Size(0.15), Color: c.Theme.Text}.Draw(sc.Px, "Fine", c.X(0.1), c.Y(0.2))
		}},
		{Title: "Code", View: func(c Ctx, sc *Scene) {
			Code{Source: longSource, Lang: "go", Title: "main.go"}.Draw(c, sc.Px, c.Rect(0.1, 0.1, 0.5, 0.3))
		}},
		{Title: "Table", View: func(c Ctx, sc *Scene) {
			rows := make([][]string, 12)
			for i := range rows {
				rows[i] = []string{fmt.Sprint("row ", i), "value"}
			}
			Table{Header: []string{"Name", "Value"}, Rows: rows}.Draw(c, sc.Px, c.Rect(0.1, 0.1, 0.8, 0.25))
		}},
		{Title: "Off canvas", View: offRight},
		{Title: "Allowed", Allow: []string{"text-offcanvas"}, View: offRight},
		{Title: "Small", View: func(c Ctx, sc *Scene) {
			Text{Font: c.Theme.Body, Size: 5, Color: c.Theme.Text}.Draw(sc.Px, "tiny print", c.X(0.1), c.Y(0.5))
		}},
		{Title: "Custom", View: func(c Ctx, sc *Scene) {
			r := c.Rect(0.1, 0.1, 0.2, 0.2)
			if c.Fits("waterfall", r, r.W, r.H) != true {
				panic("a block the rect's own size must fit")
			}
			c.Fits("waterfall", r, r.W*2, r.H)
		}},
		{Title: "Second build", Steps: 2, View: func(c Ctx, sc *Scene) {
			if c.Step == 1 {
				offRight(c, sc)
			}
		}},
		{Title: "Placed panic", View: func(c Ctx, sc *Scene) {
			sc.Place("boom", c.Frame(), func(*Pixels, Rect) { panic("placed element broke") })
		}},
	}}
}

// codes returns the issue codes per "slide.build", 1-based, sorted.
func codes(issues []Issue) map[string][]string {
	out := map[string][]string{}
	for _, is := range issues {
		k := fmt.Sprintf("%d.%d", is.Slide+1, is.Step+1)
		if !slices.Contains(out[k], is.Code) {
			out[k] = append(out[k], is.Code)
			slices.Sort(out[k])
		}
	}
	return out
}

func TestReviewFindsProblems(t *testing.T) {
	d := problemDeck()
	issues := d.Review()
	got := codes(issues)
	want := map[string][]string{
		"2.1": {"code-clipped"},
		"3.1": {"table-rows-dropped"},
		"4.1": {"text-offcanvas"},
		"6.1": {"text-small"},
		"7.1": {"overflow"},
		"8.2": {"text-offcanvas"},
		"9.1": {"panic"},
	}
	for k, w := range want {
		if !slices.Equal(got[k], w) {
			t.Errorf("slide %s: got codes %v, want %v", k, got[k], w)
		}
	}
	for k, g := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("slide %s: unexpected codes %v", k, g)
		}
	}

	// Every issue is placed in its frame, and says what and by how much.
	for _, is := range issues {
		if is.W == 0 || is.H == 0 || is.Title == "" || is.Msg == "" {
			t.Errorf("issue not filled in: %+v", is)
		}
	}
	find := func(code string, w int) Issue {
		for _, is := range issues {
			if is.Code == code && is.W == w {
				return is
			}
		}
		t.Fatalf("no %s issue at width %d", code, w)
		return Issue{}
	}
	if is := find("code-clipped", 320); !strings.Contains(is.Msg, `Code "main.go": `) || !strings.Contains(is.Msg, " of 10 lines visible") || is.Severity != SeverityError {
		t.Errorf("code-clipped: %v", is)
	}
	if is := find("overflow", 240); is.Msg != "waterfall needs 96×27px, has 48×27px" {
		t.Errorf("overflow message: %q", is.Msg)
	}
	if is := find("text-small", 682); is.Severity != SeverityWarning || !strings.Contains(is.Msg, `Text "tiny print" is 5px`) {
		t.Errorf("text-small: %v", is)
	}
	if is := find("panic", 682); is.Msg != "placed element broke" {
		t.Errorf("panic: %v", is)
	}
	if got, want := find("text-offcanvas", 240).String(), `4.1 Off canvas 240x67 error text-offcanvas: Text "Far too wide" runs`; !strings.HasPrefix(got, want) {
		t.Errorf("String() = %q, want prefix %q", got, want)
	}

	// The overlay's problems are its own.
	th := *testTheme
	th.Overlay = func(c Ctx, p *Pixels) {
		Text{Font: c.Theme.Body, Size: c.SmallText(c.Theme.Body), Color: c.Theme.Muted}.Draw(p, "page 3", c.X(0.99), c.Y(0.5))
	}
	d.Theme, d.Slides = &th, d.Slides[:1]
	is := issuesAt(d, [2]int{240, 67})
	if len(is) != 1 || !strings.HasPrefix(is[0].Msg, `Theme.Overlay: Text "page 3" runs`) {
		t.Errorf("overlay issue: %v", is)
	}
}

// TestReviewDrawsTheSameFrames checks the review only listens: frames drawn
// under review are the frames the deck shows.
func TestReviewDrawsTheSameFrames(t *testing.T) {
	d := problemDeck()
	for i, s := range d.Slides {
		for step := range d.Steps(i) {
			c := Ctx{W: 120, H: 36, T: Settled, Step: step, StepT: Settled, Theme: d.Theme}.at(d.Slides, i)
			plain := renderSlide(s, c)
			c.review = newReviewLog(c.W, 2*c.H)
			reviewed := renderSlide(s, c)
			if sha256.Sum256(pixBytes(plain.Px)) != sha256.Sum256(pixBytes(reviewed.Px)) {
				t.Errorf("slide %d step %d draws differently under review", i+1, step+1)
			}
			plain.Release()
			reviewed.Release()
		}
	}
}

func pixBytes(p *Pixels) []byte {
	b := make([]byte, 0, len(p.Pix)*3)
	for _, c := range p.Pix {
		b = append(b, byte(c.R), byte(c.G), byte(c.B))
	}
	return b
}

// issuesAt is Deck.Review at the given sizes only, for tests that need
// one size.
func issuesAt(d *Deck, sizes ...[2]int) []Issue {
	var out []Issue
	d.review(sizes, func(f *reviewedFrame) { out = append(out, f.issues...) })
	return out
}

func TestFitsWithoutReview(t *testing.T) {
	if (Ctx{}).Fits("x", Rect{W: 1, H: 1}, 5, 5) {
		t.Error("Fits: a 5×5 block fits a 1×1 rect")
	}
	if !(Ctx{}).Fits("x", Rect{W: 10, H: 10}, 10.4, 10) {
		t.Error("Fits: half a pixel over is a fit")
	}
}

func TestCodeMeasure(t *testing.T) {
	c := Ctx{W: 320, H: 90, Theme: testTheme}
	k := Code{Source: longSource, Lang: "go", Title: "main.go", LineNumbers: true}
	small := k.Measure(c, c.Rect(0.1, 0.1, 0.5, 0.3))
	if small.Fits() || small.Lines != 10 || small.Shown >= 10 || small.Shown < 1 {
		t.Errorf("small rect: %+v", small)
	}
	if small.NeedH <= small.H || small.Head <= 0 || small.Size != c.SmallText(c.Theme.Mono) {
		t.Errorf("small rect sizes: %+v", small)
	}
	big := k.Measure(c, c.Frame())
	if !big.Fits() || big.Shown != 10 || big.ShownCols != big.Cols || big.W > c.PW() || big.H > c.PH() {
		t.Errorf("whole frame: %+v", big)
	}
	// Measure is what Draw draws.
	p := NewPixels(c.W, 2*c.H, testTheme.Background)
	r := c.Rect(0.1, 0.1, 0.5, 0.3)
	if w, h := k.Draw(c, p, r); w != small.W || h != small.H {
		t.Errorf("Draw returned %v×%v, Measure %v×%v", w, h, small.W, small.H)
	}
	if empty := k.Measure(c, Rect{}); empty.W != 0 || empty.Shown != 0 || empty.Lines != 10 {
		t.Errorf("empty rect: %+v", empty)
	}
}

func TestReviewReport(t *testing.T) {
	d := problemDeck()
	d.Slides = []Slide{d.Slides[0], d.Slides[1], d.Slides[4], d.Slides[5]} // clean, code, allowed, small
	dir := t.TempDir()
	r, err := reviewDeck(d, [][2]int{{240, 67}, {320, 90}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	r.writeText(&text)
	out := text.String()
	for _, want := range []string{
		"2.1  Code", "240x67", "code-clipped", `Code "main.go"`,
		"4.1  Small", "320x90", "text-small",
		"2 errors and 2 warnings in 4 builds of 4 slides at 240x67, 320x90.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Allowed") {
		t.Errorf("text report lists an allowed issue:\n%s", out)
	}
	if err := r.save(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"frames/02-1-240x67.png", "frames/04-1-320x90.png", "sheet-240x67.png", "sheet-320x90.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("image: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "frames/01-1-240x67.png")); err == nil {
		t.Error("an image of a build with no issues")
	}
	md, _ := os.ReadFile(filepath.Join(dir, "index.md"))
	for _, want := range []string{"# Review: problems", "## 2. Code", "**Build 1**", "- **error** `code-clipped` at 240x67: Code \"main.go\"",
		"![2.1 at 240x67](frames/02-1-240x67.png)", "[240x67](sheet-240x67.png)"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("index.md lacks %q:\n%s", want, md)
		}
	}

	d.Slides = d.Slides[:1]
	clean, _ := reviewDeck(d, [][2]int{{240, 67}}, t.TempDir())
	text.Reset()
	clean.writeText(&text)
	if got := text.String(); got != "No issues in 1 build of 1 slide at 240x67.\n" {
		t.Errorf("clean report: %q", got)
	}
}

// TestReviewTouchingLines checks two blocks of text whose lines run into
// each other are an overlap even though they share few inked pixels.
func TestReviewTouchingLines(t *testing.T) {
	d := &Deck{Name: "lines", Theme: testTheme, Slides: []Slide{{View: func(c Ctx, sc *Scene) {
		tx := Text{Font: c.Theme.Body, Size: c.Size(0.08), Color: c.Theme.Text}
		_, h := tx.Draw(sc.Px, "a subtitle to copy and adapt", c.X(0.1), c.Y(0.2))
		tx.Draw(sc.Px, "Copy a recipe into your talk", c.X(0.1), c.Y(0.2)+h*0.6)
	}}}}
	if got := codes(issuesAt(d, [2]int{240, 67}))["1.1"]; !slices.Equal(got, []string{"overlap"}) {
		t.Errorf("codes %v, want [overlap]", got)
	}
}

// TestReviewFindings pins the fixes for the first code review of the review
// itself, one case each.
func TestReviewFindings(t *testing.T) {
	at := func(d *Deck) map[string][]string { return codes(issuesAt(d, [2]int{320, 90})) }
	body := func(c Ctx, sc *Scene) {
		Text{Font: c.Theme.Body, Size: c.SmallText(c.Theme.Body), Color: c.Theme.Text}.Draw(sc.Px, "body text", c.X(.2), c.Y(.2))
	}

	t.Run("overlay Code over slide text", func(t *testing.T) {
		th := *testTheme
		th.Overlay = func(c Ctx, p *Pixels) { Code{Source: "x := 1"}.Draw(c, p, c.Rect(.1, .1, .6, .6)) }
		d := &Deck{Name: "o", Theme: &th, Slides: []Slide{{View: body}}}
		d.Review() // panicked in flush, outside the render's recover
	})

	t.Run("two untitled one-line Code blocks on top of each other", func(t *testing.T) {
		d := &Deck{Name: "c", Theme: testTheme, Slides: []Slide{{View: func(c Ctx, sc *Scene) {
			Code{Source: "first := 1"}.Draw(c, sc.Px, c.Rect(.1, .1, .5, .3))
			Code{Source: "second := 2"}.Draw(c, sc.Px, c.Rect(.1, .1, .5, .3))
		}}}}
		if got := at(d)["1.1"]; !slices.Contains(got, "overlap") {
			t.Errorf("codes %v, want an overlap", got)
		}
	})

	t.Run("text beside a short line of a multi-line block", func(t *testing.T) {
		d := &Deck{Name: "m", Theme: testTheme, Slides: []Slide{{View: func(c Ctx, sc *Scene) {
			tx := Text{Font: c.Theme.Body, Size: 16, Color: c.Theme.Text}
			tx.Draw(sc.Px, "a long headline for the first row\nx", 10, 10)
			tx.Draw(sc.Px, "other text", 100, 10+16*DefaultLeading)
		}}}}
		if got := at(d)["1.1"]; slices.Contains(got, "overlap") {
			t.Errorf("codes %v, want no overlap", got)
		}
	})

	t.Run("Code hanging off the canvas", func(t *testing.T) {
		d := &Deck{Name: "e", Theme: testTheme, Slides: []Slide{{View: func(c Ctx, sc *Scene) {
			Code{Source: "hello world"}.Draw(c, sc.Px, c.Rect(.95, .2, .6, .6))
		}}}}
		if got := at(d)["1.1"]; !slices.Equal(got, []string{"text-offcanvas"}) {
			t.Errorf("codes %v, want [text-offcanvas]", got)
		}
	})

	t.Run("table not revealed yet", func(t *testing.T) {
		rows := make([][]string, 4)
		for i := range rows {
			rows[i] = []string{fmt.Sprint("row ", i)}
		}
		d := &Deck{Name: "t", Theme: testTheme, Slides: []Slide{{Steps: 2, View: func(c Ctx, sc *Scene) {
			r := c.Rect(.1, .1, .8, .1)
			if c.Step == 1 {
				r = c.Rect(.1, .1, .8, .8)
			}
			Table{Header: []string{"Name"}, Rows: rows, FirstStep: 1}.Draw(c, sc.Px, r)
		}}}}
		if got := at(d); len(got) != 0 {
			t.Errorf("codes %v, want none", got)
		}
	})

	t.Run("placed shapes covering each other", func(t *testing.T) {
		d := &Deck{Name: "p", Theme: testTheme, Slides: []Slide{{View: func(c Ctx, sc *Scene) {
			for i, col := range []RGB{c.Theme.Accent, c.Theme.Good} {
				sc.Place(fmt.Sprint("box", i), c.Rect(.2, .2, .3, .3), func(p *Pixels, r Rect) { p.Rect(r.X, r.Y, r.W, r.H, col, 1) })
			}
		}}}}
		if got := at(d)["1.1"]; !slices.Equal(got, []string{"overlap"}) {
			t.Errorf("codes %v, want [overlap]", got)
		}
	})
}

// TestReviewFindingsAgain pins the fixes for the second code review.
func TestReviewFindingsAgain(t *testing.T) {
	at := func(view func(c Ctx, sc *Scene)) []string {
		d := &Deck{Name: "a", Theme: testTheme, Slides: []Slide{{View: view}}}
		return codes(issuesAt(d, [2]int{320, 90}))["1.1"]
	}
	label := func(p *Pixels, s string, x float64) {
		Text{Font: testTheme.Body, Size: 16, Color: testTheme.Text}.Draw(p, s, x, 30)
	}
	moved := func(dx float64) func(c Ctx, sc *Scene) {
		return func(c Ctx, sc *Scene) {
			Composite{Alpha: 1, DX: dx}.Draw(sc.Px, Rect{20, 30, 120, 20}, func(p *Pixels) { label(p, "other label", 20) })
		}
	}

	for _, tc := range []struct {
		name  string
		view  func(c Ctx, sc *Scene)
		codes []string
	}{
		{"text a composite moves away from other text", func(c Ctx, sc *Scene) {
			label(sc.Px, "first label", 20)
			moved(160)(c, sc)
		}, nil},
		{"text a composite moves onto other text", func(c Ctx, sc *Scene) {
			label(sc.Px, "first label", 180)
			moved(160)(c, sc)
		}, []string{"overlap"}},
		{"text a composite moves off the canvas", moved(300), []string{"text-offcanvas"}},
		{"Code a composite moves, drawn twice to find its coverage", func(c Ctx, sc *Scene) {
			Composite{Alpha: 1, DX: 30}.Draw(sc.Px, Rect{20, 30, 200, 80}, func(p *Pixels) {
				Code{Source: "x := 1"}.Draw(c, p, Rect{20, 30, 200, 80})
			})
		}, nil},
		{"a table whose header doesn't fit", func(c Ctx, sc *Scene) {
			Table{Header: []string{"Header"}}.Draw(c, sc.Px, Rect{20, 30, 150, 1})
		}, []string{"table-rows-dropped"}},
	} {
		if got := at(tc.view); !slices.Equal(got, tc.codes) {
			t.Errorf("%s: codes %v, want %v", tc.name, got, tc.codes)
		}
	}
}
