package decker

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"math"
	pathpkg "path"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed fonts/figlet/*.flf
var figFiles embed.FS

// The default block fonts use Spleen under BSD-2-Clause; see fonts/figlet.
var (
	// BlockShadow is Spleen 6x12 with a baked-in shaded shadow.
	BlockShadow = stockFig("Spleen 6x12 Shadow")
	// BlockSolid is Spleen 6x12, solid pixel lettering for headings.
	BlockSolid = stockFig("Spleen 6x12")
	// BlockSmall is Spleen 5x8, compact pixel lettering.
	BlockSmall = stockFig("Spleen 5x8")
	// BlockHuge is Spleen 8x16, larger pixel lettering.
	BlockHuge = stockFig("Spleen 8x16")
	// BlockFancy is Spleen 12x24, more detailed pixel lettering.
	BlockFancy = stockFig("Spleen 12x24")
)

// FigletFont is a parsed FIGlet (.flf) font, classic terminal-character type drawn
// by Block. Stock fonts' files and provenance are in fonts/figlet.
type FigletFont struct {
	Name string // display name; file loaders use the file name without .flf
	// Height is the row count the font file declares. Rendered lines trim
	// blank padding rows, so use Rows for the height of drawn text.
	Height  int
	kerning bool // false = full width (each glyph keeps its own box)
	glyphs  map[rune][][]rune
}

// hardBlank stands in for the font's hardblank, which stops glyphs kerning into
// each other, until layout turns it into a space.
const hardBlank = ' '

func stockFig(name string) *FigletFont { return StockFigletFont(name) }

// StockFigletFont loads a bundled FIGlet font by name, without the .flf suffix.
// StockFigletFontNames lists the available names. Load each font once, outside
// frame functions, and pass it to Block.Font or FitBlock. It panics for an
// unknown name. Bundled fonts retain BSD or OFL terms; see fonts/figlet.
func StockFigletFont(name string) *FigletFont {
	return LoadFigletFont(figFiles, "fonts/figlet/"+name+".flf")
}

// StockFigletFontNames returns the bundled FIGlet font names in alphabetical
// order, without .flf suffixes. The returned slice belongs to the caller.
func StockFigletFontNames() []string {
	paths, err := fs.Glob(figFiles, "fonts/figlet/*.flf")
	if err != nil {
		panic(err)
	}
	names := make([]string, len(paths))
	for i, path := range paths {
		names[i] = strings.TrimSuffix(pathpkg.Base(path), ".flf")
	}
	slices.Sort(names)
	return names
}

// LoadFigletFont loads a FIGlet (.flf) font from fsys, named after its file. Fonts
// of block (█ ▀ ▄ ░) and box-drawing (═ ║ ╗) characters draw best. It panics if
// the font is missing or broken. fsys may be an embed.FS or an os.DirFS. Load
// once, outside frame functions. For bytes and error handling, use ParseFigletFont.
func LoadFigletFont(fsys fs.FS, path string) *FigletFont {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		panic(err)
	}
	name := strings.TrimSuffix(pathpkg.Base(path), ".flf")
	f, err := ParseFigletFont(name, b)
	if err != nil {
		panic(path + ": " + err.Error())
	}
	return f
}

// ParseFigletFont parses a FIGlet (.flf) font from data, using name for FigletFont.Name.
// It returns an error for missing or malformed ASCII glyph data. Only ASCII
// input glyphs are loaded; selected Unicode blocks, shades and box-drawing
// strokes are painted by Block. ASCII-art strokes such as / and _ are not.
func ParseFigletFont(name string, data []byte) (*FigletFont, error) {
	return parseFig(name, string(data))
}

// FigFont is an alias for FigletFont.
//
// Deprecated: Use FigletFont.
type FigFont = FigletFont

// StockFigFont loads a bundled FIGlet font.
//
// Deprecated: Use StockFigletFont.
func StockFigFont(name string) *FigletFont { return StockFigletFont(name) }

// StockFigFontNames lists the bundled FIGlet font names.
//
// Deprecated: Use StockFigletFontNames.
func StockFigFontNames() []string { return StockFigletFontNames() }

// LoadFigFont loads a FIGlet font from fsys.
//
// Deprecated: Use LoadFigletFont.
func LoadFigFont(fsys fs.FS, path string) *FigletFont { return LoadFigletFont(fsys, path) }

// ParseFigFont parses a FIGlet font from data.
//
// Deprecated: Use ParseFigletFont.
func ParseFigFont(name string, data []byte) (*FigletFont, error) {
	return ParseFigletFont(name, data)
}

// parseFig reads the .flf format: header, comment lines, then each character's
// rows, which end with an end mark (usually '@', doubled on the last row). Only
// ASCII is loaded.
func parseFig(name, src string) (*FigletFont, error) {
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	if !sc.Scan() {
		return nil, fmt.Errorf("empty font")
	}
	hdr := strings.Fields(sc.Text())
	if len(hdr) < 6 || !strings.HasPrefix(hdr[0], "flf2a") {
		return nil, fmt.Errorf("not a FIGlet font")
	}
	hardblank, size := utf8.DecodeRuneInString(hdr[0][5:])
	if hardblank == utf8.RuneError || size != len(hdr[0][5:]) || size == 0 {
		return nil, fmt.Errorf("invalid FIGlet hardblank")
	}
	height, err := strconv.Atoi(hdr[1])
	if err != nil || height <= 0 || height > len(src) {
		return nil, fmt.Errorf("invalid FIGlet height %q", hdr[1])
	}
	oldLayout, err := strconv.Atoi(hdr[4])
	if err != nil {
		return nil, fmt.Errorf("invalid FIGlet layout %q", hdr[4])
	}
	comments, err := strconv.Atoi(hdr[5])
	if err != nil || comments < 0 {
		return nil, fmt.Errorf("invalid FIGlet comment count %q", hdr[5])
	}
	for i := 0; i < comments; i++ {
		if !sc.Scan() {
			return nil, fmt.Errorf("truncated font comments")
		}
	}
	f := &FigletFont{Name: name, Height: height, kerning: oldLayout >= 0, glyphs: map[rune][][]rune{}}
	for r := rune(32); r <= 126; r++ {
		rows := make([][]rune, 0, height)
		for range height {
			if !sc.Scan() {
				return nil, fmt.Errorf("truncated at %q", r)
			}
			line := strings.TrimRight(sc.Text(), " \r")
			if line != "" {
				end, _ := utf8.DecodeLastRuneInString(line)
				line = strings.TrimRight(line, string(end)) // strip the end mark(s)
			}
			row := []rune(line)
			for j, ch := range row {
				if ch == hardblank {
					row[j] = hardBlank
				}
			}
			rows = append(rows, row)
		}
		f.glyphs[r] = rows
	}
	return f, nil
}

// lookup returns r's glyph, falling back to its uppercase.
func (f *FigletFont) lookup(r rune) ([][]rune, bool) {
	if g, ok := f.glyphs[r]; ok {
		return g, true
	}
	g, ok := f.glyphs[toUpper(r)]
	return g, ok
}

func (f *FigletFont) glyph(r rune) [][]rune {
	if g, ok := f.lookup(r); ok {
		return g
	}
	return f.glyphs['?']
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// figCell is one cell of a rendered line: its character, and the index of
// the source character it came from (-1 for blanks).
type figCell struct {
	r     rune
	owner int
}

type figLayout [][]figCell

func (l figLayout) width() int {
	w := 0
	for _, row := range l {
		w = max(w, len(row))
	}
	return w
}

type figKey struct {
	f *FigletFont
	s string
}

var figLayouts = memo[figKey, figLayout]{max: 4096}

// render lays out one line of text. Callers must not modify the result.
func (f *FigletFont) render(s string) figLayout {
	return figLayouts.get(figKey{f, s}, func() figLayout { return f.layout(s) })
}

func (f *FigletFont) layout(s string) figLayout {
	rows := make(figLayout, f.Height)
	for i, r := range []rune(s) {
		g := f.glyph(r)
		gw := 0
		for _, gr := range g {
			gw = max(gw, len(gr))
		}
		shift := 0
		if f.kerning && i > 0 {
			shift = kernShift(rows, g, gw)
		}
		for y := range f.Height {
			start := len(rows[y]) - shift
			for x := range gw {
				ch := ' '
				if x < len(g[y]) {
					ch = g[y][x]
				}
				for start+x >= len(rows[y]) {
					rows[y] = append(rows[y], figCell{' ', -1})
				}
				if ch != ' ' {
					rows[y][start+x] = figCell{ch, i}
				}
			}
		}
	}
	for _, row := range rows {
		for x, c := range row {
			if c.r == hardBlank {
				row[x] = figCell{' ', -1}
			}
		}
	}
	return trimBlankRows(rows)
}

// kernShift is how far glyph g (gw wide) can slide left into rows before it
// would touch what's there.
func kernShift(rows figLayout, g [][]rune, gw int) int {
	shift := math.MaxInt
	for y, row := range rows {
		trail := 0
		for x := len(row) - 1; x >= 0 && row[x].r == ' '; x-- {
			trail++
		}
		lead := 0
		for x := 0; x < len(g[y]) && g[y][x] == ' '; x++ {
			lead++
		}
		if len(g[y]) == 0 {
			lead = gw
		}
		shift = min(shift, trail+lead)
	}
	if shift == math.MaxInt {
		return 0
	}
	return shift
}

// trimBlankRows drops blank rows at the top and bottom (many fonts pad them).
func trimBlankRows(rows figLayout) figLayout {
	blank := func(row []figCell) bool {
		return !slices.ContainsFunc(row, func(c figCell) bool { return !unicode.IsSpace(c.r) })
	}
	for len(rows) > 0 && blank(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	for len(rows) > 0 && blank(rows[0]) {
		rows = rows[1:]
	}
	return rows
}

// Width returns the width in cells of s rendered on one line.
func (f *FigletFont) Width(s string) int { return f.render(s).width() }

func (f *FigletFont) widest(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, f.Width(l))
	}
	return w
}

// Rows returns the height in cells of one rendered line, measured on a sample
// with ascenders, descenders and capitals so it is the same for every line.
func (f *FigletFont) Rows() int { return len(f.render("AgjM")) }

// Wrap breaks s into lines no wider than maxW cells, at spaces, keeping "\n".
// Like Font.Wrap it balances the lines: each paragraph uses the narrowest
// width needing no more lines than maxW does.
func (f *FigletFont) Wrap(s string, maxW int) []string {
	return wrapBalanced(s, float64(maxW), func(l string) float64 { return float64(f.Width(l)) })
}

// Has reports whether the font has a visible glyph for each non-space rune of
// s.
func (f *FigletFont) Has(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\n' {
			continue
		}
		g, ok := f.lookup(r)
		if !ok || !slices.ContainsFunc(g, func(row []rune) bool {
			return slices.ContainsFunc(row, func(r rune) bool { return !unicode.IsSpace(r) })
		}) {
			return false
		}
	}
	return true
}

// DropQuotes removes quote marks the font has no glyph for, which beats falling
// back to a smaller font. FitBlock does this itself; call it before wrapping or
// measuring a string FitBlock fitted.
func (f *FigletFont) DropQuotes(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\'', '’', '‘', '"', '“', '”', '`':
			if !f.Has(string(r)) {
				return -1
			}
		}
		return r
	}, s)
}
