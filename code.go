package decker

import (
	"fmt"
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
	// below c.SmallText(Mono) unless Overflow is CodeShrink.
	Size int

	// Overflow is what the block does with source that doesn't fit its rect
	// at the smallest readable size: clip it (the default), shrink below
	// that size, or scroll to follow Focus. Code.Measure says beforehand
	// whether it fits.
	Overflow CodeOverflow

	// Excerpt shows only these lines of Source, numbered and focused as in
	// the whole source, so a long file can be shown a part at a time with
	// its real line numbers. The zero LineRange shows every line.
	Excerpt LineRange
}

// CodeOverflow is what a [Code] block does with source that doesn't fit its
// rect at the smallest readable size.
type CodeOverflow int

const (
	// CodeClip sets the code at the smallest readable size and clips what
	// still doesn't fit; a review reports it as "code-clipped".
	CodeClip CodeOverflow = iota
	// CodeShrink keeps shrinking until every line and column fits, below the
	// readable size if it must; a review reports text that small as
	// "text-small".
	CodeShrink
	// CodeScroll keeps the readable size and shows the lines that fit,
	// scrolled to keep the current Focus range in view and gliding with its
	// highlight, so a long file can be walked through over builds. A review
	// reports a focus range taller than the view, or no Focus at all, as
	// "code-clipped".
	CodeScroll
)

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

// codeGutter is the number of columns before the code: line numbers up to
// last, then, in a diff, the marker column.
func codeGutter(last int, lineNumbers, diff bool) (numCols, markCol, codeCol int) {
	if lineNumbers {
		numCols = len(strconv.Itoa(last))
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
	cols, lines    int
	w, h           float64
	minSize, maxSz int
}

var codeFits = memo[codeFitKey, int]{max: 256}

// fitCode returns the largest size in [minSize, maxSize] at which lines of
// cols columns, gutter included, fit w×h, or minSize if none does.
func fitCode(f *Font, cols, lines int, w, h float64, minSize, maxSize int) int {
	return codeFits.get(codeFitKey{f, cols, lines, w, h, minSize, maxSize}, func() int {
		return largestSize(max(maxSize, minSize), minSize, func(size int) bool {
			return float64(cols)*f.Measure("0", size) <= w && float64(lines)*float64(size)*codeLeading <= h
		})
	})
}

// focusState says which lines are lit and where the highlight bar sits.
type focusState struct {
	cur, prev     [2]int // 0-based half-open line spans; empty if none
	curOK, prevOK bool
	e             float64 // 0..1 progress from prev to cur
}

// focusAt maps the step to the ranges it moves between, as 0-based spans of
// the n lines shown, the first of which is source line first+1. Both are
// "none" (everything normal) before FirstStep or without Focus.
func (k Code) focusAt(c Ctx, first, n int) focusState {
	i := c.Step - k.FirstStep
	if i < 0 || len(k.Focus) == 0 {
		return focusState{}
	}
	pick := func(i int) (s [2]int, ok bool) {
		if i < 0 {
			return s, false
		}
		r := k.Focus[min(i, len(k.Focus)-1)]
		from, to, ok := LineRange{r.From - first, r.To - first}.span(n)
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

// CodeLayout is how a [Code] block fits a rect: the size its text is set at,
// the plate it draws and how much of the source shows. [Code.Measure]
// returns it without drawing, so a slide can pick a bigger rect, a smaller
// excerpt or a split across builds before anything is lost.
type CodeLayout struct {
	Size int     // the Mono size in pixels: Code.Size, or the size fitted to the rect
	W, H float64 // the plate, which is never larger than the rect

	// NeedW and NeedH are the plate that would show every line and column
	// at Size.
	NeedW, NeedH float64

	// Head is the plate's height above the first line: padding, plus the
	// filename tab when there is a Title.
	Head float64

	Lines, Shown    int // lines in the source (or Excerpt); lines in view
	Cols, ShownCols int // columns of the widest line, gutter included; columns drawn
}

// Fits reports whether every line and column of the source shows.
func (l CodeLayout) Fits() bool { return l.Shown >= l.Lines && l.ShownCols >= l.Cols }

// codeGeom is a CodeLayout with what Draw needs to place the text.
type codeGeom struct {
	CodeLayout
	lx                        *lexedCode
	lines                     []codeLine // the lines shown: all, or the Excerpt
	first                     int        // lines[0]'s index in the source
	rf                        *Font
	pad, adv, lineH           float64
	tabSize                   int
	numCols, markCol, codeCol int

	foc    focusState
	scroll bool    // CodeScroll with more lines than fit
	off    float64 // lines scrolled past, fractional while gliding
}

// Measure returns how the block fits r, as Draw would draw it there.
func (k Code) Measure(c Ctx, r Rect) CodeLayout {
	if r.W <= 0 || r.H <= 0 {
		g := k.geom(c, r)
		g.W, g.H, g.Shown, g.ShownCols = 0, 0, 0, 0
		return g.CodeLayout
	}
	return k.geom(c, r).CodeLayout
}

func (k Code) geom(c Ctx, r Rect) codeGeom {
	f := c.Theme.Mono
	g := codeGeom{lx: lexCode(k.Source, k.Lang, k.Diff)}
	g.lines = g.lx.lines
	maxCols := g.lx.maxCols
	if from, to, ok := k.Excerpt.span(len(g.lines)); ok {
		g.lines, g.first, maxCols = g.lines[from:to], from, 0
		for _, l := range g.lines {
			maxCols = max(maxCols, l.cols)
		}
	}
	g.pad = c.Unit(0.025)
	g.tabSize = c.SmallText(f)
	g.Head = g.pad // space above the code
	if k.Title != "" {
		g.Head = g.pad*0.6 + float64(g.tabSize)*1.8 + g.pad*0.8
	}
	g.numCols, g.markCol, g.codeCol = codeGutter(g.first+len(g.lines), k.LineNumbers, k.Diff)

	size := k.Size
	if size <= 0 {
		least := g.tabSize
		if k.Overflow == CodeShrink {
			least = minFitSize
		}
		size = fitCode(f, g.codeCol+maxCols, len(g.lines), r.W-2*g.pad, r.H-g.Head-g.pad, least, c.Size(0.1))
	}
	g.rf, g.Size = f.resolve(size)
	g.adv = g.rf.Measure("0", g.Size)
	g.lineH = float64(g.Size) * codeLeading

	g.NeedW = float64(g.codeCol+maxCols)*g.adv + 2*g.pad
	if k.Title != "" {
		g.NeedW = max(g.NeedW, f.Measure(k.Title, g.tabSize)+float64(g.tabSize)*1.4+2*g.pad)
	}
	g.NeedH = g.Head + float64(len(g.lines))*g.lineH + g.pad
	g.W, g.H = min(g.NeedW, r.W), min(g.NeedH, r.H)

	// The same tests Draw makes line by line and glyph by glyph.
	g.Lines, g.Cols = len(g.lines), g.codeCol+maxCols
	top, bottom := r.Y+g.Head, r.Y+g.H-g.pad*0.3
	for g.Shown < g.Lines && top+float64(g.Shown)*g.lineH+g.lineH <= bottom {
		g.Shown++
	}
	left, right := r.X+g.pad, r.X+g.W-g.pad*0.3
	for g.ShownCols < g.Cols && left+float64(g.ShownCols)*g.adv+g.adv <= right {
		g.ShownCols++
	}

	g.foc = k.focusAt(c, g.first, len(g.lines))
	g.scroll = k.Overflow == CodeScroll && g.Shown < g.Lines && g.Shown > 0
	if g.scroll {
		i := c.Step - k.FirstStep
		prev, prevOK := k.held(i-1, g.first, g.Lines)
		cur, curOK := k.held(i, g.first, g.Lines)
		g.off = g.scrollOffset(prev, prevOK, cur, curOK)
	}
	return g
}

// held is the span, among the n lines shown from source line first+1, of the
// last Focus range at or before index i that selects any lines: where
// CodeScroll stays through an empty range and once the steps run past the
// end of Focus. ok is false before the first such range.
func (k Code) held(i, first, n int) (s [2]int, ok bool) {
	for i = min(i, len(k.Focus)-1); i >= 0; i-- {
		r := k.Focus[i]
		if from, to, ok := (LineRange{r.From - first, r.To - first}).span(n); ok {
			return [2]int{from, to}, true
		}
	}
	return s, false
}

// scrollOffset is how many lines CodeScroll has scrolled past: enough to
// center the held focus range in view, gliding from the range held before
// it as the highlight does, and the top before any.
func (g codeGeom) scrollOffset(prev [2]int, prevOK bool, cur [2]int, curOK bool) float64 {
	place := func(s [2]int) float64 {
		n := s[1] - s[0]
		at := s[0]
		if n < g.Shown {
			at -= (g.Shown - n) / 2
		}
		return float64(min(max(at, 0), g.Lines-g.Shown))
	}
	from := 0.0
	if prevOK {
		from = place(prev)
	}
	to := from
	if curOK {
		to = place(cur)
	}
	return Lerp(from, to, g.foc.e)
}

// Draw renders the block inside r, anchored at its top-left, and returns the
// size of the plate. The plate hugs the code, and is never larger than r.
// Under review, lines or columns that don't fit are a "code-clipped" error.
func (k Code) Draw(c Ctx, p *Pixels, r Rect) (w, h float64) {
	if r.W <= 0 || r.H <= 0 {
		return 0, 0
	}
	th := c.Theme
	g := k.geom(c, r)
	if c.review != nil {
		k.report(c, r, g)
		k.reportCanvas(p, r, g)
		defer c.within(k.name(g.Lines), Rect{r.X, r.Y, g.W, g.H})()
		c.review.inkRect(c.review.scopeID, Rect{r.X, r.Y, g.W, g.H})
	}
	rf, rs, adv, lineH := g.rf, g.Size, g.adv, g.lineH
	numCols, markCol, codeCol := g.numCols, g.markCol, g.codeCol
	pad, head := g.pad, g.Head
	w, h = g.W, g.H
	pal := th.syntaxColors()

	fillPlate(p, r.X, r.Y, w, h, min(c.Unit(0.03), w/3, h/3), th.Panel)
	if k.Title != "" {
		k.drawTab(c, p, r.X+pad, r.Y+pad*0.6, float64(g.tabSize), r.X+w-pad, r.Y+h-pad*0.3)
	}

	left, top := r.X+pad, r.Y+head
	bottom, right := r.Y+h-pad*0.3, r.X+w-pad*0.3
	inset := pad * 0.4
	foc := g.foc
	// A scrolled view clips its lines to the rows in view, so a line gliding
	// past an edge is cut rather than drawn over the padding.
	clipTop, clipBottom := 0, p.H
	if g.scroll {
		top -= g.off * lineH
		clipTop, clipBottom = int(math.Round(r.Y+head)), int(math.Round(r.Y+head+float64(g.Shown)*lineH))
		bottom = math.Inf(1)
	}
	clipped := func(y0, y1 float64) (float64, float64) {
		if !g.scroll {
			return y0, y1
		}
		return max(y0, float64(clipTop)), min(y1, float64(clipBottom))
	}

	// Backgrounds first: diff stripes, then the focus bar over them.
	if k.Diff {
		for i, l := range g.lines {
			y := top + float64(i)*lineH
			if l.marker == 0 || y+lineH > bottom {
				continue
			}
			bg := pal.Added
			if l.marker == '-' {
				bg = pal.Removed
			}
			y0, y1 := clipped(y, y+lineH)
			if !g.scroll {
				p.Rect(r.X+inset, y, w-2*inset, lineH, bg, Lerp(0.45, 1, foc.weight(i)))
			} else if y1 > y0 {
				p.Rect(r.X+inset, y0, w-2*inset, y1-y0, bg, Lerp(0.45, 1, foc.weight(i)))
			}
		}
	}
	if y0, y1, a := foc.bar(); a > 0 {
		by0, by1 := top+y0*lineH, min(top+y1*lineH, bottom)
		by0, by1 = clipped(by0, by1)
		if by1 > by0 {
			p.Rect(r.X+inset, by0, w-2*inset, by1-by0, Mix(th.Panel, th.Accent, 0.16), a)
			p.Rect(r.X+inset, by0, max(c.Unit(0.006), 1.5), by1-by0, th.Accent, a)
		}
	}

	// The 0.08 matches Text's nudge that centers ink in the line box.
	baseOff := rf.Ascent(rs) + (lineH-float64(rs))/2 - float64(rs)*0.08
	numCol := Mix(th.Panel, th.Muted, 0.6)
	for i, l := range g.lines {
		y := top + float64(i)*lineH
		if y+lineH > bottom {
			break
		}
		if y+lineH <= float64(clipTop) || y >= float64(clipBottom) {
			continue
		}
		base := y + baseOff
		lit := Lerp(codeDim, 1, foc.weight(i))
		dim := func(col RGB) RGB { return Mix(th.Panel, col, lit) }
		mono := func(s string, col int, color RGB) {
			drawMono(p, rf, rs, s, left+float64(col)*adv, base, adv, right, clipTop, clipBottom, dim(color))
		}
		if k.LineNumbers {
			n := strconv.Itoa(g.first + i + 1)
			mono(n, numCols-len(n), numCol)
		}
		if l.marker != 0 {
			col := th.Good
			if l.marker == '-' {
				col = th.Warn
			}
			mono(string(rune(l.marker)), markCol, col)
		}
		col := codeCol
		for _, run := range l.runs {
			mono(run.text, col, pal.color(run.role, th.Text))
			col += run.cols
		}
	}
	return w, h
}

// name is how a review message refers to the block.
func (k Code) name(lines int) string {
	switch {
	case k.Title != "":
		return quoteText("Code", k.Title)
	case k.Lang != "":
		return fmt.Sprintf("Code (%s, %d lines)", k.Lang, lines)
	}
	return fmt.Sprintf("Code (%d lines)", lines)
}

// report records what of the block r cannot show, as its Overflow says.
func (k Code) report(c Ctx, r Rect, g codeGeom) {
	plate := Rect{r.X, r.Y, g.W, g.H}
	name := k.name(g.Lines)
	if g.Size < g.tabSize {
		c.review.add(SeverityWarning, "text-small", plate,
			fmt.Sprintf("%s is set at %dpx to fit; the smallest readable size here is %dpx", name, g.Size, g.tabSize))
	}
	var parts []string
	if g.Shown < g.Lines && (!g.scroll || len(k.Focus) == 0) {
		parts = append(parts, fmt.Sprintf("%d of %d lines visible (needs %.0fpx tall, has %.0fpx)", g.Shown, g.Lines, g.NeedH, r.H))
	}
	if g.ShownCols < g.Cols {
		parts = append(parts, fmt.Sprintf("%d of %d columns visible (needs %.0fpx wide, has %.0fpx)", g.ShownCols, g.Cols, g.NeedW, r.W))
	}
	if g.scroll && g.foc.curOK && g.foc.cur[1]-g.foc.cur[0] > g.Shown {
		f := g.foc.cur
		parts = append(parts, fmt.Sprintf("focus on lines %d–%d is %d lines; %d fit in view", g.first+f[0]+1, g.first+f[1], f[1]-f[0], g.Shown))
	}
	if len(parts) > 0 {
		at := fmt.Sprintf("at %dpx", g.Size)
		switch {
		case k.Size > 0:
			at += " (Code.Size)"
		case g.Size <= g.tabSize && k.Overflow != CodeShrink:
			at += ", the smallest readable size"
		}
		if k.Overflow == CodeScroll && len(k.Focus) == 0 && g.Shown < g.Lines {
			at += "; CodeScroll follows Focus, and there is none"
		}
		c.review.add(SeverityError, "code-clipped", plate, name+": "+strings.Join(parts, "; ")+" "+at)
	}
}

// reportCanvas reports the code's text running past the canvas edges, where
// drawMono cuts it off: a rect can fit its code and still hang off the slide.
func (k Code) reportCanvas(p *Pixels, r Rect, g codeGeom) {
	left, top := r.X+g.pad, r.Y+g.Head
	right, bottom := left+float64(g.ShownCols)*g.adv, top+float64(g.Shown)*g.lineH
	past := [4]int{
		int(math.Ceil(-left)), int(math.Ceil(-top)),
		int(math.Ceil(right)) - p.W, int(math.Ceil(bottom)) - p.H,
	}
	reportPast(p, past, g.Size, k.name(g.Lines), Rect{r.X, r.Y, g.W, g.H})
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
// small Mono size. The tab stays left of maxX and above maxY (the plate's
// padded edges): a title too long for the room is cut short with an ellipsis,
// and a tab with no room for even that is skipped. The cut is drawn as two
// strings so a frame allocates nothing.
func (k Code) drawTab(c Ctx, p *Pixels, x, y, size, maxX, maxY float64) {
	th := c.Theme
	rf, rs := th.Mono.resolve(int(size))
	adv := rf.Measure("0", rs)
	pad := size * 0.7
	h := size * 1.8
	room := int((maxX - x - 2*pad) / adv) // columns of title the tab can hold
	if room < 2 || y+h > maxY {
		return
	}
	title, cols, cut := k.Title, utf8.RuneCountInString(k.Title), false
	if cols > room {
		i := 0
		for range room - 1 {
			_, sz := utf8.DecodeRuneInString(title[i:])
			i += sz
		}
		title, cols, cut = title[:i], room, true
	}
	w := float64(cols)*adv + 2*pad
	line := max(c.Unit(0.005), 1.5)
	p.RoundRect(x, y, w, h, min(c.Unit(0.015), h/2), 0, Mix(th.Panel, th.Text, 0.09), 1)
	p.Rect(x+size*0.4, y+h-line, w-size*0.8, line, th.Accent, 1)
	base := y + h/2 + rf.CapHeight(rs)/2
	drawMono(p, rf, rs, title, x+pad, base, adv, x+w, 0, p.H, th.Text)
	if cut {
		drawMono(p, rf, rs, "\u2026", x+pad+float64(cols-1)*adv, base, adv, x+w, 0, p.H, th.Text)
	}
}

// drawMono paints s one glyph per column of width adv starting at x, with the
// baseline at base, stopping at xmax and showing only pixel rows y0 to y1. It
// blits cached glyph masks straight into p: a Text per token run would
// allocate a coverage buffer each, every frame, for every run of every line.
func drawMono(p *Pixels, f *Font, size int, s string, x, base, adv, xmax float64, y0, y1 int, col RGB) {
	oy0 := int(math.Round(base))
	for _, r := range s {
		if r != ' ' && x+adv <= xmax {
			ix := math.Floor(x)
			g := f.glyphAt(r, size, int((x-ix)*subpixel))
			blitGlyph(p, g, int(ix)+g.ox, oy0+g.oy, y0, y1, col)
		}
		x += adv
	}
}

// blitGlyph blends g's mask in col with its top-left at (ox, oy), the same
// coverage Text.Draw paints for a lone glyph, on pixel rows y0 to y1 only.
func blitGlyph(p *Pixels, g *glyph, ox, oy, y0, y1 int, col RGB) {
	if g.a == nil {
		return
	}
	for y := 0; y < g.h; y++ {
		py := oy + y
		if py < max(y0, 0) || py >= min(y1, p.H) {
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
