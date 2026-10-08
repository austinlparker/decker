package decker

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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
			c.Report(SeverityInfo, "waterfall-labels", r, "waterfall: 3 duration labels left out")
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
	issues := d.Review(ReviewOptions{})
	got := codes(issues)
	want := map[string][]string{
		"2.1": {"code-clipped"},
		"3.1": {"table-rows-dropped"},
		"4.1": {"text-offcanvas"},
		"6.1": {"text-small"},
		"7.1": {"overflow", "waterfall-labels"},
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
	d.Theme = &th
	is := d.Review(ReviewOptions{Slides: []int{0}, Sizes: [][2]int{{240, 67}}})
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

func TestReviewingOnlyUnderReview(t *testing.T) {
	var seen []bool
	d := &Deck{Name: "r", Theme: testTheme, Slides: []Slide{{View: func(c Ctx, sc *Scene) { seen = append(seen, c.Reviewing()) }}}}
	d.Render(0, Ctx{W: 40, H: 12})
	d.Review(ReviewOptions{Sizes: [][2]int{{40, 12}}})
	// Render, then the review's frame, then the plain frames it compares it
	// with.
	if len(seen) < 2 || seen[0] || !seen[1] || slices.Contains(seen[2:], true) {
		t.Errorf("Reviewing: got %v, want false, true, then false", seen)
	}
	if (Ctx{}).Fits("x", Rect{W: 1, H: 1}, 5, 5) {
		t.Error("Fits: a 5×5 block fits a 1×1 rect")
	}
	(Ctx{}).Report(SeverityError, "x", Rect{}, "nothing records this")
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
	dir := t.TempDir()
	r, err := reviewDeck(d, [][2]int{{240, 67}, {320, 90}}, []int{0, 1, 4, 5}, dir, "issues")
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	r.writeText(&text)
	out := text.String()
	for _, want := range []string{
		"2.1  Code", "240x67", "code-clipped", `Code "main.go"`,
		"6.1  Small", "320x90", "text-small",
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
	for _, name := range []string{"frames/02-1-240x67.png", "frames/06-1-320x90.png", "sheet-240x67.png", "sheet-320x90.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("image: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "frames/01-1-240x67.png")); err == nil {
		t.Error("an image of a build with no issues, with -frames issues")
	}
	md, _ := os.ReadFile(filepath.Join(dir, "index.md"))
	for _, want := range []string{"# Review: problems", "## 2. Code", "**Build 1**", "- **error** `code-clipped` at 240x67: Code \"main.go\"",
		"![2.1 at 240x67](frames/02-1-240x67.png)", "[240x67](sheet-240x67.png)"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("index.md lacks %q:\n%s", want, md)
		}
	}
	js, _ := os.ReadFile(filepath.Join(dir, "report.json"))
	var rep reviewJSON
	if err := json.Unmarshal(js, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Deck != "problems" || rep.Builds != 4 || len(rep.Issues) != 4 || rep.Issues[0].Slide != 2 || rep.Issues[0].Build != 1 || rep.Issues[0].Size != "240x67" {
		t.Errorf("report.json: %s", js)
	}

	clean, _ := reviewDeck(d, [][2]int{{240, 67}}, []int{0}, "", "none")
	text.Reset()
	clean.writeText(&text)
	if got := text.String(); got != "No issues in 1 build of 1 slide at 240x67.\n" {
		t.Errorf("clean report: %q", got)
	}
}

func TestParseSizes(t *testing.T) {
	got, err := parseSizes("240x67, 682x171")
	if err != nil || !slices.Equal(got, [][2]int{{240, 67}, {682, 171}}) {
		t.Errorf("parseSizes: %v %v", got, err)
	}
	for _, bad := range []string{"", "240", "0x10", "240x67,,", "axb"} {
		if _, err := parseSizes(bad); err == nil {
			t.Errorf("parseSizes(%q): no error", bad)
		}
	}
}

// TestReviewFrameChecks covers the checks that compare frames: builds that
// add nothing, frames that aren't pure, slides that never settle, morphs
// with nothing to move, overlaps and the overlay drawing over the slide.
func TestReviewFrameChecks(t *testing.T) {
	th := *testTheme
	th.Overlay = func(c Ctx, p *Pixels) {
		if c.Index == 8 {
			p.Rect(0, c.Y(0.2), c.PW(), c.Y(0.1), c.Theme.Accent, 1)
		}
	}
	calls := 0
	head := func(c Ctx, p *Pixels, s string, x float64) {
		Text{Font: c.Theme.Display, Size: c.Size(0.12), Color: c.Theme.Text}.Draw(p, s, x, c.Y(0.2))
	}
	box := func(key string) func(Ctx, *Scene) {
		return func(c Ctx, sc *Scene) {
			sc.Place(key, c.Rect(0.1, 0.5, 0.3, 0.2), func(p *Pixels, r Rect) { p.Rect(r.X, r.Y, r.W, r.H, c.Theme.Accent, 1) })
		}
	}
	d := &Deck{Name: "frames", Theme: &th, Slides: []Slide{
		{Title: "Dead step", Steps: 2, View: func(c Ctx, sc *Scene) { head(c, sc.Px, "Same", c.X(0.1)) }},
		{Title: "Pulse", Steps: 2, View: func(c Ctx, sc *Scene) {
			grow := 1 + 0.3*(1-Ease(c.Since(1), 0.5))
			if c.Step < 1 {
				grow = 1
			}
			sc.Px.Disc(c.X(0.5), c.Y(0.5), c.Unit(0.1)*grow, c.Theme.Accent, 1)
		}},
		{Title: "Impure", View: func(c Ctx, sc *Scene) {
			calls++
			head(c, sc.Px, fmt.Sprint("frame ", calls), c.X(0.1))
		}},
		{Title: "Spinner", View: func(c Ctx, sc *Scene) {
			sc.Px.VGradient(0, sc.Px.H-1, Mix(c.Theme.Background, c.Theme.Accent, math.Mod(c.T, 1)), c.Theme.Background)
		}},
		{Title: "Placed", View: box("a")},
		{Title: "Morph", Transition: TransitionMorph, View: box("b")},
		{Title: "Morph matched", Transition: TransitionMorph, View: box("b")},
		{Title: "Overlap", View: func(c Ctx, sc *Scene) {
			head(c, sc.Px, "First line", c.X(0.1))
			head(c, sc.Px, "Second line", c.X(0.2))
		}},
		{Title: "Overlaid", View: func(c Ctx, sc *Scene) { head(c, sc.Px, "Under the bar", c.X(0.1)) }},
	}}
	got := codes(d.Review(ReviewOptions{Sizes: [][2]int{{240, 67}}}))
	want := map[string][]string{
		"1.2": {"step-unchanged"},
		"3.1": {"impure"},
		"4.1": {"never-settles"},
		"6.1": {"morph-unmatched"},
		"8.1": {"overlap"},
		"9.1": {"overlay-collision"},
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
}

func TestSnapshotBounds(t *testing.T) {
	d := problemDeck()
	path := filepath.Join(t.TempDir(), "b.png")
	o := options{slide: 2, step: 1, at: Settled, width: 160, height: 45, png: path, bounds: true}
	if err := runSnapshot(d, o); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
		t.Errorf("-bounds wrote nothing: %v", err)
	}
	o.png = ""
	if err := runSnapshot(d, o); err == nil {
		t.Error("-bounds without -png: no error")
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
	if got := codes(d.Review(ReviewOptions{Sizes: [][2]int{{240, 67}}}))["1.1"]; !slices.Equal(got, []string{"overlap"}) {
		t.Errorf("codes %v, want [overlap]", got)
	}
}

func TestReviewNoFrames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	if _, err := reviewDeck(problemDeck(), [][2]int{{240, 67}}, []int{1}, dir, "none"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sheet-240x67.png")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "frames")); err == nil {
		t.Error("-frames none wrote a frames directory")
	}
}
