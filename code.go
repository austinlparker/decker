package decker

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// SyntaxColors is the palette [Code] colors tokens with, set on
// [Theme.Syntax]. Any field left zero falls back to the color derived from the
// theme's own palette, so a theme can override just the roles it cares about.
type SyntaxColors struct {
	Keyword RGB // control flow and declarations: if, func, return
	String  RGB // string and character literals
	Comment RGB // comments
	Number  RGB // numeric literals and named constants
	Name    RGB // functions, builtins and decorators
	Type    RGB // type, class and namespace names
	Punct   RGB // operators and punctuation

	Added   RGB // background of a diff's "+" lines
	Removed RGB // background of a diff's "-" lines
}

// syntaxColors returns t's palette with every unset field filled from the
// theme's colors. Derived backgrounds are tints of Panel, the plate the code
// sits on.
func (t *Theme) syntaxColors() SyntaxColors {
	d := SyntaxColors{
		Keyword: t.Accent,
		String:  t.Good,
		Comment: t.Muted,
		Number:  t.Warn,
		Name:    t.Accent2,
		Type:    Mix(t.Accent2, t.Text, 0.4),
		Punct:   Mix(t.Muted, t.Text, 0.5),
		Added:   Mix(t.Panel, t.Good, 0.22),
		Removed: Mix(t.Panel, t.Warn, 0.22),
	}
	s := t.Syntax
	if s == nil {
		return d
	}
	for _, f := range []struct{ dst, src *RGB }{
		{&d.Keyword, &s.Keyword}, {&d.String, &s.String}, {&d.Comment, &s.Comment},
		{&d.Number, &s.Number}, {&d.Name, &s.Name}, {&d.Type, &s.Type},
		{&d.Punct, &s.Punct}, {&d.Added, &s.Added}, {&d.Removed, &s.Removed},
	} {
		if *f.src != (RGB{}) {
			*f.dst = *f.src
		}
	}
	return d
}

// LineRange is a span of lines in a [Code] block, 1-based and inclusive. A
// range with To < From, such as the zero value, selects nothing.
type LineRange struct{ From, To int }

// span clips r to a block of n lines and returns its 0-based first line and
// one past its last, or ok false if nothing is left.
func (r LineRange) span(n int) (from, to int, ok bool) {
	from, to = max(r.From, 1)-1, min(r.To, n)
	return from, to, from < to
}

// Code draws source code on a rounded plate with syntax highlighting, and
// can pick out a few lines at each build step.
//
//	Code{Source: src, Lang: "go", Title: "main.go", LineNumbers: true,
//		Focus: []LineRange{{1, 3}, {5, 9}}, FirstStep: 1}.Draw(c, p, c.Rect(0.05, 0.2, 0.6, 0.7))
//
// The source is lexed once per (Source, Lang) and cached. Code assumes the
// theme's Mono font is monospaced.
type Code struct {
	Source string
	Lang   string // a chroma language name, alias or file extension; unknown or empty draws plain text

	LineNumbers bool
	Title       string // a filename tab above the code

	// Focus has one range per step, starting at FirstStep: at step
	// FirstStep+i the lines in Focus[i] are drawn at full strength and the rest
	// dimmed behind a highlight bar that glides from the previous range. A step
	// past the last range keeps the last; an empty range (the zero LineRange)
	// returns every line to normal. Before FirstStep, or with no Focus, every
	// line is normal.
	Focus     []LineRange
	FirstStep int

	// Diff reads Source as a unified diff: the first character of each line is
	// its marker. "+" and "-" lines get the added and removed backgrounds and
	// keep their marker in a gutter, " " marks context, and any other first
	// character is part of a context line. What follows the marker is lexed as
	// Lang.
	Diff bool

	// Size is the Mono size in pixels; 0 picks the largest that fits, but never
	// below c.SmallText(Mono) (past that the code is clipped to the plate).
	Size int
}

const (
	codeLeading = 1.25 // line height as a multiple of size; looser than prose so lines read apart
	codeDim     = 0.28 // strength of a line outside the focus
	codeEase    = 0.4  // seconds a focus change takes
	codeTabW    = 4    // columns a tab expands to
)

// codeRole is the palette entry a token is drawn with.
type codeRole uint8

const (
	rolePlain codeRole = iota
	roleKeyword
	roleString
	roleComment
	roleNumber
	roleName
	roleType
	rolePunct
)

// codeRun is a stretch of one line drawn in one color.
type codeRun struct {
	text string
	role codeRole
	cols int
}

type codeLine struct {
	runs   []codeRun
	marker byte // '+' or '-' in a diff, else 0
	cols   int
}

// lexedCode is a source after tab expansion, diff parsing and lexing: the
// pure part of drawing it.
type lexedCode struct {
	lines   []codeLine
	maxCols int
}

type lexKey struct {
	src, lang string
	diff      bool
}

var lexed = memo[lexKey, *lexedCode]{max: 256}

// expandTabs replaces each tab with codeTabW spaces, so columns are runes.
func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", strings.Repeat(" ", codeTabW)) }

// splitDiff separates each line's marker from its text. Only "+" and "-" are
// kept as markers; a leading space is dropped as the context marker.
func splitDiff(lines []string) (text []string, markers []byte) {
	text, markers = make([]string, len(lines)), make([]byte, len(lines))
	for i, l := range lines {
		text[i] = l
		if l == "" {
			continue
		}
		switch l[0] {
		case '+', '-':
			markers[i], text[i] = l[0], l[1:]
		case ' ':
			text[i] = l[1:]
		}
	}
	return text, markers
}

func lexCode(src, lang string, diff bool) *lexedCode {
	return lexed.get(lexKey{src, lang, diff}, func() *lexedCode {
		src = strings.TrimRight(strings.ReplaceAll(expandTabs(src), "\r\n", "\n"), "\n")
		lines := strings.Split(src, "\n")
		var markers []byte
		if diff {
			lines, markers = splitDiff(lines)
		}
		out := &lexedCode{lines: make([]codeLine, len(lines))}
		for i, runs := range tokenLines(strings.Join(lines, "\n"), lang, len(lines)) {
			l := codeLine{runs: runs}
			if diff {
				l.marker = markers[i]
			}
			for _, r := range runs {
				l.cols += r.cols
			}
			out.lines[i] = l
			out.maxCols = max(out.maxCols, l.cols)
		}
		return out
	})
}

// tokenLines lexes src as lang into n lines of runs, merging neighbors of the
// same role. A language chroma doesn't know, or a lexer that fails, yields
// plain text.
func tokenLines(src, lang string, n int) [][]codeRun {
	out := make([][]codeRun, n)
	add := func(line int, text string, role codeRole) {
		if text == "" || line >= n {
			return
		}
		runs := out[line]
		if k := len(runs) - 1; k >= 0 && runs[k].role == role {
			runs[k].text += text
			runs[k].cols += utf8.RuneCountInString(text)
			return
		}
		out[line] = append(runs, codeRun{text, role, utf8.RuneCountInString(text)})
	}
	var toks []chroma.Token
	if lang != "" {
		if lx := lexers.Get(lang); lx != nil {
			if it, err := lx.Tokenise(nil, src); err == nil {
				toks = it.Tokens()
			}
		}
	}
	if toks == nil {
		toks = []chroma.Token{{Type: chroma.Text, Value: src}}
	}
	line := 0
	for _, t := range toks {
		role := roleOf(t.Type)
		parts := strings.Split(t.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				line++
			}
			add(line, part, role)
		}
	}
	return out
}

// roleOf folds chroma's token taxonomy into the few roles a slide can read.
func roleOf(t chroma.TokenType) codeRole {
	switch {
	case t.InCategory(chroma.Comment):
		return roleComment
	case t == chroma.KeywordType:
		return roleType
	case t.InCategory(chroma.Keyword), t == chroma.OperatorWord, t == chroma.NameTag:
		return roleKeyword
	case t.InSubCategory(chroma.LiteralString):
		return roleString
	case t.InCategory(chroma.Literal), t == chroma.NameConstant:
		return roleNumber
	case t.InSubCategory(chroma.NameFunction), t.InSubCategory(chroma.NameBuiltin), t == chroma.NameDecorator:
		return roleName
	case t == chroma.NameClass, t == chroma.NameException, t == chroma.NameNamespace:
		return roleType
	case t.InCategory(chroma.Operator), t.InCategory(chroma.Punctuation):
		return rolePunct
	}
	return rolePlain
}

// gutter is the number of columns before the code: line numbers, then, in a
// diff, the marker column.
func (lx *lexedCode) gutter(lineNumbers, diff bool) (numCols, markCol, codeCol int) {
	if lineNumbers {
		numCols = len(strconv.Itoa(len(lx.lines)))
		markCol = numCols + 2
	}
	codeCol = markCol
	if diff {
		codeCol += 2
	}
	return numCols, markCol, codeCol
}

type codeFitKey struct {
	f              *Font
	src, lang      string
	lineNumbers    bool
	diff           bool
	w, h           float64
	minSize, maxSz int
}

var codeFits = memo[codeFitKey, int]{max: 256}

// fitCode returns the largest size in [minSize, maxSize] at which the block
// fits w×h, or minSize if none does.
func (k Code) fitCode(f *Font, lx *lexedCode, w, h float64, minSize, maxSize int) int {
	return codeFits.get(codeFitKey{f, k.Source, k.Lang, k.LineNumbers, k.Diff, w, h, minSize, maxSize}, func() int {
		_, _, cols := lx.gutter(k.LineNumbers, k.Diff)
		cols += lx.maxCols
		for size := maxSize; size > minSize; size-- {
			if float64(cols)*f.Measure("0", size) <= w && float64(len(lx.lines))*float64(size)*codeLeading <= h {
				return size
			}
		}
		return minSize
	})
}

// focusState says which lines are lit and where the highlight bar sits.
type focusState struct {
	cur, prev     [2]int // 0-based half-open line spans; empty if none
	curOK, prevOK bool
	e             float64 // 0..1 progress from prev to cur
}

// focusAt maps the step to the ranges it moves between. Both are "none"
// (everything normal) before FirstStep or without Focus.
func (k Code) focusAt(c Ctx, n int) focusState {
	i := c.Step - k.FirstStep
	if i < 0 || len(k.Focus) == 0 {
		return focusState{}
	}
	pick := func(i int) (s [2]int, ok bool) {
		if i < 0 {
			return s, false
		}
		from, to, ok := k.Focus[min(i, len(k.Focus)-1)].span(n)
		return [2]int{from, to}, ok
	}
	var f focusState
	f.cur, f.curOK = pick(i)
	f.prev, f.prevOK = pick(i - 1)
	f.e = Ease(c.Since(k.FirstStep+i), codeEase)
	return f
}

// weight is how strongly line l is lit, 0 (dimmed) to 1.
func (f focusState) weight(l int) float64 {
	in := func(s [2]int, ok bool) float64 {
		if !ok || (l >= s[0] && l < s[1]) {
			return 1
		}
		return 0
	}
	return Lerp(in(f.prev, f.prevOK), in(f.cur, f.curOK), f.e)
}

// bar returns the highlight's first and last line edges, in fractional lines,
// and its opacity; the bar slides between ranges and fades in or out at the
// ends of the focus.
func (f focusState) bar() (y0, y1, alpha float64) {
	switch {
	case f.curOK && f.prevOK:
		return Lerp(float64(f.prev[0]), float64(f.cur[0]), f.e), Lerp(float64(f.prev[1]), float64(f.cur[1]), f.e), 1
	case f.curOK:
		return float64(f.cur[0]), float64(f.cur[1]), f.e
	case f.prevOK:
		return float64(f.prev[0]), float64(f.prev[1]), 1 - f.e
	}
	return 0, 0, 0
}

// Draw renders the block inside r, anchored at its top-left, and returns the
// size of the plate. The plate hugs the code, and is never larger than r.
func (k Code) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	th := c.Theme
	f := th.Mono
	lx := lexCode(k.Source, k.Lang, k.Diff)
	pal := th.syntaxColors()

	pad := c.Unit(0.025)
	tabSize := c.SmallText(f)
	head := pad // space above the code
	if k.Title != "" {
		head = pad*0.6 + float64(tabSize)*1.8 + pad*0.8
	}

	size := k.Size
	if size <= 0 {
		size = k.fitCode(f, lx, r.W-2*pad, r.H-head-pad, tabSize, c.Size(0.1))
	}
	rf, rs := f.resolve(size)
	adv := rf.Measure("0", rs)
	lineH := float64(rs) * codeLeading
	numCols, markCol, codeCol := lx.gutter(k.LineNumbers, k.Diff)

	w = min(float64(codeCol+lx.maxCols)*adv+2*pad, r.W)
	if k.Title != "" {
		w = min(max(w, f.Measure(k.Title, tabSize)+float64(tabSize)*1.4+2*pad), r.W)
	}
	h = min(head+float64(len(lx.lines))*lineH+pad, r.H)

	fillPlate(p, r.X, r.Y, w, h, min(c.Unit(0.03), w/3, h/3), th.Panel)
	if k.Title != "" {
		k.drawTab(c, p, r.X+pad, r.Y+pad*0.6, float64(tabSize))
	}

	left, top := r.X+pad, r.Y+head
	bottom, right := r.Y+h-pad*0.3, r.X+w-pad*0.3
	inset := pad * 0.4
	foc := k.focusAt(c, len(lx.lines))

	// Backgrounds first: diff stripes, then the focus bar over them.
	if k.Diff {
		for i, l := range lx.lines {
			y := top + float64(i)*lineH
			if l.marker == 0 || y+lineH > bottom {
				continue
			}
			bg := pal.Added
			if l.marker == '-' {
				bg = pal.Removed
			}
			p.Rect(r.X+inset, y, w-2*inset, lineH, bg, Lerp(0.45, 1, foc.weight(i)))
		}
	}
	if y0, y1, a := foc.bar(); a > 0 {
		by0, by1 := top+y0*lineH, min(top+y1*lineH, bottom)
		if by1 > by0 {
			p.Rect(r.X+inset, by0, w-2*inset, by1-by0, Mix(th.Panel, th.Accent, 0.16), a)
			p.Rect(r.X+inset, by0, max(c.Unit(0.006), 1.5), by1-by0, th.Accent, a)
		}
	}

	// The 0.08 matches Text's nudge that centers ink in the line box.
	baseOff := rf.Ascent(rs) + (lineH-float64(rs))/2 - float64(rs)*0.08
	numCol := Mix(th.Panel, th.Muted, 0.6)
	for i, l := range lx.lines {
		y := top + float64(i)*lineH
		if y+lineH > bottom {
			break
		}
		base := y + baseOff
		lit := Lerp(codeDim, 1, foc.weight(i))
		dim := func(col RGB) RGB { return Mix(th.Panel, col, lit) }
		if k.LineNumbers {
			n := strconv.Itoa(i + 1)
			drawMono(p, rf, rs, n, left+float64(numCols-len(n))*adv, base, adv, right, dim(numCol))
		}
		if l.marker != 0 {
			col := th.Good
			if l.marker == '-' {
				col = th.Warn
			}
			drawMono(p, rf, rs, string(rune(l.marker)), left+float64(markCol)*adv, base, adv, right, dim(col))
		}
		col := codeCol
		for _, run := range l.runs {
			drawMono(p, rf, rs, run.text, left+float64(col)*adv, base, adv, right, dim(pal.color(run.role, th.Text)))
			col += run.cols
		}
	}
	return w, h
}

// color returns the palette color for role, or plain for no role.
func (s SyntaxColors) color(role codeRole, plain RGB) RGB {
	switch role {
	case roleKeyword:
		return s.Keyword
	case roleString:
		return s.String
	case roleComment:
		return s.Comment
	case roleNumber:
		return s.Number
	case roleName:
		return s.Name
	case roleType:
		return s.Type
	case rolePunct:
		return s.Punct
	}
	return plain
}

// drawTab draws the filename tab with its top-left at (x, y); size is the
// small Mono size.
func (k Code) drawTab(c Ctx, p *Pixels, x, y, size float64) {
	th := c.Theme
	f := th.Mono
	w, h := f.Measure(k.Title, int(size))+size*1.4, size*1.8
	p.RoundRect(x, y, w, h, min(c.Unit(0.015), h/2), 0, Mix(th.Panel, th.Text, 0.09), 1)
	p.Rect(x+size*0.4, y+h-max(c.Unit(0.005), 1.5), w-size*0.8, max(c.Unit(0.005), 1.5), th.Accent, 1)
	rf, rs := f.resolve(int(size))
	drawMono(p, rf, rs, k.Title, x+size*0.7, y+h/2+rf.CapHeight(rs)/2, rf.Measure("0", rs), x+w, th.Text)
}

// drawMono paints s one glyph per column of width adv starting at x, with the
// baseline at base, stopping at xmax. It blits cached glyph masks straight
// into p: a Text per token run would allocate a coverage buffer each, every
// frame, for every run of every line.
func drawMono(p *Pixels, f *Font, size int, s string, x, base, adv, xmax float64, col RGB) {
	oy0 := int(math.Round(base))
	for _, r := range s {
		if r != ' ' && x+adv <= xmax {
			ix := math.Floor(x)
			g := f.glyphAt(r, size, int((x-ix)*subpixel))
			blitGlyph(p, g, int(ix)+g.ox, oy0+g.oy, col)
		}
		x += adv
	}
}

// blitGlyph blends g's mask in col with its top-left at (ox, oy), the same
// coverage Text.Draw paints for a lone glyph.
func blitGlyph(p *Pixels, g *glyph, ox, oy int, col RGB) {
	if g.a == nil {
		return
	}
	for y := 0; y < g.h; y++ {
		py := oy + y
		if py < 0 || py >= p.H {
			continue
		}
		row := p.Pix[py*p.W : (py+1)*p.W]
		for x := 0; x < g.w; x++ {
			px := ox + x
			if px < 0 || px >= p.W {
				continue
			}
			a := float32(g.a[y*g.w+x]) / 255
			if a <= 0.002 {
				continue
			}
			d := &row[px]
			*d = RGB{d.R + (col.R-d.R)*a, d.G + (col.G-d.G)*a, d.B + (col.B-d.B)*a}
		}
	}
}

// fillPlate fills an opaque rounded rectangle snapped to whole pixels. RoundRect
// measures a distance at every pixel of its box, and a code plate is large and
// nearly all interior, so only two caps one corner-diameter tall go through it
// (their outer halves are exactly the plate's corners) and the rows between
// are copied over their inner halves.
func fillPlate(p *Pixels, x, y, w, h, r float64, col RGB) {
	x0, y0 := math.Round(x), math.Round(y)
	w, h = math.Round(x+w)-x0, math.Round(y+h)-y0
	r = math.Round(min(r, w/2, h/2))
	if r < 1 || h < 4*r { // the caps would overlap and blend twice
		p.RoundRect(x0, y0, w, h, r, 0, col, 1)
		return
	}
	p.RoundRect(x0, y0, w, 2*r, r, 0, col, 1)
	p.RoundRect(x0, y0+h-2*r, w, 2*r, r, 0, col, 1)
	for py := max(int(y0+r), 0); py < min(int(y0+h-r), p.H); py++ {
		row := p.Pix[py*p.W : (py+1)*p.W]
		for px := max(int(x0), 0); px < min(int(x0+w), p.W); px++ {
			row[px] = col
		}
	}
}
