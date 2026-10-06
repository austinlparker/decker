package decker

import (
	"slices"
	"strings"
	"testing"
)

// richLines returns the text of each laid-out line of spans.
func richLines(r Rich, spans []Span) []string {
	l, _ := r.layout(spans)
	var out []string
	for _, line := range l.lines {
		var sb strings.Builder
		for _, pg := range line {
			sb.WriteRune(l.glyphs[pg.g].r)
		}
		out = append(out, sb.String())
	}
	return out
}

func samePixels(t *testing.T, a, b *Pixels) {
	t.Helper()
	if !slices.Equal(a.Pix, b.Pix) {
		n := 0
		for i := range a.Pix {
			if a.Pix[i] != b.Pix[i] {
				n++
			}
		}
		t.Errorf("%d pixels differ", n)
	}
}

func litPixels(p *Pixels, bg RGB) (n int) {
	for _, px := range p.Pix {
		if px != bg {
			n++
		}
	}
	return n
}

func TestRichOneSpanMatchesText(t *testing.T) {
	bg := testTheme.Background
	for name, tc := range map[string]struct {
		s    string
		maxW float64
		fx   GlyphEffect
	}{
		"plain":   {"Hello, rich world", 0, nil},
		"breaks":  {"one\ntwo three\n\nfour", 0, nil},
		"wrapped": {"a longer paragraph that has to wrap inside its box", 150, nil},
		"fx":      {"Per glyph", 0, RiseIn(0.3, 0.05, 20)},
		"wrapfx":  {"wrapped text with an effect on it", 120, Chain(Wave(0.4, 3), TypeOn(0.5, 30))},
	} {
		a, b := NewPixels(220, 160, bg), NewPixels(220, 160, bg)
		for _, al := range []Align{Left, Center, Right} {
			tx := Text{Font: testTheme.Body, Size: 20, Color: testTheme.Text, Align: al, MaxW: tc.maxW, Glow: 0.5, FX: tc.fx}
			rt := Rich{Font: testTheme.Body, Size: 20, Color: testTheme.Text, Align: al, MaxW: tc.maxW, Glow: 0.5, FX: tc.fx}
			w1, h1 := tx.Draw(a, tc.s, 100, 10)
			w2, h2 := rt.Draw(b, []Span{{Text: tc.s}}, 100, 10)
			if w1 != w2 || h1 != h2 {
				t.Errorf("%s/%d: size %v,%v, want Text's %v,%v", name, al, w2, h2, w1, h1)
			}
		}
		if litPixels(a, bg) == 0 {
			t.Fatalf("%s: drew nothing", name)
		}
		samePixels(t, a, b)
	}
}

func TestRichWrapsAcrossSpans(t *testing.T) {
	f := testTheme.Body
	spans := []Span{
		{Text: "the quick "},
		{Text: "bold", Font: testTheme.Display},
		{Text: "ish brown "},
		{Text: "fox", Color: &testTheme.Accent},
		{Text: " jumps over"},
	}
	r := Rich{Font: f, Size: 20, Color: testTheme.Text}
	full, _ := r.Measure(spans)
	r.MaxW = full * 0.55
	lines := richLines(r, spans)
	if len(lines) < 2 {
		t.Fatalf("got %q, want a wrapped block", lines)
	}
	if got, want := strings.Join(lines, " "), "the quick boldish brown fox jumps over"; got != want {
		t.Errorf("wrapped text %q, want %q", got, want)
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "ish") || strings.HasSuffix(l, "bold") {
			t.Errorf("a word split across spans broke apart: %q", lines)
		}
	}
	w, _ := r.Measure(spans)
	if w > r.MaxW {
		t.Errorf("block is %.1fpx wide, want <= %.1f", w, r.MaxW)
	}
}

func TestRichKeepsHardBreaksAndSpaces(t *testing.T) {
	r := Rich{Font: testTheme.Body, Size: 20}
	spans := []Span{{Text: "one\ntw"}, {Text: "o  three\n"}, {Text: "  indented"}}
	want := []string{"one", "two  three", "  indented"}
	if got := richLines(r, spans); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	r.MaxW = 1000
	if got := richLines(r, spans); !slices.Equal(got, []string{"one", "two three", "indented"}) {
		t.Errorf("wrapped: got %q", got)
	}
}

func TestRichSharesABaseline(t *testing.T) {
	// "HH" has no descenders, so the lowest lit row of each run is its baseline.
	bg := testTheme.Background
	p := NewPixels(200, 60, bg)
	r := Rich{Font: testTheme.Body, Size: 24, Color: testTheme.Text}
	spans := []Span{{Text: "HH"}, {Text: "HH", Font: testTheme.Mono}, {Text: "HH", Font: testTheme.Display}}
	r.Draw(p, spans, 4, 10)
	var bottoms []int
	x := 4.0
	for _, s := range spans {
		f := s.Font
		if f == nil {
			f = r.Font
		}
		w := f.Measure(s.Text, 24)
		bottom := -1
		for y := range p.H {
			for px := int(x); px < int(x+w); px++ {
				if p.At(px, y) != bg {
					bottom = y
				}
			}
		}
		bottoms = append(bottoms, bottom)
		x += w
	}
	if bottoms[0] < 0 || bottoms[0] != bottoms[1] || bottoms[1] != bottoms[2] {
		t.Errorf("baselines differ across fonts: %v", bottoms)
	}
	if want := 10 + r.Baseline(); float64(bottoms[0]) < want-2 || float64(bottoms[0]) > want+1 {
		t.Errorf("baseline at row %d, want about %.1f", bottoms[0], want)
	}
}

func TestRichSpanStyles(t *testing.T) {
	bg := testTheme.Background
	red, mark := RGB{255, 0, 0}, RGB{0, 0, 255}
	at := func(spans []Span) *Pixels {
		p := NewPixels(160, 60, bg)
		Rich{Font: testTheme.Display, Size: 30, Color: testTheme.Text}.Draw(p, spans, 10, 10)
		return p
	}
	count := func(p *Pixels, c RGB) (n int) {
		for _, px := range p.Pix {
			if px == c {
				n++
			}
		}
		return n
	}

	if n := count(at([]Span{{Text: "HI", Color: &red}}), red); n == 0 {
		t.Error("a span's Color never reached the canvas")
	}
	plain, under, strike := at([]Span{{Text: "HI"}}), at([]Span{{Text: "HI", Underline: true}}), at([]Span{{Text: "HI", Strike: true}})
	if litPixels(under, bg) <= litPixels(plain, bg) || litPixels(strike, bg) <= litPixels(plain, bg) {
		t.Errorf("underline/strike added no ink: %d, %d vs %d", litPixels(under, bg), litPixels(strike, bg), litPixels(plain, bg))
	}
	if samePixelsCount(under, strike) {
		t.Error("underline and strike drew the same thing")
	}
	// The plate fills solid behind the ink, in its own color.
	if n := count(at([]Span{{Text: "HI", Mark: &mark}}), mark); n < 100 {
		t.Errorf("mark painted only %d pixels", n)
	}
	// Ink stays on top of the plate.
	p := at([]Span{{Text: "HI", Mark: &mark}})
	if count(p, testTheme.Text) == 0 {
		t.Error("the mark hid the text")
	}
	// A glow takes each span's own color.
	g := NewPixels(160, 60, bg)
	Rich{Font: testTheme.Display, Size: 30, Color: testTheme.Text, Glow: 1}.Draw(g, []Span{{Text: "HI", Color: &red}}, 10, 10)
	if px := g.At(8, 30); px.R <= px.B || px.R <= bg.R {
		t.Errorf("glow next to a red span is %v, want reddish", px)
	}
}

func samePixelsCount(a, b *Pixels) bool { return slices.Equal(a.Pix, b.Pix) }

func TestRichFXIndexesMatchText(t *testing.T) {
	record := func(idx *[]int) GlyphEffect {
		return func(i int) GlyphFX { *idx = append(*idx, i); return GlyphFX{Alpha: 1} }
	}
	var a, b []int
	p := NewPixels(200, 100, testTheme.Background)
	Text{Font: testTheme.Body, Size: 14, Color: testTheme.Text, FX: record(&a)}.Draw(p, "ab cd\nef", 0, 0)
	Rich{Font: testTheme.Body, Size: 14, Color: testTheme.Text, FX: record(&b)}.
		Draw(p, []Span{{Text: "ab "}, {Text: "c", Font: testTheme.Mono}, {Text: "d\ne"}, {Text: "f"}}, 0, 0)
	if !slices.Equal(a, b) {
		t.Errorf("Rich indexes %v, Text indexes %v", b, a)
	}
}

func TestRichDrawMidCentersInk(t *testing.T) {
	bg := testTheme.Background
	p := NewPixels(120, 60, bg)
	Rich{Font: testTheme.Body, Size: 20, Color: testTheme.Text}.DrawMid(p, []Span{{Text: "HH"}, {Text: "HH", Font: testTheme.Mono}}, 4, 30)
	top, bottom := -1, -1
	for y := range p.H {
		for x := range p.W {
			if p.At(x, y) != bg {
				if top < 0 {
					top = y
				}
				bottom = y
			}
		}
	}
	if mid := float64(top+bottom+1) / 2; mid < 29 || mid > 31 {
		t.Errorf("ink spans rows %d..%d, centered on %.1f, want 30", top, bottom, mid)
	}
}

func TestRichRequiresFont(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Font is required") {
			t.Errorf("panic = %v", r)
		}
	}()
	Rich{Size: 10}.Draw(NewPixels(10, 10, testTheme.Background), []Span{{Text: "x"}}, 0, 0)
}

func TestRichEmptyIsHarmless(t *testing.T) {
	p := NewPixels(40, 40, testTheme.Background)
	r := Rich{Font: testTheme.Body, Size: 12, Color: testTheme.Text}
	if w, h := r.Draw(p, nil, 0, 0); w != 0 || h != float64(12)*DefaultLeading {
		t.Errorf("empty block is %vx%v", w, h)
	}
	if litPixels(p, testTheme.Background) != 0 {
		t.Error("an empty block drew pixels")
	}
}

func TestRichFitRespectsBox(t *testing.T) {
	spans := []Span{{Text: "of weekly query volume "}, {Text: "comes", Font: testTheme.Display}, {Text: " from agents, "}, {Text: "mostly", Font: testTheme.Mono}}
	r := Rich{Font: testTheme.Body}
	for _, box := range [][2]float64{{100, 60}, {200, 40}, {60, 300}} {
		size := r.Fit(spans, box[0], box[1], 40)
		if size > 40 || size < minFitSize {
			t.Fatalf("size %d out of range", size)
		}
		r.Size, r.MaxW = size, box[0]
		w, h := r.Measure(spans)
		if w > box[0] || h > box[1] {
			t.Errorf("box %v: size %d measures %.1fx%.1f", box, size, w, h)
		}
		// One size up must not fit, or Fit left room on the table.
		if size < 40 {
			r.Size = size + 1
			if w, h := r.Measure(spans); w <= box[0] && h <= box[1] {
				t.Errorf("box %v: size %d also fits", box, size+1)
			}
		}
	}
	if r.Fit(spans, 100, 60, 40) != r.Fit(spans, 100, 60, 40) {
		t.Error("Fit is not stable across calls")
	}
	if big, small := r.Fit(spans, 300, 100, 60), r.Fit(spans, 100, 100, 60); big < small {
		t.Errorf("a wider box fits size %d, a narrower one %d", big, small)
	}
}

func TestRichFitWrapsRatherThanOverflows(t *testing.T) {
	spans := []Span{{Text: "a line that is far too long "}, {Text: "to fit", Font: testTheme.Mono}, {Text: " on one row"}}
	r := Rich{Font: testTheme.Body}
	size := r.Fit(spans, 60, 10, 20)
	r.Size, r.MaxW = size, 60
	if w, _ := r.Measure(spans); w > 60 {
		t.Errorf("fallback is %.1fpx wide at size %d, want <= 60", w, size)
	}
}

func TestRichLayoutIsCachedAcrossStyles(t *testing.T) {
	r := Rich{Font: testTheme.Body, Size: 18, MaxW: 90}
	a := []Span{{Text: "same words, different paint"}}
	b := []Span{{Text: "same words, different paint", Color: &testTheme.Warn, Underline: true}}
	la, _ := r.layout(a)
	lb, _ := r.layout(b)
	if la != lb {
		t.Error("styling changed the layout key")
	}
	c := []Span{{Text: "same words, different paint", Font: testTheme.Mono}}
	if lc, _ := r.layout(c); lc == la {
		t.Error("a different font reused the layout")
	}
}

func TestParseSpans(t *testing.T) {
	th := testTheme
	type want struct {
		text string
		font *Font
		col  *RGB
		mark bool
		u, s bool
	}
	hex := Hex("#336699")
	mut, acc, bg := th.Muted, th.Accent, th.Background
	for name, tc := range map[string]struct {
		in  string
		out []want
	}{
		"plain":                   {"hello", []want{{text: "hello"}}},
		"empty":                   {"", nil},
		"bold":                    {"a *b* c", []want{{text: "a "}, {text: "b", font: th.Display}, {text: " c"}}},
		"bold in a word":          {"*bo*ld", []want{{text: "bo", font: th.Display}, {text: "ld"}}},
		"muted":                   {"x _y_", []want{{text: "x "}, {text: "y", col: &mut}}},
		"code":                    {"run `go *test*` now", []want{{text: "run "}, {text: "go *test*", font: th.Mono, mark: true}, {text: " now"}}},
		"color":                   {"{accent:hot} {#369:cool}", []want{{text: "hot", col: &acc}, {text: " "}, {text: "cool", col: &hex}}},
		"decorations":             {"{u:a}{s:b}", []want{{text: "a", u: true}, {text: "b", s: true}}},
		"mark":                    {"{mark:hi}", []want{{text: "hi", col: &bg, mark: true}}},
		"nested":                  {"{accent:a {u:b} c}", []want{{text: "a ", col: &acc}, {text: "b", col: &acc, u: true}, {text: " c", col: &acc}}},
		"escapes":                 {`\*not bold\* a\_b \{accent:x\} \\ \q`, []want{{text: `*not bold* a_b {accent:x} \ \q`}}},
		"escaped tick":            {"`a\\`b`", []want{{text: "a`b", font: th.Mono, mark: true}}},
		"code keeps tags":         {"`{accent:x}`", []want{{text: "{accent:x}", font: th.Mono, mark: true}}},
		"unknown tag":             {"{nope:x} } {", []want{{text: "{nope:x} } {"}}},
		"unknown tag in a tag":    {"{accent:a {unknown:b} c}", []want{{text: "a {unknown:b} c", col: &acc}}},
		"braces in a tag":         {"{accent:map{k}} after", []want{{text: "map{k}", col: &acc}, {text: " after"}}},
		"deep literal braces":     {"{accent:a{b{c}d}e} f", []want{{text: "a{b{c}d}e", col: &acc}, {text: " f"}}},
		"tag in literal braces":   {"{x{accent:y}z}", []want{{text: "{x"}, {text: "y", col: &acc}, {text: "z}"}}},
		"stray close":             {"a } b {accent:c} }", []want{{text: "a } b "}, {text: "c", col: &acc}, {text: " }"}}},
		"escaped braces in a tag": {`{accent:a \} b \{ c}!`, []want{{text: "a } b { c", col: &acc}, {text: "!"}}},
		"unclosed":                {"a *b c", []want{{text: "a "}, {text: "b c", font: th.Display}}},
		"newline":                 {"a\n_b_", []want{{text: "a\n"}, {text: "b", col: &mut}}},
	} {
		got := ParseSpans(tc.in, th)
		if len(got) != len(tc.out) {
			t.Errorf("%s: got %d spans %+v, want %d", name, len(got), got, len(tc.out))
			continue
		}
		for i, w := range tc.out {
			g := got[i]
			if g.Text != w.text || g.Font != w.font || (g.Mark != nil) != w.mark || g.Underline != w.u || g.Strike != w.s ||
				(g.Color == nil) != (w.col == nil) || (w.col != nil && *g.Color != *w.col) {
				t.Errorf("%s: span %d = %+v, want %+v", name, i, g, w)
			}
		}
	}
}

func TestParseSpansDrawsLikeHandBuiltSpans(t *testing.T) {
	th := testTheme
	r := Rich{Font: th.Body, Size: 20, Color: th.Text, MaxW: 160}
	acc := th.Accent
	a, b := NewPixels(200, 120, th.Background), NewPixels(200, 120, th.Background)
	r.Draw(a, ParseSpans("some {accent:marked} text", th), 5, 5)
	r.Draw(b, []Span{{Text: "some "}, {Text: "marked", Color: &acc}, {Text: " text"}}, 5, 5)
	samePixels(t, a, b)
}
