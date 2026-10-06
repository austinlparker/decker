package decker

import (
	"strings"
	"testing"
)

func codeCtx(step int, stepT float64) Ctx {
	return Ctx{W: 200, H: 60, T: 5, Step: step, StepT: stepT, Theme: testTheme}
}

func TestExpandTabs(t *testing.T) {
	if got, want := expandTabs("\ta\t\tb"), "    a        b"; got != want {
		t.Errorf("expandTabs = %q, want %q", got, want)
	}
	lx := lexCode("\tx := 1", "go", false)
	if got := lx.lines[0].cols; got != 4+len("x := 1") {
		t.Errorf("tab line is %d columns, want %d", got, 4+len("x := 1"))
	}
}

func TestLexRoles(t *testing.T) {
	lx := lexCode("func main() {\n\treturn \"s\" // c\n}", "go", false)
	if len(lx.lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lx.lines))
	}
	role := func(line int, text string) codeRole {
		for _, r := range lx.lines[line].runs {
			if strings.Contains(r.text, text) {
				return r.role
			}
		}
		t.Fatalf("no run containing %q on line %d: %+v", text, line, lx.lines[line].runs)
		return 0
	}
	for _, tc := range []struct {
		line int
		text string
		want codeRole
	}{{0, "func", roleKeyword}, {0, "main", roleName}, {1, "return", roleKeyword}, {1, `"s"`, roleString}, {1, "// c", roleComment}} {
		if got := role(tc.line, tc.text); got != tc.want {
			t.Errorf("%q has role %d, want %d", tc.text, got, tc.want)
		}
	}
	if lx.maxCols != len("\treturn \"s\" // c")+3 {
		t.Errorf("maxCols = %d", lx.maxCols)
	}
}

func TestLexTrailingNewlinesAndRuns(t *testing.T) {
	if n := len(lexCode("a\nb\n\n", "go", false).lines); n != 2 {
		t.Errorf("trailing newlines kept: %d lines, want 2", n)
	}
	// Plain text neighbors merge into one run per line.
	for _, l := range lexCode("hello world foo", "", false).lines {
		if len(l.runs) != 1 {
			t.Errorf("plain line has %d runs, want 1", len(l.runs))
		}
	}
	if n := len(lexCode("", "go", false).lines); n != 1 {
		t.Errorf("empty source has %d lines, want 1", n)
	}
}

func TestDiffParsing(t *testing.T) {
	lx := lexCode(" keep\n-old\n+new\nbare\n+\n", "", true)
	var markers []byte
	var texts []string
	for _, l := range lx.lines {
		markers = append(markers, l.marker)
		s := ""
		for _, r := range l.runs {
			s += r.text
		}
		texts = append(texts, s)
	}
	if got, want := string(markers), "\x00-+\x00+"; got != want {
		t.Errorf("markers = %q, want %q", got, want)
	}
	if got, want := strings.Join(texts, "|"), "keep|old|new|bare|"; got != want {
		t.Errorf("texts = %q, want %q", got, want)
	}
	// The same text outside diff mode keeps its markers.
	if l := lexCode("-old", "", false).lines[0]; l.marker != 0 || l.cols != 4 {
		t.Errorf("non-diff line parsed as diff: %+v", l)
	}
}

func TestUnknownLanguage(t *testing.T) {
	c := codeCtx(0, 1)
	p := NewPixels(c.W, 2*c.H, testTheme.Background)
	for _, lang := range []string{"", "no-such-language", "GO", "text", "diff"} {
		Code{Source: "func x() {\n\t\"s\"\n}", Lang: lang, LineNumbers: true}.Draw(c, p, c.Frame())
	}
	for _, l := range lexCode("a b", "no-such-language", false).lines {
		for _, r := range l.runs {
			if r.role != rolePlain {
				t.Errorf("unknown language run has role %d", r.role)
			}
		}
	}
	// Empty and one-line sources, and a rect with no room, must not panic.
	Code{}.Draw(c, p, c.Frame())
	Code{Source: "x", Title: "t", Diff: true, Focus: []LineRange{{5, 9}}}.Draw(c, p, Rect{})
}

func TestCodeFit(t *testing.T) {
	c := codeCtx(0, 1)
	src := "short\n" + strings.Repeat("x", 30) + "\nlast"
	k := Code{Source: src}
	lx := lexCode(src, "", false)
	f := testTheme.Mono
	lo, hi := c.SmallText(f), c.Size(0.1)

	const w, h = 4000, 4000
	if got := k.fitCode(f, lx, w, h, lo, hi); got != hi {
		t.Errorf("huge rect fits %d, want the cap %d", got, hi)
	}
	if got := k.fitCode(f, lx, 1, 1, lo, hi); got != lo {
		t.Errorf("tiny rect fits %d, want the floor %d", got, lo)
	}
	// The answer is the largest size that fits: it fits, and one more doesn't.
	cw := 30*f.Measure("0", 14) + 1
	size := k.fitCode(f, lx, cw, h, 6, 60)
	if float64(30)*f.Measure("0", size) > cw || float64(30)*f.Measure("0", size+1) <= cw {
		t.Errorf("fit chose %d for width %.1f", size, cw)
	}
	if size < 14 || size > 15 {
		t.Errorf("fit chose %d, want about 14", size)
	}
	// Line count limits too: 3 lines at the leading.
	size = k.fitCode(f, lx, w, 3*20*codeLeading, 6, 60)
	if size != 20 {
		t.Errorf("height-limited fit chose %d, want 20", size)
	}
	// Line numbers take columns.
	nums := Code{Source: src, LineNumbers: true}
	if a, b := k.fitCode(f, lx, cw, h, 6, 60), nums.fitCode(f, lx, cw, h, 6, 60); b >= a {
		t.Errorf("line numbers did not shrink the fit: %d vs %d", b, a)
	}
}

func TestCodeDrawSize(t *testing.T) {
	c := codeCtx(0, 1)
	p := NewPixels(c.W, 2*c.H, testTheme.Background)
	r := Rect{10, 10, 150, 80}
	w, h := Code{Source: strings.Repeat("a line\n", 40), Title: "f.go"}.Draw(c, p, r)
	if w > r.W || h > r.H || w <= 0 || h <= 0 {
		t.Errorf("plate %.1fx%.1f outside %vx%v", w, h, r.W, r.H)
	}
	w, h = Code{Source: "x", Size: 8}.Draw(c, p, r)
	if w >= r.W/2 || h >= r.H/2 {
		t.Errorf("small block %.1fx%.1f should hug its code", w, h)
	}
}

func TestFocusSteps(t *testing.T) {
	k := Code{Focus: []LineRange{{2, 4}, {}, {7, 99}}, FirstStep: 2}
	const n = 10
	type want struct {
		cur, prev     [2]int
		curOK, prevOK bool
	}
	for step, w := range map[int]want{
		0: {},
		1: {},
		2: {cur: [2]int{1, 4}, curOK: true},
		3: {prev: [2]int{1, 4}, prevOK: true},
		4: {cur: [2]int{6, 10}, curOK: true},
		5: {cur: [2]int{6, 10}, curOK: true, prev: [2]int{6, 10}, prevOK: true}, // past the end keeps the last
	} {
		f := k.focusAt(codeCtx(step, 100), n)
		got := want{f.cur, f.prev, f.curOK, f.prevOK}
		if !f.curOK {
			got.cur = [2]int{}
		}
		if !f.prevOK {
			got.prev = [2]int{}
		}
		if got != w {
			t.Errorf("step %d: %+v, want %+v", step, got, w)
		}
	}
	if f := (Code{}).focusAt(codeCtx(3, 1), n); f.curOK || f.prevOK || f.weight(5) != 1 {
		t.Errorf("no Focus must leave every line normal: %+v", f)
	}
}

func TestFocusEasing(t *testing.T) {
	k := Code{Focus: []LineRange{{1, 2}, {5, 6}}, FirstStep: 1}
	at := func(step int, stepT float64) focusState { return k.focusAt(codeCtx(step, stepT), 8) }

	// Entering the first range: line 0 stays lit, line 5 dims as e grows.
	if w := at(1, 0).weight(5); w != 1 {
		t.Errorf("weight at the start of step 1 = %v, want 1", w)
	}
	if w := at(1, 100).weight(5); w != 0 {
		t.Errorf("weight at the end of step 1 = %v, want 0", w)
	}
	// Mid-ease weights are strictly between and the bar slides from range to range.
	f := at(2, 0.1)
	if w := f.weight(0); w <= 0 || w >= 1 {
		t.Errorf("line leaving the focus has weight %v mid-ease", w)
	}
	if w := f.weight(4); w <= 0 || w >= 1 {
		t.Errorf("line entering the focus has weight %v mid-ease", w)
	}
	y0, y1, a := f.bar()
	if y0 <= 0 || y0 >= 4 || y1 <= 1 || y1 >= 5 || a != 1 {
		t.Errorf("bar mid-slide = %v..%v alpha %v", y0, y1, a)
	}
	if _, _, a := at(1, 0).bar(); a != 0 {
		t.Errorf("bar visible at the instant focus begins: alpha %v", a)
	}
	if _, _, a := at(0, 5).bar(); a != 0 {
		t.Errorf("bar visible before FirstStep: alpha %v", a)
	}
	// Settled steps (entering a slide past the step) show the end state.
	if f := k.focusAt(Ctx{Step: 2, StepT: Settled, Theme: testTheme}, 8); f.weight(4) != 1 || f.weight(0) != 0 {
		t.Errorf("settled focus not at its end state: %+v", f)
	}
}

func TestSyntaxColors(t *testing.T) {
	th := *testTheme
	d := th.syntaxColors()
	if d.Keyword != th.Accent || d.String != th.Good || d.Comment != th.Muted || d.Number != th.Warn || d.Name != th.Accent2 {
		t.Errorf("derived palette ignores the theme: %+v", d)
	}
	if d.Added == d.Removed || d.Added == th.Panel {
		t.Errorf("diff backgrounds not distinct tints of Panel: %+v", d)
	}
	th.Syntax = &SyntaxColors{Keyword: Hex("#112233")}
	o := th.syntaxColors()
	if o.Keyword != Hex("#112233") {
		t.Errorf("Keyword override lost: %+v", o.Keyword)
	}
	o.Keyword = d.Keyword
	if o != d {
		t.Errorf("unset fields should keep the derived colors:\n got %+v\nwant %+v", o, d)
	}
}

// TestCodeFrame draws the same Ctx twice: the engine's frame-purity promise.
func TestCodeFrame(t *testing.T) {
	k := Code{Source: "func f() {}\n// x", Lang: "go", LineNumbers: true, Title: "a.go",
		Focus: []LineRange{{1, 1}, {2, 2}}, FirstStep: 0}
	c := codeCtx(1, 0.12)
	a, b := NewPixels(c.W, 2*c.H, testTheme.Background), NewPixels(c.W, 2*c.H, testTheme.Background)
	k.Draw(c, a, c.Frame())
	k.Draw(c, b, c.Frame())
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("pixel %d differs between identical draws", i)
		}
	}
}

// outsideChanged counts pixels that differ from bg outside r.
func outsideChanged(p *Pixels, bg RGB, r Rect) (n int) {
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W; x++ {
			inside := float64(x) >= r.X && float64(x) < r.Right() && float64(y) >= r.Y && float64(y) < r.Bottom()
			if !inside && p.At(x, y) != bg {
				n++
			}
		}
	}
	return n
}

// TestCodeStaysInRect: the plate, the title tab and the text never paint
// outside the rect they were given, however long the title or small the rect.
func TestCodeStaysInRect(t *testing.T) {
	c := Ctx{W: 240, H: 67, T: 5, Step: 1, StepT: 5, Theme: testTheme}
	long := "a_very_long_filename_that_cannot_fit.go"
	for name, tc := range map[string]struct {
		k Code
		r Rect
	}{
		"long title":     {Code{Source: "x", Title: long}, Rect{20, 20, 70, 30}},
		"narrow":         {Code{Source: "x", Title: long, LineNumbers: true}, Rect{20, 20, 20, 60}},
		"short":          {Code{Source: "x\ny\nz", Title: long}, Rect{20, 20, 200, 20}},
		"tiny":           {Code{Source: "x", Title: "t"}, Rect{20, 20, 3, 3}},
		"no title":       {Code{Source: strings.Repeat("long line ", 20)}, Rect{20, 20, 70, 30}},
		"diff and focus": {Code{Source: "+aaaaaaaaaaaaaaaaaaaa\n-b", Diff: true, Title: long, Focus: []LineRange{{1, 2}}}, Rect{20, 20, 60, 40}},
	} {
		p := NewPixels(c.W, 2*c.H, testTheme.Background)
		w, h := tc.k.Draw(c, p, tc.r)
		if w > tc.r.W || h > tc.r.H {
			t.Errorf("%s: plate %.1fx%.1f larger than the rect", name, w, h)
		}
		if n := outsideChanged(p, testTheme.Background, tc.r); n != 0 {
			t.Errorf("%s: %d pixels painted outside %v", name, n, tc.r)
		}
	}
}

func TestCodeEmptyRectDrawsNothing(t *testing.T) {
	c := Ctx{W: 240, H: 67, T: 5, Theme: testTheme}
	for _, r := range []Rect{{}, {20, 20, 0, 40}, {20, 20, 40, 0}, {20, 20, -5, 40}} {
		p := NewPixels(c.W, 2*c.H, testTheme.Background)
		w, h := Code{Source: "x := 1", Title: "main.go", LineNumbers: true, Focus: []LineRange{{1, 1}}}.Draw(c, p, r)
		if w != 0 || h != 0 {
			t.Errorf("%v: returned %vx%v", r, w, h)
		}
		if n := outsideChanged(p, testTheme.Background, Rect{}); n != 0 {
			t.Errorf("%v: painted %d pixels", r, n)
		}
	}
}

// TestFillPlate pins fillPlate to RoundRect, which it only speeds up.
func TestFillPlate(t *testing.T) {
	bg, col := Hex("#101820"), Hex("#FF7A00")
	for _, h := range []float64{70, 30, 12} {
		a, b := NewPixels(120, 90, bg), NewPixels(120, 90, bg)
		fillPlate(a, 10.2, 7.4, 90.1, h, 9, col)
		b.RoundRect(10, 7, 90, h, 9, 0, col, 1)
		for i := range a.Pix {
			if a.Pix[i] != b.Pix[i] {
				t.Fatalf("h=%v: pixel (%d,%d) = %v, RoundRect gives %v", h, i%a.W, i/a.W, a.Pix[i], b.Pix[i])
			}
		}
	}
}

// BenchmarkCode is a ~25-line Go block at the presenting size, with focus
// mid-ease and line numbers: the per-frame cost a slide pays for it.
func BenchmarkCode(b *testing.B) {
	var sb strings.Builder
	for i := range 25 {
		sb.WriteString("\tif err := step(" + strings.Repeat("x", i%7) + `); err != nil { return "fail", 42 } // note` + "\n")
	}
	c := Ctx{W: 682, H: 171, T: 5, Step: 2, StepT: 0.1, Theme: testTheme}
	p := NewPixels(682, 342, testTheme.Background)
	k := Code{Source: sb.String(), Lang: "go", LineNumbers: true, Title: "main.go", Size: 9,
		Focus: []LineRange{{2, 6}, {10, 14}}, FirstStep: 1}
	r := Rect{10, 10, 660, 320}
	b.ReportAllocs()
	for range b.N {
		k.Draw(c, p, r)
	}
}
